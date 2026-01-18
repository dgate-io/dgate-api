#!/bin/bash
#
# DGate v2 Performance Test Runner
#
# This script starts all necessary servers and runs performance tests.
#
# Usage:
#   ./run-tests.sh quick     # Quick 30-second test
#   ./run-tests.sh proxy     # Proxy performance test
#   ./run-tests.sh modules   # JS modules performance test
#   ./run-tests.sh admin     # Admin API performance test
#   ./run-tests.sh all       # Run all tests
#   ./run-tests.sh baseline  # Compare direct vs proxy

set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
DGATE_BIN="$PROJECT_DIR/target/release/dgate-server"
ECHO_BIN="$PROJECT_DIR/target/release/echo-server"
DGATE_PORT=8080
ADMIN_PORT=9080
ECHO_PORT=9999

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

log() { echo -e "${BLUE}[INFO]${NC} $1"; }
success() { echo -e "${GREEN}[OK]${NC} $1"; }
warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
error() { echo -e "${RED}[ERROR]${NC} $1"; }

cleanup() {
    log "Cleaning up..."
    pkill -f "dgate-server" 2>/dev/null || true
    pkill -f "echo-server" 2>/dev/null || true
    # Kill any process on our ports
    lsof -ti:$DGATE_PORT 2>/dev/null | xargs kill 2>/dev/null || true
    lsof -ti:$ADMIN_PORT 2>/dev/null | xargs kill 2>/dev/null || true
    lsof -ti:$ECHO_PORT 2>/dev/null | xargs kill 2>/dev/null || true
    sleep 1
}

check_k6() {
    if ! command -v k6 &> /dev/null; then
        error "k6 is not installed. Install it with:"
        echo "  brew install k6"
        echo "  or: https://k6.io/docs/getting-started/installation/"
        exit 1
    fi
    success "k6 found"
}

build_all() {
    log "Building DGate and Echo Server..."
    cd "$PROJECT_DIR"
    cargo build --release --bin dgate-server --bin dgate-cli 2>&1 | tail -5
    cargo build --release --bin echo-server --features perf-tools 2>&1 | tail -3
    success "Binaries built"
}

start_echo_server() {
    log "Starting Rust echo server on port $ECHO_PORT..."
    
    "$ECHO_BIN" --port $ECHO_PORT > /tmp/echo-server.log 2>&1 &
    
    # Wait for startup
    for i in {1..10}; do
        if curl -s "http://localhost:$ECHO_PORT/health" > /dev/null 2>&1; then
            success "Echo server started (Rust)"
            return 0
        fi
        sleep 0.2
    done
    
    warn "Echo server may not have started properly. Check /tmp/echo-server.log"
}

start_dgate() {
    log "Starting DGate..."
    cd "$PROJECT_DIR"
    PORT=$DGATE_PORT "$DGATE_BIN" -c "$SCRIPT_DIR/config.perf.yaml" > /tmp/dgate.log 2>&1 &
    
    # Wait for startup
    for i in {1..10}; do
        if curl -s "http://localhost:$DGATE_PORT/module/simple" > /dev/null 2>&1; then
            success "DGate started"
            return 0
        fi
        sleep 0.5
    done
    
    error "DGate failed to start. Check /tmp/dgate.log"
    cat /tmp/dgate.log
    exit 1
}

run_quick_test() {
    log "Running quick test..."
    cd "$SCRIPT_DIR"
    k6 run quick-test.js
}

run_proxy_test() {
    log "Running proxy performance test..."
    cd "$SCRIPT_DIR"
    k6 run --out json=proxy-results.json test-proxy.js
}

run_modules_test() {
    log "Running JS modules performance test..."
    cd "$SCRIPT_DIR"
    k6 run --out json=modules-results.json test-modules.js
}

run_admin_test() {
    log "Running admin API performance test..."
    cd "$SCRIPT_DIR"
    k6 run --out json=admin-results.json test-admin.js
}

run_baseline_comparison() {
    log "Running baseline comparison (direct vs proxy)..."
    cd "$SCRIPT_DIR"
    
    echo ""
    echo "========== DIRECT ECHO SERVER (Rust) =========="
    k6 run -e TARGET=direct --vus 50 --duration 15s test-proxy.js 2>/dev/null
    
    echo ""
    echo "========== VIA DGATE PROXY =========="
    k6 run -e TARGET=proxy --vus 50 --duration 15s test-proxy.js 2>/dev/null
}

# Main
trap cleanup EXIT

case "${1:-quick}" in
    quick)
        check_k6
        build_all
        cleanup
        start_echo_server
        start_dgate
        run_quick_test
        ;;
    proxy)
        check_k6
        build_all
        cleanup
        start_echo_server
        start_dgate
        run_proxy_test
        ;;
    modules)
        check_k6
        build_all
        cleanup
        start_dgate
        run_modules_test
        ;;
    admin)
        check_k6
        build_all
        cleanup
        start_dgate
        run_admin_test
        ;;
    baseline)
        check_k6
        build_all
        cleanup
        start_echo_server
        start_dgate
        run_baseline_comparison
        ;;
    all)
        check_k6
        build_all
        cleanup
        start_echo_server
        start_dgate
        echo ""
        echo "========================================="
        echo "  RUNNING ALL PERFORMANCE TESTS"
        echo "========================================="
        echo ""
        run_quick_test
        echo ""
        run_proxy_test
        echo ""
        run_modules_test
        echo ""
        run_admin_test
        ;;
    *)
        echo "Usage: $0 {quick|proxy|modules|admin|baseline|all}"
        exit 1
        ;;
esac

echo ""
success "Performance tests completed!"
