//! The storage seam. `SqliteStore` is the v1 implementation; a hosted DB or an
//! in-memory fake can implement the same trait later without touching grpc/.

use std::sync::Arc;

use crate::generated::flow_v1 as pb;
use tokio::sync::broadcast;

mod sqlite;
mod watch;
pub use sqlite::SqliteStore;
pub use watch::{Notified, Notifier};

/// The storage seam between the gRPC layer and the database. Reads plus all
/// mutations. Every mutation returns its `Mutation` payload (events, changed
/// nodes/progress, cursor) and publishes the same `Notified` to `Watch`
/// subscribers so the unary and stream paths share one apply-path.
#[::async_trait::async_trait]
pub trait Store: Send + Sync {
    // Reads
    async fn list_projects(
        &self,
        include_archived: bool,
    ) -> Result<Vec<pb::Project>, Box<dyn std::error::Error + Send + Sync>>;
    async fn get_snapshot(
        &self,
        project_id: &str,
    ) -> Result<pb::GetSnapshotResponse, Box<dyn std::error::Error + Send + Sync>>;
    async fn list_events(
        &self,
        project_id: &str,
        limit: i32,
    ) -> Result<Vec<pb::Event>, Box<dyn std::error::Error + Send + Sync>>;
    async fn search(
        &self,
        project_id: &str,
        query: &str,
        limit: i32,
    ) -> Result<Vec<pb::Node>, Box<dyn std::error::Error + Send + Sync>>;
    /// Events with seq strictly greater than `from_seq` for a project (replay).
    async fn events_after(
        &self,
        project_id: &str,
        from_seq: i64,
    ) -> Result<Vec<pb::Event>, Box<dyn std::error::Error + Send + Sync>>;

    // Writes (each returns the mutation payload + new seq and logs an event)
    async fn create_node(
        &self,
        req: pb::CreateNodeRequest,
    ) -> Result<pb::Mutation, Box<dyn std::error::Error + Send + Sync>>;
    async fn update_node(
        &self,
        req: pb::UpdateNodeRequest,
    ) -> Result<pb::Mutation, Box<dyn std::error::Error + Send + Sync>>;
    async fn delete_node(
        &self,
        req: pb::DeleteNodeRequest,
    ) -> Result<pb::Mutation, Box<dyn std::error::Error + Send + Sync>>;
    async fn set_status(
        &self,
        req: pb::SetStatusRequest,
    ) -> Result<pb::Mutation, Box<dyn std::error::Error + Send + Sync>>;
    async fn add_dependency(
        &self,
        req: pb::AddDependencyRequest,
    ) -> Result<pb::Mutation, Box<dyn std::error::Error + Send + Sync>>;
    async fn remove_dependency(
        &self,
        req: pb::RemoveDependencyRequest,
    ) -> Result<pb::Mutation, Box<dyn std::error::Error + Send + Sync>>;
    async fn report_condition(
        &self,
        req: pb::ReportConditionRequest,
    ) -> Result<pb::Mutation, Box<dyn std::error::Error + Send + Sync>>;
    async fn set_verdict(
        &self,
        req: pb::SetVerdictRequest,
    ) -> Result<pb::Mutation, Box<dyn std::error::Error + Send + Sync>>;
    async fn add_comment(
        &self,
        req: pb::AddCommentRequest,
    ) -> Result<pb::Mutation, Box<dyn std::error::Error + Send + Sync>>;
    async fn undo(
        &self,
        req: pb::UndoRequest,
    ) -> Result<pb::Mutation, Box<dyn std::error::Error + Send + Sync>>;

    /// Subscribe to mutation notifications for Watch.
    fn subscribe(&self) -> broadcast::Receiver<Notified>;
}

pub type DynStore = Arc<dyn Store>;
