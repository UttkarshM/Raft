package structs

import (
	"sync"

	"github.com/uttkarshm/raft/proto"
	"google.golang.org/grpc"
)

type NodeState int

const (
	Follower  NodeState = iota // iota starts at 0
	Candidate NodeState = 1    // automatically 1
	Leader    NodeState = 2    // automatically 2
)

type Node struct {
	state       NodeState
	currentTerm int
	votedFor    string
}

func (n *Node) BecomeCandidate() {
	n.state = Candidate
}

func (n *Node) BecomeFollower() {
	n.state = Follower
}

func (n *Node) BecomeLeader() {
	n.state = Leader
}

// LogEntry represents a single entry in the Raft log
type LogEntry struct {
	Term    int
	Index   int
	Command interface{}
}

// Server represents a Raft server/node
type Server struct {
	// Server identity
	ID      string
	Address string
	Peers   []string

	LeaderID string

	// Raft node state
	Node *Node

	// Persistent state (must be saved to stable storage before responding to RPCs)
	Log []LogEntry

	// Volatile state on all servers
	CommitIndex int // index of highest log entry known to be committed
	LastApplied int // index of highest log entry applied to state machine

	// Volatile state on leaders (reinitialized after election)
	NextIndex  map[string]int // for each server, index of next log entry to send
	MatchIndex map[string]int // for each server, index of highest log entry known to be replicated

	// Election timing
	ElectionTimeout  int
	HeartbeatTimeout int

	// Concurrency control
	mu sync.RWMutex // Protects all server state

	// Timer management
	electionTimer       chan struct{} // Channel to reset election timer
	stopElectionTimer   chan struct{} // Channel to stop election timer
	stopHeartbeat       chan struct{} // Channel to stop heartbeat
	heartbeatRunning    bool          // Whether heartbeat is currently running

	// State machine
	stateMachine StateMachine // Application state machine

	proto.UnimplementedRaftServiceServer                                    // Embedded for forward compatibility
	grpcServer                           *grpc.Server                       // The gRPC server instance
	peerConnections                      map[string]*grpc.ClientConn        // Connections to other nodes
	peerClients                          map[string]proto.RaftServiceClient // RPC clients for calling peers
}

// Client represents a Raft client that connects to the cluster
type Client struct {
	ID            string
	ClusterNodes  []string                       // addresses of all known cluster nodes
	CurrentLeader string                         // address of the current known leader
	RequestID     int                            // for tracking requests
	connections   map[string]*grpc.ClientConn    // gRPC connections to nodes
	clients       map[string]proto.RaftServiceClient // gRPC clients for nodes
	mu            sync.RWMutex                   // Protects client state
}
