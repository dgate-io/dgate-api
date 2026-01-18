//! DGate API Gateway Server - Rust Edition
//!
//! A high-performance API gateway with JavaScript module support for
//! request/response modification, routing, and more.

mod admin;
mod config;
mod modules;
mod proxy;
mod resources;
mod storage;

use axum::{
    body::Body, extract::Request, http::StatusCode, response::IntoResponse, routing::any, Router,
};
use clap::Parser;
use std::net::SocketAddr;
use std::sync::Arc;
use tokio::net::TcpListener;
use tokio::signal;
use tower_http::trace::TraceLayer;
use tracing::{info, Level};
use tracing_subscriber::{fmt, prelude::*, EnvFilter};

use crate::admin::AdminState;
use crate::config::DGateConfig;
use crate::proxy::ProxyState;

const VERSION: &str = env!("CARGO_PKG_VERSION");

const BANNER: &str = r#"
_________________      _____      
___  __ \_  ____/_____ __  /_____ 
__  / / /  / __ _  __ `/  __/  _ \
_  /_/ // /_/ / / /_/ // /_ /  __/
/_____/ \____/  \__,_/ \__/ \___/ 
                                   
DGate - API Gateway Server (Rust Edition)
-----------------------------------
"#;

/// DGate API Gateway Server
#[derive(Parser, Debug)]
#[command(author, version, about, long_about = None)]
struct Args {
    /// Path to configuration file
    #[arg(short, long)]
    config: Option<String>,
}

#[tokio::main]
async fn main() -> anyhow::Result<()> {
    // Install rustls crypto provider
    rustls::crypto::ring::default_provider()
        .install_default()
        .expect("Failed to install rustls crypto provider");

    let args = Args::parse();

    // Print banner
    if std::env::var("DG_DISABLE_BANNER").is_err() {
        print!("{}", BANNER);
        println!("Version: v{}\n", VERSION);
    }

    // Load configuration
    let config = DGateConfig::load(args.config.as_deref())?;

    // Set up logging
    setup_logging(&config);

    info!("Starting DGate server...");
    info!("PID: {}", std::process::id());

    // Create proxy state
    let proxy_state = ProxyState::new(config.clone());

    // Restore from change logs
    proxy_state.restore_from_changelogs().await?;

    // Initialize from config resources
    proxy_state.init_from_config().await?;

    // Start admin API if configured
    let admin_handle = if let Some(ref admin_config) = config.admin {
        let admin_addr = format!("{}:{}", admin_config.host, admin_config.port);
        let admin_state = AdminState {
            proxy: proxy_state.clone(),
            config: admin_config.clone(),
            version: VERSION.to_string(),
        };

        let admin_router = admin::create_router(admin_state);

        let addr: SocketAddr = admin_addr.parse()?;
        info!("Admin API listening on http://{}", addr);

        let listener = TcpListener::bind(addr).await?;

        Some(tokio::spawn(async move {
            axum::serve(listener, admin_router)
                .await
                .expect("Admin server error");
        }))
    } else {
        None
    };

    // Start proxy server
    let proxy_addr = format!("{}:{}", config.proxy.host, config.proxy.port);
    let addr: SocketAddr = proxy_addr.parse()?;

    // Create proxy router
    let proxy_state_clone = proxy_state.clone();
    let proxy_router = Router::new()
        .route(
            "/{*path}",
            any(move |req: Request| async move { proxy_state_clone.handle_request(req).await }),
        )
        .route(
            "/",
            any({
                let ps = proxy_state.clone();
                move |req: Request| async move { ps.handle_request(req).await }
            }),
        )
        .layer(TraceLayer::new_for_http());

    info!("Proxy server listening on http://{}", addr);

    let listener = TcpListener::bind(addr).await?;

    // Handle graceful shutdown
    let shutdown_signal = async {
        signal::ctrl_c()
            .await
            .expect("Failed to install CTRL+C signal handler");
        info!("Received shutdown signal...");
    };

    // Start the server with graceful shutdown
    axum::serve(listener, proxy_router)
        .with_graceful_shutdown(shutdown_signal)
        .await?;

    info!("Server shutdown complete");
    Ok(())
}

fn setup_logging(config: &DGateConfig) {
    let log_level = match config.log_level.to_lowercase().as_str() {
        "trace" => Level::TRACE,
        "debug" => Level::DEBUG,
        "info" => Level::INFO,
        "warn" | "warning" => Level::WARN,
        "error" => Level::ERROR,
        _ => Level::INFO,
    };

    let filter = EnvFilter::from_default_env().add_directive(log_level.into());

    if config.log_json {
        tracing_subscriber::registry()
            .with(filter)
            .with(fmt::layer().json())
            .init();
    } else {
        tracing_subscriber::registry()
            .with(filter)
            .with(fmt::layer().pretty())
            .init();
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_args_parsing() {
        // Test with config argument
        let args = Args::try_parse_from(["dgate-server", "--config", "test.yaml"]).unwrap();
        assert_eq!(args.config, Some("test.yaml".to_string()));

        // Test without arguments
        let args = Args::try_parse_from(["dgate-server"]).unwrap();
        assert_eq!(args.config, None);
    }
}
