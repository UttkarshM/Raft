package workflow

import (
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
)

// WorkflowStateMachine manages distributed workflow state
type WorkflowStateMachine struct {
	mu sync.RWMutex

	// Replicated state (consistent across all nodes)
	Workflows  map[string]*Workflow          // workflowID -> Workflow
	Executions map[string]*WorkflowExecution // executionID -> Execution

	// Indexes for fast queries
	WorkflowsByUser   map[string][]string // userID -> []workflowID
	ExecutionsByWorkflow map[string][]string // workflowID -> []executionID
}

// NewWorkflowStateMachine creates a new state machine
func NewWorkflowStateMachine() *WorkflowStateMachine {
	return &WorkflowStateMachine{
		Workflows:            make(map[string]*Workflow),
		Executions:           make(map[string]*WorkflowExecution),
		WorkflowsByUser:      make(map[string][]string),
		ExecutionsByWorkflow: make(map[string][]string),
	}
}

// Apply implements the Raft StateMachine interface
// This is called when a log entry is committed and needs to be applied
func (sm *WorkflowStateMachine) Apply(command interface{}) interface{} {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	// Deserialize command from []byte (Raft log entry)
	var cmdBytes []byte
	var ok bool

	// Try to convert to []byte
	if cmdBytes, ok = command.([]byte); !ok {
		// If it's already a string, convert it
		if cmdStr, ok := command.(string); ok {
			cmdBytes = []byte(cmdStr)
		} else {
			log.Printf("[WorkflowSM] Invalid command type: %T", command)
			return CommandResult{
				Success: false,
				Error:   "invalid command type",
			}
		}
	}

	var cmd WorkflowCommand
	if err := json.Unmarshal(cmdBytes, &cmd); err != nil {
		log.Printf("[WorkflowSM] Failed to unmarshal command: %v", err)
		return CommandResult{
			Success: false,
			Error:   fmt.Sprintf("failed to unmarshal command: %v", err),
		}
	}

	log.Printf("[WorkflowSM] Applying command: %s (RequestID: %s)", cmd.Type, cmd.RequestID)

	// Route to appropriate handler
	var result interface{}
	var err error

	switch cmd.Type {
	case CMD_CREATE_WORKFLOW:
		result, err = sm.applyCreateWorkflow(cmd)
	case CMD_UPDATE_WORKFLOW:
		result, err = sm.applyUpdateWorkflow(cmd)
	case CMD_DELETE_WORKFLOW:
		result, err = sm.applyDeleteWorkflow(cmd)
	case CMD_START_EXECUTION:
		result, err = sm.applyStartExecution(cmd)
	case CMD_UPDATE_EXECUTION:
		result, err = sm.applyUpdateExecution(cmd)
	case CMD_COMPLETE_EXECUTION:
		result, err = sm.applyCompleteExecution(cmd)
	case CMD_LOG_NODE_RESULT:
		result, err = sm.applyLogNodeResult(cmd)
	default:
		err = fmt.Errorf("unknown command type: %s", cmd.Type)
	}

	if err != nil {
		log.Printf("[WorkflowSM] Command failed: %v", err)
		return CommandResult{
			Success: false,
			Error:   err.Error(),
		}
	}

	return CommandResult{
		Success: true,
		Data:    result,
	}
}

// Command Handlers (deterministic operations)

func (sm *WorkflowStateMachine) applyCreateWorkflow(cmd WorkflowCommand) (interface{}, error) {
	workflowID := GetStringFromData(cmd.Data, "id")
	if workflowID == "" {
		return nil, fmt.Errorf("workflow id is required")
	}

	name := GetStringFromData(cmd.Data, "name")
	description := GetStringFromData(cmd.Data, "description")

	nodes, err := GetNodesFromData(cmd.Data)
	if err != nil {
		return nil, fmt.Errorf("invalid nodes: %v", err)
	}

	edges, err := GetEdgesFromData(cmd.Data)
	if err != nil {
		return nil, fmt.Errorf("invalid edges: %v", err)
	}

	workflow := &Workflow{
		ID:          workflowID,
		Name:        name,
		Description: description,
		Nodes:       nodes,
		Edges:       edges,
		UserID:      cmd.UserID,
		CreatedAt:   cmd.Timestamp,
		UpdatedAt:   cmd.Timestamp,
	}

	sm.Workflows[workflowID] = workflow
	sm.WorkflowsByUser[cmd.UserID] = append(sm.WorkflowsByUser[cmd.UserID], workflowID)

	log.Printf("[WorkflowSM] Created workflow: %s (%s)", workflow.Name, workflowID)

	return map[string]interface{}{
		"workflow": workflow,
	}, nil
}

func (sm *WorkflowStateMachine) applyUpdateWorkflow(cmd WorkflowCommand) (interface{}, error) {
	workflowID := GetStringFromData(cmd.Data, "id")
	if workflowID == "" {
		return nil, fmt.Errorf("workflow id is required")
	}

	workflow, exists := sm.Workflows[workflowID]
	if !exists {
		return nil, fmt.Errorf("workflow not found: %s", workflowID)
	}

	// Verify ownership
	if workflow.UserID != cmd.UserID {
		return nil, fmt.Errorf("unauthorized: workflow belongs to different user")
	}

	// Update fields if provided
	if name := GetStringFromData(cmd.Data, "name"); name != "" {
		workflow.Name = name
	}
	if description := GetStringFromData(cmd.Data, "description"); description != "" {
		workflow.Description = description
	}

	if nodes, err := GetNodesFromData(cmd.Data); err == nil && len(nodes) > 0 {
		workflow.Nodes = nodes
	}
	if edges, err := GetEdgesFromData(cmd.Data); err == nil && len(edges) > 0 {
		workflow.Edges = edges
	}

	workflow.UpdatedAt = cmd.Timestamp

	log.Printf("[WorkflowSM] Updated workflow: %s", workflowID)

	return map[string]interface{}{
		"workflow": workflow,
	}, nil
}

func (sm *WorkflowStateMachine) applyDeleteWorkflow(cmd WorkflowCommand) (interface{}, error) {
	workflowID := GetStringFromData(cmd.Data, "id")
	if workflowID == "" {
		return nil, fmt.Errorf("workflow id is required")
	}

	workflow, exists := sm.Workflows[workflowID]
	if !exists {
		return nil, fmt.Errorf("workflow not found: %s", workflowID)
	}

	// Verify ownership
	if workflow.UserID != cmd.UserID {
		return nil, fmt.Errorf("unauthorized: workflow belongs to different user")
	}

	// Remove from main map
	delete(sm.Workflows, workflowID)

	// Remove from user index
	userWorkflows := sm.WorkflowsByUser[workflow.UserID]
	for i, id := range userWorkflows {
		if id == workflowID {
			sm.WorkflowsByUser[workflow.UserID] = append(userWorkflows[:i], userWorkflows[i+1:]...)
			break
		}
	}

	log.Printf("[WorkflowSM] Deleted workflow: %s", workflowID)

	return map[string]interface{}{
		"deleted": true,
	}, nil
}

func (sm *WorkflowStateMachine) applyStartExecution(cmd WorkflowCommand) (interface{}, error) {
	executionID := GetStringFromData(cmd.Data, "id")
	workflowID := GetStringFromData(cmd.Data, "workflow_id")

	if executionID == "" || workflowID == "" {
		return nil, fmt.Errorf("execution_id and workflow_id are required")
	}

	// Verify workflow exists
	workflow, exists := sm.Workflows[workflowID]
	if !exists {
		return nil, fmt.Errorf("workflow not found: %s", workflowID)
	}

	// Verify ownership
	if workflow.UserID != cmd.UserID {
		return nil, fmt.Errorf("unauthorized: workflow belongs to different user")
	}

	inputData := GetMapFromData(cmd.Data, "input_data")
	if inputData == nil {
		inputData = make(map[string]interface{})
	}

	execution := &WorkflowExecution{
		ID:          executionID,
		WorkflowID:  workflowID,
		Status:      StatusRunning,
		InputData:   inputData,
		OutputData:  make(map[string]interface{}),
		NodeResults: make(map[string]interface{}),
		StartedAt:   cmd.Timestamp,
	}

	sm.Executions[executionID] = execution
	sm.ExecutionsByWorkflow[workflowID] = append(sm.ExecutionsByWorkflow[workflowID], executionID)

	log.Printf("[WorkflowSM] Started execution: %s for workflow: %s", executionID, workflowID)

	return map[string]interface{}{
		"execution": execution,
	}, nil
}

func (sm *WorkflowStateMachine) applyUpdateExecution(cmd WorkflowCommand) (interface{}, error) {
	executionID := GetStringFromData(cmd.Data, "id")
	if executionID == "" {
		return nil, fmt.Errorf("execution id is required")
	}

	execution, exists := sm.Executions[executionID]
	if !exists {
		return nil, fmt.Errorf("execution not found: %s", executionID)
	}

	// Verify ownership through workflow
	workflow := sm.Workflows[execution.WorkflowID]
	if workflow == nil || workflow.UserID != cmd.UserID {
		return nil, fmt.Errorf("unauthorized")
	}

	// Update fields if provided
	if status := GetStringFromData(cmd.Data, "status"); status != "" {
		execution.Status = status
	}
	if outputData := GetMapFromData(cmd.Data, "output_data"); outputData != nil {
		execution.OutputData = outputData
	}
	if errorMsg := GetStringFromData(cmd.Data, "error_message"); errorMsg != "" {
		execution.ErrorMessage = errorMsg
	}

	log.Printf("[WorkflowSM] Updated execution: %s (status: %s)", executionID, execution.Status)

	return map[string]interface{}{
		"execution": execution,
	}, nil
}

func (sm *WorkflowStateMachine) applyCompleteExecution(cmd WorkflowCommand) (interface{}, error) {
	executionID := GetStringFromData(cmd.Data, "id")
	if executionID == "" {
		return nil, fmt.Errorf("execution id is required")
	}

	execution, exists := sm.Executions[executionID]
	if !exists {
		return nil, fmt.Errorf("execution not found: %s", executionID)
	}

	// Verify ownership
	workflow := sm.Workflows[execution.WorkflowID]
	if workflow == nil || workflow.UserID != cmd.UserID {
		return nil, fmt.Errorf("unauthorized")
	}

	execution.Status = StatusCompleted
	completedAt := cmd.Timestamp
	execution.CompletedAt = &completedAt

	if outputData := GetMapFromData(cmd.Data, "output_data"); outputData != nil {
		execution.OutputData = outputData
	}

	log.Printf("[WorkflowSM] Completed execution: %s", executionID)

	return map[string]interface{}{
		"execution": execution,
	}, nil
}

func (sm *WorkflowStateMachine) applyLogNodeResult(cmd WorkflowCommand) (interface{}, error) {
	executionID := GetStringFromData(cmd.Data, "execution_id")
	nodeID := GetStringFromData(cmd.Data, "node_id")

	if executionID == "" || nodeID == "" {
		return nil, fmt.Errorf("execution_id and node_id are required")
	}

	execution, exists := sm.Executions[executionID]
	if !exists {
		return nil, fmt.Errorf("execution not found: %s", executionID)
	}

	// Store the entire node result data
	if nodeResultData := GetMapFromData(cmd.Data, "result"); nodeResultData != nil {
		execution.NodeResults[nodeID] = nodeResultData
	}

	log.Printf("[WorkflowSM] Logged result for node: %s in execution: %s", nodeID, executionID)

	return map[string]interface{}{
		"success": true,
	}, nil
}

// Read Methods (can be called without consensus)

// GetWorkflow retrieves a workflow by ID
func (sm *WorkflowStateMachine) GetWorkflow(id string) (*Workflow, error) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	workflow, exists := sm.Workflows[id]
	if !exists {
		return nil, fmt.Errorf("workflow not found")
	}
	return workflow, nil
}

// ListWorkflows retrieves workflows for a user with filters
func (sm *WorkflowStateMachine) ListWorkflows(filters WorkflowFilters) ([]*Workflow, int) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	workflowIDs := sm.WorkflowsByUser[filters.UserID]
	var results []*Workflow

	for _, id := range workflowIDs {
		workflow := sm.Workflows[id]
		if workflow == nil {
			continue
		}

		// Apply search filter
		if filters.Search != "" {
			searchLower := strings.ToLower(filters.Search)
			if !strings.Contains(strings.ToLower(workflow.Name), searchLower) &&
				!strings.Contains(strings.ToLower(workflow.Description), searchLower) {
				continue
			}
		}

		results = append(results, workflow)
	}

	// Sort by created_at desc
	sort.Slice(results, func(i, j int) bool {
		return results[i].CreatedAt.After(results[j].CreatedAt)
	})

	// Pagination
	total := len(results)
	if filters.PageSize == 0 {
		filters.PageSize = 10
	}
	if filters.Page == 0 {
		filters.Page = 1
	}

	start := (filters.Page - 1) * filters.PageSize
	end := start + filters.PageSize

	if start > total {
		return []*Workflow{}, total
	}
	if end > total {
		end = total
	}

	return results[start:end], total
}

// GetExecution retrieves an execution by ID
func (sm *WorkflowStateMachine) GetExecution(id string) (*WorkflowExecution, error) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	execution, exists := sm.Executions[id]
	if !exists {
		return nil, fmt.Errorf("execution not found")
	}
	return execution, nil
}

// ListExecutions retrieves executions for a workflow
func (sm *WorkflowStateMachine) ListExecutions(filters ExecutionFilters) ([]*WorkflowExecution, int) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	executionIDs := sm.ExecutionsByWorkflow[filters.WorkflowID]
	var results []*WorkflowExecution

	for _, id := range executionIDs {
		execution := sm.Executions[id]
		if execution == nil {
			continue
		}

		// Apply status filter
		if filters.Status != "" && execution.Status != filters.Status {
			continue
		}

		results = append(results, execution)
	}

	// Sort by started_at desc
	sort.Slice(results, func(i, j int) bool {
		return results[i].StartedAt.After(results[j].StartedAt)
	})

	// Pagination
	total := len(results)
	if filters.PageSize == 0 {
		filters.PageSize = 10
	}
	if filters.Page == 0 {
		filters.Page = 1
	}

	start := (filters.Page - 1) * filters.PageSize
	end := start + filters.PageSize

	if start > total {
		return []*WorkflowExecution{}, total
	}
	if end > total {
		end = total
	}

	return results[start:end], total
}

// GetStats returns statistics about the state machine
func (sm *WorkflowStateMachine) GetStats() map[string]interface{} {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	runningExecutions := 0
	completedExecutions := 0
	failedExecutions := 0

	for _, exec := range sm.Executions {
		switch exec.Status {
		case StatusRunning:
			runningExecutions++
		case StatusCompleted:
			completedExecutions++
		case StatusFailed:
			failedExecutions++
		}
	}

	return map[string]interface{}{
		"total_workflows":       len(sm.Workflows),
		"total_executions":      len(sm.Executions),
		"running_executions":    runningExecutions,
		"completed_executions":  completedExecutions,
		"failed_executions":     failedExecutions,
		"users_with_workflows":  len(sm.WorkflowsByUser),
	}
}
