package workflow

import (
	"encoding/json"
	"time"
)

// CommandType defines all operations on the workflow state machine
type CommandType string

const (
	CMD_CREATE_WORKFLOW   CommandType = "CREATE_WORKFLOW"
	CMD_UPDATE_WORKFLOW   CommandType = "UPDATE_WORKFLOW"
	CMD_DELETE_WORKFLOW   CommandType = "DELETE_WORKFLOW"
	CMD_START_EXECUTION   CommandType = "START_EXECUTION"
	CMD_UPDATE_EXECUTION  CommandType = "UPDATE_EXECUTION"
	CMD_COMPLETE_EXECUTION CommandType = "COMPLETE_EXECUTION"
	CMD_LOG_NODE_RESULT   CommandType = "LOG_NODE_RESULT"
)

// WorkflowCommand is the top-level command envelope sent through Raft
type WorkflowCommand struct {
	Type      CommandType            `json:"type"`
	Data      map[string]interface{} `json:"data"`
	UserID    string                 `json:"user_id"`
	Timestamp time.Time              `json:"timestamp"`
	RequestID string                 `json:"request_id"`
}

// Workflow represents a visual automation workflow
type Workflow struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Nodes       []WorkflowNode `json:"nodes"`
	Edges       []WorkflowEdge `json:"edges"`
	UserID      string         `json:"user_id"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
}

// WorkflowNode represents a single node in the workflow
type WorkflowNode struct {
	ID       string                 `json:"id"`
	Type     string                 `json:"type"` // input, prompt, api, logic, output, fileUpload, aiModel
	Data     map[string]interface{} `json:"data"`
	Position Position               `json:"position"`
}

// Position represents the X,Y coordinates of a node on the canvas
type Position struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// WorkflowEdge represents a connection between two nodes
type WorkflowEdge struct {
	ID           string `json:"id"`
	Source       string `json:"source"`
	Target       string `json:"target"`
	SourceHandle string `json:"source_handle,omitempty"`
	TargetHandle string `json:"target_handle,omitempty"`
}

// WorkflowExecution represents a single execution of a workflow
type WorkflowExecution struct {
	ID           string                 `json:"id"`
	WorkflowID   string                 `json:"workflow_id"`
	Status       string                 `json:"status"` // running, completed, failed
	InputData    map[string]interface{} `json:"input_data"`
	OutputData   map[string]interface{} `json:"output_data"`
	NodeResults  map[string]interface{} `json:"node_results"`
	StartedAt    time.Time              `json:"started_at"`
	CompletedAt  *time.Time             `json:"completed_at,omitempty"`
	ErrorMessage string                 `json:"error_message,omitempty"`
}

// ExecutionStatus constants
const (
	StatusRunning   = "running"
	StatusCompleted = "completed"
	StatusFailed    = "failed"
)

// WorkflowFilters for querying workflows
type WorkflowFilters struct {
	UserID   string
	Search   string
	Page     int
	PageSize int
}

// WorkflowListResponse for paginated workflow lists
type WorkflowListResponse struct {
	Items      []Workflow `json:"items"`
	Total      int        `json:"total"`
	TotalPages int        `json:"total_pages"`
}

// ExecutionFilters for querying executions
type ExecutionFilters struct {
	WorkflowID string
	Status     string
	Page       int
	PageSize   int
}

// CommandResult represents the result of applying a command
type CommandResult struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
}

// MarshalCommand serializes a WorkflowCommand to bytes for Raft log
func MarshalCommand(cmd WorkflowCommand) ([]byte, error) {
	return json.Marshal(cmd)
}

// UnmarshalCommand deserializes bytes from Raft log to WorkflowCommand
func UnmarshalCommand(data []byte) (*WorkflowCommand, error) {
	var cmd WorkflowCommand
	err := json.Unmarshal(data, &cmd)
	if err != nil {
		return nil, err
	}
	return &cmd, nil
}

// Helper functions for type assertions and conversions

// GetStringFromData safely extracts a string from map[string]interface{}
func GetStringFromData(data map[string]interface{}, key string) string {
	if val, ok := data[key]; ok {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return ""
}

// GetMapFromData safely extracts a map from map[string]interface{}
func GetMapFromData(data map[string]interface{}, key string) map[string]interface{} {
	if val, ok := data[key]; ok {
		if m, ok := val.(map[string]interface{}); ok {
			return m
		}
	}
	return nil
}

// GetNodesFromData extracts WorkflowNodes from data map
func GetNodesFromData(data map[string]interface{}) ([]WorkflowNode, error) {
	if nodesData, ok := data["nodes"]; ok {
		// Re-marshal and unmarshal to convert to proper type
		nodesJSON, err := json.Marshal(nodesData)
		if err != nil {
			return nil, err
		}

		var nodes []WorkflowNode
		err = json.Unmarshal(nodesJSON, &nodes)
		if err != nil {
			return nil, err
		}
		return nodes, nil
	}
	return []WorkflowNode{}, nil
}

// GetEdgesFromData extracts WorkflowEdges from data map
func GetEdgesFromData(data map[string]interface{}) ([]WorkflowEdge, error) {
	if edgesData, ok := data["edges"]; ok {
		// Re-marshal and unmarshal to convert to proper type
		edgesJSON, err := json.Marshal(edgesData)
		if err != nil {
			return nil, err
		}

		var edges []WorkflowEdge
		err = json.Unmarshal(edgesJSON, &edges)
		if err != nil {
			return nil, err
		}
		return edges, nil
	}
	return []WorkflowEdge{}, nil
}
