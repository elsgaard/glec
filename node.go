package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"sync"
	"time"
)

const (
	heartbeatInterval  = 2 * time.Second
	electionTimeoutMin = 4 * time.Second
	electionTimeoutMax = 6 * time.Second
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
	Peers         []string
	mutex         sync.Mutex
	votedFor      string
	resetTimer    chan bool
}

func NewNode(id string, peers []string) *Node {
	return &Node{
		ID:         id,
		Role:       Follower,
		Peers:      peers,
		resetTimer: make(chan bool),
	}
}

func (n *Node) Start() {
	go n.runElectionTimer()

	http.HandleFunc("/heartbeat", n.handleHeartbeat)
	http.HandleFunc("/vote", n.handleVoteRequest)
	http.HandleFunc("/status", n.handleStatus) // <-- NEW

	log.Println("Starting HTTP server on", n.ID)
	go http.ListenAndServe(n.ID, nil)

	select {} // block forever
}

func (n *Node) runElectionTimer() {
	for {
		timeout := time.Duration(rand.Intn(int(electionTimeoutMax-electionTimeoutMin))) + electionTimeoutMin
		timer := time.NewTimer(timeout)

		select {
		case <-timer.C:
			log.Println(n.ID, "Election timeout, starting election")
			n.startElection()
		case <-n.resetTimer:
			timer.Stop()
		}
	}
}

func (n *Node) startElection() {
	n.mutex.Lock()
	n.Role = Candidate
	n.votedFor = n.ID
	votes := 1
	n.mutex.Unlock()

	var wg sync.WaitGroup
	voteCh := make(chan bool, len(n.Peers))

	for _, peer := range n.Peers {
		wg.Add(1)
		go func(peer string) {
			defer wg.Done()
			url := fmt.Sprintf("http://%s/vote?id=%s", peer, n.ID)
			resp, err := http.Get(url)
			if err != nil {
				voteCh <- false
				return
			}
			if resp.StatusCode == http.StatusOK {
				voteCh <- true
			} else {
				voteCh <- false
			}
		}(peer)
	}

	wg.Wait()
	close(voteCh)

	for vote := range voteCh {
		if vote {
			votes++
		}
	}

	if votes > len(n.Peers)/2 {
		log.Println(n.ID, "won the election with", votes, "votes")
		n.becomeLeader()
	} else {
		log.Println(n.ID, "lost the election")
		n.mutex.Lock()
		n.Role = Follower
		n.votedFor = ""
		n.mutex.Unlock()
	}
}

func (n *Node) becomeLeader() {
	n.mutex.Lock()
	n.Role = Leader
	n.CurrentLeader = n.ID
	n.mutex.Unlock()

	go func() {
		for {
			n.mutex.Lock()
			if n.Role != Leader {
				n.mutex.Unlock()
				return
			}
			n.mutex.Unlock()

			for _, peer := range n.Peers {
				go func(peer string) {
					http.Get(fmt.Sprintf("http://%s/heartbeat?leader=%s", peer, n.ID))
				}(peer)
			}
			time.Sleep(heartbeatInterval)
		}
	}()
}

func (n *Node) handleHeartbeat(w http.ResponseWriter, r *http.Request) {
	leader := r.URL.Query().Get("leader")
	n.mutex.Lock()
	defer n.mutex.Unlock()

	n.CurrentLeader = leader
	n.Role = Follower
	n.votedFor = ""
	n.resetTimer <- true

	w.WriteHeader(http.StatusOK)
}

func (n *Node) handleVoteRequest(w http.ResponseWriter, r *http.Request) {
	candidateID := r.URL.Query().Get("id")
	n.mutex.Lock()
	defer n.mutex.Unlock()

	if n.votedFor == "" || n.votedFor == candidateID {
		n.votedFor = candidateID
		w.WriteHeader(http.StatusOK)
	} else {
		w.WriteHeader(http.StatusConflict)
	}
}

func (n *Node) handleStatus(w http.ResponseWriter, r *http.Request) {
	n.mutex.Lock()
	defer n.mutex.Unlock()

	status := map[string]string{
		"node":   n.ID,
		"role":   string(n.Role),
		"leader": n.CurrentLeader,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}
