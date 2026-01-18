//! Raft state machine for DGate
//!
//! The state machine applies change logs to the local storage and maintains
//! snapshot state for Raft log compaction.

use std::io::Cursor;
use std::sync::Arc;

use openraft::storage::RaftStateMachine;
use openraft::{
    BasicNode, Entry, EntryPayload, LogId, OptionalSend, RaftSnapshotBuilder, Snapshot,
    SnapshotMeta, StorageError, StoredMembership,
};
use parking_lot::RwLock;
use tokio::sync::mpsc;
use tracing::{debug, error, info};

use super::{ClientResponse, NodeId, SnapshotData, TypeConfig};
use crate::resources::ChangeLog;
use crate::storage::ProxyStore;

/// State machine for applying Raft log entries
pub struct DGateStateMachine {
    /// The underlying storage
    store: Option<Arc<ProxyStore>>,
    /// Last applied log ID
    last_applied: RwLock<Option<LogId<NodeId>>>,
    /// Last membership configuration
    last_membership: RwLock<StoredMembership<NodeId, BasicNode>>,
    /// Channel to notify proxy of applied changes
    change_tx: Option<mpsc::UnboundedSender<ChangeLog>>,
    /// Snapshot data (cached for building snapshots)
    snapshot_data: RwLock<SnapshotData>,
}

impl Default for DGateStateMachine {
    fn default() -> Self {
        Self {
            store: None,
            last_applied: RwLock::new(None),
            last_membership: RwLock::new(StoredMembership::default()),
            change_tx: None,
            snapshot_data: RwLock::new(SnapshotData::default()),
        }
    }
}

impl Clone for DGateStateMachine {
    fn clone(&self) -> Self {
        Self {
            store: self.store.clone(),
            last_applied: RwLock::new(self.last_applied.read().clone()),
            last_membership: RwLock::new(self.last_membership.read().clone()),
            change_tx: self.change_tx.clone(),
            snapshot_data: RwLock::new(self.snapshot_data.read().clone()),
        }
    }
}

impl DGateStateMachine {
    /// Create a new state machine
    #[allow(dead_code)]
    pub fn new(store: Arc<ProxyStore>) -> Self {
        Self {
            store: Some(store),
            last_applied: RwLock::new(None),
            last_membership: RwLock::new(StoredMembership::default()),
            change_tx: None,
            snapshot_data: RwLock::new(SnapshotData::default()),
        }
    }

    /// Create a new state machine with a change notification channel
    pub fn with_change_notifier(
        store: Arc<ProxyStore>,
        change_tx: mpsc::UnboundedSender<ChangeLog>,
    ) -> Self {
        Self {
            store: Some(store),
            last_applied: RwLock::new(None),
            last_membership: RwLock::new(StoredMembership::default()),
            change_tx: Some(change_tx),
            snapshot_data: RwLock::new(SnapshotData::default()),
        }
    }

    /// Get the underlying store
    #[allow(dead_code)]
    pub fn store(&self) -> Option<&ProxyStore> {
        self.store.as_ref().map(|s| s.as_ref())
    }

    /// Apply a change log to storage
    fn apply_changelog(&self, changelog: &ChangeLog) -> Result<ClientResponse, String> {
        use crate::resources::*;

        let store = match &self.store {
            Some(s) => s,
            None => return Ok(ClientResponse::default()),
        };

        let result = match changelog.cmd {
            ChangeCommand::AddNamespace => {
                let ns: Namespace =
                    serde_json::from_value(changelog.item.clone()).map_err(|e| e.to_string())?;
                store.set_namespace(&ns).map_err(|e| e.to_string())?;
                ClientResponse {
                    success: true,
                    message: Some(format!("Namespace '{}' created", ns.name)),
                }
            }
            ChangeCommand::DeleteNamespace => {
                store
                    .delete_namespace(&changelog.name)
                    .map_err(|e| e.to_string())?;
                ClientResponse {
                    success: true,
                    message: Some(format!("Namespace '{}' deleted", changelog.name)),
                }
            }
            ChangeCommand::AddRoute => {
                let route: Route =
                    serde_json::from_value(changelog.item.clone()).map_err(|e| e.to_string())?;
                store.set_route(&route).map_err(|e| e.to_string())?;
                ClientResponse {
                    success: true,
                    message: Some(format!("Route '{}' created", route.name)),
                }
            }
            ChangeCommand::DeleteRoute => {
                store
                    .delete_route(&changelog.namespace, &changelog.name)
                    .map_err(|e| e.to_string())?;
                ClientResponse {
                    success: true,
                    message: Some(format!("Route '{}' deleted", changelog.name)),
                }
            }
            ChangeCommand::AddService => {
                let service: Service =
                    serde_json::from_value(changelog.item.clone()).map_err(|e| e.to_string())?;
                store.set_service(&service).map_err(|e| e.to_string())?;
                ClientResponse {
                    success: true,
                    message: Some(format!("Service '{}' created", service.name)),
                }
            }
            ChangeCommand::DeleteService => {
                store
                    .delete_service(&changelog.namespace, &changelog.name)
                    .map_err(|e| e.to_string())?;
                ClientResponse {
                    success: true,
                    message: Some(format!("Service '{}' deleted", changelog.name)),
                }
            }
            ChangeCommand::AddModule => {
                let module: Module =
                    serde_json::from_value(changelog.item.clone()).map_err(|e| e.to_string())?;
                store.set_module(&module).map_err(|e| e.to_string())?;
                ClientResponse {
                    success: true,
                    message: Some(format!("Module '{}' created", module.name)),
                }
            }
            ChangeCommand::DeleteModule => {
                store
                    .delete_module(&changelog.namespace, &changelog.name)
                    .map_err(|e| e.to_string())?;
                ClientResponse {
                    success: true,
                    message: Some(format!("Module '{}' deleted", changelog.name)),
                }
            }
            ChangeCommand::AddDomain => {
                let domain: Domain =
                    serde_json::from_value(changelog.item.clone()).map_err(|e| e.to_string())?;
                store.set_domain(&domain).map_err(|e| e.to_string())?;
                ClientResponse {
                    success: true,
                    message: Some(format!("Domain '{}' created", domain.name)),
                }
            }
            ChangeCommand::DeleteDomain => {
                store
                    .delete_domain(&changelog.namespace, &changelog.name)
                    .map_err(|e| e.to_string())?;
                ClientResponse {
                    success: true,
                    message: Some(format!("Domain '{}' deleted", changelog.name)),
                }
            }
            ChangeCommand::AddSecret => {
                let secret: Secret =
                    serde_json::from_value(changelog.item.clone()).map_err(|e| e.to_string())?;
                store.set_secret(&secret).map_err(|e| e.to_string())?;
                ClientResponse {
                    success: true,
                    message: Some(format!("Secret '{}' created", secret.name)),
                }
            }
            ChangeCommand::DeleteSecret => {
                store
                    .delete_secret(&changelog.namespace, &changelog.name)
                    .map_err(|e| e.to_string())?;
                ClientResponse {
                    success: true,
                    message: Some(format!("Secret '{}' deleted", changelog.name)),
                }
            }
            ChangeCommand::AddCollection => {
                let collection: Collection =
                    serde_json::from_value(changelog.item.clone()).map_err(|e| e.to_string())?;
                store
                    .set_collection(&collection)
                    .map_err(|e| e.to_string())?;
                ClientResponse {
                    success: true,
                    message: Some(format!("Collection '{}' created", collection.name)),
                }
            }
            ChangeCommand::DeleteCollection => {
                store
                    .delete_collection(&changelog.namespace, &changelog.name)
                    .map_err(|e| e.to_string())?;
                ClientResponse {
                    success: true,
                    message: Some(format!("Collection '{}' deleted", changelog.name)),
                }
            }
            ChangeCommand::AddDocument => {
                let document: Document =
                    serde_json::from_value(changelog.item.clone()).map_err(|e| e.to_string())?;
                store.set_document(&document).map_err(|e| e.to_string())?;
                ClientResponse {
                    success: true,
                    message: Some(format!("Document '{}' created", document.id)),
                }
            }
            ChangeCommand::DeleteDocument => {
                let doc: Document =
                    serde_json::from_value(changelog.item.clone()).map_err(|e| e.to_string())?;
                store
                    .delete_document(&changelog.namespace, &doc.collection, &changelog.name)
                    .map_err(|e| e.to_string())?;
                ClientResponse {
                    success: true,
                    message: Some(format!("Document '{}' deleted", changelog.name)),
                }
            }
        };

        // Notify proxy about the change
        if let Some(ref tx) = self.change_tx {
            if let Err(e) = tx.send(changelog.clone()) {
                error!("Failed to notify proxy of change: {}", e);
            }
        }

        // Update snapshot data
        {
            let mut snapshot = self.snapshot_data.write();
            snapshot.changelogs.push(changelog.clone());
        }

        Ok(result)
    }
}

impl RaftStateMachine<TypeConfig> for DGateStateMachine {
    type SnapshotBuilder = DGateSnapshotBuilder;

    async fn applied_state(
        &mut self,
    ) -> Result<(Option<LogId<NodeId>>, StoredMembership<NodeId, BasicNode>), StorageError<NodeId>>
    {
        let last_applied = self.last_applied.read().clone();
        let last_membership = self.last_membership.read().clone();
        Ok((last_applied, last_membership))
    }

    async fn apply<I>(&mut self, entries: I) -> Result<Vec<ClientResponse>, StorageError<NodeId>>
    where
        I: IntoIterator<Item = Entry<TypeConfig>> + OptionalSend,
    {
        let mut responses = Vec::new();

        for entry in entries {
            debug!("Applying log entry: {:?}", entry.log_id);

            // Update last applied
            *self.last_applied.write() = Some(entry.log_id);

            match entry.payload {
                EntryPayload::Blank => {
                    responses.push(ClientResponse::default());
                }
                EntryPayload::Normal(changelog) => {
                    let response = match self.apply_changelog(&changelog) {
                        Ok(resp) => resp,
                        Err(e) => {
                            error!("Failed to apply changelog: {}", e);
                            ClientResponse {
                                success: false,
                                message: Some(e),
                            }
                        }
                    };
                    responses.push(response);
                }
                EntryPayload::Membership(membership) => {
                    info!("Applying membership change: {:?}", membership);
                    *self.last_membership.write() =
                        StoredMembership::new(Some(entry.log_id), membership);
                    responses.push(ClientResponse::default());
                }
            }
        }

        Ok(responses)
    }

    async fn get_snapshot_builder(&mut self) -> Self::SnapshotBuilder {
        let snapshot_data = self.snapshot_data.read().clone();
        let last_applied = self.last_applied.read().clone();
        let last_membership = self.last_membership.read().clone();

        DGateSnapshotBuilder {
            snapshot_data,
            last_applied,
            last_membership,
        }
    }

    async fn begin_receiving_snapshot(
        &mut self,
    ) -> Result<Box<Cursor<Vec<u8>>>, StorageError<NodeId>> {
        Ok(Box::new(Cursor::new(Vec::new())))
    }

    async fn install_snapshot(
        &mut self,
        meta: &SnapshotMeta<NodeId, BasicNode>,
        snapshot: Box<Cursor<Vec<u8>>>,
    ) -> Result<(), StorageError<NodeId>> {
        info!("Installing snapshot: {:?}", meta);

        let data = snapshot.into_inner();
        let snapshot_data: SnapshotData = serde_json::from_slice(&data).map_err(|e| {
            StorageError::from_io_error(
                openraft::ErrorSubject::Snapshot(Some(meta.signature())),
                openraft::ErrorVerb::Read,
                std::io::Error::new(std::io::ErrorKind::InvalidData, e),
            )
        })?;

        // Clear current state and replay changelogs from snapshot
        for changelog in &snapshot_data.changelogs {
            if let Err(e) = self.apply_changelog(changelog) {
                error!("Failed to apply changelog from snapshot: {}", e);
            }
        }

        *self.last_applied.write() = meta.last_log_id;
        *self.last_membership.write() = meta.last_membership.clone();
        *self.snapshot_data.write() = snapshot_data;

        Ok(())
    }

    async fn get_current_snapshot(
        &mut self,
    ) -> Result<Option<Snapshot<TypeConfig>>, StorageError<NodeId>> {
        let snapshot_data = self.snapshot_data.read().clone();
        let last_applied = self.last_applied.read().clone();
        let last_membership = self.last_membership.read().clone();

        if last_applied.is_none() {
            return Ok(None);
        }

        let data = serde_json::to_vec(&snapshot_data).map_err(|e| {
            StorageError::from_io_error(
                openraft::ErrorSubject::StateMachine,
                openraft::ErrorVerb::Read,
                std::io::Error::new(std::io::ErrorKind::InvalidData, e),
            )
        })?;

        let snapshot_id = format!(
            "{}-{}-{}",
            last_applied.as_ref().map(|l| l.leader_id.term).unwrap_or(0),
            last_applied.as_ref().map(|l| l.index).unwrap_or(0),
            uuid::Uuid::new_v4()
        );

        Ok(Some(Snapshot {
            meta: SnapshotMeta {
                last_log_id: last_applied,
                last_membership,
                snapshot_id,
            },
            snapshot: Box::new(Cursor::new(data)),
        }))
    }
}

/// Snapshot builder for the state machine
pub struct DGateSnapshotBuilder {
    snapshot_data: SnapshotData,
    last_applied: Option<LogId<NodeId>>,
    last_membership: StoredMembership<NodeId, BasicNode>,
}

impl RaftSnapshotBuilder<TypeConfig> for DGateSnapshotBuilder {
    async fn build_snapshot(&mut self) -> Result<Snapshot<TypeConfig>, StorageError<NodeId>> {
        let data = serde_json::to_vec(&self.snapshot_data).map_err(|e| {
            StorageError::from_io_error(
                openraft::ErrorSubject::StateMachine,
                openraft::ErrorVerb::Read,
                std::io::Error::new(std::io::ErrorKind::InvalidData, e),
            )
        })?;

        let snapshot_id = format!(
            "{}-{}-{}",
            self.last_applied
                .as_ref()
                .map(|l| l.leader_id.term)
                .unwrap_or(0),
            self.last_applied.as_ref().map(|l| l.index).unwrap_or(0),
            uuid::Uuid::new_v4()
        );

        Ok(Snapshot {
            meta: SnapshotMeta {
                last_log_id: self.last_applied,
                last_membership: self.last_membership.clone(),
                snapshot_id,
            },
            snapshot: Box::new(Cursor::new(data)),
        })
    }
}
