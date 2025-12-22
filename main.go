package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/uttkarshm/raft/structs"
)

func main() {
	log.Println("=== Starting Raft Cluster ===")

	// Create 3 Raft nodes on different ports
	node1 := structs.NewServer("node1", "localhost:5001", []string{"localhost:5002", "localhost:5003"})
	node2 := structs.NewServer("node2", "localhost:5002", []string{"localhost:5001", "localhost:5003"})
	node3 := structs.NewServer("node3", "localhost:5003", []string{"localhost:5001", "localhost:5002"})

	// Start all servers
	log.Println("Starting node1...")
	if err := node1.Start(); err != nil {
		log.Fatalf("Failed to start node1: %v", err)
	}

	log.Println("Starting node2...")
	if err := node2.Start(); err != nil {
		log.Fatalf("Failed to start node2: %v", err)
	}

	log.Println("Starting node3...")
	if err := node3.Start(); err != nil {
		log.Fatalf("Failed to start node3: %v", err)
	}

	log.Println("All nodes started successfully!")
	log.Println("Cluster is running. Press Ctrl+C to stop.")

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	// Cleanup
	log.Println("\nShutting down...")
	node1.Stop()
	node2.Stop()
	node3.Stop()
	log.Println("All nodes stopped.")
}
