package structs

import "log"

// StateMachine represents the application state machine
// This is what actually executes the commands once they're committed
type StateMachine interface {
	// Apply applies a committed command to the state machine
	// Returns the result of applying the command
	Apply(command interface{}) interface{}
}

// SimpleStateMachine is a basic key-value store implementation
type SimpleStateMachine struct {
	data map[string]string
}

// NewSimpleStateMachine creates a new simple state machine
func NewSimpleStateMachine() *SimpleStateMachine {
	return &SimpleStateMachine{
		data: make(map[string]string),
	}
}

// Apply applies a command to the state machine
func (sm *SimpleStateMachine) Apply(command interface{}) interface{} {
	// For now, just log the command
	// In a real implementation, you would parse the command
	// and execute operations like SET, GET, DELETE, etc.
	log.Printf("StateMachine: Applying command: %v", command)

	// Example: command could be a struct with Operation and Key/Value
	// For now, we just acknowledge it was applied
	return "OK"
}
