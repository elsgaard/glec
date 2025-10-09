package main

import (
	"flag"
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

	node := NewNode(NodeID, Peers)
	node.Start()
}
