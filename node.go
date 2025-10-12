package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

const (
	heartbeatInterval  = 2 * time.Second
	electionTimeoutMin = 4 * time.Second
	electionTimeoutMax = 6 * time.Second
	requestTimeout     = 1 * time.Second
)

type Role string

const (
	Follower  Role = "Follower"
	Candidate Role = "Candidate"
	Leader    Role = "Leader"
)

type Node struct {
	ID            string
	Role          Role
	CurrentLeader string
	CurrentTerm   int64 // Atomic: use atomic operations
	Peers         []string
	mutex         sync.RWMutex
	votedFor      string
	votedTerm     int64

	// Improved timer management
	electionTimer     *time.Timer
	electionTimerLock sync.Mutex

	// For graceful shutdown
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// HTTP client with timeout
	httpClient *http.Client
}

func NewNode(id string, peers []string) *Node {
	ctx, cancel := context.WithCancel(context.Background())
	return &Node{
		ID:     id,
		Role:   Follower,
		Peers:  peers,
		ctx:    ctx,
		cancel: cancel,
		httpClient: &http.Client{
			Timeout: requestTimeout,
		},
	}
}

func (n *Node) Start() {
	n.resetElectionTimer()
	n.wg.Add(1)
	go n.runElectionTimer()

	http.HandleFunc("/heartbeat", n.handleHeartbeat)
	http.HandleFunc("/vote", n.handleVoteRequest)
	http.HandleFunc("/status", n.handleStatus)

	log.Println("Starting HTTP server on", n.ID)
	go http.ListenAndServe(n.ID, nil)

	<-n.ctx.Done()
	n.wg.Wait()
}

func (n *Node) Shutdown() {
	n.cancel()
}

func (n *Node) resetElectionTimer() {
	n.electionTimerLock.Lock()
	defer n.electionTimerLock.Unlock()

	timeout := time.Duration(rand.Int63n(int64(electionTimeoutMax-electionTimeoutMin))) + electionTimeoutMin

	if n.electionTimer == nil {
		n.electionTimer = time.NewTimer(timeout)
	} else {
		if !n.electionTimer.Stop() {
			select {
			case <-n.electionTimer.C:
			default:
			}
		}
		n.electionTimer.Reset(timeout)
	}
}

func (n *Node) runElectionTimer() {
	defer n.wg.Done()

	for {
		select {
		case <-n.ctx.Done():
			return
		case <-n.electionTimer.C:
			n.mutex.RLock()
			role := n.Role
			n.mutex.RUnlock()

			if role != Leader {
				log.Println(n.ID, "Election timeout, starting election")
				n.startElection()
			}
			n.resetElectionTimer()
		}
	}
}

func (n *Node) startElection() {
	// Increment term and vote for self
	newTerm := atomic.AddInt64(&n.CurrentTerm, 1)

	n.mutex.Lock()
	n.Role = Candidate
	n.votedFor = n.ID
	n.votedTerm = newTerm
	n.mutex.Unlock()

	log.Printf("%s starting election for term %d", n.ID, newTerm)

	votes := int32(1)                       // Vote for self
	needed := int32((len(n.Peers) + 2) / 2) // Quorum including self

	ctx, cancel := context.WithTimeout(n.ctx, requestTimeout)
	defer cancel()

	var wg sync.WaitGroup
	for _, peer := range n.Peers {
		wg.Add(1)
		go func(peer string) {
			defer wg.Done()

			if n.requestVote(ctx, peer, newTerm) {
				atomic.AddInt32(&votes, 1)
			}
		}(peer)
	}

	wg.Wait()

	finalVotes := atomic.LoadInt32(&votes)
	currentTerm := atomic.LoadInt64(&n.CurrentTerm)

	// Check if we're still in the same term and won
	if currentTerm == newTerm && finalVotes >= needed {
		n.mutex.RLock()
		stillCandidate := n.Role == Candidate
		n.mutex.RUnlock()

		if stillCandidate {
			log.Printf("%s won election for term %d with %d votes", n.ID, newTerm, finalVotes)
			n.becomeLeader()
			return
		}
	}

	log.Printf("%s lost election for term %d (votes: %d, needed: %d)", n.ID, newTerm, finalVotes, needed)
	n.mutex.Lock()
	if n.Role == Candidate && n.votedTerm == newTerm {
		n.Role = Follower
	}
	n.mutex.Unlock()
}

func (n *Node) requestVote(ctx context.Context, peer string, term int64) bool {
	url := fmt.Sprintf("http://%s/vote?id=%s&term=%d", peer, n.ID, term)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return false
	}

	resp, err := n.httpClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK
}

func (n *Node) becomeLeader() {
	n.mutex.Lock()
	n.Role = Leader
	n.CurrentLeader = n.ID
	leaderTerm := atomic.LoadInt64(&n.CurrentTerm)
	n.mutex.Unlock()

	log.Printf("%s became leader for term %d", n.ID, leaderTerm)

	n.wg.Add(1)
	go func() {
		defer n.wg.Done()
		ticker := time.NewTicker(heartbeatInterval)
		defer ticker.Stop()

		for {
			select {
			case <-n.ctx.Done():
				return
			case <-ticker.C:
				n.mutex.RLock()
				stillLeader := n.Role == Leader
				n.mutex.RUnlock()

				if !stillLeader {
					return
				}

				n.sendHeartbeats(leaderTerm)
			}
		}
	}()
}

func (n *Node) sendHeartbeats(term int64) {
	for _, peer := range n.Peers {
		go func(peer string) {
			ctx, cancel := context.WithTimeout(n.ctx, requestTimeout)
			defer cancel()

			url := fmt.Sprintf("http://%s/heartbeat?leader=%s&term=%d", peer, n.ID, term)
			req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
			if err != nil {
				return
			}

			resp, err := n.httpClient.Do(req)
			if err != nil {
				return
			}
			resp.Body.Close()
		}(peer)
	}
}

func (n *Node) handleHeartbeat(w http.ResponseWriter, r *http.Request) {
	leader := r.URL.Query().Get("leader")
	termStr := r.URL.Query().Get("term")

	var term int64
	fmt.Sscanf(termStr, "%d", &term)

	currentTerm := atomic.LoadInt64(&n.CurrentTerm)

	// Reject stale heartbeats
	if term < currentTerm {
		w.WriteHeader(http.StatusConflict)
		return
	}

	// Update term if newer
	if term > currentTerm {
		atomic.StoreInt64(&n.CurrentTerm, term)
	}

	n.mutex.Lock()
	n.CurrentLeader = leader
	n.Role = Follower
	// Only reset votedFor if it's a new term
	if term > n.votedTerm {
		n.votedFor = ""
		n.votedTerm = 0
	}
	n.mutex.Unlock()

	n.resetElectionTimer()
	w.WriteHeader(http.StatusOK)
}

func (n *Node) handleVoteRequest(w http.ResponseWriter, r *http.Request) {
	candidateID := r.URL.Query().Get("id")
	termStr := r.URL.Query().Get("term")

	var term int64
	fmt.Sscanf(termStr, "%d", &term)

	currentTerm := atomic.LoadInt64(&n.CurrentTerm)

	n.mutex.Lock()
	defer n.mutex.Unlock()

	// Reject stale requests
	if term < currentTerm {
		w.WriteHeader(http.StatusConflict)
		return
	}

	// Update term if newer
	if term > currentTerm {
		atomic.StoreInt64(&n.CurrentTerm, term)
		n.votedFor = ""
		n.votedTerm = 0
		n.Role = Follower
	}

	// Grant vote if haven't voted in this term
	if n.votedTerm < term || n.votedFor == "" || n.votedFor == candidateID {
		n.votedFor = candidateID
		n.votedTerm = term
		n.resetElectionTimer()
		w.WriteHeader(http.StatusOK)
		log.Printf("%s voted for %s in term %d", n.ID, candidateID, term)
	} else {
		w.WriteHeader(http.StatusConflict)
		log.Printf("%s rejected vote for %s in term %d (already voted for %s)",
			n.ID, candidateID, term, n.votedFor)
	}
}

func (n *Node) handleStatus(w http.ResponseWriter, r *http.Request) {
	n.mutex.RLock()
	defer n.mutex.RUnlock()

	status := map[string]interface{}{
		"node":   n.ID,
		"role":   string(n.Role),
		"leader": n.CurrentLeader,
		"term":   atomic.LoadInt64(&n.CurrentTerm),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}
