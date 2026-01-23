package main

import (
	"fmt"
	"log"
	"strings"
	"time"

	modelrouter "github.com/uttkarshm/raft/model_router"
	"github.com/uttkarshm/raft/structs"
)

func main() {
	log.Println("=== Starting Raft-Powered AI Model Router ===\n")

	// Step 1: Create Raft cluster (3 nodes)
	log.Println("[Step 1] Creating 3-node Raft cluster...")
	node1 := structs.NewServer("router1", "localhost:6001", []string{"localhost:6002", "localhost:6003"})
	node2 := structs.NewServer("router2", "localhost:6002", []string{"localhost:6001", "localhost:6003"})
	node3 := structs.NewServer("router3", "localhost:6003", []string{"localhost:6001", "localhost:6002"})

	// Start all Raft nodes
	log.Println("[Step 1] Starting Raft nodes...")
	node1.Start()
	node2.Start()
	node3.Start()

	// Wait for leader election
	log.Println("[Step 1] Waiting for leader election...")
	time.Sleep(2 * time.Second)

	// Step 2: Create Raft routers on each node
	log.Println("\n[Step 2] Creating Raft routers...")
	router1 := modelrouter.NewRaftRouter(node1)
	router2 := modelrouter.NewRaftRouter(node2)
	router3 := modelrouter.NewRaftRouter(node3)

	// Find the leader router
	var leaderRouter *modelrouter.RaftRouter
	if node1.IsLeader() {
		leaderRouter = router1
		log.Println("[Step 2] ✓ Router1 elected as leader")
	} else if node2.IsLeader() {
		leaderRouter = router2
		log.Println("[Step 2] ✓ Router2 elected as leader")
	} else {
		leaderRouter = router3
		log.Println("[Step 2] ✓ Router3 elected as leader")
	}

	// Step 3: Register AI model servers
	log.Println("\n[Step 3] Registering AI model servers...")

	servers := []*modelrouter.ModelServer{
		{
			ID:        "gemini-us-east",
			Address:   "localhost:8001",
			ModelType: modelrouter.ModelGemini,
			IsHealthy: true,
		},
		{
			ID:        "gemini-us-west",
			Address:   "localhost:8002",
			ModelType: modelrouter.ModelGemini,
			IsHealthy: true,
		},
		{
			ID:        "bert-classifier",
			Address:   "localhost:8003",
			ModelType: modelrouter.ModelBERT,
			IsHealthy: true,
		},
		{
			ID:        "t5-translator",
			Address:   "localhost:8004",
			ModelType: modelrouter.ModelT5,
			IsHealthy: true,
		},
	}

	for _, server := range servers {
		err := leaderRouter.RegisterServer(server)
		if err != nil {
			log.Printf("[Error] Failed to register %s: %v", server.ID, err)
		} else {
			log.Printf("[Step 3] ✓ Registered %s (%s model)", server.ID, server.ModelType)
		}
	}

	// Step 4: Start health monitoring
	log.Println("\n[Step 4] Starting background health checks...")
	leaderRouter.StartHealthChecks()
	log.Println("[Step 4] ✓ Health monitoring active")

	// Step 5: Route AI model requests
	log.Println("\n[Step 5] Routing AI model requests...\n")

	testRequests := []struct {
		modelType modelrouter.ModelType
		input     string
		desc      string
	}{
		{
			modelType: modelrouter.ModelGemini,
			input:     "Explain quantum computing in simple terms",
			desc:      "Text generation",
		},
		{
			modelType: modelrouter.ModelBERT,
			input:     "This product exceeded my expectations! Amazing quality.",
			desc:      "Sentiment classification",
		},
		{
			modelType: modelrouter.ModelT5,
			input:     "translate English to French: Hello, how are you?",
			desc:      "Translation",
		},
		{
			modelType: modelrouter.ModelGemini,
			input:     "Summarize the key points of blockchain technology",
			desc:      "Summarization",
		},
	}

	for i, testData := range testRequests {
		req := &modelrouter.ModelRequest{
			ID:        fmt.Sprintf("req-%d", i+1),
			ModelType: testData.modelType,
			Input:     testData.input,
			Timeout:   5 * time.Second,
		}

		log.Printf("📤 Request %s: %s (%s)", req.ID, testData.desc, req.ModelType)

		response, err := leaderRouter.RouteRequest(req)
		if err != nil {
			log.Printf("   ❌ Failed: %v\n", err)
		} else {
			log.Printf("   ✅ Success!")
			log.Printf("   📊 Server: %s, Latency: %v\n",
				response.Metadata["server_id"],
				response.Metadata["latency"])
		}

		time.Sleep(300 * time.Millisecond)
	}

	// Step 6: Show cluster status
	log.Println("\n[Step 6] Cluster Status Report\n")
	log.Println("=" + strings.Repeat("=", 50))

	status := leaderRouter.GetClusterStatus()
	log.Printf("🎯 Leader: %s (Term %d)", status.LeaderID, status.CurrentTerm)
	log.Printf("📡 Total Servers: %d", len(status.Servers))
	log.Println("\n📊 Server Statistics:")

	for id, server := range status.Servers {
		healthIcon := "✅"
		if !server.IsHealthy {
			healthIcon = "❌"
		}

		log.Printf("\n  %s %s (%s)", healthIcon, id, server.ModelType)
		log.Printf("     Requests: %d | Errors: %d | Avg Latency: %s",
			server.RequestCount,
			server.ErrorCount,
			server.AvgLatency)
	}

	log.Println("\n" + strings.Repeat("=", 50))

	// Step 7: Demonstrate failover
	log.Println("\n[Step 7] Testing Automatic Failover...\n")

	var currentLeaderNode *structs.Server
	if node1.IsLeader() {
		currentLeaderNode = node1
		log.Println("🔴 Simulating failure of Router1 (current leader)...")
	} else if node2.IsLeader() {
		currentLeaderNode = node2
		log.Println("🔴 Simulating failure of Router2 (current leader)...")
	} else {
		currentLeaderNode = node3
		log.Println("🔴 Simulating failure of Router3 (current leader)...")
	}

	currentLeaderNode.Stop()

	log.Println("⏳ Waiting for new leader election...")
	time.Sleep(2 * time.Second)

	// Find new leader
	var newLeaderRouter *modelrouter.RaftRouter
	if node1 != currentLeaderNode && node1.IsLeader() {
		newLeaderRouter = router1
		log.Println("✅ Router1 automatically became new leader!")
	} else if node2 != currentLeaderNode && node2.IsLeader() {
		newLeaderRouter = router2
		log.Println("✅ Router2 automatically became new leader!")
	} else if node3 != currentLeaderNode && node3.IsLeader() {
		newLeaderRouter = router3
		log.Println("✅ Router3 automatically became new leader!")
	}

	// Test request after failover
	log.Println("\n📤 Testing request routing after failover...")

	failoverReq := &modelrouter.ModelRequest{
		ID:        "failover-test",
		ModelType: modelrouter.ModelGemini,
		Input:     "Test request after leader failure",
		Timeout:   5 * time.Second,
	}

	response, err := newLeaderRouter.RouteRequest(failoverReq)
	if err != nil {
		log.Printf("❌ Failover test failed: %v", err)
	} else {
		log.Printf("✅ Failover successful! Request processed by %s",
			response.Metadata["server_id"])
	}

	// Final summary
	log.Println("\n" + strings.Repeat("=", 50))
	log.Println("🎉 Demo Complete - Raft-Powered AI Model Router\n")
	log.Println("✅ Features Demonstrated:")
	log.Println("   • Leader election across 3-node Raft cluster")
	log.Println("   • Load balancing across multiple AI model servers")
	log.Println("   • Global rate limiting (100 req/min for Gemini)")
	log.Println("   • Real-time health monitoring (10s intervals)")
	log.Println("   • Automatic failover on leader failure")
	log.Println("   • Consistent routing state across all nodes")
	log.Println("\n💡 Real-World Applications:")
	log.Println("   • Visual Flow AI pipeline optimization")
	log.Println("   • Distributed API gateway for microservices")
	log.Println("   • High-availability ML inference serving")
	log.Println("   • Multi-region model deployment")
	log.Println("\n" + strings.Repeat("=", 50))

	// Cleanup
	log.Println("\nCleaning up...")
	if node1 != currentLeaderNode {
		node1.Stop()
	}
	if node2 != currentLeaderNode {
		node2.Stop()
	}
	if node3 != currentLeaderNode {
		node3.Stop()
	}

	log.Println("✅ Shutdown complete")
}
