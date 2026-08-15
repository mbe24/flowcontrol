//! SQLite implementation of `Store`, using rusqlite.
//!
//! The store is **synchronous** (rusqlite/SQLite is a synchronous in-process
//! library) behind a single `Mutex<Connection>` — matching the actual load (a
//! local single-user daemon) and giving deterministic, single-writer ordering
//! that the future Node/browser hosts share. The `Store` trait stays `async` so
//! the tonic edge is unchanged; each method's body holds no `.await`, so the
//! `MutexGuard` never crosses a suspension point.
//!
//! Every mutation runs inside one transaction (`unchecked_transaction`, which
//! rolls back on `Drop` unless committed) — an atomicity upgrade over the old
//! per-statement autocommit path. Errors carry a [`DomainError`] whose `Code` is
//! classified from the message, preserving the exact taxonomy the gRPC edge used.

use std::collections::HashSet;
use std::sync::{Arc, Mutex};

use rusqlite::{params, Connection, OptionalExtension, Row};

use super::Store;
use crate::error::DomainError;
use crate::generated::flow_v1 as pb;
use crate::store::watch::{self, Notified, Notifier};
use tokio::sync::broadcast;

type DResult<T> = Result<T, DomainError>;

impl From<rusqlite::Error> for DomainError {
    fn from(e: rusqlite::Error) -> Self {
        // The message carries SQLite's own text — including trigger `RAISE(ABORT)`
        // strings ("would create a cycle", "invalid parent kind") and FTS `MATCH`
        // syntax errors — which `classify` maps to the right code.
        DomainError::from_db_message(e.to_string())
    }
}

impl From<serde_json::Error> for DomainError {
    fn from(e: serde_json::Error) -> Self {
        DomainError::internal(format!("json: {e}"))
    }
}

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

/// SQLite-backed store. Cheap to clone: wraps an `Arc<Mutex<Connection>>`.
#[derive(Clone)]
pub struct SqliteStore {
    conn: Arc<Mutex<Connection>>,
    notifier: Notifier,
}

impl SqliteStore {
    /// Wrap an already-migrated connection.
    pub fn new(conn: Connection) -> Self {
        let (notifier, _) = watch::channel(256);
        Self {
            conn: Arc::new(Mutex::new(conn)),
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

/// Synchronous store logic. Each `*_locked` method takes the connection lock and
/// runs one operation to completion (no `.await`). The async `Store` impl below
/// wraps each in `spawn_blocking` so SQLite never blocks the tokio reactor. These
/// methods are the seed of the future `flow-core` (they'll take a `Sql` seam
/// instead of `self.conn` in Phase 2's core/edge split).
impl SqliteStore {
    fn list_projects_locked(&self, include_archived: bool) -> DResult<Vec<pb::Project>> {
        let conn = self.conn.lock().unwrap();
        let mut stmt = conn.prepare(
            "SELECT id, name, description, COALESCE(archived_at,0) AS archived_at, created_at
             FROM projects
             WHERE ?1 = 1 OR archived_at IS NULL
             ORDER BY created_at",
        )?;
        let rows = stmt.query_map(params![include_archived], row_to_project)?;
        Ok(rows.collect::<rusqlite::Result<Vec<_>>>()?)
    }

    fn get_snapshot_locked(&self, project_id: &str) -> DResult<pb::GetSnapshotResponse> {
        let conn = self.conn.lock().unwrap();

        // Project (may be absent).
        let project = conn
            .query_row(
                "SELECT id, name, description, COALESCE(archived_at,0) AS archived_at, created_at
                 FROM projects WHERE id = ?1",
                params![project_id],
                row_to_project,
            )
            .optional()?;

        // Nodes with effective status from the node_state view.
        let mut nodes = {
            let mut stmt = conn.prepare(
                "SELECT n.id, n.project_id, COALESCE(n.parent_id,'') AS parent_id,
                        n.kind, n.title, n.description, n.condition, n.note, COALESCE(n.reference,'') AS reference,
                        n.declared_status, n.wp_state, n.position, n.created_at, n.updated_at,
                        n.status AS effective_status
                 FROM node_state n
                 WHERE n.project_id = ?1",
            )?;
            let rows = stmt.query_map(params![project_id], row_to_node)?;
            rows.collect::<rusqlite::Result<Vec<_>>>()?
        };
        for node in &mut nodes {
            node.verification = verification_for(&conn, &node.id)?;
        }

        // Dependencies for the project.
        let dependencies = {
            let mut stmt = conn.prepare(
                "SELECT d.blocker_id, d.blocked_id
                 FROM dependencies d
                 JOIN nodes a ON a.id = d.blocker_id
                 JOIN nodes b ON b.id = d.blocked_id
                 WHERE a.project_id = ?1 AND b.project_id = ?1",
            )?;
            let rows = stmt.query_map(params![project_id], |r| {
                Ok(pb::Dependency {
                    blocker_id: r.get("blocker_id")?,
                    blocked_id: r.get("blocked_id")?,
                })
            })?;
            rows.collect::<rusqlite::Result<Vec<_>>>()?
        };

        // Progress per work package from the view.
        let progress = progress_for(&conn, project_id)?;

        // Recent events (limit 25) + current seq.
        let recent_events = {
            let mut stmt = conn.prepare(
                "SELECT seq, project_id, COALESCE(node_id,'') AS node_id, kind, author, summary, payload AS payload_json, created_at
                 FROM events WHERE project_id = ?1
                 ORDER BY seq DESC LIMIT 25",
            )?;
            let rows = stmt.query_map(params![project_id], proto_event)?;
            rows.collect::<rusqlite::Result<Vec<_>>>()?
        };
        let seq: i64 =
            conn.query_row("SELECT COALESCE(MAX(seq),0) FROM events", [], |r| r.get(0))?;

        Ok(pb::GetSnapshotResponse {
            project,
            nodes,
            dependencies,
            progress,
            recent_events,
            seq,
        })
    }

    fn events_after_locked(&self, project_id: &str, from_seq: i64) -> DResult<Vec<pb::Event>> {
        let conn = self.conn.lock().unwrap();
        let mut stmt = conn.prepare(
            "SELECT seq, project_id, COALESCE(node_id,'') AS node_id, kind, author, summary, payload AS payload_json, created_at
             FROM events WHERE project_id = ?1 AND seq > ?2
             ORDER BY seq ASC",
        )?;
        let rows = stmt.query_map(params![project_id, from_seq], proto_event)?;
        Ok(rows.collect::<rusqlite::Result<Vec<_>>>()?)
    }

    fn poll_changes_locked(
        &self,
        project_id: &str,
        after_seq: i64,
        limit: i32,
    ) -> DResult<pb::PollChangesResponse> {
        let lim = if limit <= 0 { 1000 } else { limit };
        let conn = self.conn.lock().unwrap();
        let mut stmt = conn.prepare(
            "SELECT seq, project_id, COALESCE(node_id,'') AS node_id, kind, author, summary, payload AS payload_json, created_at
             FROM events WHERE project_id = ?1 AND seq > ?2
             ORDER BY seq ASC LIMIT ?3",
        )?;
        let events = stmt
            .query_map(params![project_id, after_seq, lim], proto_event)?
            .collect::<rusqlite::Result<Vec<_>>>()?;
        // Next cursor: the last event returned, or the caller's cursor if nothing
        // new (never skip events on the next poll).
        let seq = events.last().map(|e| e.seq).unwrap_or(after_seq);
        Ok(pb::PollChangesResponse { events, seq })
    }

    fn list_events_locked(
        &self,
        project_id: &str,
        node_id: &str,
        before_seq: i64,
        limit: i32,
    ) -> DResult<(Vec<pb::Event>, bool)> {
        let lim = limit.max(1);
        let conn = self.conn.lock().unwrap();
        // Fetch one extra to tell whether an older page exists. An empty node_id
        // means "no node filter"; before_seq 0 means "from the newest".
        let mut stmt = conn.prepare(
            "SELECT seq, project_id, COALESCE(node_id,'') AS node_id, kind, author, summary, payload AS payload_json, created_at
             FROM events
             WHERE project_id = ?1
               AND (?2 = '' OR node_id = ?2)
               AND (?3 = 0 OR seq < ?3)
             ORDER BY seq DESC LIMIT ?4",
        )?;
        let mut rows = stmt
            .query_map(
                params![project_id, node_id, before_seq, lim + 1],
                proto_event,
            )?
            .collect::<rusqlite::Result<Vec<_>>>()?;
        let has_more = rows.len() as i32 > lim;
        rows.truncate(lim as usize);
        Ok((rows, has_more))
    }

    fn search_locked(&self, project_id: &str, query: &str, limit: i32) -> DResult<Vec<pb::Node>> {
        let conn = self.conn.lock().unwrap();
        let mut nodes = {
            let mut stmt = conn.prepare(
                "SELECT n.id, n.project_id, COALESCE(n.parent_id,'') AS parent_id,
                        n.kind, n.title, n.description, n.condition, n.note, COALESCE(n.reference,'') AS reference,
                        n.declared_status, n.wp_state, n.position, n.created_at, n.updated_at,
                        ns.status AS effective_status
                 FROM nodes n
                 JOIN node_state ns ON ns.id = n.id
                 JOIN nodes_fts f ON f.rowid = n.rowid
                 WHERE n.project_id = ?1 AND nodes_fts MATCH ?2
                 ORDER BY rank LIMIT ?3",
            )?;
            let rows = stmt.query_map(params![project_id, query, limit.max(1)], row_to_node)?;
            rows.collect::<rusqlite::Result<Vec<_>>>()?
        };
        for node in &mut nodes {
            node.verification = verification_for(&conn, &node.id)?;
        }
        Ok(nodes)
    }

    fn create_node_locked(&self, req: pb::CreateNodeRequest) -> DResult<pb::Mutation> {
        let conn = self.conn.lock().unwrap();
        if let Some(hit) =
            check_idempotency_created(&conn, &req.project_id, idem_key(req.meta.as_ref()))?
        {
            // Replay: return the node the original request created, so a retry
            // (e.g. an agent after a timeout) still learns the id.
            if let Some(eid) = &hit.entity_id {
                if let Some(n) = fetch_node(&conn, eid)? {
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
        let author = author_of(req.meta.as_ref());
        let wp = if req.kind == pb::NodeKind::WorkPackage as i32 {
            Some("PLANNED")
        } else {
            None
        };
        let tx = conn.unchecked_transaction()?;
        let node_id = new_id(&tx, "node")?;
        tx.execute(
            "INSERT INTO nodes (id, project_id, parent_id, kind, title, description, condition, note, reference, declared_status, wp_state, position)
             VALUES (?1, ?2, NULLIF(?3, ''), ?4, ?5, ?6, ?7, ?8, NULLIF(?9, ''), 'OPEN', ?10, ?11)",
            params![
                node_id, req.project_id, req.parent_id, kind_str(req.kind), req.title,
                req.description, req.condition, req.note, req.reference, wp, req.position
            ],
        )?;
        let payload =
            serde_json::json!({ "after": { "id": &node_id, "title": &req.title } }).to_string();
        let event = append_event(
            &tx,
            &req.project_id,
            &node_id,
            "NODE_CREATED",
            &author,
            &format!("created {}", node_id),
            &payload,
        )?;
        record_idempotency_created(
            &tx,
            &req.project_id,
            idem_key(req.meta.as_ref()),
            event.seq,
            &node_id,
        )?;
        tx.commit()?;
        finish_mutation(
            &conn,
            &self.notifier,
            &req.project_id,
            vec![event],
            vec![node_id],
        )
    }

    fn update_node_locked(&self, req: pb::UpdateNodeRequest) -> DResult<pb::Mutation> {
        let conn = self.conn.lock().unwrap();
        let current = fetch_node(&conn, &req.node_id)?
            .ok_or_else(|| DomainError::from_db_message("node not found"))?;
        let project_id = current.project_id.clone();
        if let Some(seq) = check_idempotency(&conn, &project_id, idem_key(req.meta.as_ref()))? {
            return Ok(Self::empty_mutation(seq));
        }
        if req.update_mask.is_empty() {
            return Err(DomainError::from_db_message("update_mask cannot be empty"));
        }
        let tx = conn.unchecked_transaction()?;
        for f in &req.update_mask {
            match f.as_str() {
                "title" => {
                    tx.execute(
                        "UPDATE nodes SET title = ?1 WHERE id = ?2",
                        params![req.title, req.node_id],
                    )?;
                }
                "description" => {
                    tx.execute(
                        "UPDATE nodes SET description = ?1 WHERE id = ?2",
                        params![req.description, req.node_id],
                    )?;
                }
                "condition" => {
                    tx.execute(
                        "UPDATE nodes SET condition = ?1 WHERE id = ?2",
                        params![req.condition, req.node_id],
                    )?;
                }
                "position" => {
                    tx.execute(
                        "UPDATE nodes SET position = ?1 WHERE id = ?2",
                        params![req.position, req.node_id],
                    )?;
                }
                "reference" => {
                    tx.execute(
                        "UPDATE nodes SET reference = ?1 WHERE id = ?2",
                        params![req.reference, req.node_id],
                    )?;
                }
                "note" => {
                    tx.execute(
                        "UPDATE nodes SET note = ?1 WHERE id = ?2",
                        params![req.note, req.node_id],
                    )?;
                }
                "wp_state" => {
                    tx.execute(
                        "UPDATE nodes SET wp_state = ?1 WHERE id = ?2",
                        params![wp_str(req.wp_state), req.node_id],
                    )?;
                }
                other => {
                    return Err(DomainError::from_db_message(format!(
                        "unknown update_mask field: {}",
                        other
                    )))
                }
            }
        }
        let author = author_of(req.meta.as_ref());
        let event = append_event(
            &tx,
            &project_id,
            &req.node_id,
            "NODE_UPDATED",
            &author,
            &format!("updated {}", req.node_id),
            "{}",
        )?;
        record_idempotency(&tx, &project_id, idem_key(req.meta.as_ref()), event.seq)?;
        tx.commit()?;
        finish_mutation(
            &conn,
            &self.notifier,
            &project_id,
            vec![event],
            vec![req.node_id.clone()],
        )
    }

    fn delete_node_locked(&self, req: pb::DeleteNodeRequest) -> DResult<pb::Mutation> {
        let conn = self.conn.lock().unwrap();
        let current = fetch_node(&conn, &req.node_id)?
            .ok_or_else(|| DomainError::from_db_message("node not found"))?;
        let project_id = current.project_id.clone();
        if let Some(seq) = check_idempotency(&conn, &project_id, idem_key(req.meta.as_ref()))? {
            return Ok(Self::empty_mutation(seq));
        }
        if req.fail_if_referenced {
            let deps: i64 = conn.query_row(
                "SELECT count(*) FROM dependencies WHERE blocker_id = ?1 OR blocked_id = ?1",
                params![req.node_id],
                |r| r.get(0),
            )?;
            if deps > 0 {
                return Err(DomainError::from_db_message("node has dependents"));
            }
        }
        let before = node_before_json(&current);
        // Snapshot the whole subtree up front: the cascade delete removes children
        // silently, so we must collect their EXTERNAL dependents (whose effective
        // status may change) and emit a delete event per node before they vanish.
        let dependents = query_strings(
            &conn,
            "WITH RECURSIVE subtree(id) AS (
                 SELECT ?1 UNION ALL
                 SELECT n.id FROM nodes n JOIN subtree s ON n.parent_id = s.id
             )
             SELECT DISTINCT d.blocked_id FROM dependencies d
             JOIN subtree s ON d.blocker_id = s.id
             WHERE d.blocked_id NOT IN (SELECT id FROM subtree)",
            params![req.node_id],
        )?;
        let descendants = query_strings(
            &conn,
            "WITH RECURSIVE subtree(id) AS (
                 SELECT ?1 UNION ALL
                 SELECT n.id FROM nodes n JOIN subtree s ON n.parent_id = s.id
             )
             SELECT id FROM subtree WHERE id <> ?1",
            params![req.node_id],
        )?;

        let author = author_of(req.meta.as_ref());
        let tx = conn.unchecked_transaction()?;
        tx.execute("DELETE FROM nodes WHERE id = ?1", params![req.node_id])?;
        // Every delete event carries node_id = '' (the row is gone; events.node_id
        // has an FK). The root's `before` payload is what undo restores; children
        // carry a minimal payload (a subtree delete is not child-wise undoable).
        let mut events = Vec::with_capacity(descendants.len() + 1);
        for cid in &descendants {
            let payload = serde_json::json!({ "before": { "id": cid } }).to_string();
            events.push(append_event(
                &tx,
                &project_id,
                "",
                "NODE_DELETED",
                &author,
                &format!("deleted {}", cid),
                &payload,
            )?);
        }
        events.push(append_event(
            &tx,
            &project_id,
            "",
            "NODE_DELETED",
            &author,
            &format!("deleted {}", req.node_id),
            &before,
        )?);
        let last_seq = events.last().map(|e| e.seq).unwrap_or(0);
        record_idempotency(&tx, &project_id, idem_key(req.meta.as_ref()), last_seq)?;
        tx.commit()?;
        finish_mutation(&conn, &self.notifier, &project_id, events, dependents)
    }

    fn set_status_locked(&self, req: pb::SetStatusRequest) -> DResult<pb::Mutation> {
        let conn = self.conn.lock().unwrap();
        let current = fetch_node(&conn, &req.node_id)?
            .ok_or_else(|| DomainError::from_db_message("node not found"))?;
        let project_id = current.project_id.clone();
        if let Some(seq) = check_idempotency(&conn, &project_id, idem_key(req.meta.as_ref()))? {
            return Ok(Self::empty_mutation(seq));
        }
        let before = declared_str(current.declared_status);
        let new = declared_str(req.declared_status);
        let dependents = query_strings(
            &conn,
            "SELECT blocked_id FROM dependencies WHERE blocker_id = ?1",
            params![req.node_id],
        )?;
        let author = author_of(req.meta.as_ref());
        let tx = conn.unchecked_transaction()?;
        tx.execute(
            "UPDATE nodes SET declared_status = ?1 WHERE id = ?2",
            params![new, req.node_id],
        )?;
        let payload = serde_json::json!({ "before": before, "after": new }).to_string();
        let event = append_event(
            &tx,
            &project_id,
            &req.node_id,
            "STATUS_SET",
            &author,
            &format!("{} -> {}", req.node_id, new),
            &payload,
        )?;
        record_idempotency(&tx, &project_id, idem_key(req.meta.as_ref()), event.seq)?;
        tx.commit()?;
        let mut affected = vec![req.node_id.clone()];
        affected.extend(dependents);
        finish_mutation(&conn, &self.notifier, &project_id, vec![event], affected)
    }

    fn add_dependency_locked(&self, req: pb::AddDependencyRequest) -> DResult<pb::Mutation> {
        let conn = self.conn.lock().unwrap();
        let project_id = project_of(&conn, &req.blocker_id)?;
        if let Some(seq) = check_idempotency(&conn, &project_id, idem_key(req.meta.as_ref()))? {
            return Ok(Self::empty_mutation(seq));
        }
        let author = author_of(req.meta.as_ref());
        let tx = conn.unchecked_transaction()?;
        tx.execute(
            "INSERT OR IGNORE INTO dependencies (blocker_id, blocked_id) VALUES (?1, ?2)",
            params![req.blocker_id, req.blocked_id],
        )?;
        let payload =
            serde_json::json!({ "blocker_id": &req.blocker_id, "blocked_id": &req.blocked_id })
                .to_string();
        let event = append_event(
            &tx,
            &project_id,
            "",
            "DEP_ADDED",
            &author,
            &format!("{} blocks {}", req.blocker_id, req.blocked_id),
            &payload,
        )?;
        record_idempotency(&tx, &project_id, idem_key(req.meta.as_ref()), event.seq)?;
        tx.commit()?;
        finish_mutation(
            &conn,
            &self.notifier,
            &project_id,
            vec![event],
            vec![req.blocked_id.clone()],
        )
    }

    fn remove_dependency_locked(&self, req: pb::RemoveDependencyRequest) -> DResult<pb::Mutation> {
        let conn = self.conn.lock().unwrap();
        let project_id = project_of(&conn, &req.blocker_id)?;
        if let Some(seq) = check_idempotency(&conn, &project_id, idem_key(req.meta.as_ref()))? {
            return Ok(Self::empty_mutation(seq));
        }
        let author = author_of(req.meta.as_ref());
        let tx = conn.unchecked_transaction()?;
        tx.execute(
            "DELETE FROM dependencies WHERE blocker_id = ?1 AND blocked_id = ?2",
            params![req.blocker_id, req.blocked_id],
        )?;
        let payload =
            serde_json::json!({ "blocker_id": &req.blocker_id, "blocked_id": &req.blocked_id })
                .to_string();
        let event = append_event(
            &tx,
            &project_id,
            "",
            "DEP_REMOVED",
            &author,
            &format!("{} no longer blocks {}", req.blocker_id, req.blocked_id),
            &payload,
        )?;
        record_idempotency(&tx, &project_id, idem_key(req.meta.as_ref()), event.seq)?;
        tx.commit()?;
        finish_mutation(
            &conn,
            &self.notifier,
            &project_id,
            vec![event],
            vec![req.blocked_id.clone()],
        )
    }

    fn report_condition_locked(&self, req: pb::ReportConditionRequest) -> DResult<pb::Mutation> {
        let conn = self.conn.lock().unwrap();
        let current = fetch_node(&conn, &req.node_id)?
            .ok_or_else(|| DomainError::from_db_message("node not found"))?;
        let project_id = current.project_id.clone();
        if let Some(seq) = check_idempotency(&conn, &project_id, idem_key(req.meta.as_ref()))? {
            return Ok(Self::empty_mutation(seq));
        }
        let result = match req.result {
            1 => "PASS",
            2 => "FAIL",
            _ => return Err(DomainError::from_db_message("invalid agent result")),
        };
        let author = author_of(req.meta.as_ref());
        let tx = conn.unchecked_transaction()?;
        tx.execute(
            "INSERT INTO verifications (node_id, agent_result, agent_name, agent_at, agent_node_rev, agent_detail)
             VALUES (?1, ?2, ?3, unixepoch(), (SELECT updated_at FROM nodes WHERE id = ?1), ?4)
             ON CONFLICT(node_id) DO UPDATE SET
               agent_result = excluded.agent_result,
               agent_name   = excluded.agent_name,
               agent_at     = excluded.agent_at,
               agent_node_rev = excluded.agent_node_rev,
               agent_detail = excluded.agent_detail",
            params![req.node_id, result, author, req.detail],
        )?;
        let payload = serde_json::json!({ "result": result, "detail": &req.detail }).to_string();
        let event = append_event(
            &tx,
            &project_id,
            &req.node_id,
            "AGENT_REPORTED",
            &author,
            &format!("agent reported {} on {}", result, req.node_id),
            &payload,
        )?;
        record_idempotency(&tx, &project_id, idem_key(req.meta.as_ref()), event.seq)?;
        tx.commit()?;
        finish_mutation(
            &conn,
            &self.notifier,
            &project_id,
            vec![event],
            vec![req.node_id.clone()],
        )
    }

    fn set_verdict_locked(&self, req: pb::SetVerdictRequest) -> DResult<pb::Mutation> {
        let conn = self.conn.lock().unwrap();
        let current = fetch_node(&conn, &req.node_id)?
            .ok_or_else(|| DomainError::from_db_message("node not found"))?;
        let project_id = current.project_id.clone();
        if let Some(seq) = check_idempotency(&conn, &project_id, idem_key(req.meta.as_ref()))? {
            return Ok(Self::empty_mutation(seq));
        }
        let author = author_of(req.meta.as_ref());
        let tx = conn.unchecked_transaction()?;
        match req.verdict {
            1 => {
                tx.execute(
                    "INSERT INTO verifications (node_id, human_verdict, human_at)
                     VALUES (?1, 'ACCEPTED', unixepoch())
                     ON CONFLICT(node_id) DO UPDATE SET human_verdict = 'ACCEPTED', human_at = unixepoch()",
                    params![req.node_id],
                )?;
            }
            2 => {
                tx.execute(
                    "INSERT INTO verifications (node_id, human_verdict, human_at)
                     VALUES (?1, 'REJECTED', unixepoch())
                     ON CONFLICT(node_id) DO UPDATE SET human_verdict = 'REJECTED', human_at = unixepoch()",
                    params![req.node_id],
                )?;
            }
            _ => {
                tx.execute(
                    "UPDATE verifications SET human_verdict = NULL, human_at = NULL WHERE node_id = ?1",
                    params![req.node_id],
                )?;
            }
        }
        let event = append_event(
            &tx,
            &project_id,
            &req.node_id,
            "VERDICT_SET",
            &author,
            &format!("verdict set on {}", req.node_id),
            "{}",
        )?;
        record_idempotency(&tx, &project_id, idem_key(req.meta.as_ref()), event.seq)?;
        tx.commit()?;
        finish_mutation(
            &conn,
            &self.notifier,
            &project_id,
            vec![event],
            vec![req.node_id.clone()],
        )
    }

    fn add_comment_locked(&self, req: pb::AddCommentRequest) -> DResult<pb::Mutation> {
        let conn = self.conn.lock().unwrap();
        let current = fetch_node(&conn, &req.node_id)?
            .ok_or_else(|| DomainError::from_db_message("node not found"))?;
        let project_id = current.project_id.clone();
        if let Some(seq) = check_idempotency(&conn, &project_id, idem_key(req.meta.as_ref()))? {
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
        let tx = conn.unchecked_transaction()?;
        let event = append_event(
            &tx,
            &project_id,
            &req.node_id,
            "COMMENT",
            &author,
            &summary,
            &payload,
        )?;
        record_idempotency(&tx, &project_id, idem_key(req.meta.as_ref()), event.seq)?;
        tx.commit()?;
        finish_mutation(
            &conn,
            &self.notifier,
            &project_id,
            vec![event],
            vec![req.node_id.clone()],
        )
    }

    fn undo_locked(&self, req: pb::UndoRequest) -> DResult<pb::Mutation> {
        const EV: &str = "SELECT seq, project_id, COALESCE(node_id,'') AS node_id, kind, author, summary, payload AS payload_json, created_at FROM events WHERE project_id = ?1 ";
        let conn = self.conn.lock().unwrap();
        let event = if req.seq > 0 {
            conn.query_row(
                &format!("{EV} AND seq = ?2"),
                params![req.project_id, req.seq],
                proto_event,
            )
            .optional()?
        } else {
            conn.query_row(
                &format!("{EV} ORDER BY seq DESC LIMIT 1"),
                params![req.project_id],
                proto_event,
            )
            .optional()?
        };
        let event = event.ok_or_else(|| DomainError::from_db_message("no event to undo"))?;
        let project_id = event.project_id.clone();
        let node_id = event.node_id.clone();
        let kind = event.kind;
        if let Some(seq) = check_idempotency(&conn, &project_id, idem_key(req.meta.as_ref()))? {
            return Ok(Self::empty_mutation(seq));
        }

        let tx = conn.unchecked_transaction()?;
        // Reversing an event. `inverse_kind` is the valid schema kind that
        // describes the reversal we just performed (the events table CHECK
        // rejects a synthetic "UNDO" kind, so we record the reversed act).
        let mut inverse_node = node_id.clone();
        let (affected, inverse_kind, inverse_summary): (Vec<String>, &str, String) = match kind {
            k if k == pb::EventKind::StatusSet as i32 => {
                let before =
                    payload_get(&event.payload_json, "before").unwrap_or_else(|| "OPEN".into());
                tx.execute(
                    "UPDATE nodes SET declared_status = ?1 WHERE id = ?2",
                    params![before, node_id],
                )?;
                (
                    downstream_of(&tx, &node_id)?,
                    "STATUS_SET",
                    format!("undid status set on {}", node_id),
                )
            }
            k if k == pb::EventKind::DepAdded as i32 => {
                let blocker = payload_get(&event.payload_json, "blocker_id")
                    .ok_or_else(|| DomainError::from_db_message("dep payload missing blocker"))?;
                let blocked = payload_get(&event.payload_json, "blocked_id")
                    .ok_or_else(|| DomainError::from_db_message("dep payload missing blocked"))?;
                tx.execute(
                    "DELETE FROM dependencies WHERE blocker_id = ?1 AND blocked_id = ?2",
                    params![blocker, blocked],
                )?;
                (
                    vec![blocked.clone()],
                    "DEP_REMOVED",
                    format!("undid: {} no longer blocks {}", blocker, blocked),
                )
            }
            k if k == pb::EventKind::DepRemoved as i32 => {
                let blocker = payload_get(&event.payload_json, "blocker_id")
                    .ok_or_else(|| DomainError::from_db_message("dep payload missing blocker"))?;
                let blocked = payload_get(&event.payload_json, "blocked_id")
                    .ok_or_else(|| DomainError::from_db_message("dep payload missing blocked"))?;
                tx.execute(
                    "INSERT OR IGNORE INTO dependencies (blocker_id, blocked_id) VALUES (?1, ?2)",
                    params![blocker, blocked],
                )?;
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
                tx.execute("DELETE FROM nodes WHERE id = ?1", params![node_id])?;
                (
                    vec![],
                    "NODE_DELETED",
                    format!("undid creation of {}", node_id),
                )
            }
            k if k == pb::EventKind::NodeDeleted as i32 => {
                let before: serde_json::Value =
                    serde_json::from_str(&event.payload_json).map_err(|_| {
                        DomainError::from_db_message("cannot undo delete: payload missing")
                    })?;
                let obj = before.get("before").cloned().ok_or_else(|| {
                    DomainError::from_db_message("cannot undo delete: payload missing")
                })?;
                restore_node(&tx, &obj)?;
                (
                    downstream_of(&tx, &node_id)?,
                    "NODE_CREATED",
                    format!("undid deletion of {}", node_id),
                )
            }
            other => {
                return Err(DomainError::from_db_message(format!(
                    "cannot undo event kind {}",
                    other
                )))
            }
        };

        let author = author_of(req.meta.as_ref());
        let inverse = append_event(
            &tx,
            &project_id,
            &inverse_node,
            inverse_kind,
            &author,
            &inverse_summary,
            "{}",
        )?;
        record_idempotency(&tx, &project_id, idem_key(req.meta.as_ref()), inverse.seq)?;
        tx.commit()?;
        finish_mutation(&conn, &self.notifier, &project_id, vec![inverse], affected)
    }

    fn move_node_locked(&self, req: pb::MoveNodeRequest) -> DResult<pb::Mutation> {
        let conn = self.conn.lock().unwrap();
        let current = fetch_node(&conn, &req.node_id)?
            .ok_or_else(|| DomainError::from_db_message("node not found"))?;
        let project_id = current.project_id.clone();
        if let Some(seq) = check_idempotency(&conn, &project_id, idem_key(req.meta.as_ref()))? {
            return Ok(Self::empty_mutation(seq));
        }
        let old_kind = current.kind;
        let new_kind = req.kind;
        let wp = pb::NodeKind::WorkPackage as i32;
        // Structural validation: scope to STEP<->TASK + reparent; the trigger
        // backstops parent-kind / cross-project / self-parent / children-validity.
        if new_kind == pb::NodeKind::Unspecified as i32 {
            return Err(DomainError::from_db_message("move requires a target kind"));
        }
        if old_kind == wp || new_kind == wp {
            return Err(DomainError::from_db_message(
                "move cannot promote or demote a work package",
            ));
        }
        if req.parent_id == req.node_id {
            return Err(DomainError::from_db_message(
                "a node cannot be its own parent",
            ));
        }
        let author = author_of(req.meta.as_ref());

        let tx = conn.unchecked_transaction()?;
        let mut events: Vec<pb::Event> = Vec::new();
        let mut affected = vec![req.node_id.clone()];

        // TASK -> STEP demote drops this node's step children (destructive, no
        // undo — consented at the client). Collect each child's dependents BEFORE
        // deleting, then write the NODE_DELETED events with node_id = ''.
        if old_kind == pb::NodeKind::Task as i32 && new_kind == pb::NodeKind::Step as i32 {
            let child_ids = query_strings(
                &tx,
                "SELECT id FROM nodes WHERE parent_id = ?1",
                params![req.node_id],
            )?;
            for cid in &child_ids {
                let deps = query_strings(
                    &tx,
                    "SELECT blocked_id FROM dependencies WHERE blocker_id = ?1",
                    params![cid],
                )?;
                affected.extend(deps);
                tx.execute("DELETE FROM nodes WHERE id = ?1", params![cid])?;
                let payload = serde_json::json!({ "before": { "id": cid } }).to_string();
                events.push(append_event(
                    &tx,
                    &project_id,
                    "",
                    "NODE_DELETED",
                    &author,
                    &format!("deleted {}", cid),
                    &payload,
                )?);
            }
        }

        // A kind change invalidates the verification: a STEP must not carry a
        // TASK's agent badge (and vice versa).
        if old_kind != new_kind {
            tx.execute(
                "DELETE FROM verifications WHERE node_id = ?1",
                params![req.node_id],
            )?;
        }

        // Append into the new parent's sibling list.
        let new_pos: i64 = tx.query_row(
            "SELECT COALESCE(MAX(position), 0) + 100 FROM nodes WHERE parent_id = ?1",
            params![req.parent_id],
            |r| r.get(0),
        )?;

        tx.execute(
            "UPDATE nodes SET parent_id = NULLIF(?1, ''), kind = ?2, position = ?3 WHERE id = ?4",
            params![
                req.parent_id,
                kind_str(new_kind),
                new_pos as i32,
                req.node_id
            ],
        )?;

        let payload = serde_json::json!({
            "before": { "parent_id": &current.parent_id, "kind": kind_str(old_kind) },
            "after": { "parent_id": &req.parent_id, "kind": kind_str(new_kind) }
        })
        .to_string();
        events.push(append_event(
            &tx,
            &project_id,
            &req.node_id,
            "NODE_UPDATED",
            &author,
            &format!("moved {}", req.node_id),
            &payload,
        )?);

        let last_seq = events.last().map(|e| e.seq).unwrap_or(0);
        record_idempotency(&tx, &project_id, idem_key(req.meta.as_ref()), last_seq)?;
        tx.commit()?;
        finish_mutation(&conn, &self.notifier, &project_id, events, affected)
    }

    fn create_project_locked(&self, req: pb::CreateProjectRequest) -> DResult<pb::Project> {
        let conn = self.conn.lock().unwrap();
        // Sentinel-scoped idempotency ('' scope): projects log no event and the id
        // is minted here, so a retry dedups under '' and returns the created row.
        if let Some(hit) = check_idempotency_created(&conn, "", idem_key(req.meta.as_ref()))? {
            if let Some(pid) = &hit.entity_id {
                if let Some(p) = fetch_project(&conn, pid)? {
                    return Ok(p);
                }
            }
        }
        let tx = conn.unchecked_transaction()?;
        let id = new_id(&tx, "prj")?;
        tx.execute(
            "INSERT INTO projects (id, name, description) VALUES (?1, ?2, ?3)",
            params![id, req.name, req.description],
        )?;
        record_idempotency_created(&tx, "", idem_key(req.meta.as_ref()), 0, &id)?;
        tx.commit()?;
        fetch_project(&conn, &id)?
            .ok_or_else(|| DomainError::from_db_message("created project not found"))
    }

    fn update_project_locked(&self, req: pb::UpdateProjectRequest) -> DResult<pb::Project> {
        let conn = self.conn.lock().unwrap();
        if fetch_project(&conn, &req.project_id)?.is_none() {
            return Err(DomainError::from_db_message("project not found"));
        }
        if req.update_mask.is_empty() {
            return Err(DomainError::from_db_message("update_mask cannot be empty"));
        }
        let tx = conn.unchecked_transaction()?;
        for f in &req.update_mask {
            match f.as_str() {
                "name" => {
                    tx.execute(
                        "UPDATE projects SET name = ?1 WHERE id = ?2",
                        params![req.name, req.project_id],
                    )?;
                }
                "description" => {
                    tx.execute(
                        "UPDATE projects SET description = ?1 WHERE id = ?2",
                        params![req.description, req.project_id],
                    )?;
                }
                other => {
                    return Err(DomainError::from_db_message(format!(
                        "unknown update_mask field: {}",
                        other
                    )))
                }
            }
        }
        tx.execute(
            "UPDATE projects SET updated_at = unixepoch() WHERE id = ?1",
            params![req.project_id],
        )?;
        tx.commit()?;
        fetch_project(&conn, &req.project_id)?
            .ok_or_else(|| DomainError::from_db_message("project not found"))
    }

    fn archive_project_locked(&self, req: pb::ArchiveProjectRequest) -> DResult<pb::Project> {
        let conn = self.conn.lock().unwrap();
        if fetch_project(&conn, &req.project_id)?.is_none() {
            return Err(DomainError::from_db_message("project not found"));
        }
        let tx = conn.unchecked_transaction()?;
        if req.archived {
            tx.execute(
                "UPDATE projects SET archived_at = unixepoch(), updated_at = unixepoch() WHERE id = ?1",
                params![req.project_id],
            )?;
        } else {
            tx.execute(
                "UPDATE projects SET archived_at = NULL, updated_at = unixepoch() WHERE id = ?1",
                params![req.project_id],
            )?;
        }
        tx.commit()?;
        fetch_project(&conn, &req.project_id)?
            .ok_or_else(|| DomainError::from_db_message("project not found"))
    }
}

/// Run a blocking store operation off the tokio reactor, mapping a task panic to
/// an internal error. SQLite is synchronous; offloading keeps it from stalling
/// the async runtime under concurrent load (see plan/design.hosting-tenancy.md).
async fn offload<T, F>(f: F) -> DResult<T>
where
    F: FnOnce() -> DResult<T> + Send + 'static,
    T: Send + 'static,
{
    tokio::task::spawn_blocking(f)
        .await
        .map_err(|e| DomainError::internal(format!("db task panicked: {e}")))?
}

/// The async `Store` edge: thin wrappers that offload each synchronous `*_locked`
/// operation onto the blocking pool. The trait stays async so a future remote-DB
/// backend (Postgres/libSQL/D1) can implement it natively — the sync-ness lives
/// only in the `*_locked` methods, which become `flow-core` in Phase 2.
#[::async_trait::async_trait]
impl Store for SqliteStore {
    async fn list_projects(&self, include_archived: bool) -> DResult<Vec<pb::Project>> {
        let this = self.clone();
        offload(move || this.list_projects_locked(include_archived)).await
    }
    async fn get_snapshot(&self, project_id: &str) -> DResult<pb::GetSnapshotResponse> {
        let this = self.clone();
        let project_id = project_id.to_string();
        offload(move || this.get_snapshot_locked(&project_id)).await
    }
    async fn events_after(&self, project_id: &str, from_seq: i64) -> DResult<Vec<pb::Event>> {
        let this = self.clone();
        let project_id = project_id.to_string();
        offload(move || this.events_after_locked(&project_id, from_seq)).await
    }
    async fn poll_changes(
        &self,
        project_id: &str,
        after_seq: i64,
        limit: i32,
    ) -> DResult<pb::PollChangesResponse> {
        let this = self.clone();
        let project_id = project_id.to_string();
        offload(move || this.poll_changes_locked(&project_id, after_seq, limit)).await
    }
    async fn list_events(
        &self,
        project_id: &str,
        node_id: &str,
        before_seq: i64,
        limit: i32,
    ) -> DResult<(Vec<pb::Event>, bool)> {
        let this = self.clone();
        let project_id = project_id.to_string();
        let node_id = node_id.to_string();
        offload(move || this.list_events_locked(&project_id, &node_id, before_seq, limit)).await
    }
    async fn search(&self, project_id: &str, query: &str, limit: i32) -> DResult<Vec<pb::Node>> {
        let this = self.clone();
        let project_id = project_id.to_string();
        let query = query.to_string();
        offload(move || this.search_locked(&project_id, &query, limit)).await
    }
    fn subscribe(&self) -> broadcast::Receiver<Notified> {
        self.notifier.subscribe()
    }
    async fn create_node(&self, req: pb::CreateNodeRequest) -> DResult<pb::Mutation> {
        let this = self.clone();
        offload(move || this.create_node_locked(req)).await
    }
    async fn update_node(&self, req: pb::UpdateNodeRequest) -> DResult<pb::Mutation> {
        let this = self.clone();
        offload(move || this.update_node_locked(req)).await
    }
    async fn delete_node(&self, req: pb::DeleteNodeRequest) -> DResult<pb::Mutation> {
        let this = self.clone();
        offload(move || this.delete_node_locked(req)).await
    }
    async fn set_status(&self, req: pb::SetStatusRequest) -> DResult<pb::Mutation> {
        let this = self.clone();
        offload(move || this.set_status_locked(req)).await
    }
    async fn add_dependency(&self, req: pb::AddDependencyRequest) -> DResult<pb::Mutation> {
        let this = self.clone();
        offload(move || this.add_dependency_locked(req)).await
    }
    async fn remove_dependency(&self, req: pb::RemoveDependencyRequest) -> DResult<pb::Mutation> {
        let this = self.clone();
        offload(move || this.remove_dependency_locked(req)).await
    }
    async fn report_condition(&self, req: pb::ReportConditionRequest) -> DResult<pb::Mutation> {
        let this = self.clone();
        offload(move || this.report_condition_locked(req)).await
    }
    async fn set_verdict(&self, req: pb::SetVerdictRequest) -> DResult<pb::Mutation> {
        let this = self.clone();
        offload(move || this.set_verdict_locked(req)).await
    }
    async fn add_comment(&self, req: pb::AddCommentRequest) -> DResult<pb::Mutation> {
        let this = self.clone();
        offload(move || this.add_comment_locked(req)).await
    }
    async fn undo(&self, req: pb::UndoRequest) -> DResult<pb::Mutation> {
        let this = self.clone();
        offload(move || this.undo_locked(req)).await
    }
    async fn move_node(&self, req: pb::MoveNodeRequest) -> DResult<pb::Mutation> {
        let this = self.clone();
        offload(move || this.move_node_locked(req)).await
    }
    async fn create_project(&self, req: pb::CreateProjectRequest) -> DResult<pb::Project> {
        let this = self.clone();
        offload(move || this.create_project_locked(req)).await
    }
    async fn update_project(&self, req: pb::UpdateProjectRequest) -> DResult<pb::Project> {
        let this = self.clone();
        offload(move || this.update_project_locked(req)).await
    }
    async fn archive_project(&self, req: pb::ArchiveProjectRequest) -> DResult<pb::Project> {
        let this = self.clone();
        offload(move || this.archive_project_locked(req)).await
    }
}

impl SqliteStore {
    /// Transport-agnostic entry point — "the service minus the socket". Decodes a
    /// protobuf request for `method`, runs the synchronous store operation, and
    /// returns the encoded protobuf response. The native tonic edge, the Node
    /// host, and the browser all funnel through this one call; it is the seam the
    /// wasm `#[wasm_bindgen]` facade wraps. Unary only — `Watch` streaming stays a
    /// native-edge concern. `method` is the proto RPC name (e.g. "CreateNode").
    pub fn dispatch(&self, method: &str, req: &[u8]) -> DResult<Vec<u8>> {
        use prost::Message;
        fn decode<T: Message + Default>(bytes: &[u8]) -> DResult<T> {
            T::decode(bytes).map_err(|e| DomainError::invalid_argument(format!("bad request: {e}")))
        }
        Ok(match method {
            // Reads
            "ListProjects" => {
                let r: pb::ListProjectsRequest = decode(req)?;
                let projects = self.list_projects_locked(r.include_archived)?;
                pb::ListProjectsResponse { projects }.encode_to_vec()
            }
            "GetSnapshot" => {
                let r: pb::GetSnapshotRequest = decode(req)?;
                self.get_snapshot_locked(&r.project_id)?.encode_to_vec()
            }
            "ListEvents" => {
                let r: pb::ListEventsRequest = decode(req)?;
                let (events, has_more) =
                    self.list_events_locked(&r.project_id, &r.node_id, r.before_seq, r.limit)?;
                pb::ListEventsResponse { events, has_more }.encode_to_vec()
            }
            "Search" => {
                let r: pb::SearchRequest = decode(req)?;
                let nodes = self.search_locked(&r.project_id, &r.query, r.limit)?;
                pb::SearchResponse { nodes }.encode_to_vec()
            }
            "PollChanges" => {
                let r: pb::PollChangesRequest = decode(req)?;
                self.poll_changes_locked(&r.project_id, r.after_seq, r.limit)?
                    .encode_to_vec()
            }
            // Writes (node/dep/status/etc. → Mutation)
            "CreateNode" => {
                let m = self.create_node_locked(decode(req)?)?;
                pb::CreateNodeResponse { mutation: Some(m) }.encode_to_vec()
            }
            "UpdateNode" => {
                let m = self.update_node_locked(decode(req)?)?;
                pb::UpdateNodeResponse { mutation: Some(m) }.encode_to_vec()
            }
            "DeleteNode" => {
                let m = self.delete_node_locked(decode(req)?)?;
                pb::DeleteNodeResponse { mutation: Some(m) }.encode_to_vec()
            }
            "SetStatus" => {
                let m = self.set_status_locked(decode(req)?)?;
                pb::SetStatusResponse { mutation: Some(m) }.encode_to_vec()
            }
            "AddDependency" => {
                let m = self.add_dependency_locked(decode(req)?)?;
                pb::AddDependencyResponse { mutation: Some(m) }.encode_to_vec()
            }
            "RemoveDependency" => {
                let m = self.remove_dependency_locked(decode(req)?)?;
                pb::RemoveDependencyResponse { mutation: Some(m) }.encode_to_vec()
            }
            "ReportCondition" => {
                let m = self.report_condition_locked(decode(req)?)?;
                pb::ReportConditionResponse { mutation: Some(m) }.encode_to_vec()
            }
            "SetVerdict" => {
                let m = self.set_verdict_locked(decode(req)?)?;
                pb::SetVerdictResponse { mutation: Some(m) }.encode_to_vec()
            }
            "AddComment" => {
                let m = self.add_comment_locked(decode(req)?)?;
                pb::AddCommentResponse { mutation: Some(m) }.encode_to_vec()
            }
            "Undo" => {
                let m = self.undo_locked(decode(req)?)?;
                pb::UndoResponse { mutation: Some(m) }.encode_to_vec()
            }
            "MoveNode" => {
                let m = self.move_node_locked(decode(req)?)?;
                pb::MoveNodeResponse { mutation: Some(m) }.encode_to_vec()
            }
            // Project lifecycle → Project
            "CreateProject" => {
                let p = self.create_project_locked(decode(req)?)?;
                pb::CreateProjectResponse { project: Some(p) }.encode_to_vec()
            }
            "UpdateProject" => {
                let p = self.update_project_locked(decode(req)?)?;
                pb::UpdateProjectResponse { project: Some(p) }.encode_to_vec()
            }
            "ArchiveProject" => {
                let p = self.archive_project_locked(decode(req)?)?;
                pb::ArchiveProjectResponse { project: Some(p) }.encode_to_vec()
            }
            other => return Err(DomainError::not_found(format!("unknown method: {other}"))),
        })
    }
}

// ── shared write helpers (free fns over a &Connection; a &Transaction coerces) ─

/// Build the shared mutation payload for a committed write, publish it to Watch
/// subscribers, and return it as the unary response (one apply-path for both).
fn finish_mutation(
    conn: &Connection,
    notifier: &Notifier,
    project_id: &str,
    events: Vec<pb::Event>,
    affected_ids: Vec<String>,
) -> DResult<pb::Mutation> {
    let seq = events.last().map(|e| e.seq).unwrap_or(0);

    let mut seen: HashSet<String> = HashSet::new();
    let mut changed_nodes = Vec::new();
    for id in affected_ids {
        if seen.insert(id.clone()) {
            if let Some(n) = fetch_node(conn, &id)? {
                changed_nodes.push(n);
            }
        }
    }

    let changed_progress = progress_for(conn, project_id)?;

    let notified = Notified {
        project_id: project_id.to_string(),
        seq,
        events: events.clone(),
        changed_nodes: changed_nodes.clone(),
        changed_progress: changed_progress.clone(),
    };
    let _ = notifier.send(notified);

    Ok(pb::Mutation {
        events,
        changed_nodes,
        changed_progress,
        seq,
    })
}

fn check_idempotency(
    conn: &Connection,
    project_id: &str,
    key: Option<&str>,
) -> DResult<Option<i64>> {
    let Some(key) = key else { return Ok(None) };
    Ok(conn
        .query_row(
            "SELECT seq FROM idempotency WHERE project_id = ?1 AND idempotency_key = ?2",
            params![project_id, key],
            |r| r.get(0),
        )
        .optional()?)
}

fn record_idempotency(
    conn: &Connection,
    project_id: &str,
    key: Option<&str>,
    seq: i64,
) -> DResult<()> {
    let Some(key) = key else { return Ok(()) };
    conn.execute(
        "INSERT OR IGNORE INTO idempotency (project_id, idempotency_key, seq) VALUES (?1, ?2, ?3)",
        params![project_id, key, seq],
    )?;
    Ok(())
}

fn downstream_of(conn: &Connection, node_id: &str) -> DResult<Vec<String>> {
    query_strings(
        conn,
        "SELECT blocked_id FROM dependencies WHERE blocker_id = ?1",
        params![node_id],
    )
}

/// Progress per work package, counting leaves (steps where they exist, else the
/// task itself) with the effective status of each leaf.
fn progress_for(conn: &Connection, project_id: &str) -> DResult<Vec<pb::Progress>> {
    let mut stmt = conn.prepare(
        "SELECT t.parent_id AS wp_id,
                count(*) AS total,
                sum(l.status = 'DONE')     AS done,
                sum(l.status = 'DEFERRED') AS deferred,
                sum(l.status = 'READY')    AS ready,
                sum(l.status = 'BLOCKED')  AS blocked
         FROM nodes t
         LEFT JOIN nodes s ON s.parent_id = t.id AND s.kind = 'STEP'
         JOIN node_state l ON l.id = COALESCE(s.id, t.id)
         WHERE t.kind = 'TASK' AND t.project_id = ?1
         GROUP BY t.parent_id
         ORDER BY t.parent_id",
    )?;
    let rows = stmt.query_map(params![project_id], |r| {
        Ok(pb::Progress {
            work_package_id: r.get::<_, String>("wp_id")?,
            total: r.get::<_, i64>("total")? as i32,
            done: r.get::<_, i64>("done")? as i32,
            ready: r.get::<_, i64>("ready")? as i32,
            blocked: r.get::<_, i64>("blocked")? as i32,
            deferred: r.get::<_, i64>("deferred")? as i32,
        })
    })?;
    Ok(rows.collect::<rusqlite::Result<Vec<_>>>()?)
}

fn verification_for(conn: &Connection, node_id: &str) -> DResult<Option<pb::Verification>> {
    Ok(conn
        .query_row(
            "SELECT v.agent_result, COALESCE(v.agent_name,'') AS agent_name,
                    COALESCE(v.agent_at,0) AS agent_at, v.agent_detail,
                    v.human_verdict, COALESCE(v.human_at,0) AS human_at,
                    (v.agent_node_rev IS NOT NULL AND n.updated_at > v.agent_node_rev) AS stale
             FROM verifications v JOIN nodes n ON n.id = v.node_id
             WHERE v.node_id = ?1",
            params![node_id],
            row_to_verification,
        )
        .optional()?)
}

/// Fetch one node by id as a proto Node (None if missing).
fn fetch_node(conn: &Connection, id: &str) -> DResult<Option<pb::Node>> {
    let mut node = conn
        .query_row(
            "SELECT n.id, n.project_id, COALESCE(n.parent_id,'') AS parent_id,
                    n.kind, n.title, n.description, n.condition, n.note, COALESCE(n.reference,'') AS reference,
                    n.declared_status, n.wp_state, n.position, n.created_at, n.updated_at,
                    n.status AS effective_status
             FROM node_state n WHERE n.id = ?1",
            params![id],
            row_to_node,
        )
        .optional()?;
    if let Some(n) = &mut node {
        n.verification = verification_for(conn, &n.id)?;
    }
    Ok(node)
}

/// Owning project of a node ('' if the node is unknown).
fn project_of(conn: &Connection, node_id: &str) -> DResult<String> {
    Ok(conn
        .query_row(
            "SELECT project_id FROM nodes WHERE id = ?1",
            params![node_id],
            |r| r.get::<_, String>(0),
        )
        .optional()?
        .unwrap_or_default())
}

/// Fetch one project as a proto Project (None if missing).
fn fetch_project(conn: &Connection, id: &str) -> DResult<Option<pb::Project>> {
    Ok(conn
        .query_row(
            "SELECT id, name, description, COALESCE(archived_at,0) AS archived_at, created_at
             FROM projects WHERE id = ?1",
            params![id],
            row_to_project,
        )
        .optional()?)
}

/// Idempotency lookup that also returns the id of the entity the original request
/// created (for create replays). Used by create_node / create_project.
fn check_idempotency_created(
    conn: &Connection,
    scope: &str,
    key: Option<&str>,
) -> DResult<Option<IdemHit>> {
    let Some(key) = key else { return Ok(None) };
    Ok(conn
        .query_row(
            "SELECT seq, entity_id FROM idempotency WHERE project_id = ?1 AND idempotency_key = ?2",
            params![scope, key],
            |r| {
                Ok(IdemHit {
                    seq: r.get("seq")?,
                    entity_id: r.get::<_, Option<String>>("entity_id")?,
                })
            },
        )
        .optional()?)
}

/// Record an idempotency row that remembers the created entity's id.
fn record_idempotency_created(
    conn: &Connection,
    scope: &str,
    key: Option<&str>,
    seq: i64,
    entity_id: &str,
) -> DResult<()> {
    let Some(key) = key else { return Ok(()) };
    conn.execute(
        "INSERT OR IGNORE INTO idempotency (project_id, idempotency_key, seq, entity_id) VALUES (?1, ?2, ?3, ?4)",
        params![scope, key, seq, entity_id],
    )?;
    Ok(())
}

/// Run a query whose first column is text and collect it into a Vec.
fn query_strings<P: rusqlite::Params>(conn: &Connection, sql: &str, p: P) -> DResult<Vec<String>> {
    let mut stmt = conn.prepare(sql)?;
    let rows = stmt.query_map(p, |r| r.get::<_, String>(0))?;
    Ok(rows.collect::<rusqlite::Result<Vec<_>>>()?)
}

/// Mint a short, sortable, unique id (`<prefix>-<unix-millis>-<8 hex>`) via SQL,
/// so no host-clock/entropy API is needed — critical for the wasm target where
/// `SystemTime::now()` panics and `RandomState` has no OS entropy.
fn new_id(conn: &Connection, prefix: &str) -> DResult<String> {
    Ok(conn.query_row(
        "SELECT ?1 || '-' || CAST(unixepoch('subsec') * 1000 AS INTEGER) || '-' || lower(hex(randomblob(4)))",
        params![prefix],
        |r| r.get::<_, String>(0),
    )?)
}

/// Insert an event row; returns the fully-materialised `pb::Event`. `payload`
/// must be valid JSON (the schema enforces it with a CHECK).
fn append_event(
    conn: &Connection,
    project_id: &str,
    node_id: &str,
    kind: &str,
    author: &str,
    summary: &str,
    payload: &str,
) -> DResult<pb::Event> {
    let node: Option<&str> = if node_id.is_empty() {
        None
    } else {
        Some(node_id)
    };
    Ok(conn.query_row(
        "INSERT INTO events (project_id, node_id, kind, author, summary, payload)
         VALUES (?1, ?2, ?3, ?4, ?5, ?6)
         RETURNING seq, project_id, COALESCE(node_id,'') AS node_id, kind, author, summary, payload AS payload_json, created_at",
        params![project_id, node, kind, author, summary, payload],
        proto_event,
    )?)
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
    // Only WORK_PACKAGE rows carry a wp_state; the schema CHECK requires TASK/STEP
    // rows to have NULL there, so leave it out for them.
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
fn restore_node(conn: &Connection, obj: &serde_json::Value) -> DResult<()> {
    let wp = if str_val(obj, "kind") == "WORK_PACKAGE" {
        wp_state_str(obj.get("wp_state").and_then(|v| v.as_str()))
    } else {
        None
    };
    conn.execute(
        "INSERT OR IGNORE INTO nodes (id, project_id, parent_id, kind, title, description, condition, note, reference, declared_status, wp_state, position)
         VALUES (?1, ?2, NULLIF(?3, ''), ?4, ?5, ?6, ?7, ?8, NULLIF(?9, ''), ?10, ?11, ?12)",
        params![
            str_val(obj, "id"),
            str_val(obj, "project_id"),
            str_val(obj, "parent_id"),
            str_val(obj, "kind"),
            str_val(obj, "title"),
            str_val(obj, "description"),
            str_val(obj, "condition"),
            str_val(obj, "note"),
            str_val(obj, "reference"),
            declared_str_from(str_val(obj, "declared_status")),
            wp,
            obj.get("position").and_then(|v| v.as_i64()).unwrap_or(0) as i32,
        ],
    )?;
    Ok(())
}

fn str_val<'a>(obj: &'a serde_json::Value, key: &str) -> &'a str {
    obj.get(key).and_then(|v| v.as_str()).unwrap_or("")
}

/// Map a stored wp_state string back to a stored kind. Kept explicit so undo
/// events survive the schema CHECK.
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

/// Convert a projects row into a proto Project.
fn row_to_project(r: &Row) -> rusqlite::Result<pb::Project> {
    Ok(pb::Project {
        id: r.get("id")?,
        name: r.get("name")?,
        description: r.get("description")?,
        archived_at: r.get("archived_at")?,
        created_at: r.get("created_at")?,
    })
}

/// Convert a node_state row into a proto Node.
fn row_to_node(r: &Row) -> rusqlite::Result<pb::Node> {
    let kind = match r.get::<_, String>("kind")?.as_str() {
        "WORK_PACKAGE" => pb::NodeKind::WorkPackage,
        "TASK" => pb::NodeKind::Task,
        _ => pb::NodeKind::Step,
    };
    let declared = match r.get::<_, String>("declared_status")?.as_str() {
        "DEFERRED" => pb::DeclaredStatus::Deferred,
        "DONE" => pb::DeclaredStatus::Done,
        _ => pb::DeclaredStatus::Open,
    };
    let wp_state = match r.get::<_, Option<String>>("wp_state")? {
        Some(s) if s == "ACTIVE" => pb::WorkPackageState::Active,
        Some(s) if s == "DONE" => pb::WorkPackageState::Done,
        Some(s) if s == "ARCHIVED" => pb::WorkPackageState::Archived,
        _ => pb::WorkPackageState::Planned,
    };
    Ok(pb::Node {
        id: r.get("id")?,
        project_id: r.get("project_id")?,
        parent_id: r.get("parent_id")?,
        kind: kind as i32,
        title: r.get("title")?,
        description: r.get("description")?,
        condition: r.get("condition")?,
        note: r.get("note")?,
        reference: r.get("reference")?,
        declared_status: declared as i32,
        status: effective_status(r.get::<_, String>("effective_status")?.as_str()),
        wp_state: wp_state as i32,
        position: r.get("position")?,
        verification: None,
        created_at: r.get("created_at")?,
        updated_at: r.get("updated_at")?,
    })
}

/// Convert a verification row into a proto Verification.
fn row_to_verification(r: &Row) -> rusqlite::Result<pb::Verification> {
    let agent_result = match r.get::<_, Option<String>>("agent_result")?.as_deref() {
        Some("PASS") => pb::AgentResult::Pass,
        Some("FAIL") => pb::AgentResult::Fail,
        _ => pb::AgentResult::Unspecified,
    };
    let human_verdict = match r.get::<_, Option<String>>("human_verdict")?.as_deref() {
        Some("ACCEPTED") => pb::HumanVerdict::Accepted,
        Some("REJECTED") => pb::HumanVerdict::Rejected,
        _ => pb::HumanVerdict::Unspecified,
    };
    Ok(pb::Verification {
        agent_result: agent_result as i32,
        agent_name: r.get("agent_name")?,
        agent_at: r.get("agent_at")?,
        agent_detail: r
            .get::<_, Option<String>>("agent_detail")?
            .unwrap_or_default(),
        human_verdict: human_verdict as i32,
        human_at: r.get("human_at")?,
        stale: r.get::<_, i64>("stale")? != 0,
    })
}

/// Map an events row to a proto `Event`. The `payload` column is aliased to
/// `payload_json` in every SELECT.
fn proto_event(r: &Row) -> rusqlite::Result<pb::Event> {
    let kind = match r.get::<_, String>("kind")?.as_str() {
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
    Ok(pb::Event {
        seq: r.get("seq")?,
        project_id: r.get("project_id")?,
        node_id: r.get("node_id")?,
        kind: kind as i32,
        author: r.get("author")?,
        summary: r.get("summary")?,
        payload_json: r.get("payload_json")?,
        created_at: r.get("created_at")?,
    })
}
