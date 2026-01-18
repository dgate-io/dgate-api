#!/bin/bash
# Common utilities for functional tests

set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m'

# Logging
log() { echo -e "${BLUE}[INFO]${NC} $1"; }
success() { echo -e "${GREEN}[PASS]${NC} $1"; }
warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
error() { echo -e "${RED}[FAIL]${NC} $1"; }
test_header() { echo -e "\n${CYAN}=== $1 ===${NC}"; }

# Paths - compute from utils.sh location
_UTILS_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$_UTILS_DIR")"
ROOT_DIR="$(dirname "$PROJECT_DIR")"
DGATE_BIN="$ROOT_DIR/target/release/dgate-server"
DGATE_CLI="$ROOT_DIR/target/release/dgate-cli"

# Ports
export DGATE_PORT=${DGATE_PORT:-8080}
export DGATE_ADMIN_PORT=${DGATE_ADMIN_PORT:-9080}
export TEST_SERVER_PORT=${TEST_SERVER_PORT:-19999}

# Test counters
TESTS_PASSED=0
TESTS_FAILED=0
TESTS_TOTAL=0

# Cleanup function
cleanup_processes() {
    log "Cleaning up processes..."
    # Disable job control messages during cleanup
    set +m 2>/dev/null || true
    pkill -f "dgate-server" 2>/dev/null || true
    pkill -f "node.*server.js" 2>/dev/null || true
    pkill -f "greeter_server" 2>/dev/null || true
    # Wait for background jobs to finish and suppress their termination messages
    wait 2>/dev/null || true
    sleep 1
}

# Build DGate if needed
build_dgate() {
    if [[ ! -f "$DGATE_BIN" ]] || [[ "$1" == "--rebuild" ]]; then
        log "Building DGate..."
        cd "$ROOT_DIR"
        cargo build --release --bin dgate-server 2>&1 | tail -5
        success "DGate built"
    fi
}

# Start DGate with a config
start_dgate() {
    local config_file="$1"
    log "Starting DGate with config: $config_file"
    PORT=$DGATE_PORT "$DGATE_BIN" -c "$config_file" > /tmp/dgate-test.log 2>&1 &
    DGATE_PID=$!
    
    # Wait for startup
    for i in {1..20}; do
        if curl -s "http://localhost:$DGATE_ADMIN_PORT/health" > /dev/null 2>&1; then
            success "DGate started (PID: $DGATE_PID)"
            return 0
        fi
        sleep 0.5
    done
    
    error "DGate failed to start"
    cat /tmp/dgate-test.log
    return 1
}

# Wait for a port to be available
wait_for_port() {
    local port=$1
    local timeout=${2:-30}
    local elapsed=0
    
    while ! nc -z localhost $port 2>/dev/null; do
        sleep 0.5
        elapsed=$((elapsed + 1))
        if [[ $elapsed -ge $((timeout * 2)) ]]; then
            error "Timeout waiting for port $port"
            return 1
        fi
    done
    return 0
}

# Assert function
assert_eq() {
    local expected="$1"
    local actual="$2"
    local message="$3"
    
    TESTS_TOTAL=$((TESTS_TOTAL + 1))
    
    if [[ "$expected" == "$actual" ]]; then
        success "$message"
        TESTS_PASSED=$((TESTS_PASSED + 1))
        return 0
    else
        error "$message"
        echo "  Expected: $expected"
        echo "  Actual:   $actual"
        TESTS_FAILED=$((TESTS_FAILED + 1))
        return 1
    fi
}

assert_contains() {
    local haystack="$1"
    local needle="$2"
    local message="$3"
    
    TESTS_TOTAL=$((TESTS_TOTAL + 1))
    
    if [[ "$haystack" == *"$needle"* ]]; then
        success "$message"
        TESTS_PASSED=$((TESTS_PASSED + 1))
        return 0
    else
        error "$message"
        echo "  String does not contain: $needle"
        echo "  Actual: $haystack"
        TESTS_FAILED=$((TESTS_FAILED + 1))
        return 1
    fi
}

assert_status() {
    local expected_status="$1"
    local url="$2"
    local message="$3"
    
    TESTS_TOTAL=$((TESTS_TOTAL + 1))
    
    local actual_status=$(curl -s -o /dev/null -w "%{http_code}" "$url")
    
    if [[ "$expected_status" == "$actual_status" ]]; then
        success "$message (status: $actual_status)"
        TESTS_PASSED=$((TESTS_PASSED + 1))
        return 0
    else
        error "$message"
        echo "  Expected status: $expected_status"
        echo "  Actual status:   $actual_status"
        TESTS_FAILED=$((TESTS_FAILED + 1))
        return 1
    fi
}

# Print test summary
print_summary() {
    echo ""
    echo "========================================="
    echo "  TEST SUMMARY"
    echo "========================================="
    echo "  Total:  $TESTS_TOTAL"
    echo -e "  Passed: ${GREEN}$TESTS_PASSED${NC}"
    echo -e "  Failed: ${RED}$TESTS_FAILED${NC}"
    echo "========================================="
    
    if [[ $TESTS_FAILED -gt 0 ]]; then
        return 1
    fi
    return 0
}
