# DGate v2 Performance Tests

Performance testing suite using [k6](https://k6.io/) to benchmark DGate v2 components.

## Prerequisites

Install k6:
```bash
# macOS
brew install k6

# Linux
sudo gpg -k
sudo gpg --no-default-keyring --keyring /usr/share/keyrings/k6-archive-keyring.gpg --keyserver hkp://keyserver.ubuntu.com:80 --recv-keys C5AD17C747E3415A3642D57D77C6C491D6AC1D69
echo "deb [signed-by=/usr/share/keyrings/k6-archive-keyring.gpg] https://dl.k6.io/deb stable main" | sudo tee /etc/apt/sources.list.d/k6.list
sudo apt-get update
sudo apt-get install k6
```

## Quick Start

```bash
# Run quick 30-second test of all components
./run-tests.sh quick

# Compare direct vs proxied performance
./run-tests.sh baseline
```

## Test Suites

### 1. Quick Test (`quick-test.js`)
A quick 30-second test hitting all components (proxy, modules, admin).

```bash
./run-tests.sh quick
# or manually:
k6 run quick-test.js
```

### 2. Proxy Test (`test-proxy.js`)
Tests proxy performance with an echo server upstream.

```bash
./run-tests.sh proxy
# or manually:
k6 run test-proxy.js

# Compare direct vs proxy
k6 run -e TARGET=direct test-proxy.js  # Hit echo server directly
k6 run -e TARGET=proxy test-proxy.js   # Hit via DGate
```

### 3. JS Modules Test (`test-modules.js`)
Tests JavaScript module execution performance.

```bash
./run-tests.sh modules
# or manually:
k6 run test-modules.js

# Test specific handler
k6 run -e HANDLER=simple test-modules.js   # Simple JSON response
k6 run -e HANDLER=echo test-modules.js     # Echo request info
k6 run -e HANDLER=compute test-modules.js  # With computation
```

### 4. Admin API Test (`test-admin.js`)
Tests Admin API performance for CRUD operations.

```bash
./run-tests.sh admin
# or manually:
k6 run test-admin.js

# Test specific operation type
k6 run -e OP=read test-admin.js   # Read operations only
k6 run -e OP=write test-admin.js  # Write operations only
k6 run -e OP=mixed test-admin.js  # 80% reads, 20% writes
```

## Running All Tests

```bash
./run-tests.sh all
```

## Performance Targets

| Component | Target p95 | Target p99 |
|-----------|-----------|-----------|
| Proxy (with upstream) | < 100ms | < 200ms |
| JS Module (simple) | < 30ms | < 50ms |
| JS Module (echo) | < 30ms | < 50ms |
| JS Module (compute) | < 50ms | < 100ms |
| Admin API (read) | < 50ms | < 100ms |
| Admin API (write) | < 100ms | < 200ms |

## Configuration

| Environment Variable | Default | Description |
|---------------------|---------|-------------|
| `DGATE_URL` | `http://localhost:8080` | DGate proxy URL |
| `ADMIN_URL` | `http://localhost:9080` | DGate admin API URL |
| `ECHO_URL` | `http://localhost:9999` | Echo server URL |
| `TARGET` | `proxy` | For proxy test: `direct` or `proxy` |
| `HANDLER` | `all` | For modules test: `simple`, `echo`, `compute`, or `all` |
| `OP` | `mixed` | For admin test: `read`, `write`, or `mixed` |

## Test Scenarios

Each test includes multiple scenarios:

- **Smoke**: 1 VU for 10s (sanity check)
- **Load**: Ramp up to 100 VUs over 3 minutes
- **Stress**: Ramp up to 500 VUs over 3 minutes

## Output Files

Tests generate JSON result files:
- `proxy-results.json`
- `modules-results.json`
- `admin-results.json`

Analyze with:
```bash
cat proxy-results.json | jq '.metrics.http_req_duration'
```

## Manual Setup

If you want to run tests manually without the script:

```bash
# Terminal 1: Start echo server (for proxy tests)
python3 -m http.server 9999

# Terminal 2: Start DGate
cd dgate-v2
PORT=8080 ./target/release/dgate-server -c perf-tests/config.perf.yaml

# Terminal 3: Run tests
cd dgate-v2/perf-tests
k6 run quick-test.js
```
