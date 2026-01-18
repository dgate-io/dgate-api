//! Raft log storage for DGate
//!
//! Provides in-memory log storage for Raft consensus. For production
//! deployments with persistence requirements, this can be extended to
//! use file-based or database-backed storage.

use std::collections::BTreeMap;
use std::fmt::Debug;
use std::ops::RangeBounds;

use openraft::storage::{LogFlushed, RaftLogReader, RaftLogStorage};
use openraft::{Entry, LogId, LogState, OptionalSend, RaftLogId, StorageError, Vote};
use parking_lot::RwLock;
use tracing::debug;

use super::{NodeId, TypeConfig};

/// In-memory log store for Raft
#[derive(Default)]
pub struct RaftLogStore {
    /// Current vote
    vote: RwLock<Option<Vote<NodeId>>>,
    /// Log entries
    logs: RwLock<BTreeMap<u64, Entry<TypeConfig>>>,
    /// Last purged log ID
    last_purged: RwLock<Option<LogId<NodeId>>>,
}

impl RaftLogStore {
    /// Create a new log store
    pub fn new() -> Self {
        Self::default()
    }
}

impl RaftLogReader<TypeConfig> for RaftLogStore {
    async fn try_get_log_entries<RB: RangeBounds<u64> + Clone + Debug + OptionalSend>(
        &mut self,
        range: RB,
    ) -> Result<Vec<Entry<TypeConfig>>, StorageError<NodeId>> {
        let logs = self.logs.read();
        let entries: Vec<_> = logs.range(range).map(|(_, v)| v.clone()).collect();
        Ok(entries)
    }
}

impl RaftLogStorage<TypeConfig> for RaftLogStore {
    type LogReader = Self;

    async fn get_log_state(&mut self) -> Result<LogState<TypeConfig>, StorageError<NodeId>> {
        let logs = self.logs.read();
        let last_purged = self.last_purged.read().clone();

        let last = logs.iter().next_back().map(|(_, v)| *v.get_log_id());

        Ok(LogState {
            last_purged_log_id: last_purged,
            last_log_id: last,
        })
    }

    async fn get_log_reader(&mut self) -> Self::LogReader {
        Self {
            vote: RwLock::new(self.vote.read().clone()),
            logs: RwLock::new(self.logs.read().clone()),
            last_purged: RwLock::new(self.last_purged.read().clone()),
        }
    }

    async fn save_vote(&mut self, vote: &Vote<NodeId>) -> Result<(), StorageError<NodeId>> {
        debug!("Saving vote: {:?}", vote);
        *self.vote.write() = Some(*vote);
        Ok(())
    }

    async fn read_vote(&mut self) -> Result<Option<Vote<NodeId>>, StorageError<NodeId>> {
        Ok(self.vote.read().clone())
    }

    async fn append<I>(
        &mut self,
        entries: I,
        callback: LogFlushed<TypeConfig>,
    ) -> Result<(), StorageError<NodeId>>
    where
        I: IntoIterator<Item = Entry<TypeConfig>> + OptionalSend,
    {
        let mut logs = self.logs.write();
        for entry in entries {
            debug!("Appending log entry: {:?}", entry.log_id);
            logs.insert(entry.log_id.index, entry);
        }
        callback.log_io_completed(Ok(()));
        Ok(())
    }

    async fn truncate(&mut self, log_id: LogId<NodeId>) -> Result<(), StorageError<NodeId>> {
        debug!("Truncating logs from: {:?}", log_id);
        let mut logs = self.logs.write();
        logs.split_off(&log_id.index);
        Ok(())
    }

    async fn purge(&mut self, log_id: LogId<NodeId>) -> Result<(), StorageError<NodeId>> {
        debug!("Purging logs up to: {:?}", log_id);
        let mut logs = self.logs.write();

        // Remove all entries up to and including log_id
        let keys_to_remove: Vec<_> = logs.range(..=log_id.index).map(|(k, _)| *k).collect();

        for key in keys_to_remove {
            logs.remove(&key);
        }

        *self.last_purged.write() = Some(log_id);
        Ok(())
    }
}
