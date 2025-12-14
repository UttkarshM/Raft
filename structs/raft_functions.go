package structs

import (
	"context"
	"fmt"
	"log"
	"math/rand/v2"
	"net"
	"sync"
	"time"

	"github.com/uttkarshm/raft/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// ============================================
// Server Methods (like class methods)
// ============================================

// NewServer creates and initializes a new Server instance (like a constructor)
func NewServer(id string, address string, peers []string) *Server {
	return &Server{
		ID:      id,
		Address: address,
		Peers:   peers,
		Node: &Node{
			state:       Follower,
			currentTerm: 0,
			votedFor:    "",
		},
		LeaderID: "",

		Log:         make([]LogEntry, 0),
		CommitIndex: 0,
		LastApplied: 0,
		NextIndex:   make(map[string]int),
		MatchIndex:  make(map[string]int),

		// Initialize timer channels
		electionTimer:     make(chan struct{}, 1),
		stopElectionTimer: make(chan struct{}),
		stopHeartbeat:     make(chan struct{}),
		heartbeatRunning:  false,

		// Initialize gRPC maps
		peerConnections: make(map[string]*grpc.ClientConn),
		peerClients:     make(map[string]proto.RaftServiceClient),

		// Initialize state machine
		stateMachine: NewSimpleStateMachine(),
	}
}

// Start initializes and starts the server
func (s *Server) Start() error {
	log.Printf("[%s] Starting Raft server on %s", s.ID, s.Address)

	// 1. Create gRPC connections to all peers
	if err := s.connectToPeers(); err != nil {
		return fmt.Errorf("failed to connect to peers: %w", err)
	}

	// 2. Start gRPC server
	lis, err := net.Listen("tcp", s.Address)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", s.Address, err)
	}

	s.grpcServer = grpc.NewServer()
	proto.RegisterRaftServiceServer(s.grpcServer, s)

	// Start gRPC server in a goroutine
	go func() {
		log.Printf("[%s] gRPC server listening on %s", s.ID, s.Address)
		if err := s.grpcServer.Serve(lis); err != nil {
			log.Printf("[%s] gRPC server error: %v", s.ID, err)
		}
	}()

	// 3. Start election timer
	go s.StartElectionTimer(s.stopElectionTimer)

	log.Printf("[%s] Raft server started successfully", s.ID)
	return nil
}

// connectToPeers establishes gRPC connections to all peer nodes
func (s *Server) connectToPeers() error {
	for _, peerAddr := range s.Peers {
		conn, err := grpc.NewClient(peerAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			return fmt.Errorf("failed to connect to peer %s: %w", peerAddr, err)
		}
		s.peerConnections[peerAddr] = conn
		s.peerClients[peerAddr] = proto.NewRaftServiceClient(conn)
		log.Printf("[%s] Connected to peer: %s", s.ID, peerAddr)
	}
	return nil
}

// Stop gracefully shuts down the server
func (s *Server) Stop() error {
	log.Printf("[%s] Stopping Raft server...", s.ID)

	// 1. Stop election timer
	close(s.stopElectionTimer)

	// 2. Stop heartbeat if running
	s.mu.Lock()
	if s.heartbeatRunning {
		close(s.stopHeartbeat)
		s.heartbeatRunning = false
	}
	s.mu.Unlock()

	// 3. Stop gRPC server
	if s.grpcServer != nil {
		s.grpcServer.GracefulStop()
	}

	// 4. Close all peer connections
	for addr, conn := range s.peerConnections {
		if err := conn.Close(); err != nil {
			log.Printf("[%s] Error closing connection to %s: %v", s.ID, addr, err)
		}
	}

	log.Printf("[%s] Raft server stopped", s.ID)
	return nil
}

// AppendEntry adds a new entry to the log
func (s *Server) AppendEntry(command interface{}) {
	entry := LogEntry{
		Term:    s.Node.currentTerm,
		Index:   len(s.Log),
		Command: command,
	}
	s.Log = append(s.Log, entry)
}

// GetState returns the current node state
func (s *Server) GetState() NodeState {
	return s.Node.state
}

// IsLeader checks if this server is the leader
func (s *Server) IsLeader() bool {
	return s.Node.state == Leader
}

// GetCurrentTerm returns the current term
func (s *Server) GetCurrentTerm() int {
	return s.Node.currentTerm
}

// IncrementTerm increases the term by 1 (used when starting election)
func (s *Server) IncrementTerm() {
	s.Node.currentTerm++
}

// SetTerm sets the current term (used when receiving higher term from other nodes)
func (s *Server) SetTerm(term int) {
	s.Node.currentTerm = term
}

// VoteFor records who this server voted for
func (s *Server) VoteFor(candidateID string) {
	s.Node.votedFor = candidateID
}

// GetVotedFor returns who this server voted for in current term
func (s *Server) GetVotedFor() string {
	return s.Node.votedFor
}

// ============================================
// Client Methods (like class methods)
// ============================================

// NewClient creates and initializes a new Client instance (like a constructor)
func NewClient(id string, clusterNodes []string) *Client {
	return &Client{
		ID:            id,
		ClusterNodes:  clusterNodes,
		CurrentLeader: "",
		RequestID:     0,
		connections:   make(map[string]*grpc.ClientConn),
		clients:       make(map[string]proto.RaftServiceClient),
	}
}

// Connect establishes connection to the cluster
func (c *Client) Connect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	log.Printf("[Client %s] Connecting to cluster nodes...", c.ID)

	// Establish gRPC connections to all cluster nodes
	for _, nodeAddr := range c.ClusterNodes {
		conn, err := grpc.NewClient(nodeAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			return fmt.Errorf("failed to connect to node %s: %w", nodeAddr, err)
		}
		c.connections[nodeAddr] = conn
		c.clients[nodeAddr] = proto.NewRaftServiceClient(conn)
		log.Printf("[Client %s] Connected to node: %s", c.ID, nodeAddr)
	}

	return nil
}

// Disconnect closes all connections
func (c *Client) Disconnect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	log.Printf("[Client %s] Disconnecting from cluster...", c.ID)

	// Close all gRPC connections
	for addr, conn := range c.connections {
		if err := conn.Close(); err != nil {
			log.Printf("[Client %s] Error closing connection to %s: %v", c.ID, addr, err)
		}
	}

	c.connections = make(map[string]*grpc.ClientConn)
	c.clients = make(map[string]proto.RaftServiceClient)

	return nil
}

// SendRequest sends a command to the cluster
// It will try to send to the known leader, or discover the leader if unknown
func (c *Client) SendRequest(command interface{}) error {
	c.mu.Lock()
	c.RequestID++
	requestID := c.RequestID
	c.mu.Unlock()

	log.Printf("[Client %s] Sending request #%d: %v", c.ID, requestID, command)

	// Try sending to known leader first, then try all nodes
	nodesToTry := c.ClusterNodes
	c.mu.RLock()
	if c.CurrentLeader != "" {
		// Put known leader first
		nodesToTry = append([]string{c.CurrentLeader}, c.ClusterNodes...)
	}
	c.mu.RUnlock()

	for _, nodeAddr := range nodesToTry {
		c.mu.RLock()
		client := c.clients[nodeAddr]
		c.mu.RUnlock()

		if client == nil {
			continue
		}

		// TODO: Implement actual client request RPC
		// For now, we'll just log that we would send it
		// In a real implementation, you would:
		// 1. Send a ClientRequest RPC with the command
		// 2. If response says "not leader", update CurrentLeader and retry
		// 3. If response is success, return the result

		log.Printf("[Client %s] Would send request to %s", c.ID, nodeAddr)

		// Placeholder: assume success for now
		c.mu.Lock()
		c.CurrentLeader = nodeAddr
		c.mu.Unlock()
		return nil
	}

	return fmt.Errorf("failed to send request to any node")
}

// GetLeader returns the current known leader address
func (c *Client) GetLeader() string {
	return c.CurrentLeader
}

// SetLeader updates the known leader (when redirected)
func (c *Client) SetLeader(leaderAddress string) {
	c.CurrentLeader = leaderAddress
}

// GetNextRequestID returns and increments the request ID
func (c *Client) GetNextRequestID() int {
	c.RequestID++
	return c.RequestID
}

// RequestVote handles voting requests from candidates during elections
// This is called when another node wants to become the leader
func (s *Server) RequestVote(ctx context.Context, req *proto.RequestVoteRequest) (*proto.RequestVoteResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Convert int32 from protobuf to int for Go
	candidateTerm := int(req.GetTerm())
	candidateID := req.GetCandidateId()
	candidateLastLogIndex := int(req.GetLastLogIndex())
	candidateLastLogTerm := int(req.GetLastLogTerm())

	log.Printf("[%s] Received RequestVote from %s for term %d (current term: %d)",
		s.ID, candidateID, candidateTerm, s.GetCurrentTerm())

	// Default response: reject the vote
	response := &proto.RequestVoteResponse{
		Term:        int32(s.GetCurrentTerm()), // Tell them our term
		VoteGranted: false,                     // Default: no vote
	}

	// ============================================
	// RULE 1: Check Term Number
	// ============================================

	// If candidate's term is less than ours, reject immediately
	// (They're from an old election)
	if candidateTerm < s.GetCurrentTerm() {
		return response, nil // Reject: outdated term
	}

	// If candidate's term is GREATER than ours:
	// - We're behind! Update our term
	// - Reset our vote (new election, we haven't voted yet)
	// - Step down to follower if we were candidate/leader
	if candidateTerm > s.GetCurrentTerm() {
		s.SetTerm(candidateTerm)             // Update to the new term
		s.VoteFor("")                        // Reset vote for new term
		s.Node.BecomeFollower()              // Step down to follower
		response.Term = int32(candidateTerm) // Update response term
	}

	// ============================================
	// RULE 2: Check If We Already Voted
	// ============================================

	// In Raft, each server can only vote ONCE per term
	// Either we haven't voted ("") OR we're voting for the same candidate again
	alreadyVoted := s.GetVotedFor()
	if alreadyVoted != "" && alreadyVoted != candidateID {
		// We already voted for someone else this term
		return response, nil // Reject: already voted
	}

	// ============================================
	// RULE 3: Check If Candidate's Log is Up-to-Date
	// ============================================

	// Get our own log information
	myLastLogIndex := len(s.Log) - 1 // Index of our last log entry
	myLastLogTerm := 0
	if myLastLogIndex >= 0 {
		myLastLogTerm = s.Log[myLastLogIndex].Term
	}

	// Election restriction: only vote for candidates who are
	// "at least as up-to-date" as us

	// Check if candidate's log is at least as up-to-date as ours
	logIsUpToDate := false

	if candidateLastLogTerm > myLastLogTerm {
		// Candidate's last entry is from a more recent term
		// Their log is definitely more up-to-date
		logIsUpToDate = true
	} else if candidateLastLogTerm == myLastLogTerm {
		// Same term, so check length
		// If their log is at least as long as ours, they're up-to-date
		if candidateLastLogIndex >= myLastLogIndex {
			logIsUpToDate = true
		}
	}
	// If candidateLastLogTerm < myLastLogTerm, they're behind (logIsUpToDate stays false)

	if !logIsUpToDate {
		// Candidate's log is behind ours
		return response, nil // Reject: outdated log
	}

	// ============================================
	// RULE 4: Grant the Vote!
	// ============================================

	// All checks passed! Grant the vote
	s.VoteFor(candidateID)      // Record our vote
	response.VoteGranted = true // Tell candidate they got our vote

	// Reset election timer - we just granted a vote, give candidate time to win
	s.ResetElectionTimer()

	return response, nil
}

// AppendEntries handles log replication and heartbeat requests from the leader
// This is called when the leader wants to:
// 1. Send heartbeats (empty entries)
// 2. Replicate log entries to followers
func (s *Server) AppendEntries(ctx context.Context, req *proto.AppendEntriesRequest) (*proto.AppendEntriesResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Convert protobuf types to Go types
	leaderTerm := int(req.GetTerm())
	leaderID := req.GetLeaderId()
	prevLogIndex := int(req.GetPrevLogIndex())
	prevLogTerm := int(req.GetPrevLogTerm())
	leaderCommit := int(req.GetLeaderCommit())
	entries := req.GetEntries() // New log entries to append

	// Default response: reject
	response := &proto.AppendEntriesResponse{
		Term:    int32(s.GetCurrentTerm()),
		Success: false,
	}

	// ============================================
	// RULE 1: Reply false if leader's term < currentTerm
	// ============================================
	if leaderTerm < s.GetCurrentTerm() {
		// Leader is outdated, reject
		return response, nil
	}

	// ============================================
	// RULE 2: If leader's term >= our term, accept them as leader
	// ============================================
	if leaderTerm > s.GetCurrentTerm() {
		// Leader has higher term, update ours
		s.SetTerm(leaderTerm)
		s.VoteFor("")                        // Reset vote for new term
		response.Term = int32(leaderTerm)    // Update response term
	}

	// Step down to follower if we were candidate/leader
	s.Node.BecomeFollower()
	s.LeaderID = leaderID

	// Reset election timer - receiving valid AppendEntries means leader is alive
	s.ResetElectionTimer()

	// ============================================
	// RULE 3: Check log consistency (prevLogIndex and prevLogTerm)
	// ============================================

	// If prevLogIndex is beyond our log, we're missing entries
	if prevLogIndex >= 0 && prevLogIndex >= len(s.Log) {
		// Our log is too short
		return response, nil // Reject: log inconsistency
	}

	// If we have an entry at prevLogIndex, check if terms match
	if prevLogIndex >= 0 {
		if s.Log[prevLogIndex].Term != prevLogTerm {
			// Term mismatch at prevLogIndex - logs diverged
			// Delete this entry and all that follow
			s.Log = s.Log[:prevLogIndex]
			return response, nil // Reject: log inconsistency
		}
	}

	// ============================================
	// RULE 4: Append new entries to log
	// ============================================

	// Log consistency check passed!
	// Now append new entries (if any)

	if len(entries) > 0 {
		// Starting position for new entries
		logIndex := prevLogIndex + 1

		for i, entry := range entries {
			currentIndex := logIndex + i

			// Convert protobuf LogEntry to our LogEntry struct
			newEntry := LogEntry{
				Term:    int(entry.GetTerm()),
				Index:   currentIndex,
				Command: entry.GetCommand(), // This is []byte from protobuf
			}

			// If we have an existing entry at this index
			if currentIndex < len(s.Log) {
				// Check if it conflicts (different term)
				if s.Log[currentIndex].Term != newEntry.Term {
					// Delete conflicting entry and all that follow
					s.Log = s.Log[:currentIndex]
					// Append the new entry
					s.Log = append(s.Log, newEntry)
				}
				// If terms match, entry is already there, skip
			} else {
				// No existing entry, just append
				s.Log = append(s.Log, newEntry)
			}
		}
	}

	// ============================================
	// RULE 5: Update commit index
	// ============================================

	// If leaderCommit > commitIndex, update commitIndex
	if leaderCommit > s.CommitIndex {
		// Set commitIndex to min(leaderCommit, index of last new entry)
		lastNewEntryIndex := len(s.Log) - 1
		if leaderCommit < lastNewEntryIndex {
			s.CommitIndex = leaderCommit
		} else {
			s.CommitIndex = lastNewEntryIndex
		}
	}

	// ============================================
	// Success! All checks passed
	// ============================================
	response.Success = true
	return response, nil
}

// SendHeartBeats sends periodic heartbeats to all followers
// This runs in a goroutine while the server is the leader
func (s *Server) SendHeartBeats(stopCh <-chan struct{}) {
	// Create a ticker that fires every 50ms (heartbeat interval)
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	// Infinite loop until stopped
	for {
		select {
		case <-stopCh:
			// Received stop signal - exit the goroutine
			return

		case <-ticker.C:
			// Ticker fired - time to send heartbeat

			// Only send if we're still the leader
			if !s.IsLeader() {
				return
			}

			// Send AppendEntries RPC to all peers
			s.sendAppendEntriesToAllPeers()
		}
	}
}

// sendAppendEntriesToAllPeers sends AppendEntries RPC to all peers in parallel
func (s *Server) sendAppendEntriesToAllPeers() {
	s.mu.RLock()
	currentTerm := s.GetCurrentTerm()
	commitIndex := s.CommitIndex
	s.mu.RUnlock()

	// Send to each peer
	for _, peerAddress := range s.Peers {
		// Launch a goroutine for each peer (parallel sending)
		go func(peer string) {
			s.mu.RLock()
			nextIndex := s.NextIndex[peer]
			prevLogTerm := s.getPrevLogTerm(peer)
			s.mu.RUnlock()

			// Create the AppendEntries request
			req := &proto.AppendEntriesRequest{
				Term:         int32(currentTerm),
				LeaderId:     s.ID,
				PrevLogIndex: int32(nextIndex - 1),
				PrevLogTerm:  int32(prevLogTerm),
				Entries:      []*proto.LogEntry{}, // Empty for heartbeat
				LeaderCommit: int32(commitIndex),
			}

			// Make actual gRPC call
			client := s.peerClients[peer]
			if client == nil {
				log.Printf("[%s] No gRPC client for peer %s", s.ID, peer)
				return
			}

			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()

			resp, err := client.AppendEntries(ctx, req)
			if err != nil {
				// Log error but don't crash - peer might be down
				log.Printf("[%s] AppendEntries to %s failed: %v", s.ID, peer, err)
				return
			}

			s.handleAppendEntriesResponse(peer, resp)
		}(peerAddress)
	}
}

// getPrevLogTerm returns the term of the log entry before nextIndex for a peer
func (s *Server) getPrevLogTerm(peerAddress string) int {
	prevLogIndex := s.NextIndex[peerAddress] - 1

	// If prevLogIndex is -1, there's no previous log
	if prevLogIndex < 0 {
		return 0
	}

	// If prevLogIndex is beyond our log, return 0
	if prevLogIndex >= len(s.Log) {
		return 0
	}

	return s.Log[prevLogIndex].Term
}

// ============================================
// Election Timer & Election Functions
// ============================================

// StartElectionTimer starts the election timeout timer
// If no heartbeat is received before timeout, start an election
func (s *Server) StartElectionTimer(stopCh <-chan struct{}) {
	// Random timeout between 150-300ms (Raft paper recommendation)
	timeout := time.Duration(150+randInt(150)) * time.Millisecond
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	for {
		select {
		case <-stopCh:
			// Stop the election timer
			return

		case <-s.electionTimer:
			// Reset the timer (heartbeat received or vote granted)
			if !timer.Stop() {
				<-timer.C
			}
			timeout = time.Duration(150+randInt(150)) * time.Millisecond
			timer.Reset(timeout)

		case <-timer.C:
			// Election timeout! No heartbeat received
			// Only start election if we're a follower or candidate
			s.mu.RLock()
			state := s.GetState()
			s.mu.RUnlock()

			if state != Leader {
				s.StartElection()
			}

			// Reset timer for next election
			timeout = time.Duration(150+randInt(150)) * time.Millisecond
			timer.Reset(timeout)
		}
	}
}

// ResetElectionTimer resets the election timer
// Called when receiving valid AppendEntries or granting vote
func (s *Server) ResetElectionTimer() {
	select {
	case s.electionTimer <- struct{}{}:
		// Successfully sent reset signal
	default:
		// Channel is full, timer will reset soon anyway
	}
}

// StartElection transitions to candidate and starts an election
func (s *Server) StartElection() {
	s.mu.Lock()
	// Transition to candidate state
	s.Node.BecomeCandidate()

	// Increment current term
	s.IncrementTerm()

	// Vote for ourselves
	s.VoteFor(s.ID)

	currentTerm := s.GetCurrentTerm()
	log.Printf("[%s] Starting election for term %d", s.ID, currentTerm)

	// Get our last log info for RequestVote
	lastLogIndex := len(s.Log) - 1
	lastLogTerm := 0
	if lastLogIndex >= 0 {
		lastLogTerm = s.Log[lastLogIndex].Term
	}
	s.mu.Unlock()

	// Count our own vote
	var votesReceived int32 = 1
	totalPeers := len(s.Peers) + 1 // Including ourselves
	majorityNeeded := int32((totalPeers / 2) + 1)

	var voteMu sync.Mutex

	// Send RequestVote RPC to all peers
	for _, peerAddress := range s.Peers {
		go func(peer string) {
			// Create RequestVote request
			req := &proto.RequestVoteRequest{
				Term:         int32(currentTerm),
				CandidateId:  s.ID,
				LastLogIndex: int32(lastLogIndex),
				LastLogTerm:  int32(lastLogTerm),
			}

			// Make actual gRPC call
			client := s.peerClients[peer]
			if client == nil {
				log.Printf("[%s] No gRPC client for peer %s", s.ID, peer)
				return
			}

			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()

			resp, err := client.RequestVote(ctx, req)
			if err != nil {
				log.Printf("[%s] RequestVote to %s failed: %v", s.ID, peer, err)
				return
			}

			s.handleRequestVoteResponse(resp, &votesReceived, majorityNeeded, &voteMu)
		}(peerAddress)
	}
}

// handleRequestVoteResponse processes the response from a RequestVote RPC
func (s *Server) handleRequestVoteResponse(resp *proto.RequestVoteResponse, votesReceived *int32, majorityNeeded int32, voteMu *sync.Mutex) {
	respTerm := int(resp.GetTerm())
	voteGranted := resp.GetVoteGranted()

	s.mu.Lock()
	currentTerm := s.GetCurrentTerm()
	state := s.GetState()
	s.mu.Unlock()

	// If response has higher term, step down
	if respTerm > currentTerm {
		s.mu.Lock()
		s.SetTerm(respTerm)
		s.VoteFor("")
		s.Node.BecomeFollower()
		s.mu.Unlock()
		return
	}

	// If still a candidate and vote was granted
	if state == Candidate && voteGranted {
		voteMu.Lock()
		*votesReceived++
		votes := *votesReceived
		voteMu.Unlock()

		// Check if we have majority
		if votes >= majorityNeeded {
			log.Printf("[%s] Won election with %d votes", s.ID, votes)
			s.BecomeLeader()
		}
	}
}

// BecomeLeader transitions the server to leader state
func (s *Server) BecomeLeader() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.Node.BecomeLeader()
	s.LeaderID = s.ID

	log.Printf("[%s] Became leader for term %d", s.ID, s.GetCurrentTerm())

	// Initialize leader state
	// nextIndex: for each server, index of next log entry to send
	// (initialized to leader's last log index + 1)
	lastLogIndex := len(s.Log)
	for _, peer := range s.Peers {
		s.NextIndex[peer] = lastLogIndex
		s.MatchIndex[peer] = 0
	}

	// Stop any existing heartbeat goroutine
	if s.heartbeatRunning {
		close(s.stopHeartbeat)
	}

	// Create new stop channel and start sending heartbeats
	s.stopHeartbeat = make(chan struct{})
	s.heartbeatRunning = true
	go s.SendHeartBeats(s.stopHeartbeat)
}

// handleAppendEntriesResponse processes the response from an AppendEntries RPC
func (s *Server) handleAppendEntriesResponse(peerAddress string, resp *proto.AppendEntriesResponse) {
	respTerm := int(resp.GetTerm())
	success := resp.GetSuccess()

	// If response has higher term, step down to follower
	if respTerm > s.GetCurrentTerm() {
		s.SetTerm(respTerm)
		s.VoteFor("")
		s.Node.BecomeFollower()
		return
	}

	// Only process if we're still the leader
	if s.GetState() != Leader {
		return
	}

	if success {
		// Follower successfully appended entries
		// Update nextIndex and matchIndex for this peer

		// TODO: Update based on actual entries sent
		// For now, this is a placeholder
		// s.MatchIndex[peerAddress] = prevLogIndex + len(entries)
		// s.NextIndex[peerAddress] = s.MatchIndex[peerAddress] + 1

		// Check if we can advance commitIndex
		s.updateCommitIndex()
	} else {
		// Follower rejected AppendEntries (log inconsistency)
		// Decrement nextIndex and retry
		if s.NextIndex[peerAddress] > 0 {
			s.NextIndex[peerAddress]--
		}
		// TODO: Retry sending AppendEntries with lower nextIndex
	}
}

// updateCommitIndex updates the commit index based on matchIndex
// A log entry is committed if it's stored on a majority of servers
func (s *Server) updateCommitIndex() {
	// Only leader can advance commit index
	if s.GetState() != Leader {
		return
	}

	// Find the highest log index that's replicated on majority of servers
	for n := len(s.Log) - 1; n > s.CommitIndex; n-- {
		// Check if this entry is from current term
		// (Leader can only commit entries from current term)
		if s.Log[n].Term != s.GetCurrentTerm() {
			continue
		}

		// Count how many servers have this entry
		replicationCount := 1 // Count ourselves

		for _, peer := range s.Peers {
			if s.MatchIndex[peer] >= n {
				replicationCount++
			}
		}

		// Check if majority
		totalServers := len(s.Peers) + 1
		if replicationCount > totalServers/2 {
			// Majority have this entry, commit it
			s.CommitIndex = n
			break
		}
	}
}

// ApplyCommittedEntries applies committed log entries to the state machine
func (s *Server) ApplyCommittedEntries() {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Apply all entries between lastApplied and commitIndex
	for s.LastApplied < s.CommitIndex {
		s.LastApplied++
		entry := s.Log[s.LastApplied]

		// Apply entry.Command to state machine
		result := s.stateMachine.Apply(entry.Command)
		log.Printf("[%s] Applied log entry %d: %v -> %v", s.ID, s.LastApplied, entry.Command, result)
	}
}

// ============================================
// Helper Functions
// ============================================

// randInt returns a random integer in range [0, max)
func randInt(max int) int {
	return rand.IntN(max)
}
