//! State management for Tempo consensus
//!
//! Tracks command status, votes, and committed operations.

#![allow(dead_code)]

use std::collections::{HashMap, HashSet};

use super::{Dot, NodeId};
use crate::resources::ChangeLog;

/// Status of a command in the Tempo protocol
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum CommandStatus {
    /// Command received, waiting for acks
    Collecting,
    /// Command committed and can be applied
    Committed,
    /// Command applied to state machine
    Applied,
}

/// Information about a command being processed
#[derive(Debug, Clone)]
pub struct CommandInfo {
    /// The command identifier
    pub dot: Dot,
    /// The changelog being replicated
    pub changelog: ChangeLog,
    /// The clock value when command was proposed
    pub clock: u64,
    /// Current status
    pub status: CommandStatus,
    /// Nodes that have acknowledged with their clocks
    pub acks: HashMap<NodeId, u64>,
    /// Whether this command has been committed
    pub committed: bool,
}

impl CommandInfo {
    /// Create a new command info
    pub fn new(dot: Dot, changelog: ChangeLog, clock: u64) -> Self {
        Self {
            dot,
            changelog,
            clock,
            status: CommandStatus::Collecting,
            acks: HashMap::new(),
            committed: false,
        }
    }

    /// Add an acknowledgment from a node
    pub fn add_ack(&mut self, node_id: NodeId, clock: u64) {
        self.acks.insert(node_id, clock);
    }

    /// Check if we have enough acks for the fast quorum
    pub fn has_fast_quorum(&self, size: usize) -> bool {
        self.acks.len() >= size
    }

    /// Check if all acks have the same clock (fast path condition)
    pub fn acks_agree(&self) -> bool {
        if self.acks.is_empty() {
            return false;
        }
        let first = self.acks.values().next().cloned();
        self.acks.values().all(|c| Some(*c) == first)
    }
}

/// State for the Tempo protocol
pub struct TempoState {
    /// This node's ID
    node_id: NodeId,
    /// Fast quorum size
    pub fast_quorum_size: usize,
    /// Write quorum size
    pub write_quorum_size: usize,
    /// Commands indexed by their dot
    commands: HashMap<Dot, CommandInfo>,
    /// Set of committed dots
    committed_dots: HashSet<Dot>,
    /// Counter for committed commands
    committed_count: u64,
    /// Last applied sequence number for this node
    last_applied: u64,
}

impl TempoState {
    /// Create a new Tempo state
    pub fn new(node_id: NodeId, fast_quorum_size: usize, write_quorum_size: usize) -> Self {
        Self {
            node_id,
            fast_quorum_size,
            write_quorum_size,
            commands: HashMap::new(),
            committed_dots: HashSet::new(),
            committed_count: 0,
            last_applied: 0,
        }
    }

    /// Add a new command
    pub fn add_command(&mut self, dot: Dot, changelog: ChangeLog, clock: u64) {
        self.commands
            .entry(dot)
            .or_insert_with(|| CommandInfo::new(dot, changelog, clock));
    }

    /// Add an acknowledgment for a command
    /// Returns true if we now have quorum
    pub fn add_ack(&mut self, dot: Dot, node_id: NodeId, clock: u64) -> bool {
        if let Some(cmd) = self.commands.get_mut(&dot) {
            cmd.add_ack(node_id, clock);

            // Check if we have enough acks for fast quorum
            cmd.acks.len() >= self.fast_quorum_size && !cmd.committed
        } else {
            false
        }
    }

    /// Check if a command has quorum
    pub fn has_quorum(&self, dot: &Dot) -> bool {
        self.commands
            .get(dot)
            .map(|cmd| cmd.acks.len() >= self.fast_quorum_size && !cmd.committed)
            .unwrap_or(false)
    }

    /// Get command info
    pub fn get_command(&self, dot: &Dot) -> Option<&CommandInfo> {
        self.commands.get(dot)
    }

    /// Mark a command as committed
    pub fn mark_committed(&mut self, dot: Dot) {
        if let Some(cmd) = self.commands.get_mut(&dot) {
            if !cmd.committed {
                cmd.committed = true;
                cmd.status = CommandStatus::Committed;
                self.committed_dots.insert(dot);
                self.committed_count += 1;

                // Update last applied if this is from our node
                if dot.node_id == self.node_id {
                    self.last_applied = self.last_applied.max(dot.sequence);
                }
            }
        } else {
            // Command wasn't tracked, just mark as committed
            self.committed_dots.insert(dot);
            self.committed_count += 1;
        }
    }

    /// Check if a command is committed
    pub fn is_committed(&self, dot: &Dot) -> bool {
        self.committed_dots.contains(dot)
    }

    /// Get the number of committed commands
    pub fn committed_count(&self) -> u64 {
        self.committed_count
    }

    /// Get the number of pending commands
    pub fn pending_count(&self) -> usize {
        self.commands.values().filter(|c| !c.committed).count()
    }

    /// Get the last applied sequence number for this node
    pub fn last_applied_sequence(&self) -> u64 {
        self.last_applied
    }

    /// Clean up old committed commands (garbage collection)
    pub fn gc(&mut self, keep_recent: usize) {
        if self.commands.len() <= keep_recent {
            return;
        }

        // Remove oldest committed commands
        let mut to_remove: Vec<Dot> = self
            .commands
            .iter()
            .filter(|(_, cmd)| cmd.committed)
            .map(|(dot, _)| *dot)
            .collect();

        // Sort by sequence (oldest first)
        to_remove.sort_by_key(|d| d.sequence);

        // Keep only the most recent
        let remove_count = to_remove.len().saturating_sub(keep_recent);
        for dot in to_remove.into_iter().take(remove_count) {
            self.commands.remove(&dot);
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn test_changelog() -> ChangeLog {
        use crate::resources::{ChangeCommand, Namespace};
        ChangeLog::new(
            ChangeCommand::AddNamespace,
            "test",
            "test",
            &Namespace::new("test"),
        )
    }

    #[test]
    fn test_command_quorum() {
        let mut state = TempoState::new(1, 2, 2);
        let dot = Dot::new(1, 1);

        state.add_command(dot, test_changelog(), 100);

        // First ack - not quorum yet
        let has_quorum = state.add_ack(dot, 1, 100);
        assert!(!has_quorum);

        // Second ack - now we have quorum
        let has_quorum = state.add_ack(dot, 2, 100);
        assert!(has_quorum);
    }

    #[test]
    fn test_mark_committed() {
        let mut state = TempoState::new(1, 2, 2);
        let dot = Dot::new(1, 1);

        state.add_command(dot, test_changelog(), 100);
        assert!(!state.is_committed(&dot));

        state.mark_committed(dot);
        assert!(state.is_committed(&dot));
        assert_eq!(state.committed_count(), 1);
    }
}
