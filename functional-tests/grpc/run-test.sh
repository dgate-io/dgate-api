#!/bin/bash
# gRPC Functional Tests

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "$SCRIPT_DIR/../common/utils.sh"

export TEST_SERVER_PORT=50051

test_header "gRPC Functional Tests"

# Check for required packages
check_npm_package() {
    if ! node -e "require('$1')" 2>/dev/null; then
        log "Installing $1 package..."
        npm install --prefix "$SCRIPT_DIR" "$1"
    fi
}

check_npm_package "@grpc/grpc-js"
check_npm_package "@grpc/proto-loader"

# Cleanup on exit
trap cleanup_processes EXIT

# Build DGate
build_dgate

# Start gRPC test server
log "Starting gRPC test server..."
node "$SCRIPT_DIR/server.js" > /tmp/grpc-server.log 2>&1 &
GRPC_PID=$!
sleep 2

if ! wait_for_port $TEST_SERVER_PORT 10; then
    error "gRPC test server failed to start"
    cat /tmp/grpc-server.log
    exit 1
fi
success "gRPC test server started"

# ========================================
# Test 1: Direct gRPC unary call
# ========================================
test_header "Test 1: Direct gRPC Unary Call"

result=$(timeout 10 node "$SCRIPT_DIR/client.js" unary "localhost:$TEST_SERVER_PORT" "DirectTest" 2>&1)
TESTS_TOTAL=$((TESTS_TOTAL + 1))
if [[ "$result" == *"PASS"* ]]; then
    success "Direct gRPC unary call works"
    TESTS_PASSED=$((TESTS_PASSED + 1))
else
    error "Direct gRPC unary call failed"
    echo "  Output: $result"
    TESTS_FAILED=$((TESTS_FAILED + 1))
fi

# Start DGate for proxy tests
# Note: gRPC proxying requires HTTP/2 support and proper handling of gRPC trailers
start_dgate "$SCRIPT_DIR/config.yaml"
sleep 1

# ========================================
# Test 2: gRPC through DGate proxy (unary)
# ========================================
test_header "Test 2: gRPC Through Proxy (Unary)"

result=$(timeout 10 node "$SCRIPT_DIR/client.js" unary "localhost:$DGATE_PORT" "ProxyTest" 2>&1)
TESTS_TOTAL=$((TESTS_TOTAL + 1))
if [[ "$result" == *"PASS"* ]]; then
    success "gRPC unary through proxy works"
    TESTS_PASSED=$((TESTS_PASSED + 1))
else
    warn "gRPC unary through proxy failed (may need HTTP/2 trailer support)"
    echo "  Output: $result"
    TESTS_FAILED=$((TESTS_FAILED + 1))
fi

# ========================================
# Test 3: gRPC server streaming through proxy
# ========================================
test_header "Test 3: gRPC Server Streaming Through Proxy"

result=$(timeout 10 node "$SCRIPT_DIR/client.js" server-stream "localhost:$DGATE_PORT" "StreamTest" 2>&1)
TESTS_TOTAL=$((TESTS_TOTAL + 1))
if [[ "$result" == *"PASS"* ]]; then
    success "gRPC server streaming through proxy works"
    TESTS_PASSED=$((TESTS_PASSED + 1))
else
    warn "gRPC server streaming through proxy failed"
    echo "  Output: $result"
    TESTS_FAILED=$((TESTS_FAILED + 1))
fi

# ========================================
# Test 4: gRPC client streaming through proxy
# ========================================
test_header "Test 4: gRPC Client Streaming Through Proxy"

result=$(timeout 10 node "$SCRIPT_DIR/client.js" client-stream "localhost:$DGATE_PORT" "Alice,Bob,Charlie" 2>&1)
TESTS_TOTAL=$((TESTS_TOTAL + 1))
if [[ "$result" == *"PASS"* ]]; then
    success "gRPC client streaming through proxy works"
    TESTS_PASSED=$((TESTS_PASSED + 1))
else
    warn "gRPC client streaming through proxy failed"
    echo "  Output: $result"
    TESTS_FAILED=$((TESTS_FAILED + 1))
fi

# ========================================
# Test 5: gRPC bidirectional streaming through proxy
# ========================================
test_header "Test 5: gRPC Bidirectional Streaming Through Proxy"

result=$(timeout 10 node "$SCRIPT_DIR/client.js" bidi-stream "localhost:$DGATE_PORT" "Alice,Bob" 2>&1)
TESTS_TOTAL=$((TESTS_TOTAL + 1))
if [[ "$result" == *"PASS"* ]]; then
    success "gRPC bidirectional streaming through proxy works"
    TESTS_PASSED=$((TESTS_PASSED + 1))
else
    warn "gRPC bidirectional streaming through proxy failed"
    echo "  Output: $result"
    TESTS_FAILED=$((TESTS_FAILED + 1))
fi

# Print summary
echo ""
echo "Note: gRPC proxy support requires proper HTTP/2 trailer handling."
echo "Some tests may fail if the proxy doesn't fully support gRPC semantics."

print_summary
exit $?
