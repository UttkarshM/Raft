package modelrouter

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// ModelType represents different AI model types
type ModelType string

const (
	ModelGemini ModelType = "gemini"
	ModelBERT   ModelType = "bert"
	ModelT5     ModelType = "t5"
)

// ModelRequest represents a request to an AI model
type ModelRequest struct {
	ID        string                 // Unique request ID
	ModelType ModelType              // Which model to use
	Input     string                 // Input text/data
	Params    map[string]interface{} // Model-specific parameters
	Timeout   time.Duration          // Request timeout
}

// ModelResponse represents a response from an AI model
type ModelResponse struct {
	RequestID string                 // Original request ID
	Output    string                 // Model output
	Metadata  map[string]interface{} // Additional metadata (latency, tokens, etc.)
	Error     error                  // Error if any
}

// ModelServer represents a single AI model server
type ModelServer struct {
	ID           string        // Unique server ID
	Address      string        // Server address (localhost:8001, etc.)
	ModelType    ModelType     // Which model this server serves
	IsHealthy    bool          // Health status
	LastHealthy  time.Time     // Last successful health check
	RequestCount int64         // Total requests handled
	ErrorCount   int64         // Total errors
	AvgLatency   time.Duration // Average response time
}

// ModelServerInterface defines operations on a model server
type ModelServerInterface interface {
	// Predict sends a request to the model server
	Predict(ctx context.Context, req *ModelRequest) (*ModelResponse, error)

	// HealthCheck checks if the server is healthy
	HealthCheck(ctx context.Context) error

	// GetMetrics returns server metrics
	GetMetrics() ServerMetrics
}

// ServerMetrics contains server performance metrics
type ServerMetrics struct {
	RequestCount int64
	ErrorCount   int64
	AvgLatency   time.Duration
	LastUpdated  time.Time
}

// ModelServerPool manages multiple model servers
type ModelServerPool struct {
	Servers map[string]*ModelServer // Server ID -> Server
	mu      sync.RWMutex            // Thread-safe access
}

// NewModelServerPool creates a new server pool
func NewModelServerPool() *ModelServerPool {
	return &ModelServerPool{
		Servers: make(map[string]*ModelServer),
	}
}

// RegisterServer adds a server to the pool
func (pool *ModelServerPool) RegisterServer(server *ModelServer) {
	pool.mu.Lock()
	defer pool.mu.Unlock()
	pool.Servers[server.ID] = server
}

// GetHealthyServers returns all healthy servers for a model type
func (pool *ModelServerPool) GetHealthyServers(modelType ModelType) []*ModelServer {
	pool.mu.RLock()
	defer pool.mu.RUnlock()

	var healthy []*ModelServer
	for _, server := range pool.Servers {
		if server.ModelType == modelType && server.IsHealthy {
			healthy = append(healthy, server)
		}
	}
	return healthy
}

// SelectServer selects the best server for a request (load balancing)
func (pool *ModelServerPool) SelectServer(modelType ModelType) (*ModelServer, error) {
	servers := pool.GetHealthyServers(modelType)
	if len(servers) == 0 {
		return nil, fmt.Errorf("no healthy servers for model type: %s", modelType)
	}

	// Round-robin or least-loaded selection
	// For now, return first healthy server
	return servers[0], nil
}
