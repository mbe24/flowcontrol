//! The native async edge over `flowcore`'s synchronous store.
//!
//! `flowcore::store::SqliteStore` is the target-agnostic sync core (its `*_locked`
//! methods run one SQLite op each). `NativeStore` wraps it, owns the Watch
//! notifier, and implements the async [`Store`] trait by offloading each op onto
//! the blocking pool (so SQLite never stalls the tokio reactor) and publishing
//! the resulting `Mutation` to Watch subscribers.
//!
//! The trait stays async so a future remote-DB backend (Postgres/libSQL/D1) can
//! implement it natively; the sync-ness lives only in `flowcore`.

use std::sync::Arc;

use flowcore::error::DomainError;
use flowcore::generated::flow_v1 as pb;
use flowcore::sql::RusqliteSql;
use tokio::sync::broadcast;

mod watch;
pub use watch::{Notified, Notifier};

// Re-export the sync core so integration tests (and the dispatch path) can name it
// without depending on `flowcore` directly.
pub use flowcore::store::SqliteStore;

/// The native store is always backed by rusqlite.
pub type Core = SqliteStore<RusqliteSql>;

/// The storage seam the gRPC layer depends on. Reads + mutations; every mutation
/// returns its `Mutation` payload and (via the edge) publishes a `Notified` to
/// Watch subscribers, so the unary and stream paths share one apply-path.
#[::async_trait::async_trait]
pub trait Store: Send + Sync {
    async fn list_projects(&self, include_archived: bool) -> Result<Vec<pb::Project>, DomainError>;
    async fn get_snapshot(&self, project_id: &str) -> Result<pb::GetSnapshotResponse, DomainError>;
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
    async fn events_after(
        &self,
        project_id: &str,
        from_seq: i64,
    ) -> Result<Vec<pb::Event>, DomainError>;
    async fn poll_changes(
        &self,
        project_id: &str,
        after_seq: i64,
        limit: i32,
    ) -> Result<pb::PollChangesResponse, DomainError>;

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
    async fn move_node(&self, req: pb::MoveNodeRequest) -> Result<pb::Mutation, DomainError>;

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

/// Run a blocking store op off the tokio reactor, mapping a task panic to an
/// internal error.
async fn offload<T, F>(f: F) -> Result<T, DomainError>
where
    F: FnOnce() -> Result<T, DomainError> + Send + 'static,
    T: Send + 'static,
{
    tokio::task::spawn_blocking(f)
        .await
        .map_err(|e| DomainError::internal(format!("db task panicked: {e}")))?
}

/// The native store: `flowcore`'s sync core + a Watch notifier.
#[derive(Clone)]
pub struct NativeStore {
    core: Core,
    notifier: Notifier,
}

impl NativeStore {
    pub fn new(core: Core) -> Self {
        let (notifier, _) = watch::channel(256);
        Self { core, notifier }
    }

    /// Publish a committed mutation to Watch subscribers. Skips no-op mutations
    /// (idempotent replays carry no events).
    fn publish(&self, m: &pb::Mutation) {
        let Some(first) = m.events.first() else {
            return;
        };
        let _ = self.notifier.send(Notified {
            project_id: first.project_id.clone(),
            seq: m.seq,
            events: m.events.clone(),
            changed_nodes: m.changed_nodes.clone(),
            changed_progress: m.changed_progress.clone(),
        });
    }
}

/// Offload a read closure `|core| -> Result<..>` to the blocking pool.
macro_rules! read {
    ($self:ident, $body:expr) => {{
        let core = $self.core.clone();
        offload(move || $body(&core)).await
    }};
}

/// Offload a write (`core.$method($req)`), then publish its mutation.
macro_rules! write_pub {
    ($self:ident, $method:ident, $req:ident) => {{
        let core = $self.core.clone();
        let m = offload(move || core.$method($req)).await?;
        $self.publish(&m);
        Ok(m)
    }};
}

#[::async_trait::async_trait]
impl Store for NativeStore {
    async fn list_projects(&self, include_archived: bool) -> Result<Vec<pb::Project>, DomainError> {
        read!(self, move |c: &Core| c
            .list_projects_locked(include_archived))
    }
    async fn get_snapshot(&self, project_id: &str) -> Result<pb::GetSnapshotResponse, DomainError> {
        let project_id = project_id.to_string();
        read!(self, move |c: &Core| c.get_snapshot_locked(&project_id))
    }
    async fn events_after(
        &self,
        project_id: &str,
        from_seq: i64,
    ) -> Result<Vec<pb::Event>, DomainError> {
        let project_id = project_id.to_string();
        read!(self, move |c: &Core| c
            .events_after_locked(&project_id, from_seq))
    }
    async fn poll_changes(
        &self,
        project_id: &str,
        after_seq: i64,
        limit: i32,
    ) -> Result<pb::PollChangesResponse, DomainError> {
        let project_id = project_id.to_string();
        read!(self, move |c: &Core| c.poll_changes_locked(
            &project_id,
            after_seq,
            limit
        ))
    }
    async fn list_events(
        &self,
        project_id: &str,
        node_id: &str,
        before_seq: i64,
        limit: i32,
    ) -> Result<(Vec<pb::Event>, bool), DomainError> {
        let project_id = project_id.to_string();
        let node_id = node_id.to_string();
        read!(self, move |c: &Core| c.list_events_locked(
            &project_id,
            &node_id,
            before_seq,
            limit
        ))
    }
    async fn search(
        &self,
        project_id: &str,
        query: &str,
        limit: i32,
    ) -> Result<Vec<pb::Node>, DomainError> {
        let project_id = project_id.to_string();
        let query = query.to_string();
        read!(self, move |c: &Core| c.search_locked(
            &project_id,
            &query,
            limit
        ))
    }

    fn subscribe(&self) -> broadcast::Receiver<Notified> {
        self.notifier.subscribe()
    }

    async fn create_node(&self, req: pb::CreateNodeRequest) -> Result<pb::Mutation, DomainError> {
        write_pub!(self, create_node_locked, req)
    }
    async fn update_node(&self, req: pb::UpdateNodeRequest) -> Result<pb::Mutation, DomainError> {
        write_pub!(self, update_node_locked, req)
    }
    async fn delete_node(&self, req: pb::DeleteNodeRequest) -> Result<pb::Mutation, DomainError> {
        write_pub!(self, delete_node_locked, req)
    }
    async fn set_status(&self, req: pb::SetStatusRequest) -> Result<pb::Mutation, DomainError> {
        write_pub!(self, set_status_locked, req)
    }
    async fn add_dependency(
        &self,
        req: pb::AddDependencyRequest,
    ) -> Result<pb::Mutation, DomainError> {
        write_pub!(self, add_dependency_locked, req)
    }
    async fn remove_dependency(
        &self,
        req: pb::RemoveDependencyRequest,
    ) -> Result<pb::Mutation, DomainError> {
        write_pub!(self, remove_dependency_locked, req)
    }
    async fn report_condition(
        &self,
        req: pb::ReportConditionRequest,
    ) -> Result<pb::Mutation, DomainError> {
        write_pub!(self, report_condition_locked, req)
    }
    async fn set_verdict(&self, req: pb::SetVerdictRequest) -> Result<pb::Mutation, DomainError> {
        write_pub!(self, set_verdict_locked, req)
    }
    async fn add_comment(&self, req: pb::AddCommentRequest) -> Result<pb::Mutation, DomainError> {
        write_pub!(self, add_comment_locked, req)
    }
    async fn undo(&self, req: pb::UndoRequest) -> Result<pb::Mutation, DomainError> {
        write_pub!(self, undo_locked, req)
    }
    async fn move_node(&self, req: pb::MoveNodeRequest) -> Result<pb::Mutation, DomainError> {
        write_pub!(self, move_node_locked, req)
    }
    async fn create_project(
        &self,
        req: pb::CreateProjectRequest,
    ) -> Result<pb::Project, DomainError> {
        let core = self.core.clone();
        offload(move || core.create_project_locked(req)).await
    }
    async fn update_project(
        &self,
        req: pb::UpdateProjectRequest,
    ) -> Result<pb::Project, DomainError> {
        let core = self.core.clone();
        offload(move || core.update_project_locked(req)).await
    }
    async fn archive_project(
        &self,
        req: pb::ArchiveProjectRequest,
    ) -> Result<pb::Project, DomainError> {
        let core = self.core.clone();
        offload(move || core.archive_project_locked(req)).await
    }
}
