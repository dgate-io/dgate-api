//! Logical clock for Tempo consensus
//!
//! Provides a hybrid logical clock that combines physical time with
//! logical ordering to provide a total order of events across the cluster.

#![allow(dead_code)]

use std::sync::atomic::{AtomicU64, Ordering};
use std::time::{SystemTime, UNIX_EPOCH};

use super::NodeId;

/// Logical clock that provides monotonically increasing timestamps
///
/// The clock value is structured as:
/// - Upper bits: Physical time component (milliseconds since epoch)
/// - Lower bits: Logical counter for events within the same millisecond
pub struct LogicalClock {
    /// The node ID (used to break ties)
    node_id: NodeId,
    /// Current clock value
    clock: AtomicU64,
}

impl LogicalClock {
    /// Create a new logical clock
    pub fn new(node_id: NodeId) -> Self {
        let now = Self::physical_time();
        Self {
            node_id,
            clock: AtomicU64::new(now),
        }
    }

    /// Get current physical time in milliseconds
    fn physical_time() -> u64 {
        SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .map(|d| d.as_millis() as u64)
            .unwrap_or(0)
    }

    /// Get the current clock value without advancing
    pub fn current(&self) -> u64 {
        self.clock.load(Ordering::SeqCst)
    }

    /// Advance the clock and return the new value
    ///
    /// The clock is advanced to max(current + 1, physical_time)
    /// to ensure monotonicity while staying close to real time.
    pub fn tick(&self) -> u64 {
        loop {
            let current = self.clock.load(Ordering::SeqCst);
            let physical = Self::physical_time();

            // New value is max(current + 1, physical_time)
            let new_value = (current + 1).max(physical);

            // Try to update atomically
            match self.clock.compare_exchange(
                current,
                new_value,
                Ordering::SeqCst,
                Ordering::SeqCst,
            ) {
                Ok(_) => return new_value,
                Err(_) => continue, // Another thread updated, retry
            }
        }
    }

    /// Update the clock based on a received timestamp
    ///
    /// The clock is updated to max(current, received) + 1 to ensure
    /// that our events happen-after the received event.
    pub fn update(&self, received: u64) {
        loop {
            let current = self.clock.load(Ordering::SeqCst);
            let physical = Self::physical_time();

            // New value is max(current, received, physical_time)
            let new_value = current.max(received).max(physical);

            if new_value <= current {
                return; // No update needed
            }

            match self.clock.compare_exchange(
                current,
                new_value,
                Ordering::SeqCst,
                Ordering::SeqCst,
            ) {
                Ok(_) => return,
                Err(_) => continue,
            }
        }
    }

    /// Get the node ID
    pub fn node_id(&self) -> NodeId {
        self.node_id
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_clock_monotonicity() {
        let clock = LogicalClock::new(1);

        let t1 = clock.tick();
        let t2 = clock.tick();
        let t3 = clock.tick();

        assert!(t2 > t1);
        assert!(t3 > t2);
    }

    #[test]
    fn test_clock_update() {
        let clock = LogicalClock::new(1);

        let t1 = clock.tick();

        // Update with a future timestamp
        let future = t1 + 1000;
        clock.update(future);

        let t2 = clock.current();
        assert!(t2 >= future);

        // Tick should give us a value greater than the update
        let t3 = clock.tick();
        assert!(t3 > t2);
    }

    #[test]
    fn test_clock_update_with_past() {
        let clock = LogicalClock::new(1);

        let t1 = clock.tick();

        // Update with a past timestamp should not decrease clock
        clock.update(t1 - 1000);

        let t2 = clock.current();
        assert!(t2 >= t1);
    }
}
