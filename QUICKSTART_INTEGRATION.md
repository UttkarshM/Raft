# Quick Start Guide: Raft + Visual-Automation Integration

This guide will get you up and running with the integrated Raft-backed workflow system in **5 minutes**.

---

## Prerequisites

- Go 1.23+ installed
- Node.js 18+ installed
- Terminal access (Bash or PowerShell)

---

## Step 1: Start the Raft Cluster

### On Linux/Mac:

```bash
cd distributed_systems
chmod +x scripts/start-cluster.sh
./scripts/start-cluster.sh
```

### On Windows:

```batch
cd distributed_systems
scripts\start-cluster.bat
```

**Expected Output**:
```
========================================
Starting Raft + Gateway Cluster
========================================
Building gateway binary...
Build successful!

Starting Node 1 (Raft: 5001, HTTP: 8001)...
Node 1 started
Starting Node 2 (Raft: 5002, HTTP: 8002)...
Node 2 started
Starting Node 3 (Raft: 5003, HTTP: 8003)...
Node 3 started

✅ Cluster Started Successfully!
```

---

## Step 2: Verify Cluster Health

```bash
# Check if nodes are running
curl http://localhost:8001/health
curl http://localhost:8002/health
curl http://localhost:8003/health

# Expected response:
# {"status":"healthy","node_id":"node1","is_leader":true}
```

**Find the leader**:
```bash
curl http://localhost:8001/api/cluster/status | jq
```

**Expected output**:
```json
{
  "is_leader": true,
  "leader_id": "node1",
  "current_term": 1,
  "node_id": "node1",
  "node_state": "Leader",
  "http_port": "8001"
}
```

---

## Step 3: Start Visual-Automation

```bash
cd Visual-Automation
npm install  # First time only
npm run dev
```

**Expected Output**:
```
ready - started server on 0.0.0.0:3000, url: http://localhost:3000
```

Open [http://localhost:3000](http://localhost:3000) in your browser.

---

## Step 4: Create Your First Workflow

1. **Open Visual-Automation** at [http://localhost:3000](http://localhost:3000)

2. **Add nodes** to the canvas:
   - Input Node
   - Prompt Node (or API Node)
   - Output Node

3. **Connect nodes** by dragging from output to input handles

4. **Save workflow** (click Save button)

5. **Check Raft logs** to see replication:
   ```bash
   tail -f distributed_systems/logs/node1.log
   ```

   **Expected log entries**:
   ```
   [WorkflowSM] Applying command: CREATE_WORKFLOW (RequestID: 1708123456789)
   [WorkflowSM] Created workflow: My First Workflow (1708123456789000)
   ```

---

## Step 5: Test Failover (Optional)

### Kill the Leader

1. **Find the current leader**:
   ```bash
   curl http://localhost:8001/api/cluster/status | jq .leader_id
   # Output: "node1"
   ```

2. **Kill the leader process**:
   ```bash
   # Linux/Mac
   pkill -f "gateway.*node1"

   # Windows
   taskkill /FI "WINDOWTITLE eq Raft Node 1"
   ```

3. **Create another workflow** in the UI immediately

4. **Verify new leader elected**:
   ```bash
   curl http://localhost:8002/api/cluster/status | jq .leader_id
   # Output: "node2" (new leader!)
   ```

**Expected**: Workflow creation succeeds with new leader in <500ms

---

## API Examples

### Create a Workflow (via Gateway)

```bash
curl -X POST http://localhost:8001/api/workflows/create \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Test Workflow",
    "description": "My first workflow",
    "nodes": [
      {"id": "1", "type": "input", "data": {}, "position": {"x": 0, "y": 0}}
    ],
    "edges": [],
    "user_id": "user123"
  }'
```

**Expected Response**:
```json
{
  "workflow": {
    "id": "1708123456789000",
    "name": "Test Workflow",
    "description": "My first workflow",
    "nodes": [...],
    "edges": [],
    "user_id": "user123",
    "created_at": "2026-02-16T10:30:00Z",
    "updated_at": "2026-02-16T10:30:00Z"
  }
}
```

### List Workflows

```bash
curl "http://localhost:8001/api/workflows/list?user_id=user123&page=1&page_size=10"
```

**Expected Response**:
```json
{
  "items": [
    {
      "id": "1708123456789000",
      "name": "Test Workflow",
      ...
    }
  ],
  "total": 1,
  "total_pages": 1
}
```

### Start Execution

```bash
curl -X POST http://localhost:8001/api/executions/start \
  -H "Content-Type: application/json" \
  -d '{
    "workflow_id": "1708123456789000",
    "input_data": {"key": "value"},
    "user_id": "user123"
  }'
```

### Check Cluster Stats

```bash
curl http://localhost:8001/api/cluster/stats | jq
```

**Expected Response**:
```json
{
  "total_workflows": 5,
  "total_executions": 12,
  "running_executions": 2,
  "completed_executions": 9,
  "failed_executions": 1,
  "users_with_workflows": 3
}
```

---

## Monitoring

### View Logs

```bash
# Node 1
tail -f distributed_systems/logs/node1.log

# Node 2
tail -f distributed_systems/logs/node2.log

# Node 3
tail -f distributed_systems/logs/node3.log
```

### Check Cluster Status

```bash
# All nodes status
for port in 8001 8002 8003; do
  echo "Node on port $port:"
  curl -s http://localhost:$port/api/cluster/status | jq '{node_id, is_leader, current_term}'
  echo ""
done
```

---

## Stopping the Cluster

### Linux/Mac:

```bash
pkill -f "gateway"
```

### Windows:

```batch
taskkill /FI "WINDOWTITLE eq Raft Node*"
```

Or simply press `Ctrl+C` in the terminal where you started the cluster.

---

## Troubleshooting

### Port Already in Use

**Error**: `bind: address already in use`

**Solution**:
```bash
# Find and kill processes on ports 5001-5003, 8001-8003
lsof -ti:5001,5002,5003,8001,8002,8003 | xargs kill -9
```

### Build Failed

**Error**: `go build: command not found`

**Solution**: Install Go from [https://go.dev/dl/](https://go.dev/dl/)

### Cannot Connect from Next.js

**Error**: `ECONNREFUSED localhost:8001`

**Solution**:
1. Check if gateways are running: `ps aux | grep gateway`
2. Check logs: `tail -f distributed_systems/logs/node1.log`
3. Restart cluster: `./scripts/start-cluster.sh`

---

## Next Steps

- **Read the full integration report**: [INTEGRATION_REPORT.md](./INTEGRATION_REPORT.md)
- **Explore the workflow state machine**: [workflow/state_machine.go](./workflow/state_machine.go)
- **Customize the RaftClient**: [Visual-Automation/src/lib/raft-client.ts](./Visual-Automation/src/lib/raft-client.ts)
- **Add persistence**: Implement disk snapshots in state machine
- **Deploy to production**: Use Docker Compose or Kubernetes

---

## Summary

You now have:
- ✅ A 3-node Raft cluster running
- ✅ HTTP gateways on ports 8001-8003
- ✅ Visual-Automation connected to Raft
- ✅ Distributed workflow coordination
- ✅ Automatic failover capability

**Happy building!** 🚀
