package main

import (
	"flag"
	"log/slog"
	"os"
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

	// Set up slog logger (basic text handler to stdout)
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	slog.Info("Starting Raft node", "id", NodeID, "peers", Peers)

	node := NewNode(NodeID, Peers)
	if node == nil {
		slog.Error("Failed to initialize node", "id", NodeID)
		os.Exit(1)
	}

	node.Start()
}
