package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
)

// Config represents the node configuration for Raft.
type Config struct {
	ID    string   `json:"id"`
	Peers []string `json:"peers"`
}

// Validate checks that required fields are set.
func (c *Config) Validate() error {
	if c.ID == "" {
		return fmt.Errorf("missing required field 'id'")
	}
	if len(c.Peers) == 0 {
		log.Printf("Warning: no peers defined (running as a single node?)")
	}
	return nil
}

// LoadConfig reads and parses a JSON config file.
func LoadConfig(path string) (*Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("could not open config file %q: %w", path, err)
	}
	defer f.Close()

	var cfg Config
	if err := json.NewDecoder(f).Decode(&cfg); err != nil {
		return nil, fmt.Errorf("failed to parse JSON in %q: %w", path, err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return &cfg, nil
}
