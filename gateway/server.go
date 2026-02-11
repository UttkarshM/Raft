package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/uttkarshm/raft/structs"
	"github.com/uttkarshm/raft/workflow"
)

// GatewayServer provides HTTP API for Next.js app to communicate with Raft cluster
type GatewayServer struct {
	raftServer   *structs.Server
	stateMachine *workflow.WorkflowStateMachine
	httpServer   *http.Server
	clusterNodes []string // All Raft node addresses for reads
	httpPort     string
}

// NewGatewayServer creates a new HTTP gateway
func NewGatewayServer(raftServer *structs.Server, stateMachine *workflow.WorkflowStateMachine, httpPort string) *GatewayServer {
	gw := &GatewayServer{
		raftServer:   raftServer,
		stateMachine: stateMachine,
		clusterNodes: raftServer.Peers,
		httpPort:     httpPort,
	}

	// Setup HTTP routes
	mux := http.NewServeMux()

	// Workflow CRUD operations
	mux.HandleFunc("/api/workflows/create", gw.handleCreateWorkflow)
	mux.HandleFunc("/api/workflows/list", gw.handleListWorkflows)
	mux.HandleFunc("/api/workflows/get", gw.handleGetWorkflow)
	mux.HandleFunc("/api/workflows/update", gw.handleUpdateWorkflow)
	mux.HandleFunc("/api/workflows/delete", gw.handleDeleteWorkflow)

	// Execution operations
	mux.HandleFunc("/api/executions/start", gw.handleStartExecution)
	mux.HandleFunc("/api/executions/update", gw.handleUpdateExecution)
	mux.HandleFunc("/api/executions/complete", gw.handleCompleteExecution)
	mux.HandleFunc("/api/executions/log-node", gw.handleLogNodeResult)
	mux.HandleFunc("/api/executions/get", gw.handleGetExecution)

	// Cluster management
	mux.HandleFunc("/api/cluster/status", gw.handleClusterStatus)
	mux.HandleFunc("/api/cluster/stats", gw.handleStats)
	mux.HandleFunc("/health", gw.handleHealth)

	gw.httpServer = &http.Server{
		Addr:         ":" + httpPort,
		Handler:      gw.corsMiddleware(mux),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	return gw
}

// Start starts the HTTP server
func (gw *GatewayServer) Start() error {
	log.Printf("[Gateway] Starting HTTP server on port %s", gw.httpPort)
	return gw.httpServer.ListenAndServe()
}

// Stop gracefully stops the HTTP server
func (gw *GatewayServer) Stop() error {
	log.Printf("[Gateway] Shutting down HTTP server...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return gw.httpServer.Shutdown(ctx)
}

// ==================== WORKFLOW HANDLERS ====================

func (gw *GatewayServer) handleCreateWorkflow(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Name        string                `json:"name"`
		Description string                `json:"description"`
		Nodes       []workflow.WorkflowNode `json:"nodes"`
		Edges       []workflow.WorkflowEdge `json:"edges"`
		UserID      string                `json:"user_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request body: %v", err), http.StatusBadRequest)
		return
	}

	// Validate required fields
	if req.Name == "" || req.UserID == "" {
		http.Error(w, "name and user_id are required", http.StatusBadRequest)
		return
	}

	// Check if we're the leader
	if !gw.raftServer.IsLeader() {
		gw.redirectToLeader(w, r)
		return
	}

	// Create command
	cmd := workflow.WorkflowCommand{
		Type:      workflow.CMD_CREATE_WORKFLOW,
		UserID:    req.UserID,
		Timestamp: time.Now(),
		RequestID: generateID(),
		Data: map[string]interface{}{
			"id":          generateID(),
			"name":        req.Name,
			"description": req.Description,
			"nodes":       req.Nodes,
			"edges":       req.Edges,
		},
	}

	// Propose to Raft and get result
	result, err := gw.proposeCommand(cmd)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to create workflow: %v", err), http.StatusInternalServerError)
		return
	}

	gw.sendJSON(w, result)
}

func (gw *GatewayServer) handleListWorkflows(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse query params
	userID := r.URL.Query().Get("user_id")
	if userID == "" {
		http.Error(w, "user_id is required", http.StatusBadRequest)
		return
	}

	search := r.URL.Query().Get("search")
	page := parseIntOrDefault(r.URL.Query().Get("page"), 1)
	pageSize := parseIntOrDefault(r.URL.Query().Get("page_size"), 10)

	filters := workflow.WorkflowFilters{
		UserID:   userID,
		Search:   search,
		Page:     page,
		PageSize: pageSize,
	}

	// Read from local state machine (no consensus needed)
	workflows, total := gw.stateMachine.ListWorkflows(filters)

	response := workflow.WorkflowListResponse{
		Items:      make([]workflow.Workflow, len(workflows)),
		Total:      total,
		TotalPages: (total + pageSize - 1) / pageSize,
	}

	// Convert pointers to values
	for i, wf := range workflows {
		response.Items[i] = *wf
	}

	gw.sendJSON(w, response)
}

func (gw *GatewayServer) handleGetWorkflow(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "id is required", http.StatusBadRequest)
		return
	}

	workflow, err := gw.stateMachine.GetWorkflow(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	gw.sendJSON(w, workflow)
}

func (gw *GatewayServer) handleUpdateWorkflow(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		ID          string                  `json:"id"`
		Name        string                  `json:"name,omitempty"`
		Description string                  `json:"description,omitempty"`
		Nodes       []workflow.WorkflowNode `json:"nodes,omitempty"`
		Edges       []workflow.WorkflowEdge `json:"edges,omitempty"`
		UserID      string                  `json:"user_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request body: %v", err), http.StatusBadRequest)
		return
	}

	if req.ID == "" || req.UserID == "" {
		http.Error(w, "id and user_id are required", http.StatusBadRequest)
		return
	}

	if !gw.raftServer.IsLeader() {
		gw.redirectToLeader(w, r)
		return
	}

	cmd := workflow.WorkflowCommand{
		Type:      workflow.CMD_UPDATE_WORKFLOW,
		UserID:    req.UserID,
		Timestamp: time.Now(),
		RequestID: generateID(),
		Data: map[string]interface{}{
			"id":          req.ID,
			"name":        req.Name,
			"description": req.Description,
			"nodes":       req.Nodes,
			"edges":       req.Edges,
		},
	}

	result, err := gw.proposeCommand(cmd)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to update workflow: %v", err), http.StatusInternalServerError)
		return
	}

	gw.sendJSON(w, result)
}

func (gw *GatewayServer) handleDeleteWorkflow(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		ID     string `json:"id"`
		UserID string `json:"user_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request body: %v", err), http.StatusBadRequest)
		return
	}

	if req.ID == "" || req.UserID == "" {
		http.Error(w, "id and user_id are required", http.StatusBadRequest)
		return
	}

	if !gw.raftServer.IsLeader() {
		gw.redirectToLeader(w, r)
		return
	}

	cmd := workflow.WorkflowCommand{
		Type:      workflow.CMD_DELETE_WORKFLOW,
		UserID:    req.UserID,
		Timestamp: time.Now(),
		RequestID: generateID(),
		Data: map[string]interface{}{
			"id": req.ID,
		},
	}

	result, err := gw.proposeCommand(cmd)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to delete workflow: %v", err), http.StatusInternalServerError)
		return
	}

	gw.sendJSON(w, result)
}

// ==================== EXECUTION HANDLERS ====================

func (gw *GatewayServer) handleStartExecution(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		WorkflowID string                 `json:"workflow_id"`
		InputData  map[string]interface{} `json:"input_data"`
		UserID     string                 `json:"user_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request body: %v", err), http.StatusBadRequest)
		return
	}

	if req.WorkflowID == "" || req.UserID == "" {
		http.Error(w, "workflow_id and user_id are required", http.StatusBadRequest)
		return
	}

	if !gw.raftServer.IsLeader() {
		gw.redirectToLeader(w, r)
		return
	}

	cmd := workflow.WorkflowCommand{
		Type:      workflow.CMD_START_EXECUTION,
		UserID:    req.UserID,
		Timestamp: time.Now(),
		RequestID: generateID(),
		Data: map[string]interface{}{
			"id":          generateID(),
			"workflow_id": req.WorkflowID,
			"input_data":  req.InputData,
		},
	}

	result, err := gw.proposeCommand(cmd)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to start execution: %v", err), http.StatusInternalServerError)
		return
	}

	gw.sendJSON(w, result)
}

func (gw *GatewayServer) handleUpdateExecution(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		ID           string                 `json:"id"`
		Status       string                 `json:"status,omitempty"`
		OutputData   map[string]interface{} `json:"output_data,omitempty"`
		ErrorMessage string                 `json:"error_message,omitempty"`
		UserID       string                 `json:"user_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request body: %v", err), http.StatusBadRequest)
		return
	}

	if req.ID == "" || req.UserID == "" {
		http.Error(w, "id and user_id are required", http.StatusBadRequest)
		return
	}

	if !gw.raftServer.IsLeader() {
		gw.redirectToLeader(w, r)
		return
	}

	cmd := workflow.WorkflowCommand{
		Type:      workflow.CMD_UPDATE_EXECUTION,
		UserID:    req.UserID,
		Timestamp: time.Now(),
		RequestID: generateID(),
		Data: map[string]interface{}{
			"id":            req.ID,
			"status":        req.Status,
			"output_data":   req.OutputData,
			"error_message": req.ErrorMessage,
		},
	}

	result, err := gw.proposeCommand(cmd)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to update execution: %v", err), http.StatusInternalServerError)
		return
	}

	gw.sendJSON(w, result)
}

func (gw *GatewayServer) handleCompleteExecution(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		ID         string                 `json:"id"`
		OutputData map[string]interface{} `json:"output_data,omitempty"`
		UserID     string                 `json:"user_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request body: %v", err), http.StatusBadRequest)
		return
	}

	if req.ID == "" || req.UserID == "" {
		http.Error(w, "id and user_id are required", http.StatusBadRequest)
		return
	}

	if !gw.raftServer.IsLeader() {
		gw.redirectToLeader(w, r)
		return
	}

	cmd := workflow.WorkflowCommand{
		Type:      workflow.CMD_COMPLETE_EXECUTION,
		UserID:    req.UserID,
		Timestamp: time.Now(),
		RequestID: generateID(),
		Data: map[string]interface{}{
			"id":          req.ID,
			"output_data": req.OutputData,
		},
	}

	result, err := gw.proposeCommand(cmd)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to complete execution: %v", err), http.StatusInternalServerError)
		return
	}

	gw.sendJSON(w, result)
}

func (gw *GatewayServer) handleLogNodeResult(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		ExecutionID string                 `json:"execution_id"`
		NodeID      string                 `json:"node_id"`
		Result      map[string]interface{} `json:"result"`
		UserID      string                 `json:"user_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request body: %v", err), http.StatusBadRequest)
		return
	}

	if req.ExecutionID == "" || req.NodeID == "" || req.UserID == "" {
		http.Error(w, "execution_id, node_id, and user_id are required", http.StatusBadRequest)
		return
	}

	if !gw.raftServer.IsLeader() {
		gw.redirectToLeader(w, r)
		return
	}

	cmd := workflow.WorkflowCommand{
		Type:      workflow.CMD_LOG_NODE_RESULT,
		UserID:    req.UserID,
		Timestamp: time.Now(),
		RequestID: generateID(),
		Data: map[string]interface{}{
			"execution_id": req.ExecutionID,
			"node_id":      req.NodeID,
			"result":       req.Result,
		},
	}

	result, err := gw.proposeCommand(cmd)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to log node result: %v", err), http.StatusInternalServerError)
		return
	}

	gw.sendJSON(w, result)
}

func (gw *GatewayServer) handleGetExecution(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "id is required", http.StatusBadRequest)
		return
	}

	execution, err := gw.stateMachine.GetExecution(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	gw.sendJSON(w, execution)
}

// ==================== CLUSTER HANDLERS ====================

func (gw *GatewayServer) handleClusterStatus(w http.ResponseWriter, r *http.Request) {
	status := map[string]interface{}{
		"is_leader":    gw.raftServer.IsLeader(),
		"leader_id":    gw.raftServer.LeaderID,
		"current_term": gw.raftServer.GetCurrentTerm(),
		"node_id":      gw.raftServer.ID,
		"node_state":   gw.raftServer.GetState(),
		"http_port":    gw.httpPort,
	}

	gw.sendJSON(w, status)
}

func (gw *GatewayServer) handleStats(w http.ResponseWriter, r *http.Request) {
	stats := gw.stateMachine.GetStats()
	gw.sendJSON(w, stats)
}

func (gw *GatewayServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	health := map[string]interface{}{
		"status": "healthy",
		"node_id": gw.raftServer.ID,
		"is_leader": gw.raftServer.IsLeader(),
	}
	gw.sendJSON(w, health)
}

// ==================== HELPER METHODS ====================

// proposeCommand proposes a command to Raft and waits for it to be committed
func (gw *GatewayServer) proposeCommand(cmd workflow.WorkflowCommand) (interface{}, error) {
	// Serialize command
	cmdBytes, err := workflow.MarshalCommand(cmd)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal command: %v", err)
	}

	// In a full implementation, this would:
	// 1. Append entry to Raft log (with proper locking)
	// 2. Replicate to followers via AppendEntries RPC
	// 3. Wait for replication to majority
	// 4. Commit the entry
	// 5. Apply to state machine
	// 6. Return result
	//
	// For now, we apply directly to the state machine without going through Raft
	// TODO: Implement proper Raft log replication pipeline
	// TODO: Use server.ProposeCommand() method once implemented

	// Append to log (simplified - in reality needs proper Raft integration)
	entry := structs.LogEntry{
		Term:    gw.raftServer.GetCurrentTerm(),
		Index:   len(gw.raftServer.Log),
		Command: cmdBytes,
	}
	gw.raftServer.Log = append(gw.raftServer.Log, entry)

	// Apply to state machine directly
	result := gw.stateMachine.Apply(cmdBytes)

	// Check if it was successful
	if cmdResult, ok := result.(workflow.CommandResult); ok {
		if !cmdResult.Success {
			return nil, fmt.Errorf("%s", cmdResult.Error)
		}
		return cmdResult.Data, nil
	}

	return result, nil
}

// redirectToLeader redirects the request to the current leader
func (gw *GatewayServer) redirectToLeader(w http.ResponseWriter, r *http.Request) {
	leaderID := gw.raftServer.LeaderID
	if leaderID == "" {
		http.Error(w, "No leader available, election in progress", http.StatusServiceUnavailable)
		return
	}

	// Map Raft node ID to HTTP gateway port
	leaderPort := gw.mapNodeIDToHTTPPort(leaderID)
	leaderURL := fmt.Sprintf("http://localhost:%s", leaderPort)

	w.Header().Set("X-Leader-Address", leaderURL)
	http.Error(w, fmt.Sprintf("Not leader, redirect to %s", leaderURL), http.StatusTemporaryRedirect)
}

// mapNodeIDToHTTPPort maps Raft node IDs to HTTP gateway ports
func (gw *GatewayServer) mapNodeIDToHTTPPort(nodeID string) string {
	// Mapping: node1 -> 8001, node2 -> 8002, node3 -> 8003
	mapping := map[string]string{
		"node1": "8001",
		"node2": "8002",
		"node3": "8003",
	}

	if port, ok := mapping[nodeID]; ok {
		return port
	}
	return "8001" // Default fallback
}

// sendJSON sends a JSON response
func (gw *GatewayServer) sendJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("[Gateway] Failed to encode JSON response: %v", err)
	}
}

// corsMiddleware adds CORS headers to all responses
func (gw *GatewayServer) corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// Helper functions

func parseIntOrDefault(s string, defaultValue int) int {
	if s == "" {
		return defaultValue
	}
	val, err := strconv.Atoi(s)
	if err != nil {
		return defaultValue
	}
	return val
}

func generateID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

// forwardToLeader forwards the request to the leader gateway
func (gw *GatewayServer) forwardToLeader(r *http.Request) (map[string]interface{}, error) {
	leaderID := gw.raftServer.LeaderID
	if leaderID == "" {
		return nil, fmt.Errorf("no leader available")
	}

	leaderPort := gw.mapNodeIDToHTTPPort(leaderID)
	leaderURL := fmt.Sprintf("http://localhost:%s%s", leaderPort, r.URL.Path)

	// Read request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read request body: %v", err)
	}

	// Forward request
	resp, err := http.Post(leaderURL, "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to forward request: %v", err)
	}
	defer resp.Body.Close()

	// Read response
	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %v", err)
	}

	return result, nil
}
