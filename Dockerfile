# DGate API Gateway - Multi-stage Dockerfile
# Build stage for Rust compilation
FROM rust:1.92-bookworm AS builder

WORKDIR /app

# Install build dependencies
RUN apt-get update && apt-get install -y \
    pkg-config \
    libssl-dev \
    libclang-dev \
    cmake \
    && rm -rf /var/lib/apt/lists/*

# Copy manifests first for better caching
COPY Cargo.toml Cargo.lock ./

# Create dummy source files to build dependencies
RUN mkdir -p src/bin && \
    echo "" > src/lib.rs && \
    echo "fn main() {}" > src/main.rs && \
    echo "fn main() {}" > src/bin/dgate-cli.rs && \
    cargo build --release --bin dgate-server --bin dgate-cli && \
    rm -rf src

# Copy actual source code
COPY src/ src/

# Build the actual binaries (touch to invalidate the dummy build cache)
RUN touch src/main.rs src/bin/dgate-cli.rs && \
    cargo build --release --bin dgate-server --bin dgate-cli

# Runtime stage - minimal image
FROM debian:bookworm-slim AS runtime

# Install runtime dependencies
RUN apt-get update && apt-get install -y \
    ca-certificates \
    libssl3 \
    && rm -rf /var/lib/apt/lists/* \
    && useradd -r -s /bin/false dgate

WORKDIR /app

# Copy binaries from builder
COPY --from=builder /app/target/release/dgate-server /usr/local/bin/
COPY --from=builder /app/target/release/dgate-cli /usr/local/bin/

# Create data directory
RUN mkdir -p /app/data && chown dgate:dgate /app/data

# Switch to non-root user
USER dgate

# Expose ports (proxy, admin)
EXPOSE 80 443 9080

# Health check
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
    CMD dgate-cli health || exit 1

# Default command
ENTRYPOINT ["dgate-server"]
CMD ["-c", "/app/config.yaml"]
