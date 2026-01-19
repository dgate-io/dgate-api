//! Cluster module for DGate
//!
//! Provides replication for resources and documents across multiple DGate nodes
//! using pluggable consensus algorithms.
//!
//! # Supported Consensus Modes
//!
//! - **Simple**: HTTP-based replication where all nodes can accept writes
//! - **Raft**: Leader-based consensus with strong consistency (via openraft)
//! - **Tempo**: Leaderless multi-master consensus with better scalability
//!
//! # Architecture
//!
//! This module uses a trait-based design to allow different consensus algorithms:
//!
//! - `Consensus` trait: Common interface for all consensus implementations
//! - `ClusterManager`: High-level cluster operations facade
//! - `raft/`: Full Raft consensus implementation
//! - `tempo/`: Tempo multi-master consensus implementation

mod consensus;
mod discovery;
pub mod raft;
pub mod tempo;

use std::sync::Arc;

use tokio::sync::mpsc;
use tracing::info;

use crate::config::{ClusterConfig, ClusterMember, ClusterMode};
use crate::resources::ChangeLog;
use crate::storage::ProxyStore;

// Re-export consensus types
pub use consensus::{Consensus, ConsensusResponse, NodeId};

// Re-export Raft types for backward compatibility and admin API
pub use raft::{DGateRaft, TypeConfig};

// Tempo types are available via the tempo module
// pub use tempo::TempoMessage;

/// Cluster manager handles all cluster operations using the configured consensus algorithm
pub struct ClusterManager {
    /// The underlying consensus implementation
    consensus: Arc<dyn Consensus>,
    /// Direct access to Raft instance (for Raft RPC handlers)
    raft_instance: Option<Arc<raft::RaftConsensus>>,
    /// Direct access to Tempo instance (for Tempo message handlers)
    #[allow(dead_code)]
    tempo_instance: Option<Arc<tempo::TempoConsensus>>,
}

impl ClusterManager {
    /// Create a new cluster manager with the appropriate consensus algorithm
    pub async fn new(
        cluster_config: ClusterConfig,
        store: Arc<ProxyStore>,
        change_tx: mpsc::UnboundedSender<ChangeLog>,
    ) -> anyhow::Result<Self> {
        let mode = cluster_config.mode;

        info!(
            "Creating cluster manager for node {} at {} (mode: {:?})",
            cluster_config.node_id, cluster_config.advertise_addr, mode
        );

        match mode {
            ClusterMode::Simple => {
                // Simple mode uses Tempo for multi-master writes with direct HTTP replication
                // This is simpler than Raft (no leader election) and allows all nodes to accept writes
                let tempo = tempo::TempoConsensus::new(cluster_config, store, change_tx).await?;
                Ok(Self {
                    consensus: tempo.clone(),
                    raft_instance: None,
                    tempo_instance: Some(tempo),
                })
            }
            ClusterMode::Raft => {
                let raft =
                    Arc::new(raft::RaftConsensus::new(cluster_config, store, change_tx).await?);
                Ok(Self {
                    consensus: raft.clone(),
                    raft_instance: Some(raft),
                    tempo_instance: None,
                })
            }
            ClusterMode::Tempo => {
                let tempo = tempo::TempoConsensus::new(cluster_config, store, change_tx).await?;
                Ok(Self {
                    consensus: tempo.clone(),
                    raft_instance: None,
                    tempo_instance: Some(tempo),
                })
            }
        }
    }

    /// Initialize the cluster (bootstrap or join)
    pub async fn initialize(&self) -> anyhow::Result<()> {
        self.consensus.initialize().await
    }

    /// Get the cluster mode
    pub fn mode(&self) -> ClusterMode {
        self.consensus.mode()
    }

    /// Check if this node can accept write requests
    pub async fn is_leader(&self) -> bool {
        self.consensus.can_write().await
    }

    /// Get the current leader ID (None for Tempo)
    pub async fn leader_id(&self) -> Option<NodeId> {
        self.consensus.leader_id().await
    }

    /// Get the Raft instance for direct access (for RPC handlers)
    /// Returns None if not in Raft mode
    pub fn raft(&self) -> Option<&Arc<DGateRaft>> {
        self.raft_instance.as_ref().map(|r| r.raft())
    }

    /// Get the Tempo instance for direct access (for message handlers)
    /// Returns None if not in Tempo mode
    pub fn tempo(&self) -> Option<&Arc<tempo::TempoConsensus>> {
        self.tempo_instance.as_ref()
    }

    /// Propose a change log to the cluster via the consensus algorithm
    pub async fn propose(&self, changelog: ChangeLog) -> anyhow::Result<ConsensusResponse> {
        self.consensus.propose(changelog).await
    }

    /// Get cluster metrics
    pub async fn metrics(&self) -> ClusterMetrics {
        let consensus_metrics = self.consensus.metrics().await;

        ClusterMetrics {
            id: consensus_metrics.id,
            mode: consensus_metrics.mode,
            is_leader: consensus_metrics.can_write,
            current_term: consensus_metrics.current_term,
            last_applied: consensus_metrics.last_applied,
            committed: consensus_metrics.committed,
            members: consensus_metrics.members,
            state: consensus_metrics.state.to_string(),
        }
    }

    /// Add a new node to the cluster
    pub async fn add_node(&self, node_id: NodeId, addr: String) -> anyhow::Result<()> {
        self.consensus.add_node(node_id, addr).await
    }

    /// Remove a node from the cluster
    pub async fn remove_node(&self, node_id: NodeId) -> anyhow::Result<()> {
        self.consensus.remove_node(node_id).await
    }
}

/// Cluster metrics for admin API (backward compatible structure)
#[derive(Debug, Clone, serde::Serialize)]
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

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_consensus_response() {
        let ok = ConsensusResponse::ok();
        assert!(ok.success);
        assert!(ok.message.is_none());

        let ok_msg = ConsensusResponse::ok_with_message("done");
        assert!(ok_msg.success);
        assert_eq!(ok_msg.message, Some("done".to_string()));

        let err = ConsensusResponse::error("failed");
        assert!(!err.success);
        assert_eq!(err.message, Some("failed".to_string()));
    }
}
