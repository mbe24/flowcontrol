//! The storage seam. `SqliteStore` is the v1 implementation; a hosted DB or an
//! in-memory fake can implement the same trait later without touching grpc/.

use std::sync::Arc;

use crate::generated::fctrl_v1 as pb;

/// Read-only view of the store the gRPC read handlers need. Mutations come in a
/// later iteration.
#[::async_trait::async_trait]
pub trait Store: Send + Sync {
    async fn list_projects(&self, include_archived: bool) -> Result<Vec<pb::Project>, Box<dyn std::error::Error + Send + Sync>>;
    async fn get_snapshot(&self, project_id: &str) -> Result<pb::GetSnapshotResponse, Box<dyn std::error::Error + Send + Sync>>;
    async fn list_events(&self, project_id: &str, limit: i32) -> Result<Vec<pb::Event>, Box<dyn std::error::Error + Send + Sync>>;
    async fn search(&self, project_id: &str, query: &str, limit: i32) -> Result<Vec<pb::Node>, Box<dyn std::error::Error + Send + Sync>>;
}

pub type DynStore = Arc<dyn Store>;
