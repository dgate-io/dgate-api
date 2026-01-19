//! Tempo consensus implementation for DGate
//!
//! Tempo is a leaderless consensus protocol that provides multi-master
#![allow(dead_code)]

//!
//! replication with better scalability than Raft. Unlike Raft where only
//! the leader can accept writes, any node in a Tempo cluster can accept
//! and coordinate writes.
//!
//! # How it works
//!
//! Tempo uses logical timestamps and quorum-based coordination:
//!
//! 1. **Command Submission**: Any node can receive a command from a client
//! 2. **MCollect Phase**: The coordinator broadcasts the command with a timestamp
//!    to all nodes and collects acknowledgments with their votes/clocks
//! 3. **Fast Path**: If enough nodes respond quickly with matching clocks,
//!    the command can be committed immediately
//! 4. **Slow Path**: If the fast path fails, a slower consensus round is used
//! 5. **Commit**: Once committed, the command is applied and the result returned
//!
//! # Reference
//!
//! Based on: https://github.com/vitorenesduarte/fantoch
//!
//! Key differences from our implementation:
//! - We use HTTP for communication instead of custom RPC
//! - We simplify the protocol for the DGate use case
//! - We focus on changelog replication rather than general key-value operations

mod clock;
mod messages;
mod network;
mod state;

use std::collections::HashMap;
use std::sync::Arc;
use std::time::Duration;

use async_trait::async_trait;
use parking_lot::RwLock;
use tokio::sync::{mpsc, oneshot};
use tokio::time::timeout;
use tracing::{debug, error, info, warn};

use super::consensus::{Consensus, ConsensusMetrics, ConsensusResponse, NodeId, NodeState};
use super::discovery::NodeDiscovery;
use crate::config::{ClusterConfig, ClusterMember, ClusterMode, TempoConfig};
use crate::resources::ChangeLog;
use crate::storage::ProxyStore;

pub use clock::LogicalClock;
pub use messages::{TempoMessage, TempoMessageType};
pub use network::TempoNetwork;
pub use state::TempoState;

/// Dot represents a unique command identifier (node_id, sequence_number)
#[derive(Debug, Clone, Copy, Hash, Eq, PartialEq, serde::Serialize, serde::Deserialize)]
pub struct Dot {
    pub node_id: NodeId,
    pub sequence: u64,
}

impl Dot {
    pub fn new(node_id: NodeId, sequence: u64) -> Self {
        Self { node_id, sequence }
    }
}

/// Tempo consensus implementation
#[allow(dead_code)]
pub struct TempoConsensus {
    /// Node configuration
    config: ClusterConfig,
    /// Tempo-specific configuration
    tempo_config: TempoConfig,
    /// Current state
    state: Arc<RwLock<TempoState>>,
    /// Logical clock for this node
    clock: Arc<LogicalClock>,
    /// Network layer for communication
    network: Arc<TempoNetwork>,
    /// Store for applying changes
    store: Arc<ProxyStore>,
    /// Channel to notify proxy of applied changes
    change_tx: mpsc::UnboundedSender<ChangeLog>,
    /// Discovery service
    discovery: Option<Arc<NodeDiscovery>>,
    /// Cached members list
    members: RwLock<Vec<ClusterMember>>,
    /// Pending commands waiting for commit
    pending: RwLock<HashMap<Dot, oneshot::Sender<ConsensusResponse>>>,
    /// Next sequence number for this node
    next_sequence: RwLock<u64>,
    /// Shutdown signal
    shutdown_tx: RwLock<Option<mpsc::Sender<()>>>,
}

impl TempoConsensus {
    /// Create a new Tempo consensus manager
    pub async fn new(
        cluster_config: ClusterConfig,
        store: Arc<ProxyStore>,
        change_tx: mpsc::UnboundedSender<ChangeLog>,
    ) -> anyhow::Result<Arc<Self>> {
        let node_id = cluster_config.node_id;
        let tempo_config = cluster_config.tempo.clone().unwrap_or_default();

        info!(
            "Creating Tempo consensus for node {} at {}",
            node_id, cluster_config.advertise_addr
        );

        // Calculate quorum sizes based on cluster size
        let n = cluster_config.initial_members.len().max(1);
        let f = (n - 1) / 2; // Maximum failures tolerated

        let fast_quorum_size = tempo_config.fast_quorum_size.unwrap_or(f + 1);
        let write_quorum_size = tempo_config.write_quorum_size.unwrap_or(f + 1);

        info!(
            "Tempo quorum sizes: fast={}, write={} (n={}, f={})",
            fast_quorum_size, write_quorum_size, n, f
        );

        // Create logical clock
        let clock = Arc::new(LogicalClock::new(node_id));

        // Create state
        let state = Arc::new(RwLock::new(TempoState::new(
            node_id,
            fast_quorum_size,
            write_quorum_size,
        )));

        // Create network layer
        let network = Arc::new(TempoNetwork::new(node_id));

        // Setup discovery if configured
        let discovery = cluster_config
            .discovery
            .as_ref()
            .map(|disc_config| Arc::new(NodeDiscovery::new(disc_config.clone())));

        // Cache initial members
        let members = RwLock::new(cluster_config.initial_members.clone());

        let consensus = Arc::new(Self {
            config: cluster_config,
            tempo_config,
            state,
            clock,
            network,
            store,
            change_tx,
            discovery,
            members,
            pending: RwLock::new(HashMap::new()),
            next_sequence: RwLock::new(1),
            shutdown_tx: RwLock::new(None),
        });

        // Start background tasks
        consensus.start_background_tasks();

        Ok(consensus)
    }

    /// Start background tasks for Tempo
    fn start_background_tasks(self: &Arc<Self>) {
        let (shutdown_tx, mut shutdown_rx) = mpsc::channel::<()>(1);
        *self.shutdown_tx.write() = Some(shutdown_tx);

        // Start clock bump task
        {
            let this = Arc::clone(self);
            let interval = Duration::from_millis(this.tempo_config.clock_bump_interval_ms);
            tokio::spawn(async move {
                let mut ticker = tokio::time::interval(interval);
                loop {
                    tokio::select! {
                        _ = ticker.tick() => {
                            this.handle_clock_bump().await;
                        }
                        _ = shutdown_rx.recv() => {
                            info!("Tempo clock bump task shutting down");
                            break;
                        }
                    }
                }
            });
        }

        // Start message handler task
        {
            let this = Arc::clone(self);
            let mut rx = this.network.message_receiver();
            tokio::spawn(async move {
                while let Some((from, msg)) = rx.recv().await {
                    this.handle_message(from, msg).await;
                }
            });
        }

        // Start discovery task if configured
        if let Some(ref discovery) = self.discovery {
            let this = Arc::clone(self);
            let discovery = Arc::clone(discovery);
            tokio::spawn(async move {
                loop {
                    tokio::time::sleep(Duration::from_secs(10)).await;
                    let nodes = discovery.discover().await;
                    for (node_id, node) in nodes {
                        let mut members = this.members.write();
                        if !members.iter().any(|m| m.id == node_id) {
                            info!("Discovered new node {} at {}", node_id, node.addr);
                            members.push(ClusterMember {
                                id: node_id,
                                addr: node.addr,
                                admin_port: None,
                                tls: false,
                            });
                        }
                    }
                }
            });
        }
    }

    /// Handle periodic clock bump
    async fn handle_clock_bump(&self) {
        // Advance clock periodically to help with synchronization
        self.clock.tick();

        // Broadcast clock to other nodes
        let msg = TempoMessage {
            msg_type: TempoMessageType::ClockBump,
            dot: None,
            clock: Some(self.clock.current()),
            changelog: None,
            votes: None,
            quorum: None,
        };

        self.broadcast_message(msg).await;
    }

    /// Handle incoming message from another node
    async fn handle_message(&self, from: NodeId, msg: TempoMessage) {
        match msg.msg_type {
            TempoMessageType::MCollect => {
                self.handle_mcollect(from, msg).await;
            }
            TempoMessageType::MCollectAck => {
                self.handle_mcollect_ack(from, msg).await;
            }
            TempoMessageType::MCommit => {
                self.handle_mcommit(from, msg).await;
            }
            TempoMessageType::ClockBump => {
                if let Some(clock) = msg.clock {
                    self.clock.update(clock);
                }
            }
        }
    }

    /// Handle MCollect message (command broadcast from coordinator)
    async fn handle_mcollect(&self, from: NodeId, msg: TempoMessage) {
        let dot = match msg.dot {
            Some(d) => d,
            None => return,
        };

        let changelog = match msg.changelog {
            Some(c) => c,
            None => return,
        };

        let coord_clock = msg.clock.unwrap_or(0);

        debug!(
            "Received MCollect from {} for {:?} with clock {}",
            from, dot, coord_clock
        );

        // Update our clock
        self.clock.update(coord_clock);
        let my_clock = self.clock.tick();

        // Store command info
        {
            let mut state = self.state.write();
            state.add_command(dot, changelog.clone(), coord_clock);
        }

        // Send acknowledgment back
        let ack = TempoMessage {
            msg_type: TempoMessageType::MCollectAck,
            dot: Some(dot),
            clock: Some(my_clock),
            changelog: None,
            votes: None,
            quorum: None,
        };

        self.send_message(from, ack).await;
    }

    /// Handle MCollectAck message (acknowledgment from a node)
    async fn handle_mcollect_ack(&self, from: NodeId, msg: TempoMessage) {
        let dot = match msg.dot {
            Some(d) => d,
            None => return,
        };

        let their_clock = msg.clock.unwrap_or(0);

        debug!(
            "Received MCollectAck from {} for {:?} with clock {}",
            from, dot, their_clock
        );

        // Update our clock
        self.clock.update(their_clock);

        // Record the ack
        let should_commit = {
            let mut state = self.state.write();
            state.add_ack(dot, from, their_clock)
        };

        // If we have enough acks, commit the command
        if should_commit {
            self.commit_command(dot).await;
        }
    }

    /// Handle MCommit message (command is committed)
    async fn handle_mcommit(&self, from: NodeId, msg: TempoMessage) {
        let dot = match msg.dot {
            Some(d) => d,
            None => return,
        };

        let changelog = match msg.changelog {
            Some(c) => c,
            None => return,
        };

        let commit_clock = msg.clock.unwrap_or(0);

        debug!(
            "Received MCommit from {} for {:?} with clock {}",
            from, dot, commit_clock
        );

        // Update our clock
        self.clock.update(commit_clock);

        // Apply the changelog if we haven't already
        let already_applied = {
            let state = self.state.read();
            state.is_committed(&dot)
        };

        if !already_applied {
            // Apply to storage
            if let Err(e) = self.apply_changelog(&changelog) {
                error!("Failed to apply changelog from MCommit: {}", e);
            }

            // Mark as committed
            {
                let mut state = self.state.write();
                state.mark_committed(dot);
            }
        }
    }

    /// Commit a command after receiving enough acks
    async fn commit_command(&self, dot: Dot) {
        let (changelog, commit_clock) = {
            let mut state = self.state.write();

            // First get the changelog and clock, then mark as committed
            let result = if let Some(cmd) = state.get_command(&dot) {
                if !cmd.committed {
                    (Some(cmd.changelog.clone()), cmd.clock)
                } else {
                    (None, 0)
                }
            } else {
                (None, 0)
            };

            // Now mark as committed if we have a changelog
            if result.0.is_some() {
                state.mark_committed(dot);
            }

            result
        };

        if let Some(changelog) = changelog {
            // Apply locally
            let result = self.apply_changelog(&changelog);

            // Notify pending waiter
            if let Some(sender) = self.pending.write().remove(&dot) {
                let response = match result {
                    Ok(msg) => ConsensusResponse::ok_with_message(msg),
                    Err(e) => ConsensusResponse::error(e),
                };
                let _ = sender.send(response);
            }

            // Broadcast commit to other nodes
            let msg = TempoMessage {
                msg_type: TempoMessageType::MCommit,
                dot: Some(dot),
                clock: Some(commit_clock),
                changelog: Some(changelog),
                votes: None,
                quorum: None,
            };

            self.broadcast_message(msg).await;
        }
    }

    /// Apply a changelog to storage
    fn apply_changelog(&self, changelog: &ChangeLog) -> Result<String, String> {
        use crate::resources::*;

        let result = match changelog.cmd {
            ChangeCommand::AddNamespace => {
                let ns: Namespace =
                    serde_json::from_value(changelog.item.clone()).map_err(|e| e.to_string())?;
                self.store.set_namespace(&ns).map_err(|e| e.to_string())?;
                format!("Namespace '{}' created", ns.name)
            }
            ChangeCommand::DeleteNamespace => {
                self.store
                    .delete_namespace(&changelog.name)
                    .map_err(|e| e.to_string())?;
                format!("Namespace '{}' deleted", changelog.name)
            }
            ChangeCommand::AddRoute => {
                let route: Route =
                    serde_json::from_value(changelog.item.clone()).map_err(|e| e.to_string())?;
                self.store.set_route(&route).map_err(|e| e.to_string())?;
                format!("Route '{}' created", route.name)
            }
            ChangeCommand::DeleteRoute => {
                self.store
                    .delete_route(&changelog.namespace, &changelog.name)
                    .map_err(|e| e.to_string())?;
                format!("Route '{}' deleted", changelog.name)
            }
            ChangeCommand::AddService => {
                let service: Service =
                    serde_json::from_value(changelog.item.clone()).map_err(|e| e.to_string())?;
                self.store
                    .set_service(&service)
                    .map_err(|e| e.to_string())?;
                format!("Service '{}' created", service.name)
            }
            ChangeCommand::DeleteService => {
                self.store
                    .delete_service(&changelog.namespace, &changelog.name)
                    .map_err(|e| e.to_string())?;
                format!("Service '{}' deleted", changelog.name)
            }
            ChangeCommand::AddModule => {
                let module: Module =
                    serde_json::from_value(changelog.item.clone()).map_err(|e| e.to_string())?;
                self.store.set_module(&module).map_err(|e| e.to_string())?;
                format!("Module '{}' created", module.name)
            }
            ChangeCommand::DeleteModule => {
                self.store
                    .delete_module(&changelog.namespace, &changelog.name)
                    .map_err(|e| e.to_string())?;
                format!("Module '{}' deleted", changelog.name)
            }
            ChangeCommand::AddDomain => {
                let domain: Domain =
                    serde_json::from_value(changelog.item.clone()).map_err(|e| e.to_string())?;
                self.store.set_domain(&domain).map_err(|e| e.to_string())?;
                format!("Domain '{}' created", domain.name)
            }
            ChangeCommand::DeleteDomain => {
                self.store
                    .delete_domain(&changelog.namespace, &changelog.name)
                    .map_err(|e| e.to_string())?;
                format!("Domain '{}' deleted", changelog.name)
            }
            ChangeCommand::AddSecret => {
                let secret: Secret =
                    serde_json::from_value(changelog.item.clone()).map_err(|e| e.to_string())?;
                self.store.set_secret(&secret).map_err(|e| e.to_string())?;
                format!("Secret '{}' created", secret.name)
            }
            ChangeCommand::DeleteSecret => {
                self.store
                    .delete_secret(&changelog.namespace, &changelog.name)
                    .map_err(|e| e.to_string())?;
                format!("Secret '{}' deleted", changelog.name)
            }
            ChangeCommand::AddCollection => {
                let collection: Collection =
                    serde_json::from_value(changelog.item.clone()).map_err(|e| e.to_string())?;
                self.store
                    .set_collection(&collection)
                    .map_err(|e| e.to_string())?;
                format!("Collection '{}' created", collection.name)
            }
            ChangeCommand::DeleteCollection => {
                self.store
                    .delete_collection(&changelog.namespace, &changelog.name)
                    .map_err(|e| e.to_string())?;
                format!("Collection '{}' deleted", changelog.name)
            }
            ChangeCommand::AddDocument => {
                let document: Document =
                    serde_json::from_value(changelog.item.clone()).map_err(|e| e.to_string())?;
                self.store
                    .set_document(&document)
                    .map_err(|e| e.to_string())?;
                format!("Document '{}' created", document.id)
            }
            ChangeCommand::DeleteDocument => {
                let doc: Document =
                    serde_json::from_value(changelog.item.clone()).map_err(|e| e.to_string())?;
                self.store
                    .delete_document(&changelog.namespace, &doc.collection, &changelog.name)
                    .map_err(|e| e.to_string())?;
                format!("Document '{}' deleted", changelog.name)
            }
        };

        // Notify proxy about the change
        if let Err(e) = self.change_tx.send(changelog.clone()) {
            error!("Failed to notify proxy of change: {}", e);
        }

        Ok(result)
    }

    /// Send a message to a specific node
    async fn send_message(&self, to: NodeId, msg: TempoMessage) {
        let addr = {
            let members = self.members.read();
            members.iter().find(|m| m.id == to).map(|m| {
                // Use admin port if available, otherwise derive from addr
                if let Some(admin_port) = m.admin_port {
                    let host = m.addr.split(':').next().unwrap_or("127.0.0.1");
                    format!("{}:{}", host, admin_port)
                } else {
                    m.addr.clone()
                }
            })
        };
        if let Some(addr) = addr {
            self.network.send(&addr, msg).await;
        }
    }

    /// Broadcast a message to all nodes
    async fn broadcast_message(&self, msg: TempoMessage) {
        let members = self.members.read().clone();
        for member in members {
            if member.id != self.config.node_id {
                // Use admin port if available, otherwise derive from addr
                let addr = if let Some(admin_port) = member.admin_port {
                    let host = member.addr.split(':').next().unwrap_or("127.0.0.1");
                    format!("{}:{}", host, admin_port)
                } else {
                    member.addr.clone()
                };
                self.network.send(&addr, msg.clone()).await;
            }
        }
    }

    /// Get the next sequence number for this node
    fn next_dot(&self) -> Dot {
        let mut seq = self.next_sequence.write();
        let dot = Dot::new(self.config.node_id, *seq);
        *seq += 1;
        dot
    }

    /// Handle an incoming message from the HTTP endpoint
    pub fn handle_incoming_message(&self, from: NodeId, msg: TempoMessage) {
        self.network.handle_incoming(from, msg);
    }
}

#[async_trait]
impl Consensus for TempoConsensus {
    fn node_id(&self) -> NodeId {
        self.config.node_id
    }

    fn mode(&self) -> ClusterMode {
        // Return the configured mode (could be Simple or Tempo)
        self.config.mode
    }

    async fn initialize(&self) -> anyhow::Result<()> {
        info!(
            "Initializing Tempo consensus for node {} at {}",
            self.config.node_id, self.config.advertise_addr
        );

        // Register with other nodes if we have initial members
        for member in &self.config.initial_members {
            if member.id != self.config.node_id {
                // Try to announce ourselves
                let msg = TempoMessage {
                    msg_type: TempoMessageType::ClockBump,
                    dot: None,
                    clock: Some(self.clock.current()),
                    changelog: None,
                    votes: None,
                    quorum: None,
                };
                self.network.send(&member.addr, msg).await;
            }
        }

        Ok(())
    }

    async fn can_write(&self) -> bool {
        // In Tempo, any node can accept writes (multi-master)
        true
    }

    async fn leader_id(&self) -> Option<NodeId> {
        // Tempo is leaderless
        None
    }

    async fn propose(&self, changelog: ChangeLog) -> anyhow::Result<ConsensusResponse> {
        let dot = self.next_dot();
        let clock = self.clock.tick();

        debug!("Proposing command {:?} with clock {}", dot, clock);

        // Create a channel to wait for commit
        let (tx, rx) = oneshot::channel();
        self.pending.write().insert(dot, tx);

        // Store command locally
        {
            let mut state = self.state.write();
            state.add_command(dot, changelog.clone(), clock);
            // Add our own ack
            state.add_ack(dot, self.config.node_id, clock);
        }

        // Broadcast MCollect to all nodes
        let msg = TempoMessage {
            msg_type: TempoMessageType::MCollect,
            dot: Some(dot),
            clock: Some(clock),
            changelog: Some(changelog.clone()),
            votes: None,
            quorum: None,
        };

        self.broadcast_message(msg).await;

        // Check if we already have enough acks (single node case)
        let should_commit = {
            let state = self.state.read();
            state.has_quorum(&dot)
        };

        if should_commit {
            self.commit_command(dot).await;
        }

        // Wait for commit with timeout
        match timeout(Duration::from_secs(5), rx).await {
            Ok(Ok(response)) => Ok(response),
            Ok(Err(_)) => {
                warn!("Commit channel closed for {:?}", dot);
                Err(anyhow::anyhow!("Commit channel closed"))
            }
            Err(_) => {
                // Timeout - the command might still commit eventually
                // For now, treat as error
                self.pending.write().remove(&dot);
                Err(anyhow::anyhow!("Command timed out waiting for quorum"))
            }
        }
    }

    async fn metrics(&self) -> ConsensusMetrics {
        let state = self.state.read();
        let members = self.members.read().clone();

        ConsensusMetrics {
            id: self.config.node_id,
            mode: ClusterMode::Tempo,
            can_write: true, // Tempo allows any node to write
            leader_id: None, // Tempo is leaderless
            state: NodeState::Active,
            current_term: Some(self.clock.current()),
            last_applied: Some(state.last_applied_sequence()),
            committed: Some(state.committed_count()),
            members,
            extra: Some(serde_json::json!({
                "fast_quorum_size": state.fast_quorum_size,
                "write_quorum_size": state.write_quorum_size,
                "pending_commands": state.pending_count(),
            })),
        }
    }

    async fn add_node(&self, node_id: NodeId, addr: String) -> anyhow::Result<()> {
        info!("Adding node {} at {} to Tempo cluster", node_id, addr);

        let mut members = self.members.write();
        if !members.iter().any(|m| m.id == node_id) {
            members.push(ClusterMember {
                id: node_id,
                addr,
                admin_port: None,
                tls: false,
            });
        }

        Ok(())
    }

    async fn remove_node(&self, node_id: NodeId) -> anyhow::Result<()> {
        info!("Removing node {} from Tempo cluster", node_id);

        let mut members = self.members.write();
        members.retain(|m| m.id != node_id);

        Ok(())
    }

    async fn members(&self) -> Vec<ClusterMember> {
        self.members.read().clone()
    }

    async fn shutdown(&self) -> anyhow::Result<()> {
        info!("Shutting down Tempo consensus");

        // Signal shutdown to background tasks
        let tx = self.shutdown_tx.write().take();
        if let Some(tx) = tx {
            let _ = tx.send(()).await;
        }

        Ok(())
    }
}
