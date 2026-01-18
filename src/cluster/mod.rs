//! Cluster module for DGate
//!
//! Provides replication for resources and documents across multiple DGate nodes.
//! Supports both static member configuration and DNS-based discovery.
//!
//! # Architecture
//!
//! This module contains two implementation approaches:
//!
//! 1. **Simple HTTP Replication** (currently active in `mod.rs`):
//!    - Uses direct HTTP calls to replicate changes to peer nodes
//!    - All nodes can accept writes and replicate to others
//!    - Simple and effective for most use cases
//!
//! 2. **Full Raft Consensus** (in `state_machine.rs` and `network.rs`):
//!    - Complete openraft integration with leader election, log replication
//!    - Provides stronger consistency guarantees
//!    - Can be enabled by wiring up the openraft components

mod discovery;

use std::collections::BTreeMap;
use std::sync::Arc;

use openraft::BasicNode;
use reqwest::Client;
use serde::{Deserialize, Serialize};
use tokio::sync::RwLock;
use tracing::{debug, info, warn};

use crate::config::{ClusterConfig, ClusterMember, ClusterMode};
use crate::resources::ChangeLog;
use crate::storage::ProxyStore;

pub use discovery::NodeDiscovery;

/// Node ID type
pub type NodeId = u64;

/// Response from the state machine after applying a log entry
#[derive(Debug, Clone, Serialize, Deserialize, Default)]
pub struct ClientResponse {
    pub success: bool,
    pub message: Option<String>,
}

/// Snapshot data for state machine (used by openraft implementation in state_machine.rs)
#[allow(dead_code)]
#[derive(Debug, Clone, Serialize, Deserialize, Default)]
pub struct SnapshotData {
    pub changelogs: Vec<ChangeLog>,
}

/// Raft type configuration (used by openraft implementation in state_machine.rs and network.rs)
#[allow(dead_code)]
pub struct TypeConfig;

/// Simplified Raft instance for the HTTP replication approach.
/// For full Raft consensus, see the openraft implementation in state_machine.rs and network.rs.
pub struct DGateRaft {
    node_id: NodeId,
    members: RwLock<BTreeMap<NodeId, BasicNode>>,
    leader_id: RwLock<Option<NodeId>>,
}

impl DGateRaft {
    fn new(node_id: NodeId) -> Self {
        Self {
            node_id,
            members: RwLock::new(BTreeMap::new()),
            leader_id: RwLock::new(Some(node_id)), // Single node is leader
        }
    }

    /// Get this node's ID
    pub fn node_id(&self) -> NodeId {
        self.node_id
    }

    pub async fn current_leader(&self) -> Option<NodeId> {
        *self.leader_id.read().await
    }

    /// Handle a vote request (stub - full implementation in network.rs)
    #[allow(dead_code)]
    pub async fn vote(
        &self,
        _req: serde_json::Value,
    ) -> Result<serde_json::Value, Box<dyn std::error::Error + Send + Sync>> {
        Ok(serde_json::json!({"vote_granted": true}))
    }

    /// Handle append entries request (stub - full implementation in network.rs)
    #[allow(dead_code)]
    pub async fn append_entries(
        &self,
        _req: serde_json::Value,
    ) -> Result<serde_json::Value, Box<dyn std::error::Error + Send + Sync>> {
        Ok(serde_json::json!({"success": true}))
    }

    /// Handle install snapshot request (stub - full implementation in network.rs)
    #[allow(dead_code)]
    pub async fn install_snapshot(
        &self,
        _req: serde_json::Value,
    ) -> Result<serde_json::Value, Box<dyn std::error::Error + Send + Sync>> {
        Ok(serde_json::json!({"success": true}))
    }
}

/// State machine for applying Raft log entries
pub struct DGateStateMachine {
    store: Arc<ProxyStore>,
    change_tx: Option<tokio::sync::mpsc::UnboundedSender<ChangeLog>>,
}

impl DGateStateMachine {
    /// Create a new state machine (without change notifications)
    #[allow(dead_code)]
    pub fn new(store: Arc<ProxyStore>) -> Self {
        Self {
            store,
            change_tx: None,
        }
    }

    /// Create a new state machine with a change notification channel
    pub fn with_change_notifier(
        store: Arc<ProxyStore>,
        change_tx: tokio::sync::mpsc::UnboundedSender<ChangeLog>,
    ) -> Self {
        Self {
            store,
            change_tx: Some(change_tx),
        }
    }

    /// Apply a changelog to storage and notify listeners
    pub fn apply(&self, changelog: &ChangeLog) -> ClientResponse {
        use crate::resources::*;

        // Apply the change to storage
        let result = match changelog.cmd {
            ChangeCommand::AddNamespace => {
                let ns: Namespace = match serde_json::from_value(changelog.item.clone()) {
                    Ok(ns) => ns,
                    Err(e) => {
                        return ClientResponse {
                            success: false,
                            message: Some(e.to_string()),
                        }
                    }
                };
                self.store.set_namespace(&ns).map_err(|e| e.to_string())
            }
            ChangeCommand::DeleteNamespace => self
                .store
                .delete_namespace(&changelog.name)
                .map_err(|e| e.to_string()),
            ChangeCommand::AddRoute => {
                let route: Route = match serde_json::from_value(changelog.item.clone()) {
                    Ok(r) => r,
                    Err(e) => {
                        return ClientResponse {
                            success: false,
                            message: Some(e.to_string()),
                        }
                    }
                };
                self.store.set_route(&route).map_err(|e| e.to_string())
            }
            ChangeCommand::DeleteRoute => self
                .store
                .delete_route(&changelog.namespace, &changelog.name)
                .map_err(|e| e.to_string()),
            ChangeCommand::AddService => {
                let service: Service = match serde_json::from_value(changelog.item.clone()) {
                    Ok(s) => s,
                    Err(e) => {
                        return ClientResponse {
                            success: false,
                            message: Some(e.to_string()),
                        }
                    }
                };
                self.store.set_service(&service).map_err(|e| e.to_string())
            }
            ChangeCommand::DeleteService => self
                .store
                .delete_service(&changelog.namespace, &changelog.name)
                .map_err(|e| e.to_string()),
            ChangeCommand::AddModule => {
                let module: Module = match serde_json::from_value(changelog.item.clone()) {
                    Ok(m) => m,
                    Err(e) => {
                        return ClientResponse {
                            success: false,
                            message: Some(e.to_string()),
                        }
                    }
                };
                self.store.set_module(&module).map_err(|e| e.to_string())
            }
            ChangeCommand::DeleteModule => self
                .store
                .delete_module(&changelog.namespace, &changelog.name)
                .map_err(|e| e.to_string()),
            ChangeCommand::AddDomain => {
                let domain: Domain = match serde_json::from_value(changelog.item.clone()) {
                    Ok(d) => d,
                    Err(e) => {
                        return ClientResponse {
                            success: false,
                            message: Some(e.to_string()),
                        }
                    }
                };
                self.store.set_domain(&domain).map_err(|e| e.to_string())
            }
            ChangeCommand::DeleteDomain => self
                .store
                .delete_domain(&changelog.namespace, &changelog.name)
                .map_err(|e| e.to_string()),
            ChangeCommand::AddSecret => {
                let secret: Secret = match serde_json::from_value(changelog.item.clone()) {
                    Ok(s) => s,
                    Err(e) => {
                        return ClientResponse {
                            success: false,
                            message: Some(e.to_string()),
                        }
                    }
                };
                self.store.set_secret(&secret).map_err(|e| e.to_string())
            }
            ChangeCommand::DeleteSecret => self
                .store
                .delete_secret(&changelog.namespace, &changelog.name)
                .map_err(|e| e.to_string()),
            ChangeCommand::AddCollection => {
                let collection: Collection = match serde_json::from_value(changelog.item.clone()) {
                    Ok(c) => c,
                    Err(e) => {
                        return ClientResponse {
                            success: false,
                            message: Some(e.to_string()),
                        }
                    }
                };
                self.store
                    .set_collection(&collection)
                    .map_err(|e| e.to_string())
            }
            ChangeCommand::DeleteCollection => self
                .store
                .delete_collection(&changelog.namespace, &changelog.name)
                .map_err(|e| e.to_string()),
            ChangeCommand::AddDocument => {
                let document: Document = match serde_json::from_value(changelog.item.clone()) {
                    Ok(d) => d,
                    Err(e) => {
                        return ClientResponse {
                            success: false,
                            message: Some(e.to_string()),
                        }
                    }
                };
                self.store
                    .set_document(&document)
                    .map_err(|e| e.to_string())
            }
            ChangeCommand::DeleteDocument => {
                let doc: Document = match serde_json::from_value(changelog.item.clone()) {
                    Ok(d) => d,
                    Err(e) => {
                        return ClientResponse {
                            success: false,
                            message: Some(e.to_string()),
                        }
                    }
                };
                self.store
                    .delete_document(&changelog.namespace, &doc.collection, &changelog.name)
                    .map_err(|e| e.to_string())
            }
        };

        if let Err(e) = result {
            return ClientResponse {
                success: false,
                message: Some(e),
            };
        }

        // Notify proxy about the change
        if let Some(ref tx) = self.change_tx {
            let _ = tx.send(changelog.clone());
        }

        ClientResponse {
            success: true,
            message: Some("Applied".to_string()),
        }
    }
}

/// Cluster metrics for admin API
#[derive(Debug, Clone, Serialize)]
pub struct ClusterMetrics {
    pub id: NodeId,
    pub mode: ClusterMode,
    pub is_leader: bool,
    pub current_term: Option<u64>,
    pub last_applied: Option<u64>,
    pub committed: Option<u64>,
    pub members: Vec<ClusterMember>,
}

/// Cluster manager handles all cluster operations
pub struct ClusterManager {
    /// Configuration
    config: ClusterConfig,
    /// The Raft instance
    raft: Arc<DGateRaft>,
    /// State machine
    state_machine: Arc<DGateStateMachine>,
    /// Node discovery service
    discovery: Option<Arc<NodeDiscovery>>,
    /// Indicates if this node is the leader
    is_leader: Arc<RwLock<bool>>,
    /// HTTP client for replication
    http_client: Client,
}

impl ClusterManager {
    /// Create a new cluster manager
    pub async fn new(
        cluster_config: ClusterConfig,
        state_machine: Arc<DGateStateMachine>,
    ) -> anyhow::Result<Self> {
        let node_id = cluster_config.node_id;
        let mode = cluster_config.mode;

        info!(
            "Creating cluster manager for node {} at {} (mode: {:?})",
            node_id, cluster_config.advertise_addr, mode
        );

        // Create simplified Raft instance
        let raft = Arc::new(DGateRaft::new(node_id));

        // Setup discovery if configured
        let discovery = cluster_config
            .discovery
            .as_ref()
            .map(|disc_config| Arc::new(NodeDiscovery::new(disc_config.clone())));

        // Create HTTP client for replication
        let http_client = Client::builder()
            .pool_max_idle_per_host(10)
            .timeout(std::time::Duration::from_secs(5))
            .build()
            .expect("Failed to create HTTP client");

        // In simple mode, all nodes are leaders (can accept writes)
        // In raft mode, only the bootstrap node starts as leader
        let is_leader = match mode {
            ClusterMode::Simple => true, // All nodes can accept writes
            ClusterMode::Raft => cluster_config.bootstrap, // Only bootstrap node starts as leader
        };

        Ok(Self {
            config: cluster_config,
            raft,
            state_machine,
            discovery,
            is_leader: Arc::new(RwLock::new(is_leader)),
            http_client,
        })
    }

    /// Initialize the cluster
    pub async fn initialize(&self) -> anyhow::Result<()> {
        let node_id = self.config.node_id;
        let mode = self.config.mode;

        match mode {
            ClusterMode::Simple => {
                info!(
                    "Initializing simple replication cluster with node_id={}",
                    node_id
                );
                // In simple mode, all nodes are peers and can accept writes
                *self.is_leader.write().await = true;
            }
            ClusterMode::Raft => {
                if self.config.bootstrap {
                    info!(
                        "Bootstrapping Raft cluster with node_id={} as leader",
                        node_id
                    );
                    *self.is_leader.write().await = true;
                } else if !self.config.initial_members.is_empty() {
                    info!(
                        "Joining Raft cluster with {} members as follower",
                        self.config.initial_members.len()
                    );
                    *self.is_leader.write().await = false;
                    // In a full Raft implementation, would:
                    // 1. Connect to existing cluster members
                    // 2. Request to join the cluster
                    // 3. Participate in leader election
                }
            }
        }

        // Start discovery background task if configured
        if let Some(ref discovery) = self.discovery {
            let discovery_clone = discovery.clone();
            tokio::spawn(async move {
                discovery_clone.run_discovery_loop_simple().await;
            });
        }

        Ok(())
    }

    /// Get the cluster mode
    pub fn mode(&self) -> ClusterMode {
        self.config.mode
    }

    /// Check if this node is the current leader
    pub async fn is_leader(&self) -> bool {
        *self.is_leader.read().await
    }

    /// Get the current leader ID
    pub async fn leader_id(&self) -> Option<NodeId> {
        self.raft.current_leader().await
    }

    /// Propose a change log to the cluster
    pub async fn propose(&self, changelog: ChangeLog) -> anyhow::Result<ClientResponse> {
        // Apply the change locally first
        let response = self.state_machine.apply(&changelog);

        if !response.success {
            return Ok(response);
        }

        // Replicate to other nodes
        self.replicate_to_peers(&changelog).await;

        Ok(response)
    }

    /// Replicate a changelog to all peer nodes
    async fn replicate_to_peers(&self, changelog: &ChangeLog) {
        let my_node_id = self.raft.node_id();

        for member in &self.config.initial_members {
            // Skip self
            if member.id == my_node_id {
                continue;
            }

            let admin_url = self.get_member_admin_url(member);
            let url = format!("{}/internal/replicate", admin_url);

            debug!(
                "Replicating changelog {} to node {} at {}",
                changelog.id, member.id, url
            );

            let client = self.http_client.clone();
            let changelog_clone = changelog.clone();
            let member_id = member.id;

            // Spawn replication as background task to not block the response
            tokio::spawn(async move {
                match client.post(&url).json(&changelog_clone).send().await {
                    Ok(resp) => {
                        if resp.status().is_success() {
                            debug!("Successfully replicated to node {}", member_id);
                        } else {
                            warn!(
                                "Failed to replicate to node {}: status {}",
                                member_id,
                                resp.status()
                            );
                        }
                    }
                    Err(e) => {
                        warn!("Failed to replicate to node {}: {}", member_id, e);
                    }
                }
            });
        }
    }

    /// Get the admin API URL for a cluster member
    fn get_member_admin_url(&self, member: &ClusterMember) -> String {
        let scheme = if member.tls { "https" } else { "http" };

        // If admin_port is specified, use it
        if let Some(admin_port) = member.admin_port {
            // Extract host from addr (format: host:port)
            let host = member.addr.split(':').next().unwrap_or("127.0.0.1");
            return format!("{}://{}:{}", scheme, host, admin_port);
        }

        // Otherwise, derive admin port from raft port (admin = raft - 10)
        // This is a convention used in the test configuration
        if let Some(port_str) = member.addr.split(':').next_back() {
            if let Ok(raft_port) = port_str.parse::<u16>() {
                let admin_port = raft_port.saturating_sub(10);
                let host = member.addr.split(':').next().unwrap_or("127.0.0.1");
                return format!("{}://{}:{}", scheme, host, admin_port);
            }
        }

        // Fallback: use addr as-is (might not work)
        format!("{}://{}", scheme, member.addr)
    }

    /// Apply a replicated changelog (from another node)
    /// This applies the change locally without re-replicating
    pub fn apply_replicated(&self, changelog: &ChangeLog) -> ClientResponse {
        debug!(
            "Applying replicated changelog {} from cluster peer",
            changelog.id
        );
        self.state_machine.apply(changelog)
    }

    /// Get cluster metrics
    pub async fn metrics(&self) -> ClusterMetrics {
        ClusterMetrics {
            id: self.raft.node_id(),
            mode: self.config.mode,
            is_leader: *self.is_leader.read().await,
            current_term: Some(1),
            last_applied: Some(0),
            committed: Some(0),
            members: self.config.initial_members.clone(),
        }
    }

    /// Get cluster members (public API for external use)
    #[allow(dead_code)]
    pub fn members(&self) -> &[ClusterMember] {
        &self.config.initial_members
    }

    /// Get the Raft instance for admin operations (public API for external use)
    #[allow(dead_code)]
    pub fn raft(&self) -> &Arc<DGateRaft> {
        &self.raft
    }

    /// Add a new node to the cluster (leader only)
    pub async fn add_node(&self, node_id: NodeId, addr: String) -> anyhow::Result<()> {
        info!("Adding node {} at {}", node_id, addr);
        let mut members = self.raft.members.write().await;
        members.insert(node_id, BasicNode { addr });
        Ok(())
    }

    /// Remove a node from the cluster (leader only)
    pub async fn remove_node(&self, node_id: NodeId) -> anyhow::Result<()> {
        info!("Removing node {}", node_id);
        let mut members = self.raft.members.write().await;
        members.remove(&node_id);
        Ok(())
    }
}

/// Cluster error types (public API for error handling)
#[allow(dead_code)]
#[derive(Debug, thiserror::Error)]
pub enum ClusterError {
    #[error("Not leader, current leader is: {0:?}")]
    NotLeader(Option<NodeId>),

    #[error("Raft error: {0}")]
    Raft(String),

    #[error("Discovery error: {0}")]
    Discovery(String),

    #[error("Storage error: {0}")]
    Storage(String),
}
