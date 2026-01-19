package modelrouter

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/uttkarshm/raft/structs"
)

// RouterCommand represents commands sent through Raft
type RouterCommand struct {
	Type      string                 // REGISTER, UNREGISTER, UPDATE_HEALTH, ROUTE_REQUEST
	ServerID  string                 // Target server ID
	Data      map[string]interface{} // Command-specific data
	Timestamp time.Time
}

// RaftRouter manages model servers using Raft consensus
type RaftRouter struct {
	// Raft cluster
	raftServer *structs.Server

	// Model server pool (replicated state)
	serverPool *ModelServerPool

	// Rate limiting (global across cluster)
	rateLimits map[ModelType]*RateLimit

	// Request routing
	mu sync.RWMutex
}

// RateLimit tracks API rate limits per model
type RateLimit struct {
	MaxRequests int           // Max requests per window
	Window      time.Duration // Time window
	Current     int           // Current count
	ResetAt     time.Time     // When to reset counter
	mu          sync.Mutex
}

// NewRaftRouter creates a new Raft-powered router
func NewRaftRouter(raftServer *structs.Server) *RaftRouter {
	router := &RaftRouter{
		raftServer: raftServer,
		serverPool: NewModelServerPool(),
		rateLimits: make(map[ModelType]*RateLimit),
	}

	// Initialize rate limits (example values)
	router.rateLimits[ModelGemini] = &RateLimit{
		MaxRequests: 100,
		Window:      time.Minute,
		ResetAt:     time.Now().Add(time.Minute),
	}
	router.rateLimits[ModelBERT] = &RateLimit{
		MaxRequests: 200,
		Window:      time.Minute,
		ResetAt:     time.Now().Add(time.Minute),
	}
	router.rateLimits[ModelT5] = &RateLimit{
		MaxRequests: 150,
		Window:      time.Minute,
		ResetAt:     time.Now().Add(time.Minute),
	}

	return router
}

// RouteRequest routes a model request through the cluster
func (r *RaftRouter) RouteRequest(req *ModelRequest) (*ModelResponse, error) {
	// Check if we're the leader
	if !r.raftServer.IsLeader() {
		return nil, fmt.Errorf("not leader, redirect to: %s", r.raftServer.LeaderID)
	}

	// Check global rate limit
	if !r.checkRateLimit(req.ModelType) {
		return nil, fmt.Errorf("rate limit exceeded for model: %s", req.ModelType)
	}

	// Select best server from pool
	server, err := r.serverPool.SelectServer(req.ModelType)
	if err != nil {
		return nil, fmt.Errorf("no available servers: %w", err)
	}

	// Forward request to selected server
	log.Printf("[Router] Routing request %s to server %s (%s)",
		req.ID, server.ID, server.Address)

	// In real implementation, this would make HTTP/gRPC call
	// For now, simulate response
	response := &ModelResponse{
		RequestID: req.ID,
		Output:    fmt.Sprintf("Response from %s server %s", req.ModelType, server.ID),
		Metadata: map[string]interface{}{
			"server_id": server.ID,
			"latency":   50 * time.Millisecond,
		},
	}

	// Update metrics through Raft
	r.updateServerMetrics(server.ID, response)

	return response, nil
}

// RegisterServer registers a new model server through Raft consensus
func (r *RaftRouter) RegisterServer(server *ModelServer) error {
	// Only leader can register servers
	if !r.raftServer.IsLeader() {
		return fmt.Errorf("not leader, redirect to: %s", r.raftServer.LeaderID)
	}

	log.Printf("[Router] Registering server %s (%s) for model %s",
		server.ID, server.Address, server.ModelType)

	// Propose command through Raft (ensures all nodes have same state)
	cmd := RouterCommand{
		Type:     "REGISTER",
		ServerID: server.ID,
		Data: map[string]interface{}{
			"address":    server.Address,
			"model_type": string(server.ModelType),
		},
		Timestamp: time.Now(),
	}

	// TODO: In full implementation, propose through Raft:
	// result := r.raftServer.ProposeAndWait(cmd)
	// For now, apply to local state directly
	_ = cmd // Prevent unused warning

	// Apply to local state
	r.serverPool.RegisterServer(server)

	log.Printf("[Router] Server %s registered successfully", server.ID)
	return nil
}

// UnregisterServer removes a server from the pool
func (r *RaftRouter) UnregisterServer(serverID string) error {
	if !r.raftServer.IsLeader() {
		return fmt.Errorf("not leader")
	}

	r.serverPool.mu.Lock()
	delete(r.serverPool.Servers, serverID)
	r.serverPool.mu.Unlock()

	log.Printf("[Router] Server %s unregistered", serverID)
	return nil
}

// checkRateLimit checks if request is within rate limit
func (r *RaftRouter) checkRateLimit(modelType ModelType) bool {
	limit, ok := r.rateLimits[modelType]
	if !ok {
		return true // No limit configured
	}

	limit.mu.Lock()
	defer limit.mu.Unlock()

	// Reset counter if window expired
	if time.Now().After(limit.ResetAt) {
		limit.Current = 0
		limit.ResetAt = time.Now().Add(limit.Window)
	}

	// Check if within limit
	if limit.Current >= limit.MaxRequests {
		return false
	}

	limit.Current++
	return true
}

// updateServerMetrics updates server performance metrics
func (r *RaftRouter) updateServerMetrics(serverID string, response *ModelResponse) {
	r.serverPool.mu.Lock()
	defer r.serverPool.mu.Unlock()

	server, ok := r.serverPool.Servers[serverID]
	if !ok {
		return
	}

	server.RequestCount++
	if response.Error != nil {
		server.ErrorCount++
	}

	// Update average latency (simple moving average)
	if latency, ok := response.Metadata["latency"].(time.Duration); ok {
		if server.RequestCount == 1 {
			server.AvgLatency = latency
		} else {
			// Exponential moving average
			alpha := 0.2
			server.AvgLatency = time.Duration(
				alpha*float64(latency) + (1-alpha)*float64(server.AvgLatency),
			)
		}
	}
}

// StartHealthChecks starts background health checking
func (r *RaftRouter) StartHealthChecks() {
	ticker := time.NewTicker(10 * time.Second)
	go func() {
		for range ticker.C {
			r.performHealthChecks()
		}
	}()
}

// performHealthChecks checks health of all registered servers
func (r *RaftRouter) performHealthChecks() {
	r.serverPool.mu.Lock()
	defer r.serverPool.mu.Unlock()

	for _, server := range r.serverPool.Servers {
		// In real implementation, this would ping the server
		// For now, simulate health check
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		err := r.pingServer(ctx, server)
		if err != nil {
			server.IsHealthy = false
			log.Printf("[HealthCheck] Server %s is unhealthy: %v", server.ID, err)
		} else {
			server.IsHealthy = true
			server.LastHealthy = time.Now()
		}
	}
}

// pingServer performs health check on a server
func (r *RaftRouter) pingServer(ctx context.Context, server *ModelServer) error {
	// Simulate health check (in real implementation, make HTTP/gRPC call)
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(100 * time.Millisecond):
		// Assume server is healthy for now
		return nil
	}
}

// GetClusterStatus returns current cluster status
func (r *RaftRouter) GetClusterStatus() ClusterStatus {
	r.mu.RLock()
	defer r.mu.RUnlock()

	status := ClusterStatus{
		IsLeader:    r.raftServer.IsLeader(),
		LeaderID:    r.raftServer.LeaderID,
		CurrentTerm: r.raftServer.GetCurrentTerm(),
		Servers:     make(map[string]ServerStatus),
	}

	r.serverPool.mu.RLock()
	for id, server := range r.serverPool.Servers {
		status.Servers[id] = ServerStatus{
			ID:           server.ID,
			ModelType:    string(server.ModelType),
			IsHealthy:    server.IsHealthy,
			RequestCount: server.RequestCount,
			ErrorCount:   server.ErrorCount,
			AvgLatency:   server.AvgLatency.String(),
		}
	}
	r.serverPool.mu.RUnlock()

	return status
}

// ClusterStatus represents the current state of the cluster
type ClusterStatus struct {
	IsLeader    bool                    `json:"is_leader"`
	LeaderID    string                  `json:"leader_id"`
	CurrentTerm int                     `json:"current_term"`
	Servers     map[string]ServerStatus `json:"servers"`
}

// ServerStatus represents status of a single server
type ServerStatus struct {
	ID           string `json:"id"`
	ModelType    string `json:"model_type"`
	IsHealthy    bool   `json:"is_healthy"`
	RequestCount int64  `json:"request_count"`
	ErrorCount   int64  `json:"error_count"`
	AvgLatency   string `json:"avg_latency"`
}
