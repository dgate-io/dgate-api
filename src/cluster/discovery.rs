//! Node discovery service for DGate cluster
//!
//! Supports DNS-based discovery for finding cluster nodes by resolving
//! a DNS hostname to a list of IP addresses. This is particularly useful
//! for Kubernetes deployments where a headless service can provide
//! pod IPs via DNS.

use std::collections::BTreeMap;
use std::net::ToSocketAddrs;
use std::time::Duration;

use openraft::BasicNode;
use tokio::sync::RwLock;
use tokio::time::interval;
use tracing::{debug, error, info, warn};

use super::NodeId;
use crate::config::{DiscoveryConfig, DiscoveryType};

/// Node discovery service
pub struct NodeDiscovery {
    config: DiscoveryConfig,
    /// Known nodes discovered so far
    known_nodes: RwLock<BTreeMap<NodeId, BasicNode>>,
}

impl NodeDiscovery {
    /// Create a new discovery service
    pub fn new(config: DiscoveryConfig) -> Self {
        Self {
            config,
            known_nodes: RwLock::new(BTreeMap::new()),
        }
    }

    /// Discover nodes using configured method
    pub async fn discover(&self) -> Vec<(NodeId, BasicNode)> {
        match self.config.discovery_type {
            DiscoveryType::Static => Vec::new(), // Static uses initial_members
            DiscoveryType::Dns => self.discover_via_dns().await,
        }
    }

    /// Discover nodes by resolving DNS hostname
    async fn discover_via_dns(&self) -> Vec<(NodeId, BasicNode)> {
        let dns_name = match &self.config.dns_name {
            Some(name) => name.clone(),
            None => {
                warn!("DNS discovery enabled but no dns_name configured");
                return Vec::new();
            }
        };

        let port = self.config.dns_port;
        let lookup_addr = format!("{}:{}", dns_name, port);

        debug!("Resolving DNS for cluster discovery: {}", lookup_addr);

        // Perform DNS lookup (blocking, so we spawn_blocking)
        let addrs = match tokio::task::spawn_blocking(move || lookup_addr.to_socket_addrs()).await {
            Ok(Ok(addrs)) => addrs.collect::<Vec<_>>(),
            Ok(Err(e)) => {
                warn!("DNS resolution failed for {}: {}", dns_name, e);
                return Vec::new();
            }
            Err(e) => {
                error!("DNS task failed: {}", e);
                return Vec::new();
            }
        };

        if addrs.is_empty() {
            warn!("DNS resolution returned no addresses for {}", dns_name);
            return Vec::new();
        }

        info!(
            "DNS discovery found {} addresses for {}",
            addrs.len(),
            dns_name
        );

        // Convert addresses to node entries
        let mut nodes = Vec::new();
        for addr in addrs {
            let addr_str = addr.to_string();
            let node_id = generate_node_id(&addr_str);
            nodes.push((node_id, BasicNode { addr: addr_str }));
        }

        nodes
    }

    /// Run a simplified discovery loop
    pub async fn run_discovery_loop_simple(&self) {
        let refresh_interval = Duration::from_secs(self.config.refresh_interval_secs);
        let mut ticker = interval(refresh_interval);

        loop {
            ticker.tick().await;

            let discovered = self.discover().await;
            if discovered.is_empty() {
                continue;
            }

            // Update known nodes
            let mut known = self.known_nodes.write().await;
            for (node_id, node) in discovered {
                if !known.contains_key(&node_id) {
                    info!("Discovered new node: {} at {}", node_id, node.addr);
                    known.insert(node_id, node);
                }
            }
        }
    }

    /// Get currently known nodes
    pub async fn known_nodes(&self) -> BTreeMap<NodeId, BasicNode> {
        self.known_nodes.read().await.clone()
    }
}

/// Generate a deterministic node ID from an address string
fn generate_node_id(addr: &str) -> NodeId {
    use std::collections::hash_map::DefaultHasher;
    use std::hash::{Hash, Hasher};

    let mut hasher = DefaultHasher::new();
    addr.hash(&mut hasher);
    // Ensure it's not 0 (reserved) and fits in a reasonable range
    (hasher.finish() % 1_000_000) + 1
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_generate_node_id() {
        let id1 = generate_node_id("192.168.1.1:9090");
        let id2 = generate_node_id("192.168.1.2:9090");
        let id3 = generate_node_id("192.168.1.1:9090");

        // Same address should give same ID
        assert_eq!(id1, id3);
        // Different addresses should give different IDs
        assert_ne!(id1, id2);
        // ID should be non-zero
        assert!(id1 > 0);
    }
}
