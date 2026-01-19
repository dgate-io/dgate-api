//! Raft consensus implementation for DGate
//!
//! This module provides a full Raft consensus implementation using openraft,
//! with leader election, log replication, and snapshot transfer.
//!
//! # Architecture
//!
//! - `RaftConsensus`: Main Raft wrapper implementing the Consensus trait
//! - `RaftLogStore`: In-memory log storage
//! - `RaftStateMachine`: State machine for applying logs
//! - `RaftNetwork`: HTTP-based networking between nodes

mod network;
mod state_machine;
mod store;

use std::collections::BTreeMap;
use std::collections::BTreeSet;
use std::io::Cursor;
use std::sync::Arc;
use std::time::Duration;

use async_trait::async_trait;
use openraft::raft::ClientWriteResponse;
use openraft::Config as RaftConfig;
use openraft::{BasicNode, ChangeMembers, Raft};
use serde::{Deserialize, Serialize};
use tokio::sync::{mpsc, RwLock};
use tracing::{debug, info, warn};

use super::consensus::{Consensus, ConsensusMetrics, ConsensusResponse, NodeId, NodeState};
use super::discovery::NodeDiscovery;
use crate::config::{ClusterConfig, ClusterMember, ClusterMode};
use crate::resources::ChangeLog;
use crate::storage::ProxyStore;

pub use network::NetworkFactory;
pub use state_machine::DGateStateMachine;
pub use store::RaftLogStore;

// Raft type configuration using openraft's declarative macro
openraft::declare_raft_types!(
    pub TypeConfig:
        D = ChangeLog,
        R = RaftClientResponse,
        Node = BasicNode,
);

/// Response from the Raft state machine after applying a log entry
#[derive(Debug, Clone, Serialize, Deserialize, Default)]
pub struct RaftClientResponse {
    pub success: bool,
    pub message: Option<String>,
}

impl From<RaftClientResponse> for ConsensusResponse {
    fn from(r: RaftClientResponse) -> Self {
        ConsensusResponse {
            success: r.success,
            message: r.message,
        }
    }
}

/// Snapshot data for state machine
#[derive(Debug, Clone, Serialize, Deserialize, Default)]
pub struct SnapshotData {
    pub changelogs: Vec<ChangeLog>,
}

/// The Raft instance type alias for convenience
pub type DGateRaft = Raft<TypeConfig>;

/// Raft consensus implementation
pub struct RaftConsensus {
    /// Configuration
    config: ClusterConfig,
    /// The Raft instance
    raft: Arc<DGateRaft>,
    /// Node discovery service
    discovery: Option<Arc<NodeDiscovery>>,
    /// Cached members list
    cached_members: RwLock<Vec<ClusterMember>>,
}

impl RaftConsensus {
    /// Create a new Raft consensus manager
    pub async fn new(
        cluster_config: ClusterConfig,
        store: Arc<ProxyStore>,
        change_tx: mpsc::UnboundedSender<ChangeLog>,
    ) -> anyhow::Result<Self> {
        let node_id = cluster_config.node_id;

        info!(
            "Creating Raft consensus for node {} at {}",
            node_id, cluster_config.advertise_addr
        );

        // Create Raft configuration
        let raft_config = RaftConfig {
            cluster_name: "dgate-cluster".to_string(),
            heartbeat_interval: 200,
            election_timeout_min: 500,
            election_timeout_max: 1000,
            snapshot_policy: openraft::SnapshotPolicy::LogsSinceLast(1000),
            max_in_snapshot_log_to_keep: 100,
            ..Default::default()
        };

        let raft_config = Arc::new(raft_config.validate()?);

        // Create log store
        let log_store = RaftLogStore::new();

        // Create network factory
        let network_factory = NetworkFactory::new();

        // Create state machine
        let state_machine = DGateStateMachine::with_change_notifier(store, change_tx);

        // Create the Raft instance
        let raft = Raft::new(
            node_id,
            raft_config,
            network_factory,
            log_store,
            state_machine,
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

    /// Get the Raft instance for direct access (e.g., for handling RPC requests)
    pub fn raft(&self) -> &Arc<DGateRaft> {
        &self.raft
    }

    /// Bootstrap this node as a single-node cluster
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
                info!("Successfully bootstrapped single-node Raft cluster");
                Ok(())
            }
            Err(e) => {
                if e.to_string().contains("already initialized") {
                    debug!("Raft cluster already initialized");
                    Ok(())
                } else {
                    Err(e.into())
                }
            }
        }
    }

    /// Bootstrap as the initial leader
    ///
    /// When bootstrapping, we include ALL initial members in the initial cluster.
    /// This avoids the complexity and race conditions of adding nodes one by one.
    async fn bootstrap_cluster(&self) -> anyhow::Result<()> {
        let node_id = self.config.node_id;
        let mut members = BTreeMap::new();

        // Add this node first
        members.insert(
            node_id,
            BasicNode {
                addr: self.config.advertise_addr.clone(),
            },
        );

        // Add all other initial members
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

        info!(
            "Bootstrapping Raft cluster with {} members (node {} as leader)",
            members.len(),
            node_id
        );

        match self.raft.initialize(members).await {
            Ok(_) => {
                info!("Successfully bootstrapped Raft cluster");
                Ok(())
            }
            Err(e) => {
                if e.to_string().contains("already initialized") {
                    debug!("Raft cluster already initialized");
                    Ok(())
                } else {
                    Err(e.into())
                }
            }
        }
    }

    /// Join an existing cluster
    ///
    /// Since the bootstrap node initializes the cluster with all members,
    /// non-bootstrap nodes just need to wait for the leader to replicate to them.
    /// The Raft protocol will handle the log synchronization automatically.
    async fn join_cluster(&self) -> anyhow::Result<()> {
        let node_id = self.config.node_id;
        let my_addr = &self.config.advertise_addr;

        info!(
            "Node {} at {} waiting to receive Raft replication from leader",
            node_id, my_addr
        );

        // The node is already included in the initial membership by the bootstrap node.
        // Raft will automatically handle log replication once the leader connects to us.
        // We just need to be ready to receive append_entries RPCs.

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
                let metrics = raft.metrics().borrow().clone();
                if metrics.current_leader == Some(my_node_id) {
                    // Step 1: Add as learner
                    let add_learner =
                        ChangeMembers::AddNodes([(node_id, node.clone())].into_iter().collect());

                    match raft.change_membership(add_learner, false).await {
                        Ok(_) => {
                            info!("Added discovered node {} as learner", node_id);
                            // Step 2: Promote to voter
                            let mut voter_ids = BTreeSet::new();
                            voter_ids.insert(node_id);
                            let promote = ChangeMembers::AddVoterIds(voter_ids);
                            match raft.change_membership(promote, false).await {
                                Ok(_) => info!("Promoted discovered node {} to voter", node_id),
                                Err(e) => debug!("Could not promote node {}: {}", node_id, e),
                            }
                        }
                        Err(e) => debug!("Could not add node {}: {}", node_id, e),
                    }
                }
            }

            tokio::time::sleep(Duration::from_secs(10)).await;
        }
    }
}

#[async_trait]
impl Consensus for RaftConsensus {
    fn node_id(&self) -> NodeId {
        self.config.node_id
    }

    fn mode(&self) -> ClusterMode {
        self.config.mode
    }

    async fn initialize(&self) -> anyhow::Result<()> {
        let node_id = self.config.node_id;

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

    async fn can_write(&self) -> bool {
        // In Raft, only the leader can accept writes
        let metrics = self.raft.metrics().borrow().clone();
        metrics.current_leader == Some(self.config.node_id)
    }

    async fn leader_id(&self) -> Option<NodeId> {
        self.raft.metrics().borrow().current_leader
    }

    async fn propose(&self, changelog: ChangeLog) -> anyhow::Result<ConsensusResponse> {
        let result: ClientWriteResponse<TypeConfig> = self
            .raft
            .client_write(changelog)
            .await
            .map_err(|e| anyhow::anyhow!("Raft write failed: {}", e))?;

        Ok(result.data.into())
    }

    async fn metrics(&self) -> ConsensusMetrics {
        let raft_metrics = self.raft.metrics().borrow().clone();
        let members = self.cached_members.read().await.clone();

        let state = match raft_metrics.state {
            openraft::ServerState::Leader => NodeState::Leader,
            openraft::ServerState::Follower => NodeState::Follower,
            openraft::ServerState::Candidate => NodeState::Candidate,
            openraft::ServerState::Learner => NodeState::Learner,
            openraft::ServerState::Shutdown => NodeState::Shutdown,
        };

        ConsensusMetrics {
            id: self.config.node_id,
            mode: self.config.mode,
            can_write: raft_metrics.current_leader == Some(self.config.node_id),
            leader_id: raft_metrics.current_leader,
            state,
            current_term: Some(raft_metrics.vote.leader_id().term),
            last_applied: raft_metrics.last_applied.map(|l| l.index),
            committed: raft_metrics.last_applied.map(|l| l.index),
            members,
            extra: None,
        }
    }

    async fn add_node(&self, node_id: NodeId, addr: String) -> anyhow::Result<()> {
        info!("Adding node {} at {} to Raft cluster", node_id, addr);

        // Step 1: Add the node as a learner first
        let node = BasicNode { addr: addr.clone() };
        let add_learner = ChangeMembers::AddNodes([(node_id, node)].into_iter().collect());

        self.raft
            .change_membership(add_learner, false)
            .await
            .map_err(|e| anyhow::anyhow!("Failed to add node as learner: {}", e))?;

        info!("Node {} added as learner, now promoting to voter", node_id);

        // Step 2: Promote the learner to a voter
        let mut voter_ids = BTreeSet::new();
        voter_ids.insert(node_id);
        let promote_voter = ChangeMembers::AddVoterIds(voter_ids);

        self.raft
            .change_membership(promote_voter, false)
            .await
            .map_err(|e| anyhow::anyhow!("Failed to promote node to voter: {}", e))?;

        info!("Node {} successfully promoted to voter", node_id);

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

    async fn remove_node(&self, node_id: NodeId) -> anyhow::Result<()> {
        info!("Removing node {} from Raft cluster", node_id);

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

    async fn members(&self) -> Vec<ClusterMember> {
        self.cached_members.read().await.clone()
    }

    async fn shutdown(&self) -> anyhow::Result<()> {
        info!("Shutting down Raft consensus");
        self.raft
            .shutdown()
            .await
            .map_err(|e| anyhow::anyhow!("Shutdown failed: {}", e))?;
        Ok(())
    }
}
