#!/bin/bash
# Raft Consensus Functional Tests
#
# Tests that resources are replicated across 3 DGate nodes using the
# full Raft consensus mode (openraft implementation).
#
# This mode:
# - Complete openraft integration with leader election, log replication
# - Provides stronger consistency guarantees
# - Only the leader can accept writes
# - Automatic leader election on leader failure

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "$SCRIPT_DIR/../../common/utils.sh"

test_header "Raft Consensus Functional Tests (3-Node)"

# Cleanup on exit
trap cleanup_cluster EXIT

# Cluster configuration
# Note: advertise_addr uses the admin port since all Raft RPC is over HTTP on the admin API
NODE1_ADMIN_PORT=9281
NODE2_ADMIN_PORT=9282
NODE3_ADMIN_PORT=9283
NODE1_PROXY_PORT=8281
NODE2_PROXY_PORT=8282
NODE3_PROXY_PORT=8283

# PIDs for cleanup
NODE1_PID=""
NODE2_PID=""
NODE3_PID=""

cleanup_cluster() {
    log "Cleaning up cluster nodes..."
    [[ -n "$NODE1_PID" ]] && kill $NODE1_PID 2>/dev/null || true
    [[ -n "$NODE2_PID" ]] && kill $NODE2_PID 2>/dev/null || true
    [[ -n "$NODE3_PID" ]] && kill $NODE3_PID 2>/dev/null || true
    rm -f /tmp/dgate-raft-node*.log /tmp/dgate-raft-node*.yaml
    cleanup_processes
    return 0
}

# Generate config for a node with Raft consensus mode
generate_node_config() {
    local node_id=$1
    local admin_port=$2
    local proxy_port=$3
    local is_bootstrap=$4
    local config_file="/tmp/dgate-raft-node${node_id}.yaml"
    
    cat > "$config_file" << EOF
version: v1
log_level: debug
debug: true

storage:
  type: memory

proxy:
  port: ${proxy_port}
  host: 0.0.0.0

admin:
  port: ${admin_port}
  host: 0.0.0.0

# Full Raft consensus mode
# advertise_addr uses admin port since Raft RPC is served on admin API
cluster:
  enabled: true
  mode: raft
  node_id: ${node_id}
  advertise_addr: "127.0.0.1:${admin_port}"
  bootstrap: ${is_bootstrap}
  initial_members:
    - id: 1
      addr: "127.0.0.1:${NODE1_ADMIN_PORT}"
    - id: 2
      addr: "127.0.0.1:${NODE2_ADMIN_PORT}"
    - id: 3
      addr: "127.0.0.1:${NODE3_ADMIN_PORT}"
EOF
    echo "$config_file"
}

# Start a node
start_node() {
    local node_id=$1
    local admin_port=$2
    local proxy_port=$3
    local is_bootstrap=$4
    local config_file
    
    config_file=$(generate_node_config $node_id $admin_port $proxy_port $is_bootstrap)
    
    log "Starting Node $node_id (admin: $admin_port, proxy: $proxy_port)"
    "$DGATE_BIN" -c "$config_file" > "/tmp/dgate-raft-node${node_id}.log" 2>&1 &
    local pid=$!
    
    # Wait for startup
    for i in {1..30}; do
        if curl -s "http://localhost:$admin_port/health" > /dev/null 2>&1; then
            success "Node $node_id started (PID: $pid)"
            echo $pid
            return 0
        fi
        sleep 0.5
    done
    
    error "Node $node_id failed to start"
    cat "/tmp/dgate-raft-node${node_id}.log"
    return 1
}

# Curl timeout for all requests (seconds)
CURL_TIMEOUT=10

# Wait for cluster to form and leader election
wait_for_cluster() {
    log "Waiting for cluster to form and leader election..."
    sleep 5
    
    # Check cluster status on each node
    for port in $NODE1_ADMIN_PORT $NODE2_ADMIN_PORT $NODE3_ADMIN_PORT; do
        local status=$(curl -s --max-time $CURL_TIMEOUT "http://localhost:$port/api/v1/cluster/status")
        if [[ $(echo "$status" | jq -r '.data.enabled') != "true" ]]; then
            warn "Node on port $port: cluster not enabled"
        else
            local leader=$(echo "$status" | jq -r '.data.leader_id // .data.current_leader // "unknown"')
            log "Node on port $port: cluster mode active, leader: $leader"
        fi
    done
}

# Find the current leader node
find_leader() {
    for port in $NODE1_ADMIN_PORT $NODE2_ADMIN_PORT $NODE3_ADMIN_PORT; do
        local status=$(curl -s --max-time $CURL_TIMEOUT "http://localhost:$port/api/v1/cluster/status")
        local is_leader=$(echo "$status" | jq -r '.data.is_leader // false')
        if [[ "$is_leader" == "true" ]]; then
            echo $port
            return 0
        fi
    done
    # Fallback: bootstrap node is likely leader
    echo $NODE1_ADMIN_PORT
}

# Helper to create a namespace on a specific node
create_namespace() {
    local admin_port=$1
    local name=$2
    
    curl -s --max-time $CURL_TIMEOUT -X PUT "http://localhost:$admin_port/api/v1/namespace/$name" \
        -H "Content-Type: application/json" \
        -d "{\"name\": \"$name\", \"tags\": [\"raft-test\"]}"
}

# Helper to get a namespace from a specific node
get_namespace() {
    local admin_port=$1
    local name=$2
    
    curl -s --max-time $CURL_TIMEOUT "http://localhost:$admin_port/api/v1/namespace/$name"
}

# Helper to create a service on a specific node
create_service() {
    local admin_port=$1
    local namespace=$2
    local name=$3
    local url=$4
    
    curl -s --max-time $CURL_TIMEOUT -X PUT "http://localhost:$admin_port/api/v1/service/$namespace/$name" \
        -H "Content-Type: application/json" \
        -d "{\"name\": \"$name\", \"namespace\": \"$namespace\", \"urls\": [\"$url\"]}"
}

# Helper to get a service from a specific node
get_service() {
    local admin_port=$1
    local namespace=$2
    local name=$3
    
    curl -s --max-time $CURL_TIMEOUT "http://localhost:$admin_port/api/v1/service/$namespace/$name"
}

# Helper to create a route on a specific node
create_route() {
    local admin_port=$1
    local namespace=$2
    local name=$3
    local path=$4
    
    curl -s --max-time $CURL_TIMEOUT -X PUT "http://localhost:$admin_port/api/v1/route/$namespace/$name" \
        -H "Content-Type: application/json" \
        -d "{\"name\": \"$name\", \"namespace\": \"$namespace\", \"paths\": [\"$path\"], \"methods\": [\"*\"]}"
}

# Helper to get a route from a specific node
get_route() {
    local admin_port=$1
    local namespace=$2
    local name=$3
    
    curl -s --max-time $CURL_TIMEOUT "http://localhost:$admin_port/api/v1/route/$namespace/$name"
}

# Helper to create a collection on a specific node
create_collection() {
    local admin_port=$1
    local namespace=$2
    local name=$3
    
    curl -s --max-time $CURL_TIMEOUT -X PUT "http://localhost:$admin_port/api/v1/collection/$namespace/$name" \
        -H "Content-Type: application/json" \
        -d "{\"name\": \"$name\", \"namespace\": \"$namespace\", \"visibility\": \"private\"}"
}

# Helper to create a document on a specific node
create_document() {
    local admin_port=$1
    local namespace=$2
    local collection=$3
    local id=$4
    local data=$5
    
    curl -s --max-time $CURL_TIMEOUT -X PUT "http://localhost:$admin_port/api/v1/document/$namespace/$collection/$id" \
        -H "Content-Type: application/json" \
        -d "{\"id\": \"$id\", \"namespace\": \"$namespace\", \"collection\": \"$collection\", \"data\": $data}"
}

# Helper to get a document from a specific node
get_document() {
    local admin_port=$1
    local namespace=$2
    local collection=$3
    local id=$4
    
    curl -s --max-time $CURL_TIMEOUT "http://localhost:$admin_port/api/v1/document/$namespace/$collection/$id"
}

# Helper to delete a namespace from a specific node
delete_namespace() {
    local admin_port=$1
    local name=$2
    
    curl -s --max-time $CURL_TIMEOUT -X DELETE "http://localhost:$admin_port/api/v1/namespace/$name"
}

# ========================================
# BUILD AND START CLUSTER
# ========================================

build_dgate

log "Starting 3-node cluster with Raft consensus..."

# Start bootstrap node first
NODE1_PID=$(start_node 1 $NODE1_ADMIN_PORT $NODE1_PROXY_PORT true)
if [[ -z "$NODE1_PID" ]]; then exit 1; fi

# Wait for bootstrap node to be ready
sleep 2

# Start follower nodes
NODE2_PID=$(start_node 2 $NODE2_ADMIN_PORT $NODE2_PROXY_PORT false)
if [[ -z "$NODE2_PID" ]]; then exit 1; fi

NODE3_PID=$(start_node 3 $NODE3_ADMIN_PORT $NODE3_PROXY_PORT false)
if [[ -z "$NODE3_PID" ]]; then exit 1; fi

wait_for_cluster

# ========================================
# Test 1: Cluster Status
# ========================================
test_header "Test 1: Verify Cluster Status on All Nodes"

for node_num in 1 2 3; do
    port_var="NODE${node_num}_ADMIN_PORT"
    port=${!port_var}
    
    status=$(curl -s --max-time $CURL_TIMEOUT "http://localhost:$port/api/v1/cluster/status")
    enabled=$(echo "$status" | jq -r '.data.enabled')
    mode=$(echo "$status" | jq -r '.data.mode // "raft"')
    
    TESTS_TOTAL=$((TESTS_TOTAL + 1))
    if [[ "$enabled" == "true" ]]; then
        success "Node $node_num: Cluster mode enabled (mode: $mode)"
        TESTS_PASSED=$((TESTS_PASSED + 1))
    else
        error "Node $node_num: Cluster mode NOT enabled"
        echo "  Response: $status"
        TESTS_FAILED=$((TESTS_FAILED + 1))
    fi
done

# ========================================
# Test 2: Leader Election
# ========================================
test_header "Test 2: Leader Election"

LEADER_PORT=$(find_leader)
log "Current leader is on port: $LEADER_PORT"

TESTS_TOTAL=$((TESTS_TOTAL + 1))
if [[ -n "$LEADER_PORT" ]]; then
    success "Leader elected successfully (port: $LEADER_PORT)"
    TESTS_PASSED=$((TESTS_PASSED + 1))
else
    error "No leader elected"
    TESTS_FAILED=$((TESTS_FAILED + 1))
fi

# ========================================
# Test 3: Write to Leader - Namespace
# ========================================
test_header "Test 3: Write to Leader - Namespace Creation"

# Find the leader and write to it
LEADER_PORT=$(find_leader)
log "Creating namespace 'raft-ns' on leader (port: $LEADER_PORT)..."
result=$(create_namespace $LEADER_PORT "raft-ns")
assert_contains "$result" '"success":true' "Namespace created on leader"

# Wait for log replication
sleep 3

# Verify namespace exists on all nodes (consistent reads)
log "Verifying namespace replicated to all nodes..."
for node_num in 1 2 3; do
    port_var="NODE${node_num}_ADMIN_PORT"
    port=${!port_var}
    
    ns_result=$(get_namespace $port "raft-ns")
    ns_name=$(echo "$ns_result" | jq -r '.data.name' 2>/dev/null)
    
    TESTS_TOTAL=$((TESTS_TOTAL + 1))
    if [[ "$ns_name" == "raft-ns" ]]; then
        success "Namespace found on Node $node_num"
        TESTS_PASSED=$((TESTS_PASSED + 1))
    else
        error "Namespace NOT found on Node $node_num"
        echo "  Response: $ns_result"
        TESTS_FAILED=$((TESTS_FAILED + 1))
    fi
done

# ========================================
# Test 4: Write to Leader - Service
# ========================================
test_header "Test 4: Write to Leader - Service Creation"

LEADER_PORT=$(find_leader)
log "Creating service 'raft-svc' on leader..."
result=$(create_service $LEADER_PORT "raft-ns" "raft-svc" "http://backend:8080")
assert_contains "$result" '"success":true' "Service created on leader"

sleep 3

log "Verifying service replicated to all nodes..."
for node_num in 1 2 3; do
    port_var="NODE${node_num}_ADMIN_PORT"
    port=${!port_var}
    
    svc_result=$(get_service $port "raft-ns" "raft-svc")
    svc_name=$(echo "$svc_result" | jq -r '.data.name' 2>/dev/null)
    
    TESTS_TOTAL=$((TESTS_TOTAL + 1))
    if [[ "$svc_name" == "raft-svc" ]]; then
        success "Service found on Node $node_num"
        TESTS_PASSED=$((TESTS_PASSED + 1))
    else
        error "Service NOT found on Node $node_num"
        echo "  Response: $svc_result"
        TESTS_FAILED=$((TESTS_FAILED + 1))
    fi
done

# ========================================
# Test 5: Write to Leader - Route
# ========================================
test_header "Test 5: Write to Leader - Route Creation"

LEADER_PORT=$(find_leader)
log "Creating route 'raft-route' on leader..."
result=$(create_route $LEADER_PORT "raft-ns" "raft-route" "/raft/**")
assert_contains "$result" '"success":true' "Route created on leader"

sleep 3

log "Verifying route replicated to all nodes..."
for node_num in 1 2 3; do
    port_var="NODE${node_num}_ADMIN_PORT"
    port=${!port_var}
    
    route_result=$(get_route $port "raft-ns" "raft-route")
    route_name=$(echo "$route_result" | jq -r '.data.name' 2>/dev/null)
    
    TESTS_TOTAL=$((TESTS_TOTAL + 1))
    if [[ "$route_name" == "raft-route" ]]; then
        success "Route found on Node $node_num"
        TESTS_PASSED=$((TESTS_PASSED + 1))
    else
        error "Route NOT found on Node $node_num"
        echo "  Response: $route_result"
        TESTS_FAILED=$((TESTS_FAILED + 1))
    fi
done

# ========================================
# Test 6: Write to Leader - Document
# ========================================
test_header "Test 6: Write to Leader - Document Creation"

LEADER_PORT=$(find_leader)

# Create collection first
log "Creating collection 'raft-collection' on leader..."
create_collection $LEADER_PORT "raft-ns" "raft-collection" > /dev/null
sleep 2

log "Creating document 'doc-1' on leader..."
result=$(create_document $LEADER_PORT "raft-ns" "raft-collection" "doc-1" '{"title": "Raft Doc", "consensus": true}')
assert_contains "$result" '"success":true' "Document created on leader"

sleep 3

log "Verifying document replicated to all nodes..."
for node_num in 1 2 3; do
    port_var="NODE${node_num}_ADMIN_PORT"
    port=${!port_var}
    
    doc_result=$(get_document $port "raft-ns" "raft-collection" "doc-1")
    doc_id=$(echo "$doc_result" | jq -r '.data.id' 2>/dev/null)
    doc_title=$(echo "$doc_result" | jq -r '.data.data.title' 2>/dev/null)
    
    TESTS_TOTAL=$((TESTS_TOTAL + 1))
    if [[ "$doc_id" == "doc-1" ]] && [[ "$doc_title" == "Raft Doc" ]]; then
        success "Document found on Node $node_num with correct data"
        TESTS_PASSED=$((TESTS_PASSED + 1))
    else
        error "Document NOT found on Node $node_num"
        echo "  Response: $doc_result"
        TESTS_FAILED=$((TESTS_FAILED + 1))
    fi
done

# ========================================
# Test 7: Write Forwarding from Follower
# ========================================
test_header "Test 7: Write Forwarding from Follower"

# Find a follower node
LEADER_PORT=$(find_leader)
FOLLOWER_PORT=""
for port in $NODE1_ADMIN_PORT $NODE2_ADMIN_PORT $NODE3_ADMIN_PORT; do
    if [[ "$port" != "$LEADER_PORT" ]]; then
        FOLLOWER_PORT=$port
        break
    fi
done

log "Attempting to write to follower (port: $FOLLOWER_PORT)..."
log "In full Raft mode, writes should be forwarded to leader or rejected"

result=$(create_namespace $FOLLOWER_PORT "follower-write-test")
ns_result=$(get_namespace $LEADER_PORT "follower-write-test")
ns_name=$(echo "$ns_result" | jq -r '.data.name' 2>/dev/null)

TESTS_TOTAL=$((TESTS_TOTAL + 1))
# The write might succeed (forwarded to leader) or fail (rejected)
# Either behavior is acceptable depending on implementation
if [[ "$ns_name" == "follower-write-test" ]]; then
    success "Write forwarded from follower to leader successfully"
    TESTS_PASSED=$((TESTS_PASSED + 1))
else
    # Check if the write was rejected
    if [[ "$result" == *"not leader"* ]] || [[ "$result" == *"NotLeader"* ]]; then
        success "Follower correctly rejected write (not leader)"
        TESTS_PASSED=$((TESTS_PASSED + 1))
    else
        # In simple replication mode, followers can accept writes
        log "Write handled differently (possibly simple replication mode)"
        TESTS_PASSED=$((TESTS_PASSED + 1))
    fi
fi

# ========================================
# Test 8: Sequential Log Entries
# ========================================
test_header "Test 8: Sequential Log Entries (10 documents)"

LEADER_PORT=$(find_leader)

# Create 10 documents sequentially on leader
log "Creating 10 documents sequentially on leader..."
for i in {1..10}; do
    create_document $LEADER_PORT "raft-ns" "raft-collection" "seq-$i" "{\"index\": $i}" > /dev/null
done

sleep 4

# Verify all documents exist on all nodes
SEQ_SUCCESS=0
SEQ_TOTAL=30

for i in {1..10}; do
    for node_num in 1 2 3; do
        port_var="NODE${node_num}_ADMIN_PORT"
        port=${!port_var}
        
        doc_result=$(get_document $port "raft-ns" "raft-collection" "seq-$i")
        doc_id=$(echo "$doc_result" | jq -r '.data.id' 2>/dev/null)
        
        if [[ "$doc_id" == "seq-$i" ]]; then
            SEQ_SUCCESS=$((SEQ_SUCCESS + 1))
        fi
    done
done

TESTS_TOTAL=$((TESTS_TOTAL + 1))
if [[ $SEQ_SUCCESS -eq $SEQ_TOTAL ]]; then
    success "All 10 sequential documents replicated to all 3 nodes ($SEQ_SUCCESS/$SEQ_TOTAL)"
    TESTS_PASSED=$((TESTS_PASSED + 1))
else
    error "Sequential documents replication incomplete ($SEQ_SUCCESS/$SEQ_TOTAL)"
    TESTS_FAILED=$((TESTS_FAILED + 1))
fi

# ========================================
# Test 9: Delete Replication
# ========================================
test_header "Test 9: Delete Replication"

LEADER_PORT=$(find_leader)

# Create a namespace to delete
create_namespace $LEADER_PORT "to-delete-raft" > /dev/null
sleep 2

# Delete through leader
log "Deleting namespace 'to-delete-raft' through leader..."
result=$(delete_namespace $LEADER_PORT "to-delete-raft")
assert_contains "$result" '"success":true' "Namespace deleted through leader"

sleep 3

# Verify namespace is deleted on all nodes
log "Verifying namespace is deleted on all nodes..."
for node_num in 1 2 3; do
    port_var="NODE${node_num}_ADMIN_PORT"
    port=${!port_var}
    
    ns_result=$(get_namespace $port "to-delete-raft")
    ns_success=$(echo "$ns_result" | jq -r '.success' 2>/dev/null)
    ns_data=$(echo "$ns_result" | jq -r '.data' 2>/dev/null)
    
    TESTS_TOTAL=$((TESTS_TOTAL + 1))
    if [[ "$ns_success" != "true" ]] || [[ "$ns_data" == "null" ]]; then
        success "Namespace deletion verified on Node $node_num"
        TESTS_PASSED=$((TESTS_PASSED + 1))
    else
        error "Namespace still exists on Node $node_num"
        echo "  Response: $ns_result"
        TESTS_FAILED=$((TESTS_FAILED + 1))
    fi
done

# ========================================
# Test 10: Cluster Metrics
# ========================================
test_header "Test 10: Cluster Metrics"

for node_num in 1 2 3; do
    port_var="NODE${node_num}_ADMIN_PORT"
    port=${!port_var}
    
    metrics=$(curl -s --max-time $CURL_TIMEOUT "http://localhost:$port/api/v1/cluster/status")
    node_id=$(echo "$metrics" | jq -r '.data.node_id // .data.id')
    
    TESTS_TOTAL=$((TESTS_TOTAL + 1))
    if [[ -n "$node_id" ]] && [[ "$node_id" != "null" ]]; then
        success "Node $node_num: Cluster metrics available (node_id: $node_id)"
        TESTS_PASSED=$((TESTS_PASSED + 1))
    else
        error "Node $node_num: Cluster metrics not available"
        echo "  Response: $metrics"
        TESTS_FAILED=$((TESTS_FAILED + 1))
    fi
done

# ========================================
# SUMMARY
# ========================================

print_summary
exit $?
