#!/bin/bash
# Run all DGate v2 Functional Tests

set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "$SCRIPT_DIR/common/utils.sh"

echo ""
echo "╔═══════════════════════════════════════════════════════════╗"
echo "║          DGate v2 Functional Test Suite                   ║"
echo "╚═══════════════════════════════════════════════════════════╝"
echo ""

# Track overall results
TOTAL_SUITES=0
PASSED_SUITES=0
FAILED_SUITES=0

run_suite() {
    local suite_name="$1"
    local suite_dir="$2"
    
    echo ""
    echo "┌─────────────────────────────────────────────────────────────┐"
    echo "│  Running: $suite_name"
    echo "└─────────────────────────────────────────────────────────────┘"
    
    TOTAL_SUITES=$((TOTAL_SUITES + 1))
    
    if [[ -f "$suite_dir/run-test.sh" ]]; then
        chmod +x "$suite_dir/run-test.sh"
        
        if "$suite_dir/run-test.sh"; then
            PASSED_SUITES=$((PASSED_SUITES + 1))
            echo -e "\n${GREEN}✓ $suite_name: PASSED${NC}\n"
        else
            FAILED_SUITES=$((FAILED_SUITES + 1))
            echo -e "\n${RED}✗ $suite_name: FAILED${NC}\n"
        fi
    else
        echo -e "${YELLOW}⚠ $suite_name: No test script found${NC}"
        FAILED_SUITES=$((FAILED_SUITES + 1))
    fi
    
    # Cleanup between suites
    cleanup_processes
    sleep 1
}

# Parse arguments
SUITES_TO_RUN=()
if [[ $# -gt 0 ]]; then
    SUITES_TO_RUN=("$@")
else
    # Default: run all test suites
    SUITES_TO_RUN=("modules" "http2" "websocket" "grpc" "quic" "cluster-simple" "cluster-raft" "cluster-tempo")
fi

# Run selected suites
for suite in "${SUITES_TO_RUN[@]}"; do
    case "$suite" in
        modules|js|ts)
            run_suite "JavaScript/TypeScript Module Tests" "$SCRIPT_DIR/modules"
            ;;
        http2)
            run_suite "HTTP/2 Tests" "$SCRIPT_DIR/http2"
            ;;
        websocket|ws)
            run_suite "WebSocket Tests" "$SCRIPT_DIR/websocket"
            ;;
        grpc)
            run_suite "gRPC Tests" "$SCRIPT_DIR/grpc"
            ;;
        quic|http3)
            run_suite "QUIC/HTTP3 Tests" "$SCRIPT_DIR/quic"
            ;;
        # Cluster tests - organized by mode
        cluster-simple|simple-replication|simple)
            run_suite "Cluster: Simple HTTP Replication Tests" "$SCRIPT_DIR/cluster/simple"
            ;;
        cluster-raft|raft-consensus|raft)
            run_suite "Cluster: Raft Consensus Tests" "$SCRIPT_DIR/cluster/raft"
            ;;
        cluster-tempo|tempo-consensus|tempo)
            run_suite "Cluster: Tempo Multi-Master Consensus Tests" "$SCRIPT_DIR/cluster/tempo"
            ;;
        # Run all cluster tests
        cluster|cluster-all)
            run_suite "Cluster: Simple HTTP Replication Tests" "$SCRIPT_DIR/cluster/simple"
            run_suite "Cluster: Raft Consensus Tests" "$SCRIPT_DIR/cluster/raft"
            run_suite "Cluster: Tempo Multi-Master Consensus Tests" "$SCRIPT_DIR/cluster/tempo"
            ;;
        # Legacy aliases (for backwards compatibility)
        simple-replication-legacy)
            run_suite "Simple HTTP Replication Tests (Legacy)" "$SCRIPT_DIR/simple-replication"
            ;;
        raft-consensus-legacy)
            run_suite "Raft Consensus Tests (Legacy)" "$SCRIPT_DIR/raft-consensus"
            ;;
        *)
            echo "Unknown test suite: $suite"
            echo ""
            echo "Available test suites:"
            echo "  Core:"
            echo "    modules        - JavaScript/TypeScript module tests"
            echo "    http2          - HTTP/2 protocol tests"
            echo "    websocket      - WebSocket protocol tests"
            echo "    grpc           - gRPC protocol tests"
            echo "    quic           - QUIC/HTTP3 protocol tests"
            echo ""
            echo "  Cluster modes:"
            echo "    cluster-simple - Simple HTTP replication (multi-master)"
            echo "    cluster-raft   - Raft consensus (leader-based)"
            echo "    cluster-tempo  - Tempo consensus (multi-master, quorum-based)"
            echo "    cluster        - Run all cluster tests"
            echo ""
            echo "  Aliases:"
            echo "    simple, raft, tempo - shortcuts for cluster tests"
            ;;
    esac
done

# Print overall summary
echo ""
echo "╔═══════════════════════════════════════════════════════════╗"
echo "║                    OVERALL SUMMARY                        ║"
echo "╠═══════════════════════════════════════════════════════════╣"
echo "║  Test Suites Run:    $TOTAL_SUITES"
echo -e "║  ${GREEN}Passed:              $PASSED_SUITES${NC}"
echo -e "║  ${RED}Failed:              $FAILED_SUITES${NC}"
echo "╚═══════════════════════════════════════════════════════════╝"
echo ""

if [[ $FAILED_SUITES -gt 0 ]]; then
    exit 1
fi
exit 0
