//! Consensus trait defining the common interface for cluster consensus algorithms
//!
//! This module provides the abstraction layer for different consensus algorithms
//! (Raft, Tempo) to be used interchangeably in DGate's cluster mode.

#![allow(dead_code)]

use async_trait::async_trait;
use serde::{Deserialize, Serialize};
use std::sync::Arc;

use crate::config::{ClusterConfig, ClusterMember, ClusterMode};
use crate::resources::ChangeLog;

/// Node ID type used across all consensus implementations
pub type NodeId = u64;

/// Response from the consensus layer after proposing a change
#[derive(Debug, Clone, Serialize, Deserialize, Default)]
pub struct ConsensusResponse {
    pub success: bool,
    pub message: Option<String>,
}

impl ConsensusResponse {
    pub fn ok() -> Self {
        Self {
            success: true,
            message: None,
        }
    }

    pub fn ok_with_message(msg: impl Into<String>) -> Self {
        Self {
            success: true,
            message: Some(msg.into()),
        }
    }

    pub fn error(msg: impl Into<String>) -> Self {
        Self {
            success: false,
            message: Some(msg.into()),
        }
    }
}

/// Node state in the cluster
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "lowercase")]
pub enum NodeState {
    /// Node is the leader (Raft) or coordinator (Tempo)
    Leader,
    /// Node is a follower (Raft)
    Follower,
    /// Node is a candidate during election (Raft)
    Candidate,
    /// Node is a learner, not yet a full member
    Learner,
    /// Node is shutting down
    Shutdown,
    /// Node is active (Tempo - all nodes can accept writes)
    Active,
}

impl Default for NodeState {
    fn default() -> Self {
        Self::Follower
    }
}

impl std::fmt::Display for NodeState {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            NodeState::Leader => write!(f, "leader"),
            NodeState::Follower => write!(f, "follower"),
            NodeState::Candidate => write!(f, "candidate"),
            NodeState::Learner => write!(f, "learner"),
            NodeState::Shutdown => write!(f, "shutdown"),
            NodeState::Active => write!(f, "active"),
        }
    }
}

/// Cluster metrics for admin API and monitoring
#[derive(Debug, Clone, Serialize)]
pub struct ConsensusMetrics {
    /// This node's ID
    pub id: NodeId,
    /// Consensus mode
    pub mode: ClusterMode,
    /// Whether this node can accept writes
    pub can_write: bool,
    /// Current leader ID (None for Tempo or if unknown)
    pub leader_id: Option<NodeId>,
    /// Current node state
    pub state: NodeState,
    /// Current term/epoch (consensus algorithm specific)
    pub current_term: Option<u64>,
    /// Last applied log index
    pub last_applied: Option<u64>,
    /// Committed log index
    pub committed: Option<u64>,
    /// Cluster members
    pub members: Vec<ClusterMember>,
    /// Algorithm-specific metrics
    #[serde(skip_serializing_if = "Option::is_none")]
    pub extra: Option<serde_json::Value>,
}

/// The main consensus trait that both Raft and Tempo implement
#[async_trait]
pub trait Consensus: Send + Sync {
    /// Get the node ID
    fn node_id(&self) -> NodeId;

    /// Get the consensus mode
    fn mode(&self) -> ClusterMode;

    /// Initialize the consensus algorithm (bootstrap or join cluster)
    async fn initialize(&self) -> anyhow::Result<()>;

    /// Check if this node can accept write requests
    /// For Raft: only the leader can accept writes
    /// For Tempo: any node can accept writes (multi-master)
    async fn can_write(&self) -> bool;

    /// Get the current leader ID (if applicable)
    /// Returns None for Tempo (leaderless) or if leader is unknown
    async fn leader_id(&self) -> Option<NodeId>;

    /// Propose a change to be replicated across the cluster
    async fn propose(&self, changelog: ChangeLog) -> anyhow::Result<ConsensusResponse>;

    /// Get cluster metrics
    async fn metrics(&self) -> ConsensusMetrics;

    /// Add a new node to the cluster
    async fn add_node(&self, node_id: NodeId, addr: String) -> anyhow::Result<()>;

    /// Remove a node from the cluster
    async fn remove_node(&self, node_id: NodeId) -> anyhow::Result<()>;

    /// Get the current members of the cluster
    async fn members(&self) -> Vec<ClusterMember>;

    /// Shutdown the consensus algorithm gracefully
    async fn shutdown(&self) -> anyhow::Result<()>;
}

/// Factory function type for creating consensus implementations
pub type ConsensusFactory = Arc<
    dyn Fn(ClusterConfig, Arc<crate::storage::ProxyStore>) -> anyhow::Result<Arc<dyn Consensus>>
        + Send
        + Sync,
>;

/// Consensus error types
#[derive(Debug, thiserror::Error)]
pub enum ConsensusError {
    #[error("Not leader, current leader is: {0:?}")]
    NotLeader(Option<NodeId>),

    #[error("Consensus error: {0}")]
    Protocol(String),

    #[error("Network error: {0}")]
    Network(String),

    #[error("Storage error: {0}")]
    Storage(String),

    #[error("Configuration error: {0}")]
    Config(String),

    #[error("Node not found: {0}")]
    NodeNotFound(NodeId),

    #[error("Timeout: {0}")]
    Timeout(String),
}
