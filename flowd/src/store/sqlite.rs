//! SQLite implementation of `Store`, using sqlx. Runs the embedded migration
//! and exposes the read operations the gRPC handlers need.

use std::collections::HashSet;
use std::sync::Arc;

use sqlx::sqlite::SqlitePool;
use sqlx::Row;

use super::Store;
use crate::generated::flow_v1 as pb;
use crate::store::watch::{self, Notified, Notifier};
use tokio::sync::broadcast;

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
    notifier: Notifier,
}

impl SqliteStore {
    /// Open a pool and run migrations against it.
    pub async fn connect(url: &str) -> Result<Self, sqlx::Error> {
        let pool = SqlitePool::connect(url).await?;
        sqlx::migrate!("./migrations").run(&pool).await?;
        let (notifier, _) = watch::channel(256);
        Ok(Self {
            pool: Arc::new(pool),
            notifier,
        })
    }

    /// Wrap an already-migrated pool. Used by tests that seed after connecting.
    pub fn from_pool(pool: SqlitePool) -> Self {
        let (notifier, _) = watch::channel(256);
        Self {
            pool: Arc::new(pool),
            notifier,
        }
    }

    /// A mutation with no events/nodes, carrying only a cursor. Returned for an
    /// idempotent retry (the original side effects already happened).
    fn empty_mutation(seq: i64) -> pb::Mutation {
        pb::Mutation {
            events: vec![],
            changed_nodes: vec![],
            changed_progress: vec![],
            seq,
        }
    }
}

#[::async_trait::async_trait]
impl Store for SqliteStore {
    async fn list_projects(
        &self,
        include_archived: bool,
    ) -> Result<Vec<pb::Project>, Box<dyn std::error::Error + Send + Sync>> {
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

    async fn get_snapshot(
        &self,
        project_id: &str,
    ) -> Result<pb::GetSnapshotResponse, Box<dyn std::error::Error + Send + Sync>> {
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
                    n.kind, n.title, n.description, n.condition, n.note, COALESCE(n.reference,'') AS reference,
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
            let mut node = row_to_node(r);
            node.verification = self.verification_for(&node.id).await?;
            nodes.push(node);
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
        let progress = self.progress_for(project_id).await?;

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

    async fn events_after(
        &self,
        project_id: &str,
        from_seq: i64,
    ) -> Result<Vec<pb::Event>, Box<dyn std::error::Error + Send + Sync>> {
        let rows = sqlx::query(
            "SELECT seq, project_id, COALESCE(node_id,'') AS node_id, kind, author, summary, payload AS payload_json, created_at
             FROM events WHERE project_id = ? AND seq > ?
             ORDER BY seq ASC",
        )
        .bind(project_id)
        .bind(from_seq)
        .fetch_all(&*self.pool)
        .await?;
        let mut out = Vec::with_capacity(rows.len());
        for r in rows {
            out.push(proto_event(r));
        }
        Ok(out)
    }

    async fn poll_changes(
        &self,
        project_id: &str,
        after_seq: i64,
        limit: i32,
    ) -> Result<pb::PollChangesResponse, Box<dyn std::error::Error + Send + Sync>> {
        let lim = if limit <= 0 { 1000 } else { limit };
        let rows = sqlx::query(
            "SELECT seq, project_id, COALESCE(node_id,'') AS node_id, kind, author, summary, payload AS payload_json, created_at
             FROM events WHERE project_id = ? AND seq > ?
             ORDER BY seq ASC LIMIT ?",
        )
        .bind(project_id)
        .bind(after_seq)
        .bind(lim)
        .fetch_all(&*self.pool)
        .await?;
        let mut events = Vec::with_capacity(rows.len());
        for r in rows {
            events.push(proto_event(r));
        }
        // Next cursor: the last event returned, or the caller's cursor if nothing
        // new (never skip events on the next poll).
        let seq = events.last().map(|e| e.seq).unwrap_or(after_seq);
        Ok(pb::PollChangesResponse { events, seq })
    }

    async fn list_events(
        &self,
        project_id: &str,
        node_id: &str,
        before_seq: i64,
        limit: i32,
    ) -> Result<(Vec<pb::Event>, bool), Box<dyn std::error::Error + Send + Sync>> {
        let lim = limit.max(1);
        // Fetch one extra to tell whether an older page exists. An empty node_id
        // means "no node filter"; before_seq 0 means "from the newest".
        let rows = sqlx::query(
            "SELECT seq, project_id, COALESCE(node_id,'') AS node_id, kind, author, summary, payload AS payload_json, created_at
             FROM events
             WHERE project_id = ?
               AND (? = '' OR node_id = ?)
               AND (? = 0 OR seq < ?)
             ORDER BY seq DESC LIMIT ?",
        )
        .bind(project_id)
        .bind(node_id)
        .bind(node_id)
        .bind(before_seq)
        .bind(before_seq)
        .bind(lim + 1)
        .fetch_all(&*self.pool)
        .await?;
        let has_more = rows.len() as i32 > lim;
        let out: Vec<pb::Event> = rows.into_iter().take(lim as usize).map(proto_event).collect();
        Ok((out, has_more))
    }

    async fn search(
        &self,
        project_id: &str,
        query: &str,
        limit: i32,
    ) -> Result<Vec<pb::Node>, Box<dyn std::error::Error + Send + Sync>> {
        let rows = sqlx::query(
            "SELECT n.id, n.project_id, COALESCE(n.parent_id,'') AS parent_id,
                    n.kind, n.title, n.description, n.condition, n.note, COALESCE(n.reference,'') AS reference,
                    n.declared_status, n.wp_state, n.position, n.created_at, n.updated_at,
                    ns.status AS effective_status
             FROM nodes n
             JOIN node_state ns ON ns.id = n.id
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
            let mut node = row_to_node(r);
            node.verification = self.verification_for(&node.id).await?;
            out.push(node);
        }
        Ok(out)
    }

    fn subscribe(&self) -> broadcast::Receiver<Notified> {
        self.notifier.subscribe()
    }

    async fn create_node(
        &self,
        req: pb::CreateNodeRequest,
    ) -> Result<pb::Mutation, Box<dyn std::error::Error + Send + Sync>> {
        if let Some(hit) = self
            .check_idempotency_created(&req.project_id, idem_key(req.meta.as_ref()))
            .await?
        {
            // Replay: return the node the original request created, so a retry
            // (e.g. an agent after a timeout) still learns the id.
            if let Some(eid) = &hit.entity_id {
                if let Some(n) = self.fetch_node(eid).await? {
                    return Ok(pb::Mutation {
                        events: vec![],
                        changed_nodes: vec![n],
                        changed_progress: vec![],
                        seq: hit.seq,
                    });
                }
            }
            return Ok(Self::empty_mutation(hit.seq));
        }
        let node_id = new_node_id();
        let wp = if req.kind == pb::NodeKind::WorkPackage as i32 {
            Some("PLANNED")
        } else {
            None
        };
        sqlx::query(
            "INSERT INTO nodes (id, project_id, parent_id, kind, title, description, condition, note, reference, declared_status, wp_state, position)
             VALUES (?, ?, NULLIF(?, ''), ?, ?, ?, ?, ?, NULLIF(?, ''), 'OPEN', ?, ?)",
        )
        .bind(&node_id)
        .bind(&req.project_id)
        .bind(&req.parent_id)
        .bind(kind_str(req.kind))
        .bind(&req.title)
        .bind(&req.description)
        .bind(&req.condition)
        .bind(&req.note)
        .bind(&req.reference)
        .bind(wp)
        .bind(req.position)
        .execute(&*self.pool)
        .await?;
        let author = author_of(req.meta.as_ref());
        let payload =
            serde_json::json!({ "after": { "id": &node_id, "title": &req.title } }).to_string();
        let event = append_event(
            &self.pool,
            &req.project_id,
            &node_id,
            "NODE_CREATED",
            &author,
            &format!("created {}", node_id),
            &payload,
        )
        .await?;
        let m = self
            .finish_mutation(&req.project_id, vec![event], vec![node_id.clone()])
            .await?;
        self.record_idempotency_created(
            &req.project_id,
            idem_key(req.meta.as_ref()),
            m.seq,
            &node_id,
        )
        .await?;
        Ok(m)
    }

    async fn update_node(
        &self,
        req: pb::UpdateNodeRequest,
    ) -> Result<pb::Mutation, Box<dyn std::error::Error + Send + Sync>> {
        let current = self
            .fetch_node(&req.node_id)
            .await?
            .ok_or("node not found")?;
        let project_id = current.project_id.clone();
        if let Some(seq) = self
            .check_idempotency(&project_id, idem_key(req.meta.as_ref()))
            .await?
        {
            return Ok(Self::empty_mutation(seq));
        }
        if req.update_mask.is_empty() {
            return Err("update_mask cannot be empty".into());
        }
        for f in &req.update_mask {
            match f.as_str() {
                "title" => {
                    sqlx::query("UPDATE nodes SET title = ? WHERE id = ?")
                        .bind(&req.title)
                        .bind(&req.node_id)
                        .execute(&*self.pool)
                        .await?;
                }
                "description" => {
                    sqlx::query("UPDATE nodes SET description = ? WHERE id = ?")
                        .bind(&req.description)
                        .bind(&req.node_id)
                        .execute(&*self.pool)
                        .await?;
                }
                "condition" => {
                    sqlx::query("UPDATE nodes SET condition = ? WHERE id = ?")
                        .bind(&req.condition)
                        .bind(&req.node_id)
                        .execute(&*self.pool)
                        .await?;
                }
                "position" => {
                    sqlx::query("UPDATE nodes SET position = ? WHERE id = ?")
                        .bind(req.position)
                        .bind(&req.node_id)
                        .execute(&*self.pool)
                        .await?;
                }
                "reference" => {
                    sqlx::query("UPDATE nodes SET reference = ? WHERE id = ?")
                        .bind(&req.reference)
                        .bind(&req.node_id)
                        .execute(&*self.pool)
                        .await?;
                }
                "note" => {
                    sqlx::query("UPDATE nodes SET note = ? WHERE id = ?")
                        .bind(&req.note)
                        .bind(&req.node_id)
                        .execute(&*self.pool)
                        .await?;
                }
                "wp_state" => {
                    sqlx::query("UPDATE nodes SET wp_state = ? WHERE id = ?")
                        .bind(wp_str(req.wp_state))
                        .bind(&req.node_id)
                        .execute(&*self.pool)
                        .await?;
                }
                other => return Err(format!("unknown update_mask field: {}", other).into()),
            }
        }
        let author = author_of(req.meta.as_ref());
        let event = append_event(
            &self.pool,
            &project_id,
            &req.node_id,
            "NODE_UPDATED",
            &author,
            &format!("updated {}", req.node_id),
            "{}",
        )
        .await?;
        let m = self
            .finish_mutation(&project_id, vec![event], vec![req.node_id.clone()])
            .await?;
        self.record_idempotency(&project_id, idem_key(req.meta.as_ref()), m.seq)
            .await?;
        Ok(m)
    }

    async fn delete_node(
        &self,
        req: pb::DeleteNodeRequest,
    ) -> Result<pb::Mutation, Box<dyn std::error::Error + Send + Sync>> {
        let current = self
            .fetch_node(&req.node_id)
            .await?
            .ok_or("node not found")?;
        let project_id = current.project_id.clone();
        if let Some(seq) = self
            .check_idempotency(&project_id, idem_key(req.meta.as_ref()))
            .await?
        {
            return Ok(Self::empty_mutation(seq));
        }
        if req.fail_if_referenced {
            let deps: i64 = sqlx::query_scalar(
                "SELECT count(*) FROM dependencies WHERE blocker_id = ? OR blocked_id = ?",
            )
            .bind(&req.node_id)
            .bind(&req.node_id)
            .fetch_one(&*self.pool)
            .await?;
            if deps > 0 {
                return Err("node has dependents".into());
            }
        }
        let before = node_before_json(&current);
        // Snapshot the whole subtree up front: the cascade delete removes children
        // silently, so we must collect their EXTERNAL dependents (whose effective
        // status may change) and emit a delete event per node before they vanish.
        let dependents: Vec<String> = sqlx::query_scalar(
            "WITH RECURSIVE subtree(id) AS (
                 SELECT ? UNION ALL
                 SELECT n.id FROM nodes n JOIN subtree s ON n.parent_id = s.id
             )
             SELECT DISTINCT d.blocked_id FROM dependencies d
             JOIN subtree s ON d.blocker_id = s.id
             WHERE d.blocked_id NOT IN (SELECT id FROM subtree)",
        )
        .bind(&req.node_id)
        .fetch_all(&*self.pool)
        .await?;
        let descendants: Vec<String> = sqlx::query_scalar(
            "WITH RECURSIVE subtree(id) AS (
                 SELECT ? UNION ALL
                 SELECT n.id FROM nodes n JOIN subtree s ON n.parent_id = s.id
             )
             SELECT id FROM subtree WHERE id <> ?",
        )
        .bind(&req.node_id)
        .bind(&req.node_id)
        .fetch_all(&*self.pool)
        .await?;
        sqlx::query("DELETE FROM nodes WHERE id = ?")
            .bind(&req.node_id)
            .execute(&*self.pool)
            .await?;
        let author = author_of(req.meta.as_ref());
        // Every delete event carries node_id = '' (the row is gone; events.node_id
        // has an FK). The root's `before` payload is what undo restores; children
        // carry a minimal payload (a subtree delete is not child-wise undoable).
        let mut events = Vec::with_capacity(descendants.len() + 1);
        for cid in &descendants {
            let payload = serde_json::json!({ "before": { "id": cid } }).to_string();
            events.push(
                append_event(
                    &self.pool,
                    &project_id,
                    "",
                    "NODE_DELETED",
                    &author,
                    &format!("deleted {}", cid),
                    &payload,
                )
                .await?,
            );
        }
        events.push(
            append_event(
                &self.pool,
                &project_id,
                "",
                "NODE_DELETED",
                &author,
                &format!("deleted {}", req.node_id),
                &before,
            )
            .await?,
        );
        let m = self
            .finish_mutation(&project_id, events, dependents)
            .await?;
        self.record_idempotency(&project_id, idem_key(req.meta.as_ref()), m.seq)
            .await?;
        Ok(m)
    }

    async fn set_status(
        &self,
        req: pb::SetStatusRequest,
    ) -> Result<pb::Mutation, Box<dyn std::error::Error + Send + Sync>> {
        let current = self
            .fetch_node(&req.node_id)
            .await?
            .ok_or("node not found")?;
        let project_id = current.project_id.clone();
        if let Some(seq) = self
            .check_idempotency(&project_id, idem_key(req.meta.as_ref()))
            .await?
        {
            return Ok(Self::empty_mutation(seq));
        }
        let before = declared_str(current.declared_status);
        let new = declared_str(req.declared_status);
        let dependents: Vec<String> =
            sqlx::query_scalar("SELECT blocked_id FROM dependencies WHERE blocker_id = ?")
                .bind(&req.node_id)
                .fetch_all(&*self.pool)
                .await?;
        sqlx::query("UPDATE nodes SET declared_status = ? WHERE id = ?")
            .bind(new)
            .bind(&req.node_id)
            .execute(&*self.pool)
            .await?;
        let author = author_of(req.meta.as_ref());
        let payload = serde_json::json!({ "before": before, "after": new }).to_string();
        let event = append_event(
            &self.pool,
            &project_id,
            &req.node_id,
            "STATUS_SET",
            &author,
            &format!("{} -> {}", req.node_id, new),
            &payload,
        )
        .await?;
        let mut affected = vec![req.node_id.clone()];
        affected.extend(dependents);
        let m = self
            .finish_mutation(&project_id, vec![event], affected)
            .await?;
        self.record_idempotency(&project_id, idem_key(req.meta.as_ref()), m.seq)
            .await?;
        Ok(m)
    }

    async fn add_dependency(
        &self,
        req: pb::AddDependencyRequest,
    ) -> Result<pb::Mutation, Box<dyn std::error::Error + Send + Sync>> {
        let project_id = self.project_of(&req.blocker_id).await?;
        if let Some(seq) = self
            .check_idempotency(&project_id, idem_key(req.meta.as_ref()))
            .await?
        {
            return Ok(Self::empty_mutation(seq));
        }
        sqlx::query("INSERT OR IGNORE INTO dependencies (blocker_id, blocked_id) VALUES (?, ?)")
            .bind(&req.blocker_id)
            .bind(&req.blocked_id)
            .execute(&*self.pool)
            .await?;
        let author = author_of(req.meta.as_ref());
        let payload =
            serde_json::json!({ "blocker_id": &req.blocker_id, "blocked_id": &req.blocked_id })
                .to_string();
        let event = append_event(
            &self.pool,
            &project_id,
            "",
            "DEP_ADDED",
            &author,
            &format!("{} blocks {}", req.blocker_id, req.blocked_id),
            &payload,
        )
        .await?;
        let m = self
            .finish_mutation(&project_id, vec![event], vec![req.blocked_id.clone()])
            .await?;
        self.record_idempotency(&project_id, idem_key(req.meta.as_ref()), m.seq)
            .await?;
        Ok(m)
    }

    async fn remove_dependency(
        &self,
        req: pb::RemoveDependencyRequest,
    ) -> Result<pb::Mutation, Box<dyn std::error::Error + Send + Sync>> {
        let project_id = self.project_of(&req.blocker_id).await?;
        if let Some(seq) = self
            .check_idempotency(&project_id, idem_key(req.meta.as_ref()))
            .await?
        {
            return Ok(Self::empty_mutation(seq));
        }
        sqlx::query("DELETE FROM dependencies WHERE blocker_id = ? AND blocked_id = ?")
            .bind(&req.blocker_id)
            .bind(&req.blocked_id)
            .execute(&*self.pool)
            .await?;
        let author = author_of(req.meta.as_ref());
        let payload =
            serde_json::json!({ "blocker_id": &req.blocker_id, "blocked_id": &req.blocked_id })
                .to_string();
        let event = append_event(
            &self.pool,
            &project_id,
            "",
            "DEP_REMOVED",
            &author,
            &format!("{} no longer blocks {}", req.blocker_id, req.blocked_id),
            &payload,
        )
        .await?;
        let m = self
            .finish_mutation(&project_id, vec![event], vec![req.blocked_id.clone()])
            .await?;
        self.record_idempotency(&project_id, idem_key(req.meta.as_ref()), m.seq)
            .await?;
        Ok(m)
    }

    async fn report_condition(
        &self,
        req: pb::ReportConditionRequest,
    ) -> Result<pb::Mutation, Box<dyn std::error::Error + Send + Sync>> {
        let current = self
            .fetch_node(&req.node_id)
            .await?
            .ok_or("node not found")?;
        let project_id = current.project_id.clone();
        if let Some(seq) = self
            .check_idempotency(&project_id, idem_key(req.meta.as_ref()))
            .await?
        {
            return Ok(Self::empty_mutation(seq));
        }
        let result = match req.result {
            1 => "PASS",
            2 => "FAIL",
            _ => return Err("invalid agent result".into()),
        };
        sqlx::query(
            "INSERT INTO verifications (node_id, agent_result, agent_name, agent_at, agent_node_rev, agent_detail)
             VALUES (?, ?, ?, unixepoch(), (SELECT updated_at FROM nodes WHERE id = ?), ?)
             ON CONFLICT(node_id) DO UPDATE SET
               agent_result = excluded.agent_result,
               agent_name   = excluded.agent_name,
               agent_at     = excluded.agent_at,
               agent_node_rev = excluded.agent_node_rev,
               agent_detail = excluded.agent_detail",
        )
        .bind(&req.node_id)
        .bind(result)
        .bind(author_of(req.meta.as_ref()))
        .bind(&req.node_id)
        .bind(&req.detail)
        .execute(&*self.pool)
        .await?;
        let author = author_of(req.meta.as_ref());
        let payload = serde_json::json!({ "result": result, "detail": &req.detail }).to_string();
        let event = append_event(
            &self.pool,
            &project_id,
            &req.node_id,
            "AGENT_REPORTED",
            &author,
            &format!("agent reported {} on {}", result, req.node_id),
            &payload,
        )
        .await?;
        let m = self
            .finish_mutation(&project_id, vec![event], vec![req.node_id.clone()])
            .await?;
        self.record_idempotency(&project_id, idem_key(req.meta.as_ref()), m.seq)
            .await?;
        Ok(m)
    }

    async fn set_verdict(
        &self,
        req: pb::SetVerdictRequest,
    ) -> Result<pb::Mutation, Box<dyn std::error::Error + Send + Sync>> {
        let current = self
            .fetch_node(&req.node_id)
            .await?
            .ok_or("node not found")?;
        let project_id = current.project_id.clone();
        if let Some(seq) = self
            .check_idempotency(&project_id, idem_key(req.meta.as_ref()))
            .await?
        {
            return Ok(Self::empty_mutation(seq));
        }
        match req.verdict {
            1 => {
                sqlx::query(
                    "INSERT INTO verifications (node_id, human_verdict, human_at)
                     VALUES (?, 'ACCEPTED', unixepoch())
                     ON CONFLICT(node_id) DO UPDATE SET human_verdict = 'ACCEPTED', human_at = unixepoch()",
                )
                .bind(&req.node_id).execute(&*self.pool).await?;
            }
            2 => {
                sqlx::query(
                    "INSERT INTO verifications (node_id, human_verdict, human_at)
                     VALUES (?, 'REJECTED', unixepoch())
                     ON CONFLICT(node_id) DO UPDATE SET human_verdict = 'REJECTED', human_at = unixepoch()",
                )
                .bind(&req.node_id).execute(&*self.pool).await?;
            }
            _ => {
                sqlx::query("UPDATE verifications SET human_verdict = NULL, human_at = NULL WHERE node_id = ?")
                    .bind(&req.node_id).execute(&*self.pool).await?;
            }
        }
        let author = author_of(req.meta.as_ref());
        let event = append_event(
            &self.pool,
            &project_id,
            &req.node_id,
            "VERDICT_SET",
            &author,
            &format!("verdict set on {}", req.node_id),
            "{}",
        )
        .await?;
        let m = self
            .finish_mutation(&project_id, vec![event], vec![req.node_id.clone()])
            .await?;
        self.record_idempotency(&project_id, idem_key(req.meta.as_ref()), m.seq)
            .await?;
        Ok(m)
    }

    async fn add_comment(
        &self,
        req: pb::AddCommentRequest,
    ) -> Result<pb::Mutation, Box<dyn std::error::Error + Send + Sync>> {
        let current = self
            .fetch_node(&req.node_id)
            .await?
            .ok_or("node not found")?;
        let project_id = current.project_id.clone();
        if let Some(seq) = self
            .check_idempotency(&project_id, idem_key(req.meta.as_ref()))
            .await?
        {
            return Ok(Self::empty_mutation(seq));
        }
        let author = author_of(req.meta.as_ref());
        let trimmed: String = req.text.chars().take(96).collect();
        let summary = if req.text.chars().count() > 96 {
            format!("{}…", trimmed)
        } else {
            req.text.clone()
        };
        let payload = serde_json::json!({ "text": &req.text, "author": &author }).to_string();
        let event = append_event(
            &self.pool,
            &project_id,
            &req.node_id,
            "COMMENT",
            &author,
            &summary,
            &payload,
        )
        .await?;
        let m = self
            .finish_mutation(&project_id, vec![event], vec![req.node_id.clone()])
            .await?;
        self.record_idempotency(&project_id, idem_key(req.meta.as_ref()), m.seq)
            .await?;
        Ok(m)
    }

    async fn undo(
        &self,
        req: pb::UndoRequest,
    ) -> Result<pb::Mutation, Box<dyn std::error::Error + Send + Sync>> {
        const EV: &str = "SELECT seq, project_id, COALESCE(node_id,'') AS node_id, kind, author, summary, payload AS payload_json, created_at FROM events WHERE project_id = ? ";
        let row = if req.seq > 0 {
            let sql = format!("{EV} AND seq = ?");
            sqlx::query(&sql)
                .bind(&req.project_id)
                .bind(req.seq)
                .fetch_optional(&*self.pool)
                .await?
        } else {
            let sql = format!("{EV} ORDER BY seq DESC LIMIT 1");
            sqlx::query(&sql)
                .bind(&req.project_id)
                .fetch_optional(&*self.pool)
                .await?
        };
        let row = row.ok_or("no event to undo")?;
        let event = proto_event(row);
        let project_id = event.project_id.clone();
        let node_id = event.node_id.clone();
        let kind = event.kind;
        if let Some(seq) = self
            .check_idempotency(&project_id, idem_key(req.meta.as_ref()))
            .await?
        {
            return Ok(Self::empty_mutation(seq));
        }

        // Reversing an event. `inverse_kind` is the valid schema kind that
        // describes the reversal we just performed (the events table CHECK
        // rejects a synthetic "UNDO" kind, so we record the reversed act).
        let mut inverse_node = node_id.clone();
        let (affected, inverse_kind, inverse_summary) = match kind {
            k if k == pb::EventKind::StatusSet as i32 => {
                let before =
                    payload_get(&event.payload_json, "before").unwrap_or_else(|| "OPEN".into());
                sqlx::query("UPDATE nodes SET declared_status = ? WHERE id = ?")
                    .bind(before)
                    .bind(&node_id)
                    .execute(&*self.pool)
                    .await?;
                (
                    self.downstream_of(&node_id).await?,
                    "STATUS_SET",
                    format!("undid status set on {}", node_id),
                )
            }
            k if k == pb::EventKind::DepAdded as i32 => {
                let blocker = payload_get(&event.payload_json, "blocker_id")
                    .ok_or("dep payload missing blocker")?;
                let blocked = payload_get(&event.payload_json, "blocked_id")
                    .ok_or("dep payload missing blocked")?;
                sqlx::query("DELETE FROM dependencies WHERE blocker_id = ? AND blocked_id = ?")
                    .bind(&blocker)
                    .bind(&blocked)
                    .execute(&*self.pool)
                    .await?;
                (
                    vec![blocked.clone()],
                    "DEP_REMOVED",
                    format!("undid: {} no longer blocks {}", blocker, blocked),
                )
            }
            k if k == pb::EventKind::DepRemoved as i32 => {
                let blocker = payload_get(&event.payload_json, "blocker_id")
                    .ok_or("dep payload missing blocker")?;
                let blocked = payload_get(&event.payload_json, "blocked_id")
                    .ok_or("dep payload missing blocked")?;
                sqlx::query(
                    "INSERT OR IGNORE INTO dependencies (blocker_id, blocked_id) VALUES (?, ?)",
                )
                .bind(&blocker)
                .bind(&blocked)
                .execute(&*self.pool)
                .await?;
                (
                    vec![blocked.clone()],
                    "DEP_ADDED",
                    format!("undid: {} blocks {}", blocker, blocked),
                )
            }
            k if k == pb::EventKind::NodeCreated as i32 => {
                // The node no longer exists, so the inverse event must not
                // reference it (the events.node_id FK would reject the row).
                inverse_node.clear();
                sqlx::query("DELETE FROM nodes WHERE id = ?")
                    .bind(&node_id)
                    .execute(&*self.pool)
                    .await?;
                (
                    vec![],
                    "NODE_DELETED",
                    format!("undid creation of {}", node_id),
                )
            }
            k if k == pb::EventKind::NodeDeleted as i32 => {
                let before: serde_json::Value = serde_json::from_str(&event.payload_json)
                    .map_err(|_| "cannot undo delete: payload missing")?;
                let obj = before
                    .get("before")
                    .cloned()
                    .ok_or("cannot undo delete: payload missing")?;
                restore_node(&self.pool, &obj).await?;
                (
                    self.downstream_of(&node_id).await?,
                    "NODE_CREATED",
                    format!("undid deletion of {}", node_id),
                )
            }
            other => return Err(format!("cannot undo event kind {}", other).into()),
        };

        let author = author_of(req.meta.as_ref());
        let inverse = append_event(
            &self.pool,
            &project_id,
            &inverse_node,
            inverse_kind,
            &author,
            &inverse_summary,
            "{}",
        )
        .await?;
        let m = self
            .finish_mutation(&project_id, vec![inverse], affected)
            .await?;
        self.record_idempotency(&project_id, idem_key(req.meta.as_ref()), m.seq)
            .await?;
        Ok(m)
    }

    async fn move_node(
        &self,
        req: pb::MoveNodeRequest,
    ) -> Result<pb::Mutation, Box<dyn std::error::Error + Send + Sync>> {
        let current = self
            .fetch_node(&req.node_id)
            .await?
            .ok_or("node not found")?;
        let project_id = current.project_id.clone();
        if let Some(seq) = self
            .check_idempotency(&project_id, idem_key(req.meta.as_ref()))
            .await?
        {
            return Ok(Self::empty_mutation(seq));
        }
        let old_kind = current.kind;
        let new_kind = req.kind;
        let wp = pb::NodeKind::WorkPackage as i32;
        // Structural validation: scope to STEP<->TASK + reparent; the trigger
        // backstops parent-kind / cross-project / self-parent / children-validity.
        if new_kind == pb::NodeKind::Unspecified as i32 {
            return Err("move requires a target kind".into());
        }
        if old_kind == wp || new_kind == wp {
            return Err("move cannot promote or demote a work package".into());
        }
        if req.parent_id == req.node_id {
            return Err("a node cannot be its own parent".into());
        }
        let author = author_of(req.meta.as_ref());

        let mut tx = self.pool.begin().await?;
        let mut events: Vec<pb::Event> = Vec::new();
        let mut affected = vec![req.node_id.clone()];

        // TASK -> STEP demote drops this node's step children (destructive, no
        // undo — consented at the client). Collect each child's dependents BEFORE
        // deleting, and write the NODE_DELETED events with node_id = '' (the child
        // is gone; the events.node_id FK would reject its id).
        if old_kind == pb::NodeKind::Task as i32 && new_kind == pb::NodeKind::Step as i32 {
            let child_ids: Vec<String> =
                sqlx::query_scalar("SELECT id FROM nodes WHERE parent_id = ?")
                    .bind(&req.node_id)
                    .fetch_all(&mut *tx)
                    .await?;
            for cid in &child_ids {
                let deps: Vec<String> =
                    sqlx::query_scalar("SELECT blocked_id FROM dependencies WHERE blocker_id = ?")
                        .bind(cid)
                        .fetch_all(&mut *tx)
                        .await?;
                affected.extend(deps);
                sqlx::query("DELETE FROM nodes WHERE id = ?")
                    .bind(cid)
                    .execute(&mut *tx)
                    .await?;
                let payload = serde_json::json!({ "before": { "id": cid } }).to_string();
                let row = sqlx::query(
                    "INSERT INTO events (project_id, node_id, kind, author, summary, payload)
                     VALUES (?, NULL, 'NODE_DELETED', ?, ?, ?)
                     RETURNING seq, project_id, COALESCE(node_id,'') AS node_id, kind, author, summary, payload AS payload_json, created_at",
                )
                .bind(&project_id)
                .bind(&author)
                .bind(format!("deleted {}", cid))
                .bind(&payload)
                .fetch_one(&mut *tx)
                .await?;
                events.push(proto_event(row));
            }
        }

        // A kind change invalidates the verification: a STEP must not carry a
        // TASK's agent badge (and vice versa).
        if old_kind != new_kind {
            sqlx::query("DELETE FROM verifications WHERE node_id = ?")
                .bind(&req.node_id)
                .execute(&mut *tx)
                .await?;
        }

        // Append into the new parent's sibling list.
        let new_pos: i64 = sqlx::query_scalar(
            "SELECT COALESCE(MAX(position), 0) + 100 FROM nodes WHERE parent_id = ?",
        )
        .bind(&req.parent_id)
        .fetch_one(&mut *tx)
        .await?;

        sqlx::query(
            "UPDATE nodes SET parent_id = NULLIF(?, ''), kind = ?, position = ? WHERE id = ?",
        )
        .bind(&req.parent_id)
        .bind(kind_str(new_kind))
        .bind(new_pos as i32)
        .bind(&req.node_id)
        .execute(&mut *tx)
        .await?;

        let payload = serde_json::json!({
            "before": { "parent_id": &current.parent_id, "kind": kind_str(old_kind) },
            "after": { "parent_id": &req.parent_id, "kind": kind_str(new_kind) }
        })
        .to_string();
        let row = sqlx::query(
            "INSERT INTO events (project_id, node_id, kind, author, summary, payload)
             VALUES (?, ?, 'NODE_UPDATED', ?, ?, ?)
             RETURNING seq, project_id, COALESCE(node_id,'') AS node_id, kind, author, summary, payload AS payload_json, created_at",
        )
        .bind(&project_id)
        .bind(&req.node_id)
        .bind(&author)
        .bind(format!("moved {}", req.node_id))
        .bind(&payload)
        .fetch_one(&mut *tx)
        .await?;
        events.push(proto_event(row));

        tx.commit().await?;

        let m = self.finish_mutation(&project_id, events, affected).await?;
        self.record_idempotency(&project_id, idem_key(req.meta.as_ref()), m.seq)
            .await?;
        Ok(m)
    }

    async fn create_project(
        &self,
        req: pb::CreateProjectRequest,
    ) -> Result<pb::Project, Box<dyn std::error::Error + Send + Sync>> {
        // Sentinel-scoped idempotency ('' scope): projects log no event and the id
        // is minted here, so a retry dedups under '' and returns the created row.
        if let Some(hit) = self
            .check_idempotency_created("", idem_key(req.meta.as_ref()))
            .await?
        {
            if let Some(pid) = &hit.entity_id {
                if let Some(p) = self.fetch_project(pid).await? {
                    return Ok(p);
                }
            }
        }
        let id = new_project_id();
        sqlx::query("INSERT INTO projects (id, name, description) VALUES (?, ?, ?)")
            .bind(&id)
            .bind(&req.name)
            .bind(&req.description)
            .execute(&*self.pool)
            .await?;
        self.record_idempotency_created("", idem_key(req.meta.as_ref()), 0, &id)
            .await?;
        match self.fetch_project(&id).await? {
            Some(p) => Ok(p),
            None => Err("created project not found".into()),
        }
    }

    async fn update_project(
        &self,
        req: pb::UpdateProjectRequest,
    ) -> Result<pb::Project, Box<dyn std::error::Error + Send + Sync>> {
        if self.fetch_project(&req.project_id).await?.is_none() {
            return Err("project not found".into());
        }
        if req.update_mask.is_empty() {
            return Err("update_mask cannot be empty".into());
        }
        for f in &req.update_mask {
            match f.as_str() {
                "name" => {
                    sqlx::query("UPDATE projects SET name = ? WHERE id = ?")
                        .bind(&req.name)
                        .bind(&req.project_id)
                        .execute(&*self.pool)
                        .await?;
                }
                "description" => {
                    sqlx::query("UPDATE projects SET description = ? WHERE id = ?")
                        .bind(&req.description)
                        .bind(&req.project_id)
                        .execute(&*self.pool)
                        .await?;
                }
                other => return Err(format!("unknown update_mask field: {}", other).into()),
            }
        }
        sqlx::query("UPDATE projects SET updated_at = unixepoch() WHERE id = ?")
            .bind(&req.project_id)
            .execute(&*self.pool)
            .await?;
        match self.fetch_project(&req.project_id).await? {
            Some(p) => Ok(p),
            None => Err("project not found".into()),
        }
    }

    async fn archive_project(
        &self,
        req: pb::ArchiveProjectRequest,
    ) -> Result<pb::Project, Box<dyn std::error::Error + Send + Sync>> {
        if self.fetch_project(&req.project_id).await?.is_none() {
            return Err("project not found".into());
        }
        if req.archived {
            sqlx::query("UPDATE projects SET archived_at = unixepoch(), updated_at = unixepoch() WHERE id = ?")
                .bind(&req.project_id)
                .execute(&*self.pool)
                .await?;
        } else {
            sqlx::query(
                "UPDATE projects SET archived_at = NULL, updated_at = unixepoch() WHERE id = ?",
            )
            .bind(&req.project_id)
            .execute(&*self.pool)
            .await?;
        }
        match self.fetch_project(&req.project_id).await? {
            Some(p) => Ok(p),
            None => Err("project not found".into()),
        }
    }
}

// ── shared write helpers ─────────────────────────────────────────────────────

/// Build the shared mutation payload for a committed write, publish it to Watch
/// subscribers, and return it as the unary response (one apply-path for both).
impl SqliteStore {
    async fn finish_mutation(
        &self,
        project_id: &str,
        events: Vec<pb::Event>,
        affected_ids: Vec<String>,
    ) -> Result<pb::Mutation, Box<dyn std::error::Error + Send + Sync>> {
        let seq = events.last().map(|e| e.seq).unwrap_or(0);

        let mut seen: HashSet<String> = HashSet::new();
        let mut changed_nodes = Vec::new();
        for id in affected_ids {
            if seen.insert(id.clone()) {
                if let Some(n) = self.fetch_node(&id).await? {
                    changed_nodes.push(n);
                }
            }
        }

        let changed_progress = self.progress_for(project_id).await?;

        let notified = Notified {
            project_id: project_id.to_string(),
            seq,
            events: events.clone(),
            changed_nodes: changed_nodes.clone(),
            changed_progress: changed_progress.clone(),
        };
        let _ = self.notifier.send(notified);

        Ok(pb::Mutation {
            events,
            changed_nodes,
            changed_progress,
            seq,
        })
    }

    async fn check_idempotency(
        &self,
        project_id: &str,
        key: Option<&str>,
    ) -> Result<Option<i64>, Box<dyn std::error::Error + Send + Sync>> {
        let Some(key) = key else { return Ok(None) };
        Ok(sqlx::query_scalar(
            "SELECT seq FROM idempotency WHERE project_id = ? AND idempotency_key = ?",
        )
        .bind(project_id)
        .bind(key)
        .fetch_optional(&*self.pool)
        .await?)
    }

    async fn record_idempotency(
        &self,
        project_id: &str,
        key: Option<&str>,
        seq: i64,
    ) -> Result<(), Box<dyn std::error::Error + Send + Sync>> {
        let Some(key) = key else { return Ok(()) };
        sqlx::query(
            "INSERT OR IGNORE INTO idempotency (project_id, idempotency_key, seq) VALUES (?, ?, ?)",
        )
        .bind(project_id)
        .bind(key)
        .bind(seq)
        .execute(&*self.pool)
        .await?;
        Ok(())
    }

    async fn downstream_of(
        &self,
        node_id: &str,
    ) -> Result<Vec<String>, Box<dyn std::error::Error + Send + Sync>> {
        Ok(
            sqlx::query_scalar("SELECT blocked_id FROM dependencies WHERE blocker_id = ?")
                .bind(node_id)
                .fetch_all(&*self.pool)
                .await?,
        )
    }

    /// Progress per work package, counting leaves (steps where they exist, else
    /// the task itself) with the effective status of each leaf.
    async fn progress_for(
        &self,
        project_id: &str,
    ) -> Result<Vec<pb::Progress>, Box<dyn std::error::Error + Send + Sync>> {
        let rows = sqlx::query(
            "SELECT t.parent_id AS wp_id,
                    count(*) AS total,
                    sum(l.status = 'DONE')     AS done,
                    sum(l.status = 'DEFERRED') AS deferred,
                    sum(l.status = 'READY')    AS ready,
                    sum(l.status = 'BLOCKED')  AS blocked
             FROM nodes t
             LEFT JOIN nodes s ON s.parent_id = t.id AND s.kind = 'STEP'
             JOIN node_state l ON l.id = COALESCE(s.id, t.id)
             WHERE t.kind = 'TASK' AND t.project_id = ?
             GROUP BY t.parent_id
             ORDER BY t.parent_id",
        )
        .bind(project_id)
        .fetch_all(&*self.pool)
        .await?;
        let mut out = Vec::with_capacity(rows.len());
        for r in rows {
            out.push(pb::Progress {
                work_package_id: r.get::<String, _>("wp_id"),
                total: r.get::<i64, _>("total") as i32,
                done: r.get::<i64, _>("done") as i32,
                ready: r.get::<i64, _>("ready") as i32,
                blocked: r.get::<i64, _>("blocked") as i32,
                deferred: r.get::<i64, _>("deferred") as i32,
            });
        }
        Ok(out)
    }

    async fn verification_for(
        &self,
        node_id: &str,
    ) -> Result<Option<pb::Verification>, Box<dyn std::error::Error + Send + Sync>> {
        let row = sqlx::query(
            "SELECT v.agent_result, COALESCE(v.agent_name,'') AS agent_name,
                    COALESCE(v.agent_at,0) AS agent_at, v.agent_detail,
                    v.human_verdict, COALESCE(v.human_at,0) AS human_at,
                    (v.agent_node_rev IS NOT NULL AND n.updated_at > v.agent_node_rev) AS stale
             FROM verifications v JOIN nodes n ON n.id = v.node_id
             WHERE v.node_id = ?",
        )
        .bind(node_id)
        .fetch_optional(&*self.pool)
        .await?;
        Ok(row.map(row_to_verification))
    }

    /// Fetch one node by id as a proto Node (None if missing).
    async fn fetch_node(
        &self,
        id: &str,
    ) -> Result<Option<pb::Node>, Box<dyn std::error::Error + Send + Sync>> {
        let row = sqlx::query(
            "SELECT n.id, n.project_id, COALESCE(n.parent_id,'') AS parent_id,
                    n.kind, n.title, n.description, n.condition, n.note, COALESCE(n.reference,'') AS reference,
                    n.declared_status, n.wp_state, n.position, n.created_at, n.updated_at,
                    n.status AS effective_status
             FROM node_state n WHERE n.id = ?",
        )
        .bind(id)
        .fetch_optional(&*self.pool)
        .await?;
        let mut node = row.map(row_to_node);
        if let Some(n) = &mut node {
            n.verification = self.verification_for(&n.id).await?;
        }
        Ok(node)
    }

    /// Owning project of a node.
    async fn project_of(
        &self,
        node_id: &str,
    ) -> Result<String, Box<dyn std::error::Error + Send + Sync>> {
        Ok(
            sqlx::query_scalar("SELECT project_id FROM nodes WHERE id = ?")
                .bind(node_id)
                .fetch_optional(&*self.pool)
                .await?
                .unwrap_or_default(),
        )
    }

    /// Fetch one project as a proto Project (None if missing).
    async fn fetch_project(
        &self,
        id: &str,
    ) -> Result<Option<pb::Project>, Box<dyn std::error::Error + Send + Sync>> {
        let row = sqlx::query(
            "SELECT id, name, description, COALESCE(archived_at,0) AS archived_at, created_at
             FROM projects WHERE id = ?",
        )
        .bind(id)
        .fetch_optional(&*self.pool)
        .await?;
        Ok(row.map(|r| pb::Project {
            id: r.get("id"),
            name: r.get("name"),
            description: r.get("description"),
            archived_at: r.get("archived_at"),
            created_at: r.get("created_at"),
        }))
    }

    /// Idempotency lookup that also returns the id of the entity the original
    /// request created (for create replays). Used by create_node / create_project.
    async fn check_idempotency_created(
        &self,
        scope: &str,
        key: Option<&str>,
    ) -> Result<Option<IdemHit>, Box<dyn std::error::Error + Send + Sync>> {
        let Some(key) = key else { return Ok(None) };
        let row = sqlx::query(
            "SELECT seq, entity_id FROM idempotency WHERE project_id = ? AND idempotency_key = ?",
        )
        .bind(scope)
        .bind(key)
        .fetch_optional(&*self.pool)
        .await?;
        Ok(row.map(|r| IdemHit {
            seq: r.get("seq"),
            entity_id: r.get::<Option<String>, _>("entity_id"),
        }))
    }

    /// Record an idempotency row that remembers the created entity's id.
    async fn record_idempotency_created(
        &self,
        scope: &str,
        key: Option<&str>,
        seq: i64,
        entity_id: &str,
    ) -> Result<(), Box<dyn std::error::Error + Send + Sync>> {
        let Some(key) = key else { return Ok(()) };
        sqlx::query(
            "INSERT OR IGNORE INTO idempotency (project_id, idempotency_key, seq, entity_id) VALUES (?, ?, ?, ?)",
        )
        .bind(scope)
        .bind(key)
        .bind(seq)
        .bind(entity_id)
        .execute(&*self.pool)
        .await?;
        Ok(())
    }
}

// ── free helpers ─────────────────────────────────────────────────────────────

/// An idempotency-ledger hit: the recorded cursor plus the entity the original
/// create minted (None for non-create mutations).
struct IdemHit {
    seq: i64,
    entity_id: Option<String>,
}

fn author_of(meta: Option<&pb::WriteMeta>) -> String {
    meta.map(|m| m.author.clone()).unwrap_or_default()
}

/// The idempotency key from a request's meta, or None when empty.
fn idem_key(meta: Option<&pb::WriteMeta>) -> Option<&str> {
    let key = meta.map(|m| m.idempotency_key.as_str())?;
    if key.is_empty() {
        None
    } else {
        Some(key)
    }
}

/// Read a string field out of a JSON payload.
fn payload_get(payload: &str, key: &str) -> Option<String> {
    serde_json::from_str::<serde_json::Value>(payload)
        .ok()?
        .get(key)?
        .as_str()
        .map(String::from)
}

/// Serialize enough of a node to re-insert it after a delete (undo).
fn node_before_json(n: &pb::Node) -> String {
    // Only WORK_PACKAGE rows carry a wp_state; the schema CHECK requires
    // TASK/STEP rows to have NULL there, so leave it out for them.
    let wp = if n.kind == pb::NodeKind::WorkPackage as i32 {
        wp_str(n.wp_state)
    } else {
        None
    };
    serde_json::json!({
        "before": {
            "id": &n.id,
            "project_id": &n.project_id,
            "parent_id": &n.parent_id,
            "kind": kind_str(n.kind),
            "title": &n.title,
            "description": &n.description,
            "condition": &n.condition,
            "note": &n.note,
            "reference": &n.reference,
            "declared_status": declared_str(n.declared_status),
            "wp_state": wp,
            "position": n.position,
        }
    })
    .to_string()
}

/// Re-insert a node from the `before` payload written by a delete.
async fn restore_node(
    pool: &Arc<SqlitePool>,
    obj: &serde_json::Value,
) -> Result<(), Box<dyn std::error::Error + Send + Sync>> {
    let wp = if str_val(obj, "kind") == "WORK_PACKAGE" {
        wp_state_str(obj.get("wp_state").and_then(|v| v.as_str()))
    } else {
        None
    };
    sqlx::query(
        "INSERT OR IGNORE INTO nodes (id, project_id, parent_id, kind, title, description, condition, note, reference, declared_status, wp_state, position)
         VALUES (?, ?, NULLIF(?, ''), ?, ?, ?, ?, ?, NULLIF(?, ''), ?, ?, ?)",
    )
    .bind(str_val(obj, "id"))
    .bind(str_val(obj, "project_id"))
    .bind(str_val(obj, "parent_id"))
    .bind(str_val(obj, "kind"))
    .bind(str_val(obj, "title"))
    .bind(str_val(obj, "description"))
    .bind(str_val(obj, "condition"))
    .bind(str_val(obj, "note"))
    .bind(str_val(obj, "reference"))
    .bind(declared_str_from(str_val(obj, "declared_status")))
    .bind(wp)
    .bind(obj.get("position").and_then(|v| v.as_i64()).unwrap_or(0) as i32)
    .execute(&**pool)
    .await?;
    Ok(())
}

fn str_val<'a>(obj: &'a serde_json::Value, key: &str) -> &'a str {
    obj.get(key).and_then(|v| v.as_str()).unwrap_or("")
}

/// Convert an "UNDO"-adjacent stored kind string back to a stored kind. Kept
/// explicit so undo events survive the schema CHECK.
fn wp_state_str(s: Option<&str>) -> Option<&'static str> {
    match s {
        Some("PLANNED") => Some("PLANNED"),
        Some("ACTIVE") => Some("ACTIVE"),
        Some("DONE") => Some("DONE"),
        Some("ARCHIVED") => Some("ARCHIVED"),
        _ => None,
    }
}

fn declared_str_from(s: &str) -> &'static str {
    match s {
        "DEFERRED" => "DEFERRED",
        "DONE" => "DONE",
        _ => "OPEN",
    }
}

fn kind_str(k: i32) -> &'static str {
    match k {
        1 => "WORK_PACKAGE",
        2 => "TASK",
        _ => "STEP",
    }
}
fn declared_str(d: i32) -> &'static str {
    match d {
        1 => "OPEN",
        2 => "DEFERRED",
        _ => "DONE",
    }
}
fn wp_str(w: i32) -> Option<&'static str> {
    match w {
        1 => Some("PLANNED"),
        2 => Some("ACTIVE"),
        3 => Some("DONE"),
        4 => Some("ARCHIVED"),
        _ => None,
    }
}

use std::time::{SystemTime, UNIX_EPOCH};

/// Short, sortable, unique node id: unix-millis + random hex suffix.
fn new_node_id() -> String {
    let ms = SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .unwrap_or_default()
        .as_millis();
    let rand = {
        use std::collections::hash_map::RandomState;
        use std::hash::{BuildHasher, Hasher};
        RandomState::new().build_hasher().finish()
    };
    format!("node-{}-{:08x}", ms, rand)
}

/// Short, unique project id, same scheme as node ids.
fn new_project_id() -> String {
    let ms = SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .unwrap_or_default()
        .as_millis();
    let rand = {
        use std::collections::hash_map::RandomState;
        use std::hash::{BuildHasher, Hasher};
        RandomState::new().build_hasher().finish()
    };
    format!("prj-{}-{:08x}", ms, rand)
}

/// Convert a node_state row into a proto Node.
fn row_to_node(r: sqlx::sqlite::SqliteRow) -> pb::Node {
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
    pb::Node {
        id: r.get("id"),
        project_id: r.get("project_id"),
        parent_id: r.get("parent_id"),
        kind: kind as i32,
        title: r.get("title"),
        description: r.get("description"),
        condition: r.get("condition"),
        note: r.get("note"),
        reference: r.get("reference"),
        declared_status: declared as i32,
        status: effective_status(r.get::<String, _>("effective_status").as_str()),
        wp_state: wp_state as i32,
        position: r.get("position"),
        verification: None,
        created_at: r.get("created_at"),
        updated_at: r.get("updated_at"),
    }
}

/// Convert a verification row into a proto Verification.
fn row_to_verification(r: sqlx::sqlite::SqliteRow) -> pb::Verification {
    let agent_result = match r.get::<Option<String>, _>("agent_result").as_deref() {
        Some("PASS") => pb::AgentResult::Pass,
        Some("FAIL") => pb::AgentResult::Fail,
        _ => pb::AgentResult::Unspecified,
    };
    let human_verdict = match r.get::<Option<String>, _>("human_verdict").as_deref() {
        Some("ACCEPTED") => pb::HumanVerdict::Accepted,
        Some("REJECTED") => pb::HumanVerdict::Rejected,
        _ => pb::HumanVerdict::Unspecified,
    };
    pb::Verification {
        agent_result: agent_result as i32,
        agent_name: r.get("agent_name"),
        agent_at: r.get("agent_at"),
        agent_detail: r.get("agent_detail"),
        human_verdict: human_verdict as i32,
        human_at: r.get("human_at"),
        stale: r.get::<i64, _>("stale") != 0,
    }
}

/// Insert an event row; returns the fully-materialised `pb::Event`. `payload`
/// must be valid JSON (the schema enforces it with a CHECK).
async fn append_event(
    pool: &Arc<SqlitePool>,
    project_id: &str,
    node_id: &str,
    kind: &str,
    author: &str,
    summary: &str,
    payload: &str,
) -> Result<pb::Event, Box<dyn std::error::Error + Send + Sync>> {
    let row = sqlx::query(
        "INSERT INTO events (project_id, node_id, kind, author, summary, payload)
         VALUES (?, ?, ?, ?, ?, ?)
         RETURNING seq, project_id, COALESCE(node_id,'') AS node_id, kind, author, summary, payload AS payload_json, created_at",
    )
    .bind(project_id)
    .bind(if node_id.is_empty() { None } else { Some(node_id) })
    .bind(kind)
    .bind(author)
    .bind(summary)
    .bind(payload)
    .fetch_one(&**pool)
    .await?;
    Ok(proto_event(row))
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
