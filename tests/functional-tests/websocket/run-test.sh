#!/bin/bash
# WebSocket Functional Tests

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "$SCRIPT_DIR/../common/utils.sh"

test_header "WebSocket Functional Tests"

# Check for ws package
if ! node -e "require('ws')" 2>/dev/null; then
    log "Installing ws package..."
    npm install --prefix "$SCRIPT_DIR" ws
fi

# Cleanup on exit
trap cleanup_processes EXIT

# Build DGate
build_dgate

# Start WebSocket test server
log "Starting WebSocket test server..."
node "$SCRIPT_DIR/server.js" > /tmp/ws-server.log 2>&1 &
WS_PID=$!
sleep 2

if ! wait_for_port $TEST_SERVER_PORT 10; then
    error "WebSocket test server failed to start"
    exit 1
fi
success "WebSocket test server started"

# Start DGate
start_dgate "$SCRIPT_DIR/config.yaml"
sleep 1

# ========================================
# Test 1: Direct WebSocket connection to server
# ========================================
test_header "Test 1: Direct WebSocket Connection"

result=$(timeout 5 node "$SCRIPT_DIR/client.js" connect "ws://localhost:$TEST_SERVER_PORT/echo" 2>&1)
TESTS_TOTAL=$((TESTS_TOTAL + 1))
if [[ "$result" == *"welcome"* ]]; then
    success "Direct WebSocket connection works"
    TESTS_PASSED=$((TESTS_PASSED + 1))
else
    error "Direct WebSocket connection failed"
    echo "  Output: $result"
    TESTS_FAILED=$((TESTS_FAILED + 1))
fi

# ========================================
# Test 2: WebSocket through DGate proxy
# ========================================
test_header "Test 2: WebSocket Through Proxy"

result=$(timeout 5 node "$SCRIPT_DIR/client.js" connect "ws://localhost:$DGATE_PORT/ws/echo" 2>&1)
TESTS_TOTAL=$((TESTS_TOTAL + 1))
if [[ "$result" == *"welcome"* ]]; then
    success "WebSocket through proxy works"
    TESTS_PASSED=$((TESTS_PASSED + 1))
else
    error "WebSocket through proxy failed"
    echo "  Output: $result"
    TESTS_FAILED=$((TESTS_FAILED + 1))
fi

# ========================================
# Test 3: Ping/Pong through proxy
# ========================================
test_header "Test 3: Ping/Pong Through Proxy"

result=$(timeout 5 node "$SCRIPT_DIR/client.js" ping "ws://localhost:$DGATE_PORT/ws/echo" 2>&1)
TESTS_TOTAL=$((TESTS_TOTAL + 1))
if [[ "$result" == *"PASS"* ]]; then
    success "Ping/Pong through proxy works"
    TESTS_PASSED=$((TESTS_PASSED + 1))
else
    error "Ping/Pong through proxy failed"
    echo "  Output: $result"
    TESTS_FAILED=$((TESTS_FAILED + 1))
fi

# ========================================
# Test 4: Echo message through proxy
# ========================================
test_header "Test 4: Echo Message Through Proxy"

result=$(timeout 5 node "$SCRIPT_DIR/client.js" echo "ws://localhost:$DGATE_PORT/ws/echo" "Hello DGate!" 2>&1)
TESTS_TOTAL=$((TESTS_TOTAL + 1))
if [[ "$result" == *"PASS"* ]]; then
    success "Echo message through proxy works"
    TESTS_PASSED=$((TESTS_PASSED + 1))
else
    error "Echo message through proxy failed"
    echo "  Output: $result"
    TESTS_FAILED=$((TESTS_FAILED + 1))
fi

# ========================================
# Test 5: Binary message through proxy
# ========================================
test_header "Test 5: Binary Message Through Proxy"

result=$(timeout 5 node "$SCRIPT_DIR/client.js" binary "ws://localhost:$DGATE_PORT/ws/echo" 2>&1)
TESTS_TOTAL=$((TESTS_TOTAL + 1))
if [[ "$result" == *"PASS"* ]]; then
    success "Binary message through proxy works"
    TESTS_PASSED=$((TESTS_PASSED + 1))
else
    error "Binary message through proxy failed"
    echo "  Output: $result"
    TESTS_FAILED=$((TESTS_FAILED + 1))
fi

# ========================================
# Test 6: Multiple roundtrips
# ========================================
test_header "Test 6: Multiple Roundtrips (50 messages)"

result=$(timeout 30 node "$SCRIPT_DIR/client.js" roundtrip "ws://localhost:$DGATE_PORT/ws/echo" 50 2>&1)
TESTS_TOTAL=$((TESTS_TOTAL + 1))
if [[ "$result" == *"Average"* ]]; then
    success "Multiple roundtrips completed"
    echo "$result" | head -5
    TESTS_PASSED=$((TESTS_PASSED + 1))
else
    error "Multiple roundtrips failed"
    echo "  Output: $result"
    TESTS_FAILED=$((TESTS_FAILED + 1))
fi

# Print summary
print_summary
exit $?
