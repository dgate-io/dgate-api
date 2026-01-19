#!/bin/bash
# HTTP/2 Functional Tests

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "$SCRIPT_DIR/../common/utils.sh"

test_header "HTTP/2 Functional Tests"

# Cleanup on exit
trap cleanup_processes EXIT

# Build DGate
build_dgate

# Start HTTP/2 test server
log "Starting HTTP/2 test server..."
node "$SCRIPT_DIR/server.js" > /tmp/h2-server.log 2>&1 &
H2_PID=$!
sleep 2

if ! wait_for_port $TEST_SERVER_PORT 10; then
    error "HTTP/2 test server failed to start"
    exit 1
fi
success "HTTP/2 test server started"

# Start DGate
start_dgate "$SCRIPT_DIR/config.yaml"
sleep 1

# ========================================
# Test 1: Basic HTTP/1.1 proxy
# ========================================
test_header "Test 1: HTTP/1.1 Proxying"

response=$(curl -s "http://localhost:$DGATE_PORT/h2/test")
assert_contains "$response" '"method": "GET"' "GET request proxied correctly"
assert_contains "$response" '"path": "/"' "Path stripped correctly"

# ========================================
# Test 2: HTTP/2 with curl --http2
# ========================================
test_header "Test 2: HTTP/2 Request (curl --http2)"

# Note: curl --http2 will negotiate HTTP/2 if available
response=$(curl -s --http2 "http://localhost:$DGATE_PORT/h2/test")
assert_contains "$response" '"method": "GET"' "HTTP/2 GET request works"

# ========================================
# Test 3: POST with body
# ========================================
test_header "Test 3: POST Request with Body"

response=$(curl -s -X POST -d '{"name":"test"}' \
    -H "Content-Type: application/json" \
    "http://localhost:$DGATE_PORT/h2/data")
assert_contains "$response" '"method": "POST"' "POST method proxied"
assert_contains "$response" 'name' "POST body proxied"

# ========================================
# Test 4: Multiple concurrent requests
# ========================================
test_header "Test 4: Concurrent Requests"

# Send 10 concurrent requests with timeout
# Disable job control and error handling for background jobs
set +e +m

concurrent_pids=""
for i in 1 2 3 4 5 6 7 8 9 10; do
    curl -s --max-time 5 "http://localhost:$DGATE_PORT/h2/concurrent/$i" > /tmp/h2-req-$i.txt &
    concurrent_pids="$concurrent_pids $!"
done

# Wait for all curl processes
for pid in $concurrent_pids; do
    wait $pid 2>/dev/null
done

set -e

# Check all succeeded
all_passed=true
for i in 1 2 3 4 5 6 7 8 9 10; do
    if ! grep -q '"method": "GET"' /tmp/h2-req-$i.txt 2>/dev/null; then
        all_passed=false
        break
    fi
done

TESTS_TOTAL=$((TESTS_TOTAL + 1))
if $all_passed; then
    success "10 concurrent requests succeeded"
    TESTS_PASSED=$((TESTS_PASSED + 1))
else
    error "Some concurrent requests failed"
    TESTS_FAILED=$((TESTS_FAILED + 1))
fi

# ========================================
# Test 5: Large request body
# ========================================
test_header "Test 5: Large Request Body"

# Generate 100KB of data
large_body=$(head -c 102400 /dev/urandom | base64)
response=$(curl -s --max-time 30 -X POST -d "$large_body" \
    -H "Content-Type: text/plain" \
    "http://localhost:$DGATE_PORT/h2/large")

TESTS_TOTAL=$((TESTS_TOTAL + 1))
if [[ $(echo "$response" | jq -r '.method') == "POST" ]]; then
    success "Large body (100KB) proxied successfully"
    TESTS_PASSED=$((TESTS_PASSED + 1))
else
    error "Large body request failed"
    TESTS_FAILED=$((TESTS_FAILED + 1))
fi

# ========================================
# Test 6: Custom headers preserved
# ========================================
test_header "Test 6: Custom Headers"

response=$(curl -s -H "X-Custom-Header: test-value" \
    -H "X-Another-Header: another-value" \
    "http://localhost:$DGATE_PORT/h2/headers")

assert_contains "$response" '"x-custom-header": "test-value"' "Custom header preserved"
assert_contains "$response" '"x-another-header": "another-value"' "Another header preserved"

# Print summary
print_summary
exit $?
