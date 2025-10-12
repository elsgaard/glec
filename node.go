package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
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

// State queries and updates via channels
type getStateReq struct {
	resp chan NodeState
}

type updateStateReq struct {
	fn func(*NodeState)
}

type voteReq struct {
	candidateID string
	term        int64
	resp        chan bool
}

type heartbeatReq struct {
	leader string
	term   int64
}

type NodeState struct {
	ID            string
	Role          Role
	CurrentLeader string
	CurrentTerm   int64
	VotedFor      string
	VotedTerm     int64
}

type Node struct {
	state      NodeState
	peers      []string
	httpClient *http.Client

	// Channels for state management
	getState    chan getStateReq
	updateState chan updateStateReq
	voteReq     chan voteReq
	heartbeat   chan heartbeatReq

	// Control channels
	resetTimer    chan struct{}
	startElection chan struct{}

	ctx    context.Context
	cancel context.CancelFunc
}

func NewNode(id string, peers []string) *Node {
	ctx, cancel := context.WithCancel(context.Background())
	return &Node{
		state: NodeState{
			ID:   id,
			Role: Follower,
		},
		peers: peers,
		httpClient: &http.Client{
			Timeout: requestTimeout,
		},
		getState:      make(chan getStateReq),
		updateState:   make(chan updateStateReq),
		voteReq:       make(chan voteReq),
		heartbeat:     make(chan heartbeatReq),
		resetTimer:    make(chan struct{}, 1),
		startElection: make(chan struct{}, 1),
		ctx:           ctx,
		cancel:        cancel,
	}
}

func (n *Node) Start() {
	// Single goroutine manages all state
	go n.stateManager()

	// Election timer goroutine
	go n.electionTimer()

	// HTTP handlers
	http.HandleFunc("/heartbeat", n.handleHeartbeat)
	http.HandleFunc("/vote", n.handleVoteRequest)
	http.HandleFunc("/status", n.handleStatus)

	log.Println("Starting HTTP server on", n.state.ID)
	go http.ListenAndServe(n.state.ID, nil)

	<-n.ctx.Done()
}

func (n *Node) Shutdown() {
	n.cancel()
}

// Single goroutine that owns all state - no locks needed!
func (n *Node) stateManager() {
	for {
		select {
		case <-n.ctx.Done():
			return

		case req := <-n.getState:
			// Return a copy of state
			req.resp <- n.state

		case req := <-n.updateState:
			// Apply update function
			req.fn(&n.state)

		case req := <-n.voteReq:
			granted := n.handleVote(req.candidateID, req.term)
			req.resp <- granted

		case req := <-n.heartbeat:
			n.handleHeartbeatInternal(req.leader, req.term)

		case <-n.startElection:
			go n.runElection()
		}
	}
}

func (n *Node) handleVote(candidateID string, term int64) bool {
	// Reject stale requests
	if term < n.state.CurrentTerm {
		return false
	}

	// Update term if newer
	if term > n.state.CurrentTerm {
		n.state.CurrentTerm = term
		n.state.VotedFor = ""
		n.state.VotedTerm = 0
		n.state.Role = Follower
	}

	// Grant vote if haven't voted in this term
	if n.state.VotedTerm < term || n.state.VotedFor == "" || n.state.VotedFor == candidateID {
		n.state.VotedFor = candidateID
		n.state.VotedTerm = term
		n.triggerTimerReset()
		log.Printf("%s voted for %s in term %d", n.state.ID, candidateID, term)
		return true
	}

	log.Printf("%s rejected vote for %s in term %d (already voted for %s)",
		n.state.ID, candidateID, term, n.state.VotedFor)
	return false
}

func (n *Node) handleHeartbeatInternal(leader string, term int64) {
	// Reject stale heartbeats
	if term < n.state.CurrentTerm {
		return
	}

	// Update term if newer
	if term > n.state.CurrentTerm {
		n.state.CurrentTerm = term
	}

	n.state.CurrentLeader = leader
	n.state.Role = Follower

	// Only reset votedFor if it's a new term
	if term > n.state.VotedTerm {
		n.state.VotedFor = ""
		n.state.VotedTerm = 0
	}

	n.triggerTimerReset()
}

func (n *Node) electionTimer() {
	timer := time.NewTimer(n.randomTimeout())

	for {
		select {
		case <-n.ctx.Done():
			timer.Stop()
			return

		case <-n.resetTimer:
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(n.randomTimeout())

		case <-timer.C:
			// Get current state
			req := getStateReq{resp: make(chan NodeState)}
			select {
			case n.getState <- req:
				state := <-req.resp
				if state.Role != Leader {
					log.Println(state.ID, "Election timeout, starting election")
					select {
					case n.startElection <- struct{}{}:
					default:
					}
				}
			case <-n.ctx.Done():
				return
			}

			timer.Reset(n.randomTimeout())
		}
	}
}

func (n *Node) randomTimeout() time.Duration {
	return time.Duration(rand.Int63n(int64(electionTimeoutMax-electionTimeoutMin))) + electionTimeoutMin
}

func (n *Node) triggerTimerReset() {
	select {
	case n.resetTimer <- struct{}{}:
	default:
	}
}

func (n *Node) runElection() {
	// Increment term and become candidate
	var term int64
	var nodeID string

	update := updateStateReq{
		fn: func(s *NodeState) {
			s.CurrentTerm++
			s.Role = Candidate
			s.VotedFor = s.ID
			s.VotedTerm = s.CurrentTerm
			term = s.CurrentTerm
			nodeID = s.ID
		},
	}

	select {
	case n.updateState <- update:
	case <-n.ctx.Done():
		return
	}

	log.Printf("%s starting election for term %d", nodeID, term)

	// Vote for self
	votes := 1
	needed := (len(n.peers) + 2) / 2

	// Request votes from peers
	voteChan := make(chan bool, len(n.peers))

	for _, peer := range n.peers {
		go func(peer string) {
			ctx, cancel := context.WithTimeout(n.ctx, requestTimeout)
			defer cancel()

			voteChan <- n.requestVote(ctx, peer, nodeID, term)
		}(peer)
	}

	// Collect votes
	for i := 0; i < len(n.peers); i++ {
		select {
		case granted := <-voteChan:
			if granted {
				votes++
			}
		case <-n.ctx.Done():
			return
		}
	}

	// Check if won
	req := getStateReq{resp: make(chan NodeState)}
	select {
	case n.getState <- req:
		state := <-req.resp

		if state.CurrentTerm == term && votes >= needed && state.Role == Candidate {
			log.Printf("%s won election for term %d with %d votes", nodeID, term, votes)
			n.becomeLeader(term)
		} else {
			log.Printf("%s lost election for term %d (votes: %d, needed: %d)", nodeID, term, votes, needed)
			// Revert to follower if still candidate
			update := updateStateReq{
				fn: func(s *NodeState) {
					if s.Role == Candidate && s.VotedTerm == term {
						s.Role = Follower
					}
				},
			}
			select {
			case n.updateState <- update:
			case <-n.ctx.Done():
			}
		}
	case <-n.ctx.Done():
		return
	}
}

func (n *Node) requestVote(ctx context.Context, peer string, candidateID string, term int64) bool {
	url := fmt.Sprintf("http://%s/vote?id=%s&term=%d", peer, candidateID, term)

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

func (n *Node) becomeLeader(term int64) {
	update := updateStateReq{
		fn: func(s *NodeState) {
			s.Role = Leader
			s.CurrentLeader = s.ID
		},
	}

	select {
	case n.updateState <- update:
	case <-n.ctx.Done():
		return
	}

	log.Printf("Node became leader for term %d", term)

	// Start sending heartbeats
	go func() {
		ticker := time.NewTicker(heartbeatInterval)
		defer ticker.Stop()

		for {
			select {
			case <-n.ctx.Done():
				return
			case <-ticker.C:
				// Check if still leader
				req := getStateReq{resp: make(chan NodeState)}
				select {
				case n.getState <- req:
					state := <-req.resp
					if state.Role != Leader {
						return
					}
					n.sendHeartbeats(state.ID, term)
				case <-n.ctx.Done():
					return
				}
			}
		}
	}()
}

func (n *Node) sendHeartbeats(leaderID string, term int64) {
	for _, peer := range n.peers {
		go func(peer string) {
			ctx, cancel := context.WithTimeout(n.ctx, requestTimeout)
			defer cancel()

			url := fmt.Sprintf("http://%s/heartbeat?leader=%s&term=%d", peer, leaderID, term)
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

	req := heartbeatReq{
		leader: leader,
		term:   term,
	}

	// Get current term to check if stale
	getReq := getStateReq{resp: make(chan NodeState)}
	select {
	case n.getState <- getReq:
		state := <-getReq.resp
		if term < state.CurrentTerm {
			w.WriteHeader(http.StatusConflict)
			return
		}
	case <-n.ctx.Done():
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}

	select {
	case n.heartbeat <- req:
		w.WriteHeader(http.StatusOK)
	case <-n.ctx.Done():
		w.WriteHeader(http.StatusServiceUnavailable)
	}
}

func (n *Node) handleVoteRequest(w http.ResponseWriter, r *http.Request) {
	candidateID := r.URL.Query().Get("id")
	termStr := r.URL.Query().Get("term")

	var term int64
	fmt.Sscanf(termStr, "%d", &term)

	req := voteReq{
		candidateID: candidateID,
		term:        term,
		resp:        make(chan bool),
	}

	select {
	case n.voteReq <- req:
		granted := <-req.resp
		if granted {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusConflict)
		}
	case <-n.ctx.Done():
		w.WriteHeader(http.StatusServiceUnavailable)
	}
}

func (n *Node) handleStatus(w http.ResponseWriter, r *http.Request) {
	req := getStateReq{resp: make(chan NodeState)}

	select {
	case n.getState <- req:
		state := <-req.resp

		status := map[string]interface{}{
			"node":   state.ID,
			"role":   string(state.Role),
			"leader": state.CurrentLeader,
			"term":   state.CurrentTerm,
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(status)
	case <-n.ctx.Done():
		w.WriteHeader(http.StatusServiceUnavailable)
	}
}
