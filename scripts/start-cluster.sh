#!/bin/bash

# Raft + Gateway Cluster Startup Script
# Starts 3 Raft nodes with HTTP gateways for Visual-Automation integration

echo "========================================"
echo "Starting Raft + Gateway Cluster"
echo "========================================"

# Color codes
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Kill any existing instances
echo -e "${YELLOW}Cleaning up existing processes...${NC}"
pkill -f "cmd/gateway/main" 2>/dev/null || true
sleep 1

# Build the gateway binary
echo -e "${BLUE}Building gateway binary...${NC}"
cd "$(dirname "$0")/.." || exit 1
go build -o bin/gateway cmd/gateway/main.go

if [ $? -ne 0 ]; then
    echo -e "${YELLOW}Build failed! Exiting.${NC}"
    exit 1
fi

echo -e "${GREEN}Build successful!${NC}"
echo ""

# Start Node 1
echo -e "${BLUE}Starting Node 1 (Raft: 5001, HTTP: 8001)...${NC}"
./bin/gateway -id=node1 -raft-port=5001 -http-port=8001 > logs/node1.log 2>&1 &
NODE1_PID=$!
echo -e "${GREEN}Node 1 started (PID: $NODE1_PID)${NC}"

# Start Node 2
echo -e "${BLUE}Starting Node 2 (Raft: 5002, HTTP: 8002)...${NC}"
./bin/gateway -id=node2 -raft-port=5002 -http-port=8002 > logs/node2.log 2>&1 &
NODE2_PID=$!
echo -e "${GREEN}Node 2 started (PID: $NODE2_PID)${NC}"

# Start Node 3
echo -e "${BLUE}Starting Node 3 (Raft: 5003, HTTP: 8003)...${NC}"
./bin/gateway -id=node3 -raft-port=5003 -http-port=8003 > logs/node3.log 2>&1 &
NODE3_PID=$!
echo -e "${GREEN}Node 3 started (PID: $NODE3_PID)${NC}"

echo ""
echo "========================================"
echo -e "${GREEN}✅ Cluster Started Successfully!${NC}"
echo "========================================"
echo ""
echo "Nodes:"
echo "  Node 1: Raft=localhost:5001 | HTTP=localhost:8001"
echo "  Node 2: Raft=localhost:5002 | HTTP=localhost:8002"
echo "  Node 3: Raft=localhost:5003 | HTTP=localhost:8003"
echo ""
echo "Logs:"
echo "  tail -f logs/node1.log"
echo "  tail -f logs/node2.log"
echo "  tail -f logs/node3.log"
echo ""
echo "Health Check:"
echo "  curl http://localhost:8001/health"
echo ""
echo "Cluster Status:"
echo "  curl http://localhost:8001/api/cluster/status"
echo ""
echo "Stop Cluster:"
echo "  pkill -f 'cmd/gateway/main'"
echo ""
echo "Press Ctrl+C to stop all nodes"
echo "========================================"

# Wait for interrupt
trap "echo ''; echo 'Shutting down cluster...'; kill $NODE1_PID $NODE2_PID $NODE3_PID 2>/dev/null; exit 0" INT TERM

# Keep script running
wait
