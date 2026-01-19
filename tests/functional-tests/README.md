# DGate v2 Functional Tests

This directory contains functional tests to verify DGate v2's compatibility with various protocols.

## Prerequisites

- Rust toolchain (for building DGate and test servers)
- Node.js (for some test clients)
- Go (for gRPC tests - uses existing Go test servers)
- curl, wscat, grpcurl (command-line tools)

### Install test tools

```bash
# WebSocket client
npm install -g wscat

# gRPC client
brew install grpcurl

# HTTP/2 testing
brew install nghttp2  # provides nghttp and h2load
```

## Running Tests

```bash
# Run all tests
./run-all-tests.sh

# Run specific test suite
./http2/run-test.sh
./websocket/run-test.sh
./grpc/run-test.sh
./quic/run-test.sh
```

## Test Suites

### HTTP/2 Tests
- H2C (HTTP/2 cleartext) proxying
- HTTPS with HTTP/2 (ALPN negotiation)
- Server push (if supported)
- Stream multiplexing

### WebSocket Tests  
- WebSocket upgrade through proxy
- Bidirectional message passing
- Connection keep-alive
- Binary and text frames

### gRPC Tests
- Unary RPC through proxy
- Server streaming
- Client streaming
- Bidirectional streaming

### QUIC/HTTP3 Tests
- HTTP/3 upstream connections
- Connection migration
- 0-RTT resumption
