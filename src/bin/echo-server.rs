//! Ultra-fast Echo Server for Performance Testing
//!
//! This is a minimal echo server designed for high concurrency.
//!
//! Run with: cargo run --release --bin echo-server -- --port 9999

use std::convert::Infallible;
use std::net::SocketAddr;
use std::sync::atomic::{AtomicU64, Ordering};
use std::sync::Arc;

use http_body_util::Full;
use hyper::body::Bytes;
use hyper::server::conn::http1;
use hyper::service::service_fn;
use hyper::{Request, Response};
use hyper_util::rt::TokioIo;
use tokio::net::TcpListener;

static REQUEST_COUNT: AtomicU64 = AtomicU64::new(0);

async fn echo(_req: Request<hyper::body::Incoming>) -> Result<Response<Full<Bytes>>, Infallible> {
    let count = REQUEST_COUNT.fetch_add(1, Ordering::Relaxed);

    // Super minimal response for maximum speed
    let body = format!(r#"{{"ok":true,"n":{}}}"#, count);

    Ok(Response::builder()
        .status(200)
        .header("Content-Type", "application/json")
        .body(Full::new(Bytes::from(body)))
        .unwrap())
}

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error + Send + Sync>> {
    let port: u16 = std::env::args()
        .skip_while(|a| a != "--port")
        .nth(1)
        .and_then(|p| p.parse().ok())
        .unwrap_or(9999);

    let addr = SocketAddr::from(([0, 0, 0, 0], port));
    let listener = TcpListener::bind(addr).await?;

    println!("Echo server listening on http://{}", addr);

    loop {
        match listener.accept().await {
            Ok((stream, _)) => {
                let io = TokioIo::new(stream);

                tokio::task::spawn(async move {
                    let conn = http1::Builder::new()
                        .keep_alive(true)
                        .serve_connection(io, service_fn(echo));

                    if let Err(err) = conn.await {
                        // Only log if it's not a normal connection close
                        if !err.is_incomplete_message() {
                            eprintln!("Connection error: {:?}", err);
                        }
                    }
                });
            }
            Err(e) => {
                eprintln!("Accept error: {}", e);
            }
        }
    }
}
