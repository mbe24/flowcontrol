//! SQLite implementation of `Store`, using sqlx. Runs the embedded migration
//! and exposes the read operations the gRPC handlers need.

use std::sync::Arc;

use sqlx::sqlite::SqlitePool;
use sqlx::Row;

use super::Store;
use crate::generated::flow_v1 as pb;

/// Converts a status string (as stored / from the node_state view) to the proto
/// `EffectiveStatus` enum.
fn effective_status(s: &str) -> i32 {
    match s {
        "READY" => pb::EffectiveStatus::Ready as i32,
        "BLOCKED" => pb::EffectiveStatus::Blocked as i32,
        "DEFERRED" => pb::EffectiveStatus::Deferred as i32,
        "DONE" => pb::EffectiveStatus::Done as i32,
        _ => pb::EffectiveStatus::Unspecified as i32,
    }
}

/// SQLite-backed store. Cheap to clone: wraps an `Arc<SqlitePool>`.
#[derive(Clone)]
pub struct SqliteStore {
    pool: Arc<SqlitePool>,
}

impl SqliteStore {
    /// Open a pool and run migrations against it.
    pub async fn connect(url: &str) -> Result<Self, sqlx::Error> {
        let pool = SqlitePool::connect(url).await?;
        sqlx::migrate!("./migrations").run(&pool).await?;
        Ok(Self { pool: Arc::new(pool) })
    }
    
    /// Wrap an already-migrated pool. Used by tests that seed after connecting.
    pub fn from_pool(pool: SqlitePool) -> Self {
        Self { pool: Arc::new(pool) }
    }
}

#[::async_trait::async_trait]
impl Store for SqliteStore {
    async fn list_projects(&self, include_archived: bool) -> Result<Vec<pb::Project>, Box<dyn std::error::Error + Send + Sync>> {
        let rows = sqlx::query(
            "SELECT id, name, description, COALESCE(archived_at,0) AS archived_at, created_at
             FROM projects
             WHERE ? = 1 OR archived_at IS NULL
             ORDER BY created_at",
        )
        .bind(include_archived as i32)
        .fetch_all(&*self.pool)
        .await?;

        let mut out = Vec::with_capacity(rows.len());
        for r in rows {
            out.push(pb::Project {
                id: r.get("id"),
                name: r.get("name"),
                description: r.get("description"),
                archived_at: r.get("archived_at"),
                created_at: r.get("created_at"),
            });
        }
        Ok(out)
    }

    async fn get_snapshot(&self, project_id: &str) -> Result<pb::GetSnapshotResponse, Box<dyn std::error::Error + Send + Sync>> {
        // Project (may be absent).
        let proj = sqlx::query(
            "SELECT id, name, description, COALESCE(archived_at,0) AS archived_at, created_at
             FROM projects WHERE id = ?",
        )
        .bind(project_id)
        .fetch_optional(&*self.pool)
        .await?;

        // Nodes with effective status from the node_state view.
        let rows = sqlx::query(
            "SELECT n.id, n.project_id, COALESCE(n.parent_id,'') AS parent_id,
                    n.kind, n.title, n.description, n.condition, COALESCE(n.reference,'') AS reference,
                    n.declared_status, n.wp_state, n.position, n.created_at, n.updated_at,
                    n.status AS effective_status
             FROM node_state n
             WHERE n.project_id = ?",
        )
        .bind(project_id)
        .fetch_all(&*self.pool)
        .await?;

        let mut nodes = Vec::with_capacity(rows.len());
        for r in rows {
            let kind = match r.get::<String, _>("kind").as_str() {
                "WORK_PACKAGE" => pb::NodeKind::WorkPackage,
                "TASK" => pb::NodeKind::Task,
                _ => pb::NodeKind::Step,
            };
            let declared = match r.get::<String, _>("declared_status").as_str() {
                "DEFERRED" => pb::DeclaredStatus::Deferred,
                "DONE" => pb::DeclaredStatus::Done,
                _ => pb::DeclaredStatus::Open,
            };
            let wp_state = match r.get::<Option<String>, _>("wp_state") {
                Some(s) if s == "ACTIVE" => pb::WorkPackageState::Active,
                Some(s) if s == "DONE" => pb::WorkPackageState::Done,
                Some(s) if s == "ARCHIVED" => pb::WorkPackageState::Archived,
                _ => pb::WorkPackageState::Planned,
            };
            nodes.push(pb::Node {
                id: r.get("id"),
                project_id: r.get("project_id"),
                parent_id: r.get("parent_id"),
                kind: kind as i32,
                title: r.get("title"),
                description: r.get("description"),
                condition: r.get("condition"),
                reference: r.get("reference"),
                declared_status: declared as i32,
                status: effective_status(r.get::<String, _>("effective_status").as_str()),
                wp_state: wp_state as i32,
                position: r.get("position"),
                verification: None,
                created_at: r.get("created_at"),
                updated_at: r.get("updated_at"),
            });
        }

        // Dependencies for the project.
        let dep_rows = sqlx::query(
            "SELECT d.blocker_id, d.blocked_id
             FROM dependencies d
             JOIN nodes a ON a.id = d.blocker_id
             JOIN nodes b ON b.id = d.blocked_id
             WHERE a.project_id = ? AND b.project_id = ?",
        )
        .bind(project_id)
        .bind(project_id)
        .fetch_all(&*self.pool)
        .await?;
        let mut deps = Vec::with_capacity(dep_rows.len());
        for r in dep_rows {
            deps.push(pb::Dependency {
                blocker_id: r.get("blocker_id"),
                blocked_id: r.get("blocked_id"),
            });
        }

        // Progress per work package from the view.
        let prog_rows = sqlx::query(
            "SELECT wp_id, total, done, deferred, open_count FROM wp_progress",
        )
        .fetch_all(&*self.pool)
        .await?;
        let mut progress = Vec::with_capacity(prog_rows.len());
        for r in prog_rows {
            progress.push(pb::Progress {
                work_package_id: r.get::<String, _>("wp_id"),
                total: r.get::<i64, _>("total") as i32,
                done: r.get::<i64, _>("done") as i32,
                ready: 0,
                blocked: 0,
                deferred: r.get::<i64, _>("deferred") as i32,
            });
        }

        // Recent events (limit 25) + current seq.
        let event_rows = sqlx::query(
            "SELECT seq, project_id, COALESCE(node_id,'') AS node_id, kind, author, summary, payload AS payload_json, created_at
             FROM events WHERE project_id = ?
             ORDER BY seq DESC LIMIT 25",
        )
        .bind(project_id)
        .fetch_all(&*self.pool)
        .await?;
        let mut events = Vec::with_capacity(event_rows.len());
        for r in event_rows {
            events.push(proto_event(r));
        }
        let seq_row = sqlx::query("SELECT COALESCE(MAX(seq),0) AS seq FROM events")
            .fetch_one(&*self.pool)
            .await?;
        let seq: i64 = seq_row.get("seq");

        Ok(pb::GetSnapshotResponse {
            project: proj.map(|r| pb::Project {
                id: r.get("id"),
                name: r.get("name"),
                description: r.get("description"),
                archived_at: r.get("archived_at"),
                created_at: r.get("created_at"),
            }),
            nodes,
            dependencies: deps,
            progress,
            recent_events: events,
            seq,
        })
    }

    async fn list_events(&self, project_id: &str, limit: i32) -> Result<Vec<pb::Event>, Box<dyn std::error::Error + Send + Sync>> {
        let rows = sqlx::query(
            "SELECT seq, project_id, COALESCE(node_id,'') AS node_id, kind, author, summary, payload AS payload_json, created_at
             FROM events WHERE project_id = ?
             ORDER BY seq DESC LIMIT ?",
        )
        .bind(project_id)
        .bind(limit.max(1))
        .fetch_all(&*self.pool)
        .await?;
        let mut out = Vec::with_capacity(rows.len());
        for r in rows {
            out.push(proto_event(r));
        }
        Ok(out)
    }

    async fn search(&self, project_id: &str, query: &str, limit: i32) -> Result<Vec<pb::Node>, Box<dyn std::error::Error + Send + Sync>> {
        let rows = sqlx::query(
            "SELECT n.id, n.project_id, COALESCE(n.parent_id,'') AS parent_id,
                    n.kind, n.title, n.description, n.condition, COALESCE(n.reference,'') AS reference,
                    n.declared_status, n.wp_state, n.position, n.created_at, n.updated_at
             FROM nodes n
             JOIN nodes_fts f ON f.rowid = n.rowid
             WHERE n.project_id = ? AND nodes_fts MATCH ?
             ORDER BY rank LIMIT ?",
        )
        .bind(project_id)
        .bind(query)
        .bind(limit.max(1))
        .fetch_all(&*self.pool)
        .await?;
        let mut out = Vec::with_capacity(rows.len());
        for r in rows {
            let kind = match r.get::<String, _>("kind").as_str() {
                "WORK_PACKAGE" => pb::NodeKind::WorkPackage,
                "TASK" => pb::NodeKind::Task,
                _ => pb::NodeKind::Step,
            };
            let declared = match r.get::<String, _>("declared_status").as_str() {
                "DEFERRED" => pb::DeclaredStatus::Deferred,
                "DONE" => pb::DeclaredStatus::Done,
                _ => pb::DeclaredStatus::Open,
            };
            out.push(pb::Node {
                id: r.get("id"),
                project_id: r.get("project_id"),
                parent_id: r.get("parent_id"),
                kind: kind as i32,
                title: r.get("title"),
                description: r.get("description"),
                condition: r.get("condition"),
                reference: r.get("reference"),
                declared_status: declared as i32,
                status: pb::EffectiveStatus::Unspecified as i32,
                wp_state: 0,
                position: r.get("position"),
                verification: None,
                created_at: r.get("created_at"),
                updated_at: r.get("updated_at"),
            });
        }
        Ok(out)
    }
}

/// Map a row to a proto `Event`. `payload` column is named `payload` in the
/// schema (see events table); the proto field is `payload_json`.
fn proto_event(r: sqlx::sqlite::SqliteRow) -> pb::Event {
    let kind = match r.get::<String, _>("kind").as_str() {
        "NODE_CREATED" => pb::EventKind::NodeCreated,
        "NODE_UPDATED" => pb::EventKind::NodeUpdated,
        "NODE_DELETED" => pb::EventKind::NodeDeleted,
        "STATUS_SET" => pb::EventKind::StatusSet,
        "DEP_ADDED" => pb::EventKind::DepAdded,
        "DEP_REMOVED" => pb::EventKind::DepRemoved,
        "AGENT_REPORTED" => pb::EventKind::AgentReported,
        "VERDICT_SET" => pb::EventKind::VerdictSet,
        "COMMENT" => pb::EventKind::Comment,
        _ => pb::EventKind::Unspecified,
    };
    pb::Event {
        seq: r.get("seq"),
        project_id: r.get("project_id"),
        node_id: r.get("node_id"),
        kind: kind as i32,
        author: r.get("author"),
        summary: r.get("summary"),
        payload_json: r.get("payload_json"),
        created_at: r.get("created_at"),
    }
}
