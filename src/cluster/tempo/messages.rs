//! Tempo protocol messages
//!
//! Defines the message types used in the Tempo consensus protocol.

#![allow(dead_code)]

use serde::{Deserialize, Serialize};
use std::collections::HashMap;

use super::{Dot, NodeId};
use crate::resources::ChangeLog;

/// Types of messages in the Tempo protocol
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
pub enum TempoMessageType {
    /// Phase 1: Coordinator broadcasts command with timestamp
    /// Sent by the coordinator to all nodes when a new command is submitted
    MCollect,

    /// Phase 1 Response: Node acknowledges with its clock/vote
    /// Sent back to the coordinator after receiving MCollect
    MCollectAck,

    /// Phase 2: Coordinator broadcasts commit decision
    /// Sent when enough acks are received to commit
    MCommit,

    /// Periodic clock synchronization message
    /// Sent to keep clocks loosely synchronized across nodes
    ClockBump,
}

/// A Tempo protocol message
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct TempoMessage {
    /// The type of message
    pub msg_type: TempoMessageType,

    /// Command identifier (node_id, sequence)
    /// Present in MCollect, MCollectAck, MCommit
    #[serde(skip_serializing_if = "Option::is_none")]
    pub dot: Option<Dot>,

    /// Logical clock value
    /// - In MCollect: coordinator's clock when command was submitted
    /// - In MCollectAck: responder's clock
    /// - In MCommit: commit clock
    /// - In ClockBump: sender's current clock
    #[serde(skip_serializing_if = "Option::is_none")]
    pub clock: Option<u64>,

    /// The changelog being replicated
    /// Present in MCollect and MCommit
    #[serde(skip_serializing_if = "Option::is_none")]
    pub changelog: Option<ChangeLog>,

    /// Votes from nodes (node_id -> clock)
    /// Used in slow path consensus
    #[serde(skip_serializing_if = "Option::is_none")]
    pub votes: Option<HashMap<NodeId, u64>>,

    /// Quorum information
    #[serde(skip_serializing_if = "Option::is_none")]
    pub quorum: Option<Vec<NodeId>>,
}

impl TempoMessage {
    /// Create a new MCollect message
    pub fn mcollect(dot: Dot, clock: u64, changelog: ChangeLog) -> Self {
        Self {
            msg_type: TempoMessageType::MCollect,
            dot: Some(dot),
            clock: Some(clock),
            changelog: Some(changelog),
            votes: None,
            quorum: None,
        }
    }

    /// Create a new MCollectAck message
    pub fn mcollect_ack(dot: Dot, clock: u64) -> Self {
        Self {
            msg_type: TempoMessageType::MCollectAck,
            dot: Some(dot),
            clock: Some(clock),
            changelog: None,
            votes: None,
            quorum: None,
        }
    }

    /// Create a new MCommit message
    pub fn mcommit(dot: Dot, clock: u64, changelog: ChangeLog) -> Self {
        Self {
            msg_type: TempoMessageType::MCommit,
            dot: Some(dot),
            clock: Some(clock),
            changelog: Some(changelog),
            votes: None,
            quorum: None,
        }
    }

    /// Create a new ClockBump message
    pub fn clock_bump(clock: u64) -> Self {
        Self {
            msg_type: TempoMessageType::ClockBump,
            dot: None,
            clock: Some(clock),
            changelog: None,
            votes: None,
            quorum: None,
        }
    }
}
