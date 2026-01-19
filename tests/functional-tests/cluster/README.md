# DGate Cluster Mode Functional Tests

This directory contains functional tests for the three cluster modes supported by DGate:

## Cluster Modes

### 1. Simple HTTP Replication (`simple/`)

**Mode:** `cluster.mode: simple`

- Uses direct HTTP calls to replicate changes to peer nodes
- All nodes can accept writes (multi-master)
- Simple and effective for most use cases
- Does NOT require leader election
- Best for: Small clusters, simple setups, eventually consistent scenarios

```bash
./simple/run-test.sh
```

### 2. Raft Consensus (`raft/`)

**Mode:** `cluster.mode: raft`

- Full openraft integration with leader election and log replication
- Provides strong consistency guarantees
- Only the leader can accept writes (single-master)
- Automatic leader election on leader failure
- Best for: Strong consistency requirements, critical data

```bash
./raft/run-test.sh
```

### 3. Tempo Multi-Master Consensus (`tempo/`)

**Mode:** `cluster.mode: tempo`

- Multi-master consensus - any node can accept writes
- Leaderless coordination using logical clocks
- Quorum-based commit with fast path optimization
- Better write scalability than Raft (no leader bottleneck)
- Clock synchronization ensures consistent ordering
- Best for: High write throughput, distributed writes, scaling

```bash
./tempo/run-test.sh
```

## Running Tests

### Run individual cluster tests:

```bash
# Simple replication
./run-all-tests.sh cluster-simple

# Raft consensus  
./run-all-tests.sh cluster-raft

# Tempo consensus
./run-all-tests.sh cluster-tempo
```

### Run all cluster tests:

```bash
./run-all-tests.sh cluster
```

### Shorthand aliases:

```bash
./run-all-tests.sh simple  # same as cluster-simple
./run-all-tests.sh raft    # same as cluster-raft
./run-all-tests.sh tempo   # same as cluster-tempo
```

## Test Structure

Each test suite validates:

1. **Cluster Formation** - All nodes join and report cluster status
2. **Write Capability** - Writes are accepted (multi-master for simple/tempo, leader-only for raft)
3. **Namespace Replication** - Namespaces propagate to all nodes
4. **Service Replication** - Services propagate to all nodes
5. **Route Replication** - Routes propagate to all nodes
6. **Document Replication** - Documents propagate to all nodes
7. **Concurrent Writes** - Multiple simultaneous writes handled correctly
8. **Rapid Writes** - High-volume sequential writes
9. **Delete Replication** - Deletes propagate to all nodes
10. **Cluster Metrics** - Status/metrics endpoints work

## Configuration Examples

### Simple Replication

```yaml
cluster:
  enabled: true
  mode: simple
  node_id: 1
  initial_members:
    - id: 1
      addr: "127.0.0.1:9191"
      admin_port: 9181
    - id: 2
      addr: "127.0.0.1:9192"
      admin_port: 9182
```

### Raft Consensus

```yaml
cluster:
  enabled: true
  mode: raft
  node_id: 1
  bootstrap: true  # Only for first node
  initial_members:
    - id: 1
      addr: "127.0.0.1:9291"
      admin_port: 9281
```

### Tempo Consensus

```yaml
cluster:
  enabled: true
  mode: tempo
  node_id: 1
  initial_members:
    - id: 1
      addr: "127.0.0.1:9391"
      admin_port: 9381
  tempo:
    fast_quorum_size: 2      # Fast path quorum
    write_quorum_size: 2     # Minimum for commit
    clock_bump_interval_ms: 50
```

## Comparison

| Feature | Simple | Raft | Tempo |
|---------|--------|------|-------|
| Write location | Any node | Leader only | Any node |
| Consistency | Eventual | Strong | Strong (quorum) |
| Leader election | No | Yes | No |
| Write scalability | High | Limited by leader | High |
| Complexity | Low | Medium | Medium |
| Fault tolerance | N-1 nodes | (N-1)/2 nodes | (N-1)/2 nodes |
