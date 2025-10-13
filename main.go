package main

import (
	"flag"
	"log"
	"strings"
)

func main() {

	id := flag.String("id", "localhost:8000", "Node ID (host:port)")
	peers := flag.String("peers", "", "Comma-separated list of peer addresses")

	flag.Parse()

	NodeID = *id
	if *peers != "" {
		Peers = strings.Split(*peers, ",")
	}

	log.Printf("Starting Raft node with ID %s and peers %v", NodeID, Peers)

	node := NewNode(NodeID, Peers)
	if node == nil {
		log.Fatalf("Failed to initialize node for ID %s", NodeID)
	}

	node.Start()
}
