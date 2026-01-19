//! Network layer for Tempo consensus
//!
//! Provides HTTP-based communication between Tempo nodes.

#![allow(dead_code)]

use std::time::Duration;

use reqwest::Client;
use tokio::sync::mpsc;
use tracing::{debug, warn};

use super::{NodeId, TempoMessage};

/// Network layer for Tempo inter-node communication
pub struct TempoNetwork {
    /// This node's ID
    node_id: NodeId,
    /// HTTP client for sending messages
    client: Client,
    /// Channel for receiving messages from other nodes
    message_tx: mpsc::UnboundedSender<(NodeId, TempoMessage)>,
    /// Receiver handle (taken once by the consensus layer)
    message_rx: parking_lot::Mutex<Option<mpsc::UnboundedReceiver<(NodeId, TempoMessage)>>>,
}

impl TempoNetwork {
    /// Create a new network layer
    pub fn new(node_id: NodeId) -> Self {
        let client = Client::builder()
            .pool_max_idle_per_host(10)
            .timeout(Duration::from_secs(5))
            .build()
            .expect("Failed to create HTTP client");

        let (tx, rx) = mpsc::unbounded_channel();

        Self {
            node_id,
            client,
            message_tx: tx,
            message_rx: parking_lot::Mutex::new(Some(rx)),
        }
    }

    /// Get the message receiver (can only be called once)
    pub fn message_receiver(&self) -> mpsc::UnboundedReceiver<(NodeId, TempoMessage)> {
        self.message_rx
            .lock()
            .take()
            .expect("message_receiver already taken")
    }

    /// Handle an incoming message from the HTTP endpoint
    pub fn handle_incoming(&self, from: NodeId, msg: TempoMessage) {
        if let Err(e) = self.message_tx.send((from, msg)) {
            warn!("Failed to queue incoming Tempo message: {}", e);
        }
    }

    /// Send a message to another node
    pub async fn send(&self, addr: &str, msg: TempoMessage) {
        let url = format!("http://{}/tempo/message", addr);

        debug!("Sending {:?} message to {}", msg.msg_type, addr);

        let request = TempoRequest {
            from: self.node_id,
            message: msg,
        };

        match self.client.post(&url).json(&request).send().await {
            Ok(resp) if resp.status().is_success() => {
                debug!("Successfully sent message to {}", addr);
            }
            Ok(resp) => {
                let status = resp.status();
                let body = resp.text().await.unwrap_or_default();
                debug!("Failed to send message to {}: {} - {}", addr, status, body);
            }
            Err(e) => {
                debug!("Network error sending to {}: {}", addr, e);
            }
        }
    }

    /// Broadcast a message to multiple nodes
    pub async fn broadcast(&self, addrs: &[String], msg: TempoMessage) {
        for addr in addrs {
            self.send(addr, msg.clone()).await;
        }
    }
}

/// Request body for Tempo messages sent via HTTP
#[derive(Debug, Clone, serde::Serialize, serde::Deserialize)]
pub struct TempoRequest {
    pub from: NodeId,
    pub message: TempoMessage,
}
