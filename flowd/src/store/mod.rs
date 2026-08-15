//! The storage seam. `SqliteStore` is the v1 implementation; a hosted DB or an
//! in-memory fake can implement the same trait later without touching grpc/.

use std::sync::Arc;

use crate::error::DomainError;
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
    async fn list_projects(&self, include_archived: bool) -> Result<Vec<pb::Project>, DomainError>;
    async fn get_snapshot(&self, project_id: &str) -> Result<pb::GetSnapshotResponse, DomainError>;
    /// Page the event log backwards (newest first), optionally filtered to one
    /// node, from before `before_seq` (0 = newest). Returns the page plus whether
    /// an older page exists.
    async fn list_events(
        &self,
        project_id: &str,
        node_id: &str,
        before_seq: i64,
        limit: i32,
    ) -> Result<(Vec<pb::Event>, bool), DomainError>;
    async fn search(
        &self,
        project_id: &str,
        query: &str,
        limit: i32,
    ) -> Result<Vec<pb::Node>, DomainError>;
    /// Events with seq strictly greater than `from_seq` for a project (replay).
    async fn events_after(
        &self,
        project_id: &str,
        from_seq: i64,
    ) -> Result<Vec<pb::Event>, DomainError>;
    /// Forward poll: events after a cursor plus the next cursor. The stateless
    /// substitute for Watch (unary, oldest-first).
    async fn poll_changes(
        &self,
        project_id: &str,
        after_seq: i64,
        limit: i32,
    ) -> Result<pb::PollChangesResponse, DomainError>;

    // Writes (each returns the mutation payload + new seq and logs an event)
    async fn create_node(&self, req: pb::CreateNodeRequest) -> Result<pb::Mutation, DomainError>;
    async fn update_node(&self, req: pb::UpdateNodeRequest) -> Result<pb::Mutation, DomainError>;
    async fn delete_node(&self, req: pb::DeleteNodeRequest) -> Result<pb::Mutation, DomainError>;
    async fn set_status(&self, req: pb::SetStatusRequest) -> Result<pb::Mutation, DomainError>;
    async fn add_dependency(
        &self,
        req: pb::AddDependencyRequest,
    ) -> Result<pb::Mutation, DomainError>;
    async fn remove_dependency(
        &self,
        req: pb::RemoveDependencyRequest,
    ) -> Result<pb::Mutation, DomainError>;
    async fn report_condition(
        &self,
        req: pb::ReportConditionRequest,
    ) -> Result<pb::Mutation, DomainError>;
    async fn set_verdict(&self, req: pb::SetVerdictRequest) -> Result<pb::Mutation, DomainError>;
    async fn add_comment(&self, req: pb::AddCommentRequest) -> Result<pb::Mutation, DomainError>;
    async fn undo(&self, req: pb::UndoRequest) -> Result<pb::Mutation, DomainError>;
    /// Reparent / change the kind of a node. A node activity, so it returns a
    /// `Mutation` and logs a `NODE_UPDATED` event (plus `NODE_DELETED` for any
    /// step children dropped by a TASK→STEP demote).
    async fn move_node(&self, req: pb::MoveNodeRequest) -> Result<pb::Mutation, DomainError>;

    // Project lifecycle. Projects are a namespace: these log no event and return
    // the affected `Project` rather than a `Mutation`.
    async fn create_project(
        &self,
        req: pb::CreateProjectRequest,
    ) -> Result<pb::Project, DomainError>;
    async fn update_project(
        &self,
        req: pb::UpdateProjectRequest,
    ) -> Result<pb::Project, DomainError>;
    async fn archive_project(
        &self,
        req: pb::ArchiveProjectRequest,
    ) -> Result<pb::Project, DomainError>;

    /// Subscribe to mutation notifications for Watch.
    fn subscribe(&self) -> broadcast::Receiver<Notified>;
}

pub type DynStore = Arc<dyn Store>;
