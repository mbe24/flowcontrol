//! Change notification shared by the store's write path and the gRPC Watch
//! stream. The store publishes after a committed mutation; each Watch subscriber
//! receives the same `Notified` (a broadcast channel). Clients treat this as
//! "here is what changed + the activity events" — they get current state via
//! GetSnapshot and do not reconstruct state from the event log.

use crate::generated::flow_v1 as pb;
use tokio::sync::broadcast;

/// What a Watch subscriber is told after a committed mutation.
#[derive(Clone)]
pub struct Notified {
    pub project_id: String,
    pub seq: i64,
    pub events: Vec<pb::Event>,
    pub changed_nodes: Vec<pb::Node>,
    pub changed_progress: Vec<pb::Progress>,
}

/// Broadcasts `Notified` to all connected Watch subscribers.
pub type Notifier = broadcast::Sender<Notified>;

/// Open a new broadcast channel with the given capacity.
pub fn channel(capacity: usize) -> (Notifier, broadcast::Receiver<Notified>) {
    broadcast::channel(capacity)
}
