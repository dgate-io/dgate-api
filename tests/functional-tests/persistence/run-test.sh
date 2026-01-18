#!/bin/bash
# Persistence Functional Tests
# Tests that resources are properly saved to disk and reloaded after server restart

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "$SCRIPT_DIR/../common/utils.sh"

test_header "Persistence Functional Tests (Standalone Node)"

# Cleanup on exit
cleanup() {
    log "Cleaning up..."
    [[ -n "$DGATE_PID" ]] && kill $DGATE_PID 2>/dev/null || true
    rm -rf "$DATA_DIR" 2>/dev/null || true
    rm -f /tmp/dgate-persistence-*.log 2>/dev/null || true
    cleanup_processes
}
trap cleanup EXIT

# Test configuration
DATA_DIR="/tmp/dgate-persistence-test-$$"
CONFIG_FILE="/tmp/dgate-persistence-$$.yaml"
ADMIN_PORT=9180
PROXY_PORT=8180

# Build DGate
build_dgate

# Generate config with file-based storage
generate_config() {
    cat > "$CONFIG_FILE" << EOF
version: v1
log_level: debug
debug: true

storage:
  type: file
  dir: ${DATA_DIR}

proxy:
  port: ${PROXY_PORT}
  host: 0.0.0.0

admin:
  port: ${ADMIN_PORT}
  host: 0.0.0.0
EOF
}

# Start the server
start_server() {
    log "Starting DGate server..."
    "$DGATE_BIN" -c "$CONFIG_FILE" > /tmp/dgate-persistence-$$.log 2>&1 &
    DGATE_PID=$!
    
    # Wait for startup
    for i in {1..30}; do
        if curl -s "http://localhost:$ADMIN_PORT/health" > /dev/null 2>&1; then
            success "DGate started (PID: $DGATE_PID)"
            return 0
        fi
        sleep 0.5
    done
    
    error "DGate failed to start"
    cat /tmp/dgate-persistence-$$.log
    return 1
}

# Stop the server gracefully
stop_server() {
    if [[ -n "$DGATE_PID" ]]; then
        log "Stopping DGate server (PID: $DGATE_PID)..."
        kill $DGATE_PID 2>/dev/null || true
        # Wait for it to fully stop
        for i in {1..20}; do
            if ! kill -0 $DGATE_PID 2>/dev/null; then
                success "DGate stopped"
                DGATE_PID=""
                return 0
            fi
            sleep 0.5
        done
        # Force kill if still running
        kill -9 $DGATE_PID 2>/dev/null || true
        DGATE_PID=""
    fi
}

# ========================================
# Setup
# ========================================
log "Creating data directory: $DATA_DIR"
mkdir -p "$DATA_DIR"

log "Generating config file..."
generate_config

# ========================================
# Phase 1: Create resources
# ========================================
test_header "Phase 1: Create Resources"

start_server
sleep 1

# Create namespace
log "Creating namespace 'test-ns'..."
ns_response=$(curl -s -X PUT "http://localhost:$ADMIN_PORT/api/v1/namespace/test-ns" \
    -H "Content-Type: application/json" \
    -d '{"name": "test-ns", "tags": ["persistence", "test"]}')
assert_contains "$ns_response" '"success":true' "Namespace 'test-ns' created"

# Create service
log "Creating service 'test-svc'..."
svc_response=$(curl -s -X PUT "http://localhost:$ADMIN_PORT/api/v1/service/test-ns/test-svc" \
    -H "Content-Type: application/json" \
    -d '{"name": "test-svc", "namespace": "test-ns", "urls": ["http://localhost:8000"]}')
assert_contains "$svc_response" '"success":true' "Service 'test-svc' created"

# Create route
log "Creating route 'test-route'..."
route_response=$(curl -s -X PUT "http://localhost:$ADMIN_PORT/api/v1/route/test-ns/test-route" \
    -H "Content-Type: application/json" \
    -d '{"name": "test-route", "namespace": "test-ns", "paths": ["/api/test"], "methods": ["GET", "POST"], "service": "test-svc", "strip_path": true}')
assert_contains "$route_response" '"success":true' "Route 'test-route' created"

# Create collection
log "Creating collection 'test-collection'..."
col_response=$(curl -s -X PUT "http://localhost:$ADMIN_PORT/api/v1/collection/test-ns/test-collection" \
    -H "Content-Type: application/json" \
    -d '{"name": "test-collection", "namespace": "test-ns", "schema": {"type": "object"}}')
assert_contains "$col_response" '"success":true' "Collection 'test-collection' created"

# Create documents
log "Creating documents..."
for i in 1 2 3; do
    doc_response=$(curl -s -X PUT "http://localhost:$ADMIN_PORT/api/v1/document/test-ns/test-collection/doc-$i" \
        -H "Content-Type: application/json" \
        -d "{\"id\": \"doc-$i\", \"namespace\": \"test-ns\", \"collection\": \"test-collection\", \"data\": {\"index\": $i, \"name\": \"Document $i\"}}")
    assert_contains "$doc_response" '"success":true' "Document 'doc-$i' created"
done

# Create secret
log "Creating secret 'test-secret'..."
secret_response=$(curl -s -X PUT "http://localhost:$ADMIN_PORT/api/v1/secret/test-ns/test-secret" \
    -H "Content-Type: application/json" \
    -d '{"name": "test-secret", "namespace": "test-ns", "data": "supersecretvalue123"}')
assert_contains "$secret_response" '"success":true' "Secret 'test-secret' created"

# Create domain
log "Creating domain 'test-domain'..."
domain_response=$(curl -s -X PUT "http://localhost:$ADMIN_PORT/api/v1/domain/test-ns/test-domain" \
    -H "Content-Type: application/json" \
    -d '{"name": "test-domain", "namespace": "test-ns", "patterns": ["*.example.com"]}')
assert_contains "$domain_response" '"success":true' "Domain 'test-domain' created"

# Verify all resources exist before restart
test_header "Verify Resources Before Restart"

ns_check=$(curl -s "http://localhost:$ADMIN_PORT/api/v1/namespace/test-ns")
assert_contains "$ns_check" '"name":"test-ns"' "Namespace exists before restart"

svc_check=$(curl -s "http://localhost:$ADMIN_PORT/api/v1/service/test-ns/test-svc")
assert_contains "$svc_check" '"name":"test-svc"' "Service exists before restart"

route_check=$(curl -s "http://localhost:$ADMIN_PORT/api/v1/route/test-ns/test-route")
assert_contains "$route_check" '"name":"test-route"' "Route exists before restart"

col_check=$(curl -s "http://localhost:$ADMIN_PORT/api/v1/collection/test-ns/test-collection")
assert_contains "$col_check" '"name":"test-collection"' "Collection exists before restart"

for i in 1 2 3; do
    doc_check=$(curl -s "http://localhost:$ADMIN_PORT/api/v1/document/test-ns/test-collection/doc-$i")
    assert_contains "$doc_check" "\"index\":$i" "Document 'doc-$i' exists before restart"
done

# ========================================
# Phase 2: Restart server
# ========================================
test_header "Phase 2: Restart Server"

stop_server
sleep 2

# Verify data directory exists and has data
if [[ -d "$DATA_DIR" ]] && [[ "$(ls -A $DATA_DIR 2>/dev/null)" ]]; then
    success "Data directory contains persisted data"
    TESTS_TOTAL=$((TESTS_TOTAL + 1))
    TESTS_PASSED=$((TESTS_PASSED + 1))
else
    error "Data directory is empty or missing"
    TESTS_TOTAL=$((TESTS_TOTAL + 1))
    TESTS_FAILED=$((TESTS_FAILED + 1))
fi

log "Restarting server with same data directory..."
start_server
sleep 2

# ========================================
# Phase 3: Verify resources after restart
# ========================================
test_header "Phase 3: Verify Resources After Restart"

# Check namespace
ns_after=$(curl -s "http://localhost:$ADMIN_PORT/api/v1/namespace/test-ns")
assert_contains "$ns_after" '"name":"test-ns"' "Namespace 'test-ns' persisted after restart"
assert_contains "$ns_after" '"persistence"' "Namespace tags persisted"

# Check service
svc_after=$(curl -s "http://localhost:$ADMIN_PORT/api/v1/service/test-ns/test-svc")
assert_contains "$svc_after" '"name":"test-svc"' "Service 'test-svc' persisted after restart"
assert_contains "$svc_after" '"http://localhost:8000"' "Service URLs persisted"

# Check route
route_after=$(curl -s "http://localhost:$ADMIN_PORT/api/v1/route/test-ns/test-route")
assert_contains "$route_after" '"name":"test-route"' "Route 'test-route' persisted after restart"
assert_contains "$route_after" '"/api/test"' "Route paths persisted"
assert_contains "$route_after" '"service":"test-svc"' "Route service reference persisted"

# Check collection
col_after=$(curl -s "http://localhost:$ADMIN_PORT/api/v1/collection/test-ns/test-collection")
assert_contains "$col_after" '"name":"test-collection"' "Collection 'test-collection' persisted after restart"

# Check documents
for i in 1 2 3; do
    doc_after=$(curl -s "http://localhost:$ADMIN_PORT/api/v1/document/test-ns/test-collection/doc-$i")
    assert_contains "$doc_after" "\"id\":\"doc-$i\"" "Document 'doc-$i' persisted after restart"
    assert_contains "$doc_after" "\"index\":$i" "Document 'doc-$i' data persisted correctly"
    assert_contains "$doc_after" "\"name\":\"Document $i\"" "Document 'doc-$i' name field persisted"
done

# Check secret (note: data will be redacted in response)
secret_after=$(curl -s "http://localhost:$ADMIN_PORT/api/v1/secret/test-ns/test-secret")
assert_contains "$secret_after" '"name":"test-secret"' "Secret 'test-secret' persisted after restart"

# Check domain
domain_after=$(curl -s "http://localhost:$ADMIN_PORT/api/v1/domain/test-ns/test-domain")
assert_contains "$domain_after" '"name":"test-domain"' "Domain 'test-domain' persisted after restart"
assert_contains "$domain_after" '"*.example.com"' "Domain patterns persisted"

# ========================================
# Phase 4: Modify resources and verify persistence
# ========================================
test_header "Phase 4: Modify Resources and Verify"

# Update an existing document
log "Updating document 'doc-1'..."
update_response=$(curl -s -X PUT "http://localhost:$ADMIN_PORT/api/v1/document/test-ns/test-collection/doc-1" \
    -H "Content-Type: application/json" \
    -d '{"id": "doc-1", "namespace": "test-ns", "collection": "test-collection", "data": {"index": 1, "name": "Updated Document 1", "modified": true}}')
assert_contains "$update_response" '"success":true' "Document 'doc-1' updated"

# Delete a document
log "Deleting document 'doc-3'..."
delete_response=$(curl -s -X DELETE "http://localhost:$ADMIN_PORT/api/v1/document/test-ns/test-collection/doc-3")
assert_contains "$delete_response" '"success":true' "Document 'doc-3' deleted"

# Verify deletion
doc3_check=$(curl -s "http://localhost:$ADMIN_PORT/api/v1/document/test-ns/test-collection/doc-3")
assert_contains "$doc3_check" '"error"' "Document 'doc-3' no longer exists"

# Create a new namespace
log "Creating namespace 'new-ns'..."
new_ns_response=$(curl -s -X PUT "http://localhost:$ADMIN_PORT/api/v1/namespace/new-ns" \
    -H "Content-Type: application/json" \
    -d '{"name": "new-ns", "tags": ["new"]}')
assert_contains "$new_ns_response" '"success":true' "Namespace 'new-ns' created"

# Verify update was applied before restart
log "Verifying update was applied before restart..."
doc1_verify=$(curl -s "http://localhost:$ADMIN_PORT/api/v1/document/test-ns/test-collection/doc-1")
assert_contains "$doc1_verify" '"modified":true' "Document 'doc-1' update visible before restart"

# Give the database time to sync writes to disk
sleep 1

# Restart again to verify modifications persisted
test_header "Phase 5: Verify Modifications After Second Restart"

stop_server
sleep 2

log "Restarting server again..."
start_server
sleep 2

# Verify updated document
doc1_final=$(curl -s "http://localhost:$ADMIN_PORT/api/v1/document/test-ns/test-collection/doc-1")
assert_contains "$doc1_final" '"modified":true' "Document 'doc-1' update persisted"
assert_contains "$doc1_final" '"Updated Document 1"' "Document 'doc-1' name update persisted"

# Verify deleted document stays deleted
doc3_final=$(curl -s "http://localhost:$ADMIN_PORT/api/v1/document/test-ns/test-collection/doc-3")
assert_contains "$doc3_final" '"error"' "Document 'doc-3' deletion persisted"

# Verify document 2 still exists
doc2_final=$(curl -s "http://localhost:$ADMIN_PORT/api/v1/document/test-ns/test-collection/doc-2")
assert_contains "$doc2_final" '"id":"doc-2"' "Document 'doc-2' still exists after restart"

# Verify new namespace
new_ns_final=$(curl -s "http://localhost:$ADMIN_PORT/api/v1/namespace/new-ns")
assert_contains "$new_ns_final" '"name":"new-ns"' "Namespace 'new-ns' persisted after restart"

# Verify original namespace still exists
orig_ns_final=$(curl -s "http://localhost:$ADMIN_PORT/api/v1/namespace/test-ns")
assert_contains "$orig_ns_final" '"name":"test-ns"' "Original namespace 'test-ns' still exists"

# ========================================
# Summary
# ========================================
print_summary
exit $?
