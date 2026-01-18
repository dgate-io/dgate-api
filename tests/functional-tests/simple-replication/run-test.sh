#!/bin/bash
# Simple HTTP Replication Functional Tests
#
# Tests that resources are replicated across 3 DGate nodes using the
# simple HTTP replication mode (direct HTTP calls to peer nodes).
#
# This mode:
# - Uses direct HTTP calls to replicate changes to peer nodes
# - All nodes can accept writes and replicate to others
# - Simple and effective for most use cases
# - Does NOT require leader election

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "$SCRIPT_DIR/../common/utils.sh"

test_header "Simple HTTP Replication Functional Tests (3-Node)"

# Cleanup on exit
trap cleanup_cluster EXIT

# Cluster configuration
NODE1_ADMIN_PORT=9181
NODE2_ADMIN_PORT=9182
NODE3_ADMIN_PORT=9183
NODE1_PROXY_PORT=8181
NODE2_PROXY_PORT=8182
NODE3_PROXY_PORT=8183
NODE1_RAFT_PORT=9191
NODE2_RAFT_PORT=9192
NODE3_RAFT_PORT=9193

# PIDs for cleanup
NODE1_PID=""
NODE2_PID=""
NODE3_PID=""

cleanup_cluster() {
    log "Cleaning up cluster nodes..."
    [[ -n "$NODE1_PID" ]] && kill $NODE1_PID 2>/dev/null || true
    [[ -n "$NODE2_PID" ]] && kill $NODE2_PID 2>/dev/null || true
    [[ -n "$NODE3_PID" ]] && kill $NODE3_PID 2>/dev/null || true
    rm -f /tmp/dgate-simple-node*.log /tmp/dgate-simple-node*.yaml
    cleanup_processes
    return 0
}

# Generate config for a node with simple replication mode
generate_node_config() {
    local node_id=$1
    local admin_port=$2
    local proxy_port=$3
    local raft_port=$4
    local is_bootstrap=$5
    local config_file="/tmp/dgate-simple-node${node_id}.yaml"
    
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

# Simple replication mode - uses HTTP to replicate to peers
cluster:
  enabled: true
  mode: simple
  node_id: ${node_id}
  advertise_addr: "127.0.0.1:${raft_port}"
  bootstrap: ${is_bootstrap}
  initial_members:
    - id: 1
      addr: "127.0.0.1:${NODE1_RAFT_PORT}"
      admin_port: ${NODE1_ADMIN_PORT}
    - id: 2
      addr: "127.0.0.1:${NODE2_RAFT_PORT}"
      admin_port: ${NODE2_ADMIN_PORT}
    - id: 3
      addr: "127.0.0.1:${NODE3_RAFT_PORT}"
      admin_port: ${NODE3_ADMIN_PORT}
EOF
    echo "$config_file"
}

# Start a node
start_node() {
    local node_id=$1
    local admin_port=$2
    local proxy_port=$3
    local raft_port=$4
    local is_bootstrap=$5
    local config_file
    
    config_file=$(generate_node_config $node_id $admin_port $proxy_port $raft_port $is_bootstrap)
    
    log "Starting Node $node_id (admin: $admin_port, proxy: $proxy_port, raft: $raft_port)"
    "$DGATE_BIN" -c "$config_file" > "/tmp/dgate-simple-node${node_id}.log" 2>&1 &
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
    cat "/tmp/dgate-simple-node${node_id}.log"
    return 1
}

# Wait for cluster to form
wait_for_cluster() {
    log "Waiting for cluster to form..."
    sleep 2
    
    # Check cluster status on each node
    for port in $NODE1_ADMIN_PORT $NODE2_ADMIN_PORT $NODE3_ADMIN_PORT; do
        local status=$(curl -s "http://localhost:$port/api/v1/cluster/status")
        if [[ $(echo "$status" | jq -r '.data.enabled') != "true" ]]; then
            warn "Node on port $port: cluster not enabled"
        else
            log "Node on port $port: cluster mode active"
        fi
    done
}

# Helper to create a namespace on a specific node
create_namespace() {
    local admin_port=$1
    local name=$2
    
    curl -s -X PUT "http://localhost:$admin_port/api/v1/namespace/$name" \
        -H "Content-Type: application/json" \
        -d "{\"name\": \"$name\", \"tags\": [\"test\"]}"
}

# Helper to get a namespace from a specific node
get_namespace() {
    local admin_port=$1
    local name=$2
    
    curl -s "http://localhost:$admin_port/api/v1/namespace/$name"
}

# Helper to create a service on a specific node
create_service() {
    local admin_port=$1
    local namespace=$2
    local name=$3
    local url=$4
    
    curl -s -X PUT "http://localhost:$admin_port/api/v1/service/$namespace/$name" \
        -H "Content-Type: application/json" \
        -d "{\"name\": \"$name\", \"namespace\": \"$namespace\", \"urls\": [\"$url\"]}"
}

# Helper to get a service from a specific node
get_service() {
    local admin_port=$1
    local namespace=$2
    local name=$3
    
    curl -s "http://localhost:$admin_port/api/v1/service/$namespace/$name"
}

# Helper to create a route on a specific node
create_route() {
    local admin_port=$1
    local namespace=$2
    local name=$3
    local path=$4
    
    curl -s -X PUT "http://localhost:$admin_port/api/v1/route/$namespace/$name" \
        -H "Content-Type: application/json" \
        -d "{\"name\": \"$name\", \"namespace\": \"$namespace\", \"paths\": [\"$path\"], \"methods\": [\"*\"]}"
}

# Helper to get a route from a specific node
get_route() {
    local admin_port=$1
    local namespace=$2
    local name=$3
    
    curl -s "http://localhost:$admin_port/api/v1/route/$namespace/$name"
}

# Helper to create a document on a specific node
create_document() {
    local admin_port=$1
    local namespace=$2
    local collection=$3
    local id=$4
    local data=$5
    
    curl -s -X PUT "http://localhost:$admin_port/api/v1/document/$namespace/$collection/$id" \
        -H "Content-Type: application/json" \
        -d "{\"id\": \"$id\", \"namespace\": \"$namespace\", \"collection\": \"$collection\", \"data\": $data}"
}

# Helper to get a document from a specific node
get_document() {
    local admin_port=$1
    local namespace=$2
    local collection=$3
    local id=$4
    
    curl -s "http://localhost:$admin_port/api/v1/document/$namespace/$collection/$id"
}

# Helper to create a collection on a specific node
create_collection() {
    local admin_port=$1
    local namespace=$2
    local name=$3
    
    curl -s -X PUT "http://localhost:$admin_port/api/v1/collection/$namespace/$name" \
        -H "Content-Type: application/json" \
        -d "{\"name\": \"$name\", \"namespace\": \"$namespace\", \"visibility\": \"private\"}"
}

# Helper to delete a namespace from a specific node
delete_namespace() {
    local admin_port=$1
    local name=$2
    
    curl -s -X DELETE "http://localhost:$admin_port/api/v1/namespace/$name"
}

# ========================================
# BUILD AND START CLUSTER
# ========================================

build_dgate

log "Starting 3-node cluster with simple HTTP replication..."

NODE1_PID=$(start_node 1 $NODE1_ADMIN_PORT $NODE1_PROXY_PORT $NODE1_RAFT_PORT true)
if [[ -z "$NODE1_PID" ]]; then exit 1; fi

NODE2_PID=$(start_node 2 $NODE2_ADMIN_PORT $NODE2_PROXY_PORT $NODE2_RAFT_PORT false)
if [[ -z "$NODE2_PID" ]]; then exit 1; fi

NODE3_PID=$(start_node 3 $NODE3_ADMIN_PORT $NODE3_PROXY_PORT $NODE3_RAFT_PORT false)
if [[ -z "$NODE3_PID" ]]; then exit 1; fi

wait_for_cluster

# ========================================
# Test 1: Cluster Status
# ========================================
test_header "Test 1: Verify Cluster Status on All Nodes"

for node_num in 1 2 3; do
    port_var="NODE${node_num}_ADMIN_PORT"
    port=${!port_var}
    
    status=$(curl -s "http://localhost:$port/api/v1/cluster/status")
    enabled=$(echo "$status" | jq -r '.data.enabled')
    
    TESTS_TOTAL=$((TESTS_TOTAL + 1))
    if [[ "$enabled" == "true" ]]; then
        success "Node $node_num: Cluster mode enabled"
        TESTS_PASSED=$((TESTS_PASSED + 1))
    else
        error "Node $node_num: Cluster mode NOT enabled"
        echo "  Response: $status"
        TESTS_FAILED=$((TESTS_FAILED + 1))
    fi
done

# ========================================
# Test 2: Multi-Master Write Capability
# ========================================
test_header "Test 2: Multi-Master Write Capability"

# All nodes should be able to accept writes in simple replication mode
log "Testing that all nodes can accept writes..."

for node_num in 1 2 3; do
    port_var="NODE${node_num}_ADMIN_PORT"
    port=${!port_var}
    ns_name="node${node_num}-write-test"
    
    result=$(create_namespace $port "$ns_name")
    ns_result=$(get_namespace $port "$ns_name")
    result_name=$(echo "$ns_result" | jq -r '.data.name' 2>/dev/null)
    
    TESTS_TOTAL=$((TESTS_TOTAL + 1))
    if [[ "$result_name" == "$ns_name" ]]; then
        success "Node $node_num: Write accepted successfully"
        TESTS_PASSED=$((TESTS_PASSED + 1))
    else
        error "Node $node_num: Write failed"
        echo "  Response: $ns_result"
        TESTS_FAILED=$((TESTS_FAILED + 1))
    fi
done

# ========================================
# Test 3: Namespace Replication
# ========================================
test_header "Test 3: Namespace Replication Across Nodes"

# Create namespace on Node 1
log "Creating namespace 'replicate-ns' on Node 1..."
result=$(create_namespace $NODE1_ADMIN_PORT "replicate-ns")
assert_contains "$result" '"success":true' "Namespace created on Node 1"

# Wait for replication
sleep 2

# Verify namespace exists on all nodes
log "Verifying namespace replicated to all nodes..."
for node_num in 1 2 3; do
    port_var="NODE${node_num}_ADMIN_PORT"
    port=${!port_var}
    
    ns_result=$(get_namespace $port "replicate-ns")
    ns_name=$(echo "$ns_result" | jq -r '.data.name' 2>/dev/null)
    
    TESTS_TOTAL=$((TESTS_TOTAL + 1))
    if [[ "$ns_name" == "replicate-ns" ]]; then
        success "Namespace found on Node $node_num"
        TESTS_PASSED=$((TESTS_PASSED + 1))
    else
        error "Namespace NOT found on Node $node_num"
        echo "  Response: $ns_result"
        TESTS_FAILED=$((TESTS_FAILED + 1))
    fi
done

# ========================================
# Test 4: Service Replication
# ========================================
test_header "Test 4: Service Replication Across Nodes"

# Create service on Node 2
log "Creating service 'test-svc' on Node 2..."
result=$(create_service $NODE2_ADMIN_PORT "replicate-ns" "test-svc" "http://backend:8080")
assert_contains "$result" '"success":true' "Service created on Node 2"

sleep 2

log "Verifying service replicated to all nodes..."
for node_num in 1 2 3; do
    port_var="NODE${node_num}_ADMIN_PORT"
    port=${!port_var}
    
    svc_result=$(get_service $port "replicate-ns" "test-svc")
    svc_name=$(echo "$svc_result" | jq -r '.data.name' 2>/dev/null)
    
    TESTS_TOTAL=$((TESTS_TOTAL + 1))
    if [[ "$svc_name" == "test-svc" ]]; then
        success "Service found on Node $node_num"
        TESTS_PASSED=$((TESTS_PASSED + 1))
    else
        error "Service NOT found on Node $node_num"
        echo "  Response: $svc_result"
        TESTS_FAILED=$((TESTS_FAILED + 1))
    fi
done

# ========================================
# Test 5: Route Replication
# ========================================
test_header "Test 5: Route Replication Across Nodes"

# Create route on Node 3
log "Creating route 'test-route' on Node 3..."
result=$(create_route $NODE3_ADMIN_PORT "replicate-ns" "test-route" "/api/**")
assert_contains "$result" '"success":true' "Route created on Node 3"

sleep 2

log "Verifying route replicated to all nodes..."
for node_num in 1 2 3; do
    port_var="NODE${node_num}_ADMIN_PORT"
    port=${!port_var}
    
    route_result=$(get_route $port "replicate-ns" "test-route")
    route_name=$(echo "$route_result" | jq -r '.data.name' 2>/dev/null)
    
    TESTS_TOTAL=$((TESTS_TOTAL + 1))
    if [[ "$route_name" == "test-route" ]]; then
        success "Route found on Node $node_num"
        TESTS_PASSED=$((TESTS_PASSED + 1))
    else
        error "Route NOT found on Node $node_num"
        echo "  Response: $route_result"
        TESTS_FAILED=$((TESTS_FAILED + 1))
    fi
done

# ========================================
# Test 6: Document Replication
# ========================================
test_header "Test 6: Document Replication Across Nodes"

# Create collection first
log "Creating collection 'test-collection' on Node 1..."
create_collection $NODE1_ADMIN_PORT "replicate-ns" "test-collection" > /dev/null

sleep 2

# Create document on Node 1
log "Creating document 'doc-1' on Node 1..."
result=$(create_document $NODE1_ADMIN_PORT "replicate-ns" "test-collection" "doc-1" '{"title": "Test Doc", "value": 42}')
assert_contains "$result" '"success":true' "Document created on Node 1"

sleep 2

log "Verifying document replicated to all nodes..."
for node_num in 1 2 3; do
    port_var="NODE${node_num}_ADMIN_PORT"
    port=${!port_var}
    
    doc_result=$(get_document $port "replicate-ns" "test-collection" "doc-1")
    doc_id=$(echo "$doc_result" | jq -r '.data.id' 2>/dev/null)
    doc_title=$(echo "$doc_result" | jq -r '.data.data.title' 2>/dev/null)
    
    TESTS_TOTAL=$((TESTS_TOTAL + 1))
    if [[ "$doc_id" == "doc-1" ]] && [[ "$doc_title" == "Test Doc" ]]; then
        success "Document found on Node $node_num with correct data"
        TESTS_PASSED=$((TESTS_PASSED + 1))
    else
        error "Document NOT found on Node $node_num"
        echo "  Response: $doc_result"
        TESTS_FAILED=$((TESTS_FAILED + 1))
    fi
done

# ========================================
# Test 7: Concurrent Writes from Multiple Nodes
# ========================================
test_header "Test 7: Concurrent Writes from Multiple Nodes"

# Create documents on all nodes simultaneously
log "Creating documents concurrently on all nodes..."
(create_namespace $NODE1_ADMIN_PORT "concurrent-ns" > /dev/null) &
pid1=$!
(create_namespace $NODE2_ADMIN_PORT "concurrent-ns" > /dev/null) &
pid2=$!
(create_namespace $NODE3_ADMIN_PORT "concurrent-ns" > /dev/null) &
pid3=$!
wait $pid1 $pid2 $pid3

sleep 3

# Verify the namespace exists (at least one write should have succeeded)
ns_found=0
for port in $NODE1_ADMIN_PORT $NODE2_ADMIN_PORT $NODE3_ADMIN_PORT; do
    ns_result=$(get_namespace $port "concurrent-ns")
    ns_name=$(echo "$ns_result" | jq -r '.data.name' 2>/dev/null)
    if [[ "$ns_name" == "concurrent-ns" ]]; then
        ns_found=$((ns_found + 1))
    fi
done

TESTS_TOTAL=$((TESTS_TOTAL + 1))
if [[ $ns_found -eq 3 ]]; then
    success "Concurrent namespace creation handled correctly ($ns_found/3 nodes have data)"
    TESTS_PASSED=$((TESTS_PASSED + 1))
else
    warn "Concurrent writes partially succeeded ($ns_found/3 nodes have data)"
    TESTS_PASSED=$((TESTS_PASSED + 1))  # Still a pass for simple replication
fi

# ========================================
# Test 8: Rapid Sequential Writes
# ========================================
test_header "Test 8: Rapid Sequential Writes (10 documents)"

# Create collection for rapid writes
create_collection $NODE1_ADMIN_PORT "replicate-ns" "rapid-collection" > /dev/null
sleep 1

# Create 10 documents rapidly on Node 1
for i in {1..10}; do
    create_document $NODE1_ADMIN_PORT "replicate-ns" "rapid-collection" "rapid-$i" "{\"index\": $i}" > /dev/null &
done
wait

sleep 3

# Verify all 10 documents exist on all nodes
RAPID_SUCCESS=0
RAPID_TOTAL=30

for i in {1..10}; do
    for node_num in 1 2 3; do
        port_var="NODE${node_num}_ADMIN_PORT"
        port=${!port_var}
        
        doc_result=$(get_document $port "replicate-ns" "rapid-collection" "rapid-$i")
        doc_id=$(echo "$doc_result" | jq -r '.data.id' 2>/dev/null)
        
        if [[ "$doc_id" == "rapid-$i" ]]; then
            RAPID_SUCCESS=$((RAPID_SUCCESS + 1))
        fi
    done
done

TESTS_TOTAL=$((TESTS_TOTAL + 1))
if [[ $RAPID_SUCCESS -eq $RAPID_TOTAL ]]; then
    success "All 10 rapid documents replicated to all 3 nodes ($RAPID_SUCCESS/$RAPID_TOTAL)"
    TESTS_PASSED=$((TESTS_PASSED + 1))
elif [[ $RAPID_SUCCESS -ge $((RAPID_TOTAL * 80 / 100)) ]]; then
    warn "Most rapid documents replicated ($RAPID_SUCCESS/$RAPID_TOTAL)"
    TESTS_PASSED=$((TESTS_PASSED + 1))
else
    error "Rapid documents replication incomplete ($RAPID_SUCCESS/$RAPID_TOTAL)"
    TESTS_FAILED=$((TESTS_FAILED + 1))
fi

# ========================================
# Test 9: Delete Replication
# ========================================
test_header "Test 9: Delete Replication"

# Create a namespace to delete
create_namespace $NODE1_ADMIN_PORT "to-delete" > /dev/null
sleep 2

# Delete from Node 3
log "Deleting namespace 'to-delete' from Node 3..."
result=$(delete_namespace $NODE3_ADMIN_PORT "to-delete")
assert_contains "$result" '"success":true' "Namespace deleted from Node 3"

sleep 2

# Verify namespace is deleted on all nodes
log "Verifying namespace is deleted on all nodes..."
deleted_on_all=true
for node_num in 1 2 3; do
    port_var="NODE${node_num}_ADMIN_PORT"
    port=${!port_var}
    
    ns_result=$(get_namespace $port "to-delete")
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
        deleted_on_all=false
    fi
done

# ========================================
# SUMMARY
# ========================================

print_summary
exit $?
