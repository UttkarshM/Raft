package test

import (
	"testing"
	"time"

	"github.com/uttkarshm/raft/structs"
)

// TestLeaderElection tests basic leader election in a 3-node cluster
func TestLeaderElection(t *testing.T) {
	// Create 3 Raft nodes
	node1 := structs.NewServer("node1", "localhost:5001", []string{"localhost:5002", "localhost:5003"})
	node2 := structs.NewServer("node2", "localhost:5002", []string{"localhost:5001", "localhost:5003"})
	node3 := structs.NewServer("node3", "localhost:5003", []string{"localhost:5001", "localhost:5002"})

	// Start all nodes
	if err := node1.Start(); err != nil {
		t.Fatalf("Failed to start node1: %v", err)
	}
	defer node1.Stop()

	if err := node2.Start(); err != nil {
		t.Fatalf("Failed to start node2: %v", err)
	}
	defer node2.Stop()

	if err := node3.Start(); err != nil {
		t.Fatalf("Failed to start node3: %v", err)
	}
	defer node3.Stop()

	t.Log("All nodes started successfully")

	// Wait for election to complete (up to 2 seconds)
	// Elections typically complete within 300-600ms
	time.Sleep(2 * time.Second)

	// Check that exactly one leader was elected
	leaders := 0
	var leaderID string

	if node1.IsLeader() {
		leaders++
		leaderID = node1.ID
	}
	if node2.IsLeader() {
		leaders++
		leaderID = node2.ID
	}
	if node3.IsLeader() {
		leaders++
		leaderID = node3.ID
	}

	if leaders != 1 {
		t.Errorf("Expected exactly 1 leader, got %d", leaders)
	} else {
		t.Logf("✓ Leader elected: %s", leaderID)
	}

	// Verify all nodes are in the same term
	term1 := node1.GetCurrentTerm()
	term2 := node2.GetCurrentTerm()
	term3 := node3.GetCurrentTerm()

	if term1 != term2 || term2 != term3 {
		t.Errorf("Nodes have different terms: node1=%d, node2=%d, node3=%d", term1, term2, term3)
	} else {
		t.Logf("✓ All nodes in term %d", term1)
	}

	// Verify non-leaders are followers
	if !node1.IsLeader() && node1.GetState() != structs.Follower {
		t.Errorf("node1 should be a follower, got state %v", node1.GetState())
	}
	if !node2.IsLeader() && node2.GetState() != structs.Follower {
		t.Errorf("node2 should be a follower, got state %v", node2.GetState())
	}
	if !node3.IsLeader() && node3.GetState() != structs.Follower {
		t.Errorf("node3 should be a follower, got state %v", node3.GetState())
	}

	t.Log("✓ All non-leaders are followers")
}

// TestLeaderFailover tests that a new leader is elected when the current leader fails
func TestLeaderFailover(t *testing.T) {
	// Create 3 Raft nodes
	node1 := structs.NewServer("node1", "localhost:6001", []string{"localhost:6002", "localhost:6003"})
	node2 := structs.NewServer("node2", "localhost:6002", []string{"localhost:6001", "localhost:6003"})
	node3 := structs.NewServer("node3", "localhost:6003", []string{"localhost:6001", "localhost:6002"})

	// Start all nodes
	if err := node1.Start(); err != nil {
		t.Fatalf("Failed to start node1: %v", err)
	}

	if err := node2.Start(); err != nil {
		t.Fatalf("Failed to start node2: %v", err)
	}

	if err := node3.Start(); err != nil {
		t.Fatalf("Failed to start node3: %v", err)
	}

	t.Log("All nodes started successfully")

	// Wait for initial election
	time.Sleep(2 * time.Second)

	// Find the initial leader
	var initialLeader *structs.Server
	nodes := []*structs.Server{node1, node2, node3}

	for _, node := range nodes {
		if node.IsLeader() {
			initialLeader = node
			break
		}
	}

	if initialLeader == nil {
		t.Fatal("No initial leader elected")
	}

	t.Logf("Initial leader: %s", initialLeader.ID)

	// Stop the leader
	t.Logf("Stopping leader %s...", initialLeader.ID)
	if err := initialLeader.Stop(); err != nil {
		t.Fatalf("Failed to stop leader: %v", err)
	}

	// Wait for new election (up to 2 seconds)
	time.Sleep(2 * time.Second)

	// Check that a new leader was elected
	newLeaders := 0
	var newLeaderID string

	for _, node := range nodes {
		if node != initialLeader && node.IsLeader() {
			newLeaders++
			newLeaderID = node.ID
		}
	}

	if newLeaders != 1 {
		t.Errorf("Expected exactly 1 new leader after failover, got %d", newLeaders)
	} else {
		t.Logf("✓ New leader elected after failover: %s", newLeaderID)
	}

	// Cleanup remaining nodes
	for _, node := range nodes {
		if node != initialLeader {
			node.Stop()
		}
	}
}

// TestMultipleElections tests that elections stabilize after multiple rounds
func TestMultipleElections(t *testing.T) {
	// Create 3 Raft nodes
	node1 := structs.NewServer("node1", "localhost:7001", []string{"localhost:7002", "localhost:7003"})
	node2 := structs.NewServer("node2", "localhost:7002", []string{"localhost:7001", "localhost:7003"})
	node3 := structs.NewServer("node3", "localhost:7003", []string{"localhost:7001", "localhost:7002"})

	// Start all nodes
	if err := node1.Start(); err != nil {
		t.Fatalf("Failed to start node1: %v", err)
	}
	defer node1.Stop()

	if err := node2.Start(); err != nil {
		t.Fatalf("Failed to start node2: %v", err)
	}
	defer node2.Stop()

	if err := node3.Start(); err != nil {
		t.Fatalf("Failed to start node3: %v", err)
	}
	defer node3.Stop()

	// Wait and check for stable leadership over 5 seconds
	time.Sleep(5 * time.Second)

	// Count leaders
	leaders := 0
	if node1.IsLeader() {
		leaders++
	}
	if node2.IsLeader() {
		leaders++
	}
	if node3.IsLeader() {
		leaders++
	}

	if leaders != 1 {
		t.Errorf("After 5 seconds, expected 1 stable leader, got %d", leaders)
	} else {
		t.Log("✓ Leadership remained stable")
	}

	// Check that terms didn't increase too much (shouldn't have many failed elections)
	maxTerm := 0
	if term := node1.GetCurrentTerm(); term > maxTerm {
		maxTerm = term
	}
	if term := node2.GetCurrentTerm(); term > maxTerm {
		maxTerm = term
	}
	if term := node3.GetCurrentTerm(); term > maxTerm {
		maxTerm = term
	}

	// With randomized timeouts, we shouldn't have more than 3-4 elections in 5 seconds
	if maxTerm > 5 {
		t.Logf("Warning: High term number (%d) suggests many elections occurred", maxTerm)
	} else {
		t.Logf("✓ Term number is reasonable: %d", maxTerm)
	}
}
