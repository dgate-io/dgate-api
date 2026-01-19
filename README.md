# DGate v2 - Rust Edition

A high-performance API Gateway written in Rust, featuring JavaScript module support via QuickJS engine.

[![CI](https://github.com/dgate-io/dgate/actions/workflows/ci.yml/badge.svg)](https://github.com/dgate-io/dgate/actions/workflows/ci.yml)
[![Crates.io](https://img.shields.io/crates/v/dgate.svg)](https://crates.io/crates/dgate)
[![Docker](https://img.shields.io/badge/docker-ghcr.io-blue)](https://github.com/dgate-io/dgate/pkgs/container/dgate)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

## Features

- **High Performance**: Built with Rust and async I/O using Tokio
- **Dynamic Routing**: Configure routes, services, and domains dynamically via Admin API
- **Namespace Isolation**: Organize resources into namespaces for multi-tenancy
- **Simple KV Storage**: Document storage without JSON schema requirements
- **TLS Support**: HTTPS with dynamic certificate loading per domain
- **Reverse Proxy**: Forward requests to upstream services with load balancing support

## Supported

- HTTP/1.1 ✅
- HTTP/2 ✅
- WebSocket ✅
- gRPC ✅
- QUIC/HTTP3 ❌

## Resource Diagram

```
┌─────────────────────────────────────────────────────────────┐
│                        DGate V2                              │
├─────────────────────────────────────────────────────────────┤
│                                                              │
│  ┌─────────────┐    ┌─────────────┐    ┌─────────────┐     │
│  │   Domains   │───▶│  Namespaces │───▶│   Routes    │     │
│  └─────────────┘    └─────────────┘    └─────────────┘     │
│                                              │               │
│                                              ▼               │
│  ┌─────────────┐    ┌─────────────┐    ┌─────────────┐     │
│  │   Modules   │◀───│   Handler   │───▶│  Services   │     │
│  │  (QuickJS)  │    └─────────────┘    │  (Upstream) │     │
│  └─────────────┘                       └─────────────┘     │
│                                                              │
│  ┌─────────────┐    ┌─────────────┐    ┌─────────────┐     │
│  │  Documents  │    │   Secrets   │    │ Collections │     │
│  │    (KV)     │    └─────────────┘    └─────────────┘     │
│  └─────────────┘                                            │
│                                                              │
└─────────────────────────────────────────────────────────────┘
```

## Installation

### From Cargo (crates.io)

```bash
cargo install dgate
```

### From Source

```bash
git clone https://github.com/dgate-io/dgate.git
cd dgate
cargo build --release
```

### Docker

```bash
# Pull from GitHub Container Registry
docker pull ghcr.io/dgate-io/dgate:latest

# Run with a config file
docker run -d \
  -p 80:80 -p 443:443 -p 9080:9080 \
  -v $(pwd)/config.yaml:/app/config.yaml \
  ghcr.io/dgate-io/dgate:latest
```

## Quick Start

### Run

```bash
# With default config
./target/release/dgate-server

# With custom config
./target/release/dgate-server -c config.dgate.yaml
```

### Configuration

Create a `config.dgate.yaml` file:

```yaml
version: v1
log_level: info

storage:
  type: file
  dir: .dgate/data/

proxy:
  port: 80
  host: 0.0.0.0

admin:
  port: 9080
  host: 0.0.0.0
```

## Resources

### Namespaces

Namespaces organize resources and provide multi-tenancy:

```bash
# Create a namespace
curl -X PUT http://localhost:9080/api/v1/namespace/myapp \
  -H "Content-Type: application/json" \
  -d '{"tags": ["production"]}'
```

### Services

Services define upstream endpoints:

```bash
curl -X PUT http://localhost:9080/api/v1/service/myapp/backend \
  -H "Content-Type: application/json" \
  -d '{
    "urls": ["http://backend-1:8080", "http://backend-2:8080"],
    "request_timeout_ms": 30000,
    "retries": 3
  }'
```

### Routes

Routes match incoming requests to services:

```bash
curl -X PUT http://localhost:9080/api/v1/route/myapp/api-route \
  -H "Content-Type: application/json" \
  -d '{
    "paths": ["/api/**"],
    "methods": ["GET", "POST", "PUT", "DELETE"],
    "service": "backend",
    "strip_path": true
  }'
```

### Modules

JavaScript modules for request/response processing:

```bash
# Create a module
curl -X PUT http://localhost:9080/api/v1/module/myapp/auth-check \
  -H "Content-Type: application/json" \
  -d '{
    "moduleType": "javascript",
    "payload": "BASE64_ENCODED_JAVASCRIPT"
  }'
```

**Module Functions:**

```javascript
// Modify request before proxying
function requestModifier(ctx) {
  ctx.request.headers['X-Custom'] = 'value';
}

// Handle request without upstream (serverless-style)
function requestHandler(ctx) {
  ctx.setStatus(200);
  ctx.json({ message: 'Hello from DGate!' });
}

// Modify response after upstream
function responseModifier(ctx, res) {
  res.headers['X-Processed'] = 'true';
}

// Handle errors
function errorHandler(ctx, error) {
  ctx.setStatus(500);
  ctx.json({ error: error.message });
}

// Custom upstream URL selection
function fetchUpstreamUrl(ctx) {
  return 'http://custom-backend:8080';
}
```

### Domains

Control ingress traffic routing:

```bash
curl -X PUT http://localhost:9080/api/v1/domain/myapp/main \
  -H "Content-Type: application/json" \
  -d '{
    "patterns": ["*.myapp.com", "myapp.local"],
    "priority": 10
  }'
```

### Collections & Documents

Simple KV storage:

```bash
# Create a collection
curl -X PUT http://localhost:9080/api/v1/collection/myapp/users \
  -H "Content-Type: application/json" \
  -d '{"visibility": "private"}'

# Create a document
curl -X PUT http://localhost:9080/api/v1/document/myapp/users/user-123 \
  -H "Content-Type: application/json" \
  -d '{
    "data": {
      "name": "John Doe",
      "email": "john@example.com"
    }
  }'

# Get document
curl http://localhost:9080/api/v1/document/myapp/users/user-123
```

### Secrets

Store sensitive data:

```bash
curl -X PUT http://localhost:9080/api/v1/secret/myapp/api-key \
  -H "Content-Type: application/json" \
  -d '{"data": "sk-secret-key-123"}'
```

## API Reference

### Admin Endpoints

| Method | Path | Description |
|--------|------|-------------|
| GET | `/` | Root info |
| GET | `/health` | Health check |
| GET | `/readyz` | Readiness check |
| GET | `/api/v1/namespace` | List namespaces |
| PUT | `/api/v1/namespace/:name` | Create/update namespace |
| DELETE | `/api/v1/namespace/:name` | Delete namespace |
| GET | `/api/v1/route` | List routes |
| PUT | `/api/v1/route/:ns/:name` | Create/update route |
| DELETE | `/api/v1/route/:ns/:name` | Delete route |
| GET | `/api/v1/service` | List services |
| PUT | `/api/v1/service/:ns/:name` | Create/update service |
| DELETE | `/api/v1/service/:ns/:name` | Delete service |
| GET | `/api/v1/module` | List modules |
| PUT | `/api/v1/module/:ns/:name` | Create/update module |
| DELETE | `/api/v1/module/:ns/:name` | Delete module |
| GET | `/api/v1/domain` | List domains |
| PUT | `/api/v1/domain/:ns/:name` | Create/update domain |
| DELETE | `/api/v1/domain/:ns/:name` | Delete domain |
| GET | `/api/v1/collection` | List collections |
| PUT | `/api/v1/collection/:ns/:name` | Create/update collection |
| DELETE | `/api/v1/collection/:ns/:name` | Delete collection |
| GET | `/api/v1/document?namespace=x&collection=y` | List documents |
| PUT | `/api/v1/document/:ns/:col/:id` | Create/update document |
| DELETE | `/api/v1/document/:ns/:col/:id` | Delete document |
| GET | `/api/v1/secret` | List secrets |
| PUT | `/api/v1/secret/:ns/:name` | Create/update secret |
| DELETE | `/api/v1/secret/:ns/:name` | Delete secret |
| GET | `/api/v1/changelog` | List change logs |

## Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `LOG_LEVEL` | Logging level | `info` |
| `PORT` | Proxy server port | `80` |
| `PORT_SSL` | HTTPS port | `443` |
| `DG_DISABLE_BANNER` | Disable startup banner | - |

## Performance

DGate v2 is built for high performance:

- **Async I/O**: Non-blocking operations with Tokio
- **Zero-copy**: Efficient buffer handling
- **Connection Pooling**: Reuse upstream connections
- **Module Caching**: Compiled JS modules are cached

## Comparison with v1 (Go)

| Feature | v1 (Go) | v2 (Rust) |
|---------|---------|-----------|
| Runtime | Go 1.21+ | Rust 1.75+ |
| JS Engine | goja | QuickJS (rquickjs) |
| HTTP | chi/stdlib | axum/hyper |
| Storage | badger/file | redb/memory |
| Documents | JSON Schema | Simple KV |

## Development

### Running Tests

```bash
# Unit tests
cargo test

# Functional tests
./functional-tests/run-all-tests.sh

# Performance tests (requires k6)
./perf-tests/run-tests.sh quick
```

### Building for Release

```bash
cargo build --release
```

## License

MIT License
