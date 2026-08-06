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
    async fn create_node(&self, req: pb::CreateNodeRequest) -> Result<pb::Mutation, Box<dyn std::error::Error + Send + Sync>> {
        let node_id = new_node_id();
        let wp = if req.kind == pb::NodeKind::WorkPackage as i32 { Some("PLANNED") } else { None };
        sqlx::query(
            "INSERT INTO nodes (id, project_id, parent_id, kind, title, description, condition, reference, declared_status, wp_state, position)
             VALUES (?, ?, NULLIF(?, ''), ?, ?, ?, ?, NULLIF(?, ''), 'OPEN', ?, ?)",
        )
        .bind(&node_id)
        .bind(&req.project_id)
        .bind(&req.parent_id)
        .bind(kind_str(req.kind))
        .bind(&req.title)
        .bind(&req.description)
        .bind(&req.condition)
        .bind(&req.reference)
        .bind(wp)
        .bind(req.position)
        .execute(&*self.pool)
        .await?;
        let author = req.meta.as_ref().map(|m| m.author.as_str()).unwrap_or("").to_string();
        let seq = append_event(&self.pool, &req.project_id, &node_id, "NODE_CREATED", &author, &format!("created {}", node_id)).await?;
        let nodes = self.fetch_node(&node_id).await?.into_iter().collect();
        Ok(pb::Mutation { events: vec![], changed_nodes: nodes, changed_progress: vec![], seq })
    }

    async fn update_node(&self, req: pb::UpdateNodeRequest) -> Result<pb::Mutation, Box<dyn std::error::Error + Send + Sync>> {
        let current = self.fetch_node(&req.node_id).await?.ok_or("node not found")?;
        let project_id = current.project_id.clone();
        if req.update_mask.is_empty() {
            return Err("update_mask cannot be empty".into());
        }
        for f in &req.update_mask {
            match f.as_str() {
                "title" => { sqlx::query("UPDATE nodes SET title = ? WHERE id = ?").bind(&req.title).bind(&req.node_id).execute(&*self.pool).await?; }
                "description" => { sqlx::query("UPDATE nodes SET description = ? WHERE id = ?").bind(&req.description).bind(&req.node_id).execute(&*self.pool).await?; }
                "condition" => { sqlx::query("UPDATE nodes SET condition = ? WHERE id = ?").bind(&req.condition).bind(&req.node_id).execute(&*self.pool).await?; }
                "position" => { sqlx::query("UPDATE nodes SET position = ? WHERE id = ?").bind(req.position).bind(&req.node_id).execute(&*self.pool).await?; }
                "reference" => { sqlx::query("UPDATE nodes SET reference = ? WHERE id = ?").bind(&req.reference).bind(&req.node_id).execute(&*self.pool).await?; }
                "wp_state" => { sqlx::query("UPDATE nodes SET wp_state = ? WHERE id = ?").bind(wp_str(req.wp_state)).bind(&req.node_id).execute(&*self.pool).await?; }
                other => return Err(format!("unknown update_mask field: {}", other).into()),
            }
        }
        let author = req.meta.as_ref().map(|m| m.author.as_str()).unwrap_or("").to_string();
        let seq = append_event(&self.pool, &project_id, &req.node_id, "NODE_UPDATED", &author, &format!("updated {}", req.node_id)).await?;
        let nodes = self.fetch_node(&req.node_id).await?.into_iter().collect();
        Ok(pb::Mutation { events: vec![], changed_nodes: nodes, changed_progress: vec![], seq })
    }

    async fn delete_node(&self, req: pb::DeleteNodeRequest) -> Result<pb::Mutation, Box<dyn std::error::Error + Send + Sync>> {
        let current = self.fetch_node(&req.node_id).await?.ok_or("node not found")?;
        let project_id = current.project_id.clone();
        if req.fail_if_referenced {
            let deps: i64 = sqlx::query_scalar("SELECT count(*) FROM dependencies WHERE blocker_id = ? OR blocked_id = ?")
                .bind(&req.node_id).bind(&req.node_id).fetch_one(&*self.pool).await?;
            if deps > 0 { return Err("node has dependents".into()); }
        }
        sqlx::query("DELETE FROM nodes WHERE id = ?").bind(&req.node_id).execute(&*self.pool).await?;
        let author = req.meta.as_ref().map(|m| m.author.as_str()).unwrap_or("").to_string();
        let seq = append_event(&self.pool, &project_id, &req.node_id, "NODE_DELETED", &author, &format!("deleted {}", req.node_id)).await?;
        Ok(pb::Mutation { events: vec![], changed_nodes: vec![], changed_progress: vec![], seq })
    }

    async fn set_status(&self, req: pb::SetStatusRequest) -> Result<pb::Mutation, Box<dyn std::error::Error + Send + Sync>> {
        let current = self.fetch_node(&req.node_id).await?.ok_or("node not found")?;
        let project_id = current.project_id.clone();
        let new = declared_str(req.declared_status);
        sqlx::query("UPDATE nodes SET declared_status = ? WHERE id = ?").bind(new).bind(&req.node_id).execute(&*self.pool).await?;
        let author = req.meta.as_ref().map(|m| m.author.as_str()).unwrap_or("").to_string();
        let seq = append_event(&self.pool, &project_id, &req.node_id, "STATUS_SET", &author, &format!("{} -> {}", req.node_id, new)).await?;
        let nodes = self.fetch_node(&req.node_id).await?.into_iter().collect();
        Ok(pb::Mutation { events: vec![], changed_nodes: nodes, changed_progress: vec![], seq })
    }

    async fn add_dependency(&self, req: pb::AddDependencyRequest) -> Result<pb::Mutation, Box<dyn std::error::Error + Send + Sync>> {
        sqlx::query("INSERT OR IGNORE INTO dependencies (blocker_id, blocked_id) VALUES (?, ?)")
            .bind(&req.blocker_id).bind(&req.blocked_id).execute(&*self.pool).await?;
        let project_id = self.project_of(&req.blocker_id).await?;
        let author = req.meta.as_ref().map(|m| m.author.as_str()).unwrap_or("").to_string();
        let seq = append_event(&self.pool, &project_id, "", "DEP_ADDED", &author, &format!("{} blocks {}", req.blocker_id, req.blocked_id)).await?;
        Ok(pb::Mutation { events: vec![], changed_nodes: vec![], changed_progress: vec![], seq })
    }

    async fn remove_dependency(&self, req: pb::RemoveDependencyRequest) -> Result<pb::Mutation, Box<dyn std::error::Error + Send + Sync>> {
        sqlx::query("DELETE FROM dependencies WHERE blocker_id = ? AND blocked_id = ?")
            .bind(&req.blocker_id).bind(&req.blocked_id).execute(&*self.pool).await?;
        let project_id = self.project_of(&req.blocker_id).await?;
        let author = req.meta.as_ref().map(|m| m.author.as_str()).unwrap_or("").to_string();
        let seq = append_event(&self.pool, &project_id, "", "DEP_REMOVED", &author, &format!("{} no longer blocks {}", req.blocker_id, req.blocked_id)).await?;
        Ok(pb::Mutation { events: vec![], changed_nodes: vec![], changed_progress: vec![], seq })
    }
}

// ── writes ────────────────────────────────────────────────────────────────

use std::time::{SystemTime, UNIX_EPOCH};

/// Short, sortable, unique node id: unix-millis + random hex suffix.
fn new_node_id() -> String {
    let ms = SystemTime::now().duration_since(UNIX_EPOCH).unwrap_or_default().as_millis();
    let rand = {
        use std::collections::hash_map::RandomState;
        use std::hash::{BuildHasher, Hasher};
        RandomState::new().build_hasher().finish()
    };
    format!("node-{}-{:08x}", ms, rand)
}

fn kind_str(k: i32) -> &'static str {
    match k { 1 => "WORK_PACKAGE", 2 => "TASK", _ => "STEP" }
}
fn declared_str(d: i32) -> &'static str {
    match d { 1 => "OPEN", 2 => "DEFERRED", _ => "DONE" }
}
fn wp_str(w: i32) -> Option<&'static str> {
    match w { 1 => Some("PLANNED"), 2 => Some("ACTIVE"), 3 => Some("DONE"), 4 => Some("ARCHIVED"), _ => None }
}

impl SqliteStore {
    /// Fetch one node by id as a proto Node (None if missing).
    async fn fetch_node(&self, id: &str) -> Result<Option<pb::Node>, Box<dyn std::error::Error + Send + Sync>> {
        let row = sqlx::query(
            "SELECT n.id, n.project_id, COALESCE(n.parent_id,'') AS parent_id,
                    n.kind, n.title, n.description, n.condition, n.reference,
                    n.declared_status, n.wp_state, n.position, n.created_at, n.updated_at,
                    n.status AS effective_status
             FROM node_state n WHERE n.id = ?",
        )
        .bind(id).fetch_optional(&*self.pool).await?;
        Ok(row.map(row_to_node))
    }

    /// Owning project of a node.
    async fn project_of(&self, node_id: &str) -> Result<String, Box<dyn std::error::Error + Send + Sync>> {
        Ok(sqlx::query_scalar("SELECT project_id FROM nodes WHERE id = ?")
            .bind(node_id).fetch_optional(&*self.pool).await?.unwrap_or_default())
    }
}
/// Convert a node_state row into a proto Node.
fn row_to_node(r: sqlx::sqlite::SqliteRow) -> pb::Node {
    let kind = match r.get::<String,_>("kind").as_str() {
        "WORK_PACKAGE" => pb::NodeKind::WorkPackage,
        "TASK" => pb::NodeKind::Task,
        _ => pb::NodeKind::Step,
    };
    let declared = match r.get::<String,_>("declared_status").as_str() {
        "DEFERRED" => pb::DeclaredStatus::Deferred,
        "DONE" => pb::DeclaredStatus::Done,
        _ => pb::DeclaredStatus::Open,
    };
    let wp_state = match r.get::<Option<String>,_>("wp_state") {
        Some(s) if s == "ACTIVE" => pb::WorkPackageState::Active,
        Some(s) if s == "DONE" => pb::WorkPackageState::Done,
        Some(s) if s == "ARCHIVED" => pb::WorkPackageState::Archived,
        _ => pb::WorkPackageState::Planned,
    };
    pb::Node {
        id: r.get("id"),
        project_id: r.get("project_id"),
        parent_id: r.get("parent_id"),
        kind: kind as i32,
        title: r.get("title"),
        description: r.get("description"),
        condition: r.get("condition"),
        reference: r.get("reference"),
        declared_status: declared as i32,
        status: effective_status(r.get::<String,_>("effective_status").as_str()),
        wp_state: wp_state as i32,
        position: r.get("position"),
        verification: None,
        created_at: r.get("created_at"),
        updated_at: r.get("updated_at"),
    }
}

/// Insert an event row; returns its seq.
async fn append_event(pool: &Arc<SqlitePool>, project_id: &str, node_id: &str, kind: &str, author: &str, summary: &str) -> Result<i64, Box<dyn std::error::Error + Send + Sync>> {
    let row = sqlx::query(
        "INSERT INTO events (project_id, node_id, kind, author, summary, payload)
         VALUES (?, ?, ?, ?, ?, '{}') RETURNING seq",
    )
    .bind(project_id)
    .bind(if node_id.is_empty() { None } else { Some(node_id) })
    .bind(kind)
    .bind(author)
    .bind(summary)
    .fetch_one(&**pool)
    .await?;
    Ok(row.get("seq"))
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
