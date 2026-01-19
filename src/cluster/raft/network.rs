//! Raft network implementation for DGate
//!
//! Provides HTTP-based communication between Raft nodes for voting,
//! log replication, and snapshot transfer.

use openraft::error::{InstallSnapshotError, RPCError, RaftError};
use openraft::network::{RPCOption, RaftNetwork as RaftNetworkTrait, RaftNetworkFactory};
use openraft::raft::{
    AppendEntriesRequest, AppendEntriesResponse, InstallSnapshotRequest, InstallSnapshotResponse,
    VoteRequest, VoteResponse,
};
use openraft::BasicNode;
use reqwest::Client;
use tracing::{debug, warn};

use super::{NodeId, TypeConfig};

/// Network factory for creating Raft network connections
pub struct NetworkFactory {
    client: Client,
}

impl NetworkFactory {
    pub fn new() -> Self {
        let client = Client::builder()
            .pool_max_idle_per_host(10)
            .timeout(std::time::Duration::from_secs(10))
            .build()
            .expect("Failed to create HTTP client");

        Self { client }
    }
}

impl Default for NetworkFactory {
    fn default() -> Self {
        Self::new()
    }
}

impl RaftNetworkFactory<TypeConfig> for NetworkFactory {
    type Network = RaftNetwork;

    async fn new_client(&mut self, target: NodeId, node: &BasicNode) -> Self::Network {
        RaftNetwork {
            target,
            addr: node.addr.clone(),
            client: self.client.clone(),
        }
    }
}

/// Raft network connection to a single node
pub struct RaftNetwork {
    target: NodeId,
    addr: String,
    client: Client,
}

impl RaftNetwork {
    fn endpoint(&self, path: &str) -> String {
        format!("http://{}/raft{}", self.addr, path)
    }
}

impl RaftNetworkTrait<TypeConfig> for RaftNetwork {
    async fn append_entries(
        &mut self,
        req: AppendEntriesRequest<TypeConfig>,
        _option: RPCOption,
    ) -> Result<AppendEntriesResponse<NodeId>, RPCError<NodeId, BasicNode, RaftError<NodeId>>> {
        debug!("Sending append_entries to node {}", self.target);

        let url = self.endpoint("/append");
        let resp = self
            .client
            .post(&url)
            .json(&req)
            .send()
            .await
            .map_err(|e| RPCError::Network(openraft::error::NetworkError::new(&e)))?;

        if !resp.status().is_success() {
            let status = resp.status();
            let body = resp.text().await.unwrap_or_default();
            warn!(
                "append_entries failed to node {}: {} - {}",
                self.target, status, body
            );
            return Err(RPCError::Network(openraft::error::NetworkError::new(
                &std::io::Error::other(format!("HTTP {}: {}", status, body)),
            )));
        }

        let result: AppendEntriesResponse<NodeId> = resp
            .json()
            .await
            .map_err(|e| RPCError::Network(openraft::error::NetworkError::new(&e)))?;

        Ok(result)
    }

    async fn install_snapshot(
        &mut self,
        req: InstallSnapshotRequest<TypeConfig>,
        _option: RPCOption,
    ) -> Result<
        InstallSnapshotResponse<NodeId>,
        RPCError<NodeId, BasicNode, RaftError<NodeId, InstallSnapshotError>>,
    > {
        debug!("Sending install_snapshot to node {}", self.target);

        let url = self.endpoint("/snapshot");
        let resp = self
            .client
            .post(&url)
            .json(&req)
            .send()
            .await
            .map_err(|e| RPCError::Network(openraft::error::NetworkError::new(&e)))?;

        if !resp.status().is_success() {
            let status = resp.status();
            let body = resp.text().await.unwrap_or_default();
            warn!(
                "install_snapshot failed to node {}: {} - {}",
                self.target, status, body
            );
            return Err(RPCError::Network(openraft::error::NetworkError::new(
                &std::io::Error::other(format!("HTTP {}: {}", status, body)),
            )));
        }

        let result: InstallSnapshotResponse<NodeId> = resp
            .json()
            .await
            .map_err(|e| RPCError::Network(openraft::error::NetworkError::new(&e)))?;

        Ok(result)
    }

    async fn vote(
        &mut self,
        req: VoteRequest<NodeId>,
        _option: RPCOption,
    ) -> Result<VoteResponse<NodeId>, RPCError<NodeId, BasicNode, RaftError<NodeId>>> {
        debug!("Sending vote request to node {}", self.target);

        let url = self.endpoint("/vote");
        let resp = self
            .client
            .post(&url)
            .json(&req)
            .send()
            .await
            .map_err(|e| RPCError::Network(openraft::error::NetworkError::new(&e)))?;

        if !resp.status().is_success() {
            let status = resp.status();
            let body = resp.text().await.unwrap_or_default();
            warn!("vote failed to node {}: {} - {}", self.target, status, body);
            return Err(RPCError::Network(openraft::error::NetworkError::new(
                &std::io::Error::other(format!("HTTP {}: {}", status, body)),
            )));
        }

        let result: VoteResponse<NodeId> = resp
            .json()
            .await
            .map_err(|e| RPCError::Network(openraft::error::NetworkError::new(&e)))?;

        Ok(result)
    }
}
