//! Cluster module for DGate
//!
//! Provides replication for resources and documents across multiple DGate nodes
//! using the Raft consensus protocol via openraft.
//!
//! # Architecture
//!
//! This module implements full Raft consensus with:
//! - Leader election
//! - Log replication
//! - Snapshot transfer
//! - Dynamic membership changes
//!
//! Key components:
//! - `TypeConfig`: Raft type configuration
//! - `ClusterManager`: High-level cluster operations
//! - `RaftLogStore`: In-memory log storage (in `store.rs`)
//! - `DGateStateMachine`: State machine for applying logs (in `state_machine.rs`)
//! - `NetworkFactory`: HTTP-based Raft networking (in `network.rs`)

mod discovery;
mod network;
mod state_machine;
mod store;

use std::collections::BTreeMap;
use std::collections::BTreeSet;
use std::io::Cursor;
use std::sync::Arc;
use std::time::Duration;

use openraft::raft::ClientWriteResponse;
use openraft::Config as RaftConfig;
use openraft::{BasicNode, ChangeMembers, Raft};
use serde::{Deserialize, Serialize};
use tokio::sync::RwLock;
use tracing::{debug, info, warn};

use crate::config::{ClusterConfig, ClusterMember, ClusterMode};
use crate::resources::ChangeLog;

pub use discovery::NodeDiscovery;
pub use network::NetworkFactory;
pub use state_machine::DGateStateMachine;
pub use store::RaftLogStore;

/// Node ID type
pub type NodeId = u64;

// Raft type configuration using openraft's declarative macro
openraft::declare_raft_types!(
    pub TypeConfig:
        D = ChangeLog,
        R = ClientResponse,
        Node = BasicNode,
);

/// Response from the state machine after applying a log entry
#[derive(Debug, Clone, Serialize, Deserialize, Default)]
pub struct ClientResponse {
    pub success: bool,
    pub message: Option<String>,
}

/// Snapshot data for state machine
#[derive(Debug, Clone, Serialize, Deserialize, Default)]
pub struct SnapshotData {
    pub changelogs: Vec<ChangeLog>,
}

/// The Raft instance type alias for convenience
pub type DGateRaft = Raft<TypeConfig>;

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
    pub state: String,
}

/// Cluster manager handles all cluster operations
pub struct ClusterManager {
    /// Configuration
    config: ClusterConfig,
    /// The real Raft instance
    raft: Arc<DGateRaft>,
    /// Node discovery service
    discovery: Option<Arc<NodeDiscovery>>,
    /// Cached members list (updated from Raft metrics)
    cached_members: RwLock<Vec<ClusterMember>>,
}

impl ClusterManager {
    /// Create a new cluster manager with full Raft consensus
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

        // Create Raft configuration
        let raft_config = RaftConfig {
            cluster_name: "dgate-cluster".to_string(),
            heartbeat_interval: 200,
            election_timeout_min: 500,
            election_timeout_max: 1000,
            // Enable automatic snapshot when log grows
            snapshot_policy: openraft::SnapshotPolicy::LogsSinceLast(1000),
            max_in_snapshot_log_to_keep: 100,
            ..Default::default()
        };

        let raft_config = Arc::new(raft_config.validate()?);

        // Create log store
        let log_store = RaftLogStore::new();

        // Create network factory
        let network_factory = NetworkFactory::new();

        // Create the Raft instance
        let raft = Raft::new(
            node_id,
            raft_config,
            network_factory,
            log_store,
            state_machine.as_ref().clone(),
        )
        .await?;

        let raft = Arc::new(raft);

        // Setup discovery if configured
        let discovery = cluster_config
            .discovery
            .as_ref()
            .map(|disc_config| Arc::new(NodeDiscovery::new(disc_config.clone())));

        // Cache initial members from config
        let cached_members = RwLock::new(cluster_config.initial_members.clone());

        Ok(Self {
            config: cluster_config,
            raft,
            discovery,
            cached_members,
        })
    }

    /// Initialize the cluster - bootstrap or join existing cluster
    pub async fn initialize(&self) -> anyhow::Result<()> {
        let node_id = self.config.node_id;
        let mode = self.config.mode;

        match mode {
            ClusterMode::Simple => {
                info!(
                    "Initializing simple replication cluster with node_id={}",
                    node_id
                );
                // In simple mode, just bootstrap as a single-node cluster
                self.bootstrap_single_node().await?;
            }
            ClusterMode::Raft => {
                if self.config.bootstrap {
                    info!(
                        "Bootstrapping Raft cluster with node_id={} as initial leader",
                        node_id
                    );
                    self.bootstrap_cluster().await?;
                } else if !self.config.initial_members.is_empty() {
                    info!(
                        "Joining existing Raft cluster with {} known members",
                        self.config.initial_members.len()
                    );
                    self.join_cluster().await?;
                } else {
                    warn!("No bootstrap flag and no initial members - starting as isolated node");
                    self.bootstrap_single_node().await?;
                }
            }
        }

        // Start discovery background task if configured
        if let Some(ref discovery) = self.discovery {
            let discovery_clone = discovery.clone();
            let raft_clone = self.raft.clone();
            let my_node_id = self.config.node_id;
            tokio::spawn(async move {
                Self::run_discovery_loop(discovery_clone, raft_clone, my_node_id).await;
            });
        }

        Ok(())
    }

    /// Bootstrap this node as a single-node cluster (becomes leader immediately)
    async fn bootstrap_single_node(&self) -> anyhow::Result<()> {
        let node_id = self.config.node_id;
        let mut members = BTreeMap::new();
        members.insert(
            node_id,
            BasicNode {
                addr: self.config.advertise_addr.clone(),
            },
        );

        match self.raft.initialize(members).await {
            Ok(_) => {
                info!("Successfully bootstrapped single-node cluster");
                Ok(())
            }
            Err(e) => {
                // If already initialized, that's fine
                if e.to_string().contains("already initialized") {
                    debug!("Cluster already initialized");
                    Ok(())
                } else {
                    Err(e.into())
                }
            }
        }
    }

    /// Bootstrap as the initial leader with configured members
    async fn bootstrap_cluster(&self) -> anyhow::Result<()> {
        let node_id = self.config.node_id;
        let mut members = BTreeMap::new();

        // Add self first
        members.insert(
            node_id,
            BasicNode {
                addr: self.config.advertise_addr.clone(),
            },
        );

        // Add other initial members
        for member in &self.config.initial_members {
            if member.id != node_id {
                members.insert(
                    member.id,
                    BasicNode {
                        addr: member.addr.clone(),
                    },
                );
            }
        }

        info!("Bootstrapping cluster with {} members", members.len());

        match self.raft.initialize(members).await {
            Ok(_) => {
                info!("Successfully bootstrapped cluster as leader");
                Ok(())
            }
            Err(e) => {
                if e.to_string().contains("already initialized") {
                    debug!("Cluster already initialized");
                    Ok(())
                } else {
                    Err(e.into())
                }
            }
        }
    }

    /// Join an existing cluster by contacting known members
    async fn join_cluster(&self) -> anyhow::Result<()> {
        let node_id = self.config.node_id;
        let my_addr = &self.config.advertise_addr;

        info!(
            "Attempting to join cluster as node {} at {}",
            node_id, my_addr
        );

        // Try to contact each known member and request to be added
        let client = reqwest::Client::builder()
            .timeout(Duration::from_secs(5))
            .build()?;

        for member in &self.config.initial_members {
            if member.id == node_id {
                continue; // Skip self
            }

            let url = format!("http://{}/api/v1/cluster/members/{}", member.addr, node_id);

            info!(
                "Requesting to join cluster via member {} at {}",
                member.id, member.addr
            );

            let result = client
                .put(&url)
                .json(&serde_json::json!({
                    "addr": my_addr
                }))
                .send()
                .await;

            match result {
                Ok(resp) if resp.status().is_success() => {
                    info!("Successfully joined cluster via node {}", member.id);
                    return Ok(());
                }
                Ok(resp) => {
                    let status = resp.status();
                    let body = resp.text().await.unwrap_or_default();
                    warn!(
                        "Failed to join via node {}: {} - {}",
                        member.id, status, body
                    );
                }
                Err(e) => {
                    warn!("Failed to contact node {}: {}", member.id, e);
                }
            }
        }

        // If we couldn't join any existing cluster, wait for someone to add us
        warn!("Could not join any existing cluster member - waiting to be added");
        Ok(())
    }

    /// Discovery loop that periodically checks for new nodes
    async fn run_discovery_loop(
        discovery: Arc<NodeDiscovery>,
        raft: Arc<DGateRaft>,
        my_node_id: NodeId,
    ) {
        loop {
            let nodes = discovery.discover().await;
            for (node_id, node) in nodes {
                // Try to add discovered nodes if we're the leader
                let metrics = raft.metrics().borrow().clone();
                if metrics.current_leader == Some(my_node_id) {
                    let change =
                        ChangeMembers::AddNodes([(node_id, node.clone())].into_iter().collect());

                    match raft.change_membership(change, false).await {
                        Ok(_) => info!("Added discovered node {} at {}", node_id, node.addr),
                        Err(e) => debug!("Could not add node {}: {}", node_id, e),
                    }
                }
            }

            tokio::time::sleep(Duration::from_secs(10)).await;
        }
    }

    /// Get the cluster mode
    #[allow(dead_code)]
    pub fn mode(&self) -> ClusterMode {
        self.config.mode
    }

    /// Check if this node is the current leader
    pub async fn is_leader(&self) -> bool {
        let metrics = self.raft.metrics().borrow().clone();
        metrics.current_leader == Some(self.config.node_id)
    }

    /// Get the current leader ID
    pub async fn leader_id(&self) -> Option<NodeId> {
        self.raft.metrics().borrow().current_leader
    }

    /// Get the Raft instance for direct access (e.g., for handling RPC requests)
    pub fn raft(&self) -> &Arc<DGateRaft> {
        &self.raft
    }

    /// Propose a change log to the cluster via Raft consensus
    pub async fn propose(&self, changelog: ChangeLog) -> anyhow::Result<ClientResponse> {
        // Submit the changelog through Raft
        let result: ClientWriteResponse<TypeConfig> = self
            .raft
            .client_write(changelog)
            .await
            .map_err(|e| anyhow::anyhow!("Raft write failed: {}", e))?;

        Ok(result.data)
    }

    /// Get cluster metrics
    pub async fn metrics(&self) -> ClusterMetrics {
        let raft_metrics = self.raft.metrics().borrow().clone();
        let members = self.cached_members.read().await.clone();

        let state = match raft_metrics.state {
            openraft::ServerState::Leader => "leader",
            openraft::ServerState::Follower => "follower",
            openraft::ServerState::Candidate => "candidate",
            openraft::ServerState::Learner => "learner",
            openraft::ServerState::Shutdown => "shutdown",
        };

        ClusterMetrics {
            id: self.config.node_id,
            mode: self.config.mode,
            is_leader: raft_metrics.current_leader == Some(self.config.node_id),
            current_term: Some(raft_metrics.vote.leader_id().term),
            last_applied: raft_metrics.last_applied.map(|l| l.index),
            committed: raft_metrics.last_applied.map(|l| l.index), // Use last_applied as committed approximation
            members,
            state: state.to_string(),
        }
    }

    /// Add a new node to the cluster (leader only)
    pub async fn add_node(&self, node_id: NodeId, addr: String) -> anyhow::Result<()> {
        info!("Adding node {} at {} to cluster", node_id, addr);

        let node = BasicNode { addr: addr.clone() };
        let change = ChangeMembers::AddNodes([(node_id, node)].into_iter().collect());

        self.raft
            .change_membership(change, false)
            .await
            .map_err(|e| anyhow::anyhow!("Failed to add node: {}", e))?;

        // Update cached members
        let mut cached = self.cached_members.write().await;
        if !cached.iter().any(|m| m.id == node_id) {
            cached.push(ClusterMember {
                id: node_id,
                addr,
                admin_port: None,
                tls: false,
            });
        }

        Ok(())
    }

    /// Remove a node from the cluster (leader only)
    pub async fn remove_node(&self, node_id: NodeId) -> anyhow::Result<()> {
        info!("Removing node {} from cluster", node_id);

        let mut remove_set = BTreeSet::new();
        remove_set.insert(node_id);

        let change = ChangeMembers::RemoveNodes(remove_set);

        self.raft
            .change_membership(change, false)
            .await
            .map_err(|e| anyhow::anyhow!("Failed to remove node: {}", e))?;

        // Update cached members
        let mut cached = self.cached_members.write().await;
        cached.retain(|m| m.id != node_id);

        Ok(())
    }

    /// Apply a replicated changelog (called when receiving from Raft log, not external)
    /// This is used for backward compatibility with the simple replication mode
    #[allow(dead_code)]
    pub fn apply_replicated(&self, _changelog: &ChangeLog) -> ClientResponse {
        // In full Raft mode, changes are applied through the state machine
        // This method exists for API compatibility but shouldn't be called directly
        warn!("apply_replicated called directly - in Raft mode, use propose() instead");
        ClientResponse {
            success: false,
            message: Some("Use propose() for Raft mode".to_string()),
        }
    }
}

/// Cluster error types
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
