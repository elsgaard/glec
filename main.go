package main

import (
	"flag"
	"log"
)

func main() {
	configPath := flag.String("config", "config.json", "Path to JSON config file")
	flag.Parse()

	cfg, err := LoadConfig(*configPath)
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("Starting Raft node with ID %s and peers %v", cfg.ID, cfg.Peers)

	node := NewNode(cfg.ID, cfg.Peers)
	if node == nil {
		log.Fatalf("Failed to initialize node for ID %s", cfg.ID)
	}

	node.Start()
}
