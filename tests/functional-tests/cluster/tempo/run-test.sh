#!/bin/bash
# Tempo Consensus Functional Tests
#
# Tests that resources are replicated across 3 DGate nodes using the
# Tempo multi-master consensus mode.
#
# This mode:
# - Multi-master consensus - any node can accept writes
# - Leaderless coordination using logical clocks
# - Quorum-based commit with fast path optimization
# - Better write scalability than Raft (no leader bottleneck)
# - Clock synchronization ensures consistent ordering

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "$SCRIPT_DIR/../../common/utils.sh"

test_header "Tempo Multi-Master Consensus Functional Tests (3-Node)"

# Cleanup on exit
trap cleanup_cluster EXIT

# Cluster configuration
# Note: advertise_addr uses the admin port since all Tempo messages are over HTTP on the admin API
NODE1_ADMIN_PORT=9381
NODE2_ADMIN_PORT=9382
NODE3_ADMIN_PORT=9383
NODE1_PROXY_PORT=8381
NODE2_PROXY_PORT=8382
NODE3_PROXY_PORT=8383

# PIDs for cleanup
NODE1_PID=""
NODE2_PID=""
NODE3_PID=""

cleanup_cluster() {
    log "Cleaning up cluster nodes..."
    [[ -n "$NODE1_PID" ]] && kill $NODE1_PID 2>/dev/null || true
    [[ -n "$NODE2_PID" ]] && kill $NODE2_PID 2>/dev/null || true
    [[ -n "$NODE3_PID" ]] && kill $NODE3_PID 2>/dev/null || true
    rm -f /tmp/dgate-tempo-node*.log /tmp/dgate-tempo-node*.yaml
    cleanup_processes
    return 0
}

# Generate config for a node with Tempo consensus mode
generate_node_config() {
    local node_id=$1
    local admin_port=$2
    local proxy_port=$3
    local is_bootstrap=$4
    local config_file="/tmp/dgate-tempo-node${node_id}.yaml"
    
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

# Tempo multi-master consensus mode
# advertise_addr uses admin port since Tempo messages are served on admin API
cluster:
  enabled: true
  mode: tempo
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
  # Tempo-specific configuration
  tempo:
    fast_quorum_size: 2
    write_quorum_size: 2
    clock_bump_interval_ms: 50
    detached_send_interval_ms: 100
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
    "$DGATE_BIN" -c "$config_file" > "/tmp/dgate-tempo-node${node_id}.log" 2>&1 &
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
    cat "/tmp/dgate-tempo-node${node_id}.log"
    return 1
}

# Curl timeout for all requests (seconds)
CURL_TIMEOUT=10

# Wait for cluster to form
wait_for_cluster() {
    log "Waiting for cluster to form..."
    sleep 3
    
    # Check cluster status on each node
    for port in $NODE1_ADMIN_PORT $NODE2_ADMIN_PORT $NODE3_ADMIN_PORT; do
        local status=$(curl -s --max-time $CURL_TIMEOUT "http://localhost:$port/api/v1/cluster/status")
        if [[ $(echo "$status" | jq -r '.data.enabled') != "true" ]]; then
            warn "Node on port $port: cluster not enabled"
        else
            local mode=$(echo "$status" | jq -r '.data.mode // "tempo"')
            log "Node on port $port: cluster mode active (mode: $mode)"
        fi
    done
}

# Helper to create a namespace on a specific node
create_namespace() {
    local admin_port=$1
    local name=$2
    
    curl -s --max-time $CURL_TIMEOUT -X PUT "http://localhost:$admin_port/api/v1/namespace/$name" \
        -H "Content-Type: application/json" \
        -d "{\"name\": \"$name\", \"tags\": [\"tempo-test\"]}"
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

log "Starting 3-node cluster with Tempo multi-master consensus..."

NODE1_PID=$(start_node 1 $NODE1_ADMIN_PORT $NODE1_PROXY_PORT true)
if [[ -z "$NODE1_PID" ]]; then exit 1; fi

NODE2_PID=$(start_node 2 $NODE2_ADMIN_PORT $NODE2_PROXY_PORT false)
if [[ -z "$NODE2_PID" ]]; then exit 1; fi

NODE3_PID=$(start_node 3 $NODE3_ADMIN_PORT $NODE3_PROXY_PORT false)
if [[ -z "$NODE3_PID" ]]; then exit 1; fi

wait_for_cluster

# ========================================
# Test 1: Cluster Status - Tempo Mode
# ========================================
test_header "Test 1: Verify Tempo Cluster Status on All Nodes"

for node_num in 1 2 3; do
    port_var="NODE${node_num}_ADMIN_PORT"
    port=${!port_var}
    
    status=$(curl -s --max-time $CURL_TIMEOUT "http://localhost:$port/api/v1/cluster/status")
    enabled=$(echo "$status" | jq -r '.data.enabled')
    mode=$(echo "$status" | jq -r '.data.mode // "unknown"')
    
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
# Test 2: Multi-Master Write - All Nodes Accept Writes
# ========================================
test_header "Test 2: Multi-Master Write Capability"

# All nodes should be able to accept writes in Tempo mode (leaderless)
log "Testing that all nodes can accept writes (multi-master)..."

for node_num in 1 2 3; do
    port_var="NODE${node_num}_ADMIN_PORT"
    port=${!port_var}
    ns_name="node${node_num}-tempo-write"
    
    result=$(create_namespace $port "$ns_name")
    sleep 1
    ns_result=$(get_namespace $port "$ns_name")
    result_name=$(echo "$ns_result" | jq -r '.data.name' 2>/dev/null)
    
    TESTS_TOTAL=$((TESTS_TOTAL + 1))
    if [[ "$result_name" == "$ns_name" ]]; then
        success "Node $node_num: Write accepted successfully (multi-master)"
        TESTS_PASSED=$((TESTS_PASSED + 1))
    else
        error "Node $node_num: Write failed"
        echo "  Response: $ns_result"
        TESTS_FAILED=$((TESTS_FAILED + 1))
    fi
done

# ========================================
# Test 3: Namespace Replication with Tempo
# ========================================
test_header "Test 3: Namespace Replication Across Nodes"

# Create namespace on Node 1
log "Creating namespace 'tempo-ns' on Node 1..."
result=$(create_namespace $NODE1_ADMIN_PORT "tempo-ns")
assert_contains "$result" '"success":true' "Namespace created on Node 1"

# Wait for quorum-based replication
sleep 3

# Verify namespace exists on all nodes
log "Verifying namespace replicated to all nodes..."
for node_num in 1 2 3; do
    port_var="NODE${node_num}_ADMIN_PORT"
    port=${!port_var}
    
    ns_result=$(get_namespace $port "tempo-ns")
    ns_name=$(echo "$ns_result" | jq -r '.data.name' 2>/dev/null)
    
    TESTS_TOTAL=$((TESTS_TOTAL + 1))
    if [[ "$ns_name" == "tempo-ns" ]]; then
        success "Namespace found on Node $node_num"
        TESTS_PASSED=$((TESTS_PASSED + 1))
    else
        error "Namespace NOT found on Node $node_num"
        echo "  Response: $ns_result"
        TESTS_FAILED=$((TESTS_FAILED + 1))
    fi
done

# ========================================
# Test 4: Service Replication with Tempo
# ========================================
test_header "Test 4: Service Replication Across Nodes"

# Create service on Node 2 (testing multi-master writes)
log "Creating service 'tempo-svc' on Node 2..."
result=$(create_service $NODE2_ADMIN_PORT "tempo-ns" "tempo-svc" "http://backend:8080")
assert_contains "$result" '"success":true' "Service created on Node 2"

sleep 3

log "Verifying service replicated to all nodes..."
for node_num in 1 2 3; do
    port_var="NODE${node_num}_ADMIN_PORT"
    port=${!port_var}
    
    svc_result=$(get_service $port "tempo-ns" "tempo-svc")
    svc_name=$(echo "$svc_result" | jq -r '.data.name' 2>/dev/null)
    
    TESTS_TOTAL=$((TESTS_TOTAL + 1))
    if [[ "$svc_name" == "tempo-svc" ]]; then
        success "Service found on Node $node_num"
        TESTS_PASSED=$((TESTS_PASSED + 1))
    else
        error "Service NOT found on Node $node_num"
        echo "  Response: $svc_result"
        TESTS_FAILED=$((TESTS_FAILED + 1))
    fi
done

# ========================================
# Test 5: Route Replication with Tempo
# ========================================
test_header "Test 5: Route Replication Across Nodes"

# Create route on Node 3 (testing multi-master writes)
log "Creating route 'tempo-route' on Node 3..."
result=$(create_route $NODE3_ADMIN_PORT "tempo-ns" "tempo-route" "/tempo/**")
assert_contains "$result" '"success":true' "Route created on Node 3"

sleep 3

log "Verifying route replicated to all nodes..."
for node_num in 1 2 3; do
    port_var="NODE${node_num}_ADMIN_PORT"
    port=${!port_var}
    
    route_result=$(get_route $port "tempo-ns" "tempo-route")
    route_name=$(echo "$route_result" | jq -r '.data.name' 2>/dev/null)
    
    TESTS_TOTAL=$((TESTS_TOTAL + 1))
    if [[ "$route_name" == "tempo-route" ]]; then
        success "Route found on Node $node_num"
        TESTS_PASSED=$((TESTS_PASSED + 1))
    else
        error "Route NOT found on Node $node_num"
        echo "  Response: $route_result"
        TESTS_FAILED=$((TESTS_FAILED + 1))
    fi
done

# ========================================
# Test 6: Document Replication with Tempo
# ========================================
test_header "Test 6: Document Replication Across Nodes"

# Create collection first
log "Creating collection 'tempo-collection' on Node 1..."
create_collection $NODE1_ADMIN_PORT "tempo-ns" "tempo-collection" > /dev/null
sleep 2

log "Creating document 'doc-1' on Node 1..."
result=$(create_document $NODE1_ADMIN_PORT "tempo-ns" "tempo-collection" "doc-1" '{"title": "Tempo Doc", "consensus": "multi-master"}')
assert_contains "$result" '"success":true' "Document created on Node 1"

sleep 3

log "Verifying document replicated to all nodes..."
for node_num in 1 2 3; do
    port_var="NODE${node_num}_ADMIN_PORT"
    port=${!port_var}
    
    doc_result=$(get_document $port "tempo-ns" "tempo-collection" "doc-1")
    doc_id=$(echo "$doc_result" | jq -r '.data.id' 2>/dev/null)
    doc_title=$(echo "$doc_result" | jq -r '.data.data.title' 2>/dev/null)
    
    TESTS_TOTAL=$((TESTS_TOTAL + 1))
    if [[ "$doc_id" == "doc-1" ]] && [[ "$doc_title" == "Tempo Doc" ]]; then
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
test_header "Test 7: Concurrent Writes from All Nodes (Multi-Master)"

# Create documents on all nodes simultaneously - Tempo handles conflicts
log "Creating documents concurrently on all nodes..."
(create_document $NODE1_ADMIN_PORT "tempo-ns" "tempo-collection" "concurrent-1" '{"source": "node1"}' > /dev/null) &
pid1=$!
(create_document $NODE2_ADMIN_PORT "tempo-ns" "tempo-collection" "concurrent-2" '{"source": "node2"}' > /dev/null) &
pid2=$!
(create_document $NODE3_ADMIN_PORT "tempo-ns" "tempo-collection" "concurrent-3" '{"source": "node3"}' > /dev/null) &
pid3=$!
wait $pid1 $pid2 $pid3

sleep 3

# Verify all 3 documents exist on all nodes
log "Verifying concurrent writes replicated..."
docs_found=0
for doc_id in "concurrent-1" "concurrent-2" "concurrent-3"; do
    for port in $NODE1_ADMIN_PORT $NODE2_ADMIN_PORT $NODE3_ADMIN_PORT; do
        doc_result=$(get_document $port "tempo-ns" "tempo-collection" "$doc_id")
        result_id=$(echo "$doc_result" | jq -r '.data.id' 2>/dev/null)
        if [[ "$result_id" == "$doc_id" ]]; then
            docs_found=$((docs_found + 1))
        fi
    done
done

TESTS_TOTAL=$((TESTS_TOTAL + 1))
if [[ $docs_found -eq 9 ]]; then  # 3 docs x 3 nodes
    success "All concurrent documents replicated to all nodes ($docs_found/9)"
    TESTS_PASSED=$((TESTS_PASSED + 1))
elif [[ $docs_found -ge 6 ]]; then
    warn "Most concurrent documents replicated ($docs_found/9)"
    TESTS_PASSED=$((TESTS_PASSED + 1))
else
    error "Concurrent documents replication incomplete ($docs_found/9)"
    TESTS_FAILED=$((TESTS_FAILED + 1))
fi

# ========================================
# Test 8: Rapid Sequential Writes (Tempo Fast Path)
# ========================================
test_header "Test 8: Rapid Sequential Writes (10 documents)"

# Test the fast path optimization in Tempo
log "Creating 10 documents rapidly on Node 1..."
for i in {1..10}; do
    create_document $NODE1_ADMIN_PORT "tempo-ns" "tempo-collection" "rapid-$i" "{\"index\": $i}" > /dev/null &
done
wait

sleep 4

# Verify all 10 documents exist on all nodes
RAPID_SUCCESS=0
RAPID_TOTAL=30

for i in {1..10}; do
    for node_num in 1 2 3; do
        port_var="NODE${node_num}_ADMIN_PORT"
        port=${!port_var}
        
        doc_result=$(get_document $port "tempo-ns" "tempo-collection" "rapid-$i")
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
# Test 9: Distributed Writes (Round-Robin)
# ========================================
test_header "Test 9: Distributed Writes Across Nodes"

# Write documents to different nodes in round-robin fashion
log "Creating 9 documents distributed across nodes..."
for i in {1..9}; do
    case $((i % 3)) in
        1) port=$NODE1_ADMIN_PORT ;;
        2) port=$NODE2_ADMIN_PORT ;;
        0) port=$NODE3_ADMIN_PORT ;;
    esac
    create_document $port "tempo-ns" "tempo-collection" "distributed-$i" "{\"node\": $((i % 3 + 1))}" > /dev/null
done

sleep 4

# Verify all 9 documents exist on all nodes
DIST_SUCCESS=0
DIST_TOTAL=27

for i in {1..9}; do
    for node_num in 1 2 3; do
        port_var="NODE${node_num}_ADMIN_PORT"
        port=${!port_var}
        
        doc_result=$(get_document $port "tempo-ns" "tempo-collection" "distributed-$i")
        doc_id=$(echo "$doc_result" | jq -r '.data.id' 2>/dev/null)
        
        if [[ "$doc_id" == "distributed-$i" ]]; then
            DIST_SUCCESS=$((DIST_SUCCESS + 1))
        fi
    done
done

TESTS_TOTAL=$((TESTS_TOTAL + 1))
if [[ $DIST_SUCCESS -eq $DIST_TOTAL ]]; then
    success "All distributed documents replicated ($DIST_SUCCESS/$DIST_TOTAL)"
    TESTS_PASSED=$((TESTS_PASSED + 1))
elif [[ $DIST_SUCCESS -ge $((DIST_TOTAL * 80 / 100)) ]]; then
    warn "Most distributed documents replicated ($DIST_SUCCESS/$DIST_TOTAL)"
    TESTS_PASSED=$((TESTS_PASSED + 1))
else
    error "Distributed documents replication incomplete ($DIST_SUCCESS/$DIST_TOTAL)"
    TESTS_FAILED=$((TESTS_FAILED + 1))
fi

# ========================================
# Test 10: Delete Replication with Tempo
# ========================================
test_header "Test 10: Delete Replication"

# Create a namespace to delete
create_namespace $NODE1_ADMIN_PORT "to-delete-tempo" > /dev/null
sleep 2

# Delete from Node 2 (testing multi-master delete)
log "Deleting namespace 'to-delete-tempo' from Node 2..."
result=$(delete_namespace $NODE2_ADMIN_PORT "to-delete-tempo")
assert_contains "$result" '"success":true' "Namespace deleted from Node 2"

sleep 3

# Verify namespace is deleted on all nodes
log "Verifying namespace is deleted on all nodes..."
for node_num in 1 2 3; do
    port_var="NODE${node_num}_ADMIN_PORT"
    port=${!port_var}
    
    ns_result=$(get_namespace $port "to-delete-tempo")
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
# Test 11: Cluster Metrics
# ========================================
test_header "Test 11: Cluster Metrics"

for node_num in 1 2 3; do
    port_var="NODE${node_num}_ADMIN_PORT"
    port=${!port_var}
    
    metrics=$(curl -s --max-time $CURL_TIMEOUT "http://localhost:$port/api/v1/cluster/status")
    node_id=$(echo "$metrics" | jq -r '.data.node_id // .data.id')
    mode=$(echo "$metrics" | jq -r '.data.mode // "tempo"')
    
    TESTS_TOTAL=$((TESTS_TOTAL + 1))
    if [[ -n "$node_id" ]] && [[ "$node_id" != "null" ]]; then
        success "Node $node_num: Cluster metrics available (node_id: $node_id, mode: $mode)"
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
