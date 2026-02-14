package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/uttkarshm/raft/gateway"
	"github.com/uttkarshm/raft/structs"
	"github.com/uttkarshm/raft/workflow"
)

func main() {
	// Command-line flags
	nodeID := flag.String("id", "node1", "Node ID (node1, node2, node3)")
	raftPort := flag.String("raft-port", "5001", "Raft cluster port")
	httpPort := flag.String("http-port", "8001", "HTTP gateway port")
	flag.Parse()

	log.Printf("=== Starting Raft Gateway Node: %s ===", *nodeID)
	log.Printf("Raft Port: %s, HTTP Port: %s", *raftPort, *httpPort)

	// Define peer addresses based on node ID
	peers := getPeers(*nodeID)
	raftAddress := "localhost:" + *raftPort

	// Create workflow state machine
	stateMachine := workflow.NewWorkflowStateMachine()
	log.Println("[Main] Created workflow state machine")

	// Create Raft server with state machine
	raftServer := structs.NewServer(*nodeID, raftAddress, peers)
	raftServer.SetStateMachine(stateMachine)
	log.Printf("[Main] Created Raft server: %s at %s", *nodeID, raftAddress)

	// Start Raft server
	if err := raftServer.Start(); err != nil {
		log.Fatalf("[Main] Failed to start Raft server: %v", err)
	}
	log.Println("[Main] Raft server started successfully")

	// Create and start HTTP Gateway
	gatewayServer := gateway.NewGatewayServer(raftServer, stateMachine, *httpPort)
	go func() {
		if err := gatewayServer.Start(); err != nil {
			log.Fatalf("[Main] Failed to start gateway server: %v", err)
		}
	}()
	log.Printf("[Main] HTTP Gateway started on port %s", *httpPort)

	log.Println("===========================================")
	log.Printf("✅ Node %s is ready!", *nodeID)
	log.Printf("Raft: %s | HTTP: localhost:%s", raftAddress, *httpPort)
	log.Println("Press Ctrl+C to stop")
	log.Println("===========================================")

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	// Graceful shutdown
	log.Println("\n[Main] Shutting down...")
	gatewayServer.Stop()
	raftServer.Stop()
	log.Println("[Main] Shutdown complete")
}

// getPeers returns peer addresses based on the current node ID
func getPeers(nodeID string) []string {
	allNodes := map[string]string{
		"node1": "localhost:5001",
		"node2": "localhost:5002",
		"node3": "localhost:5003",
	}

	var peers []string
	for id, addr := range allNodes {
		if id != nodeID {
			peers = append(peers, addr)
		}
	}

	return peers
}
