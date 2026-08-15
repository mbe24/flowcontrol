//! The synchronous store — the heart of `flowcore`, target-agnostic and
//! wasm-compilable (no tokio, no tonic, no direct rusqlite here).
//!
//! Written against the [`Sql`] seam (`crate::sql`): each public `*_locked` method
//! opens one [`Session`] (which holds the connection lock for its whole duration),
//! reads, runs writes inside one [`transaction`], and returns — no `.await`. The
//! native edge wraps these in `spawn_blocking`; the wasm host calls [`SqliteStore::dispatch`].
//! The same code runs over rusqlite (native / self-contained wasm) or a
//! host-imported driver (Node `node:sqlite`, browser `sqlite-wasm`).
//!
//! Errors carry a [`DomainError`] whose `Code` is classified from the message,
//! preserving one taxonomy across hosts.

use std::collections::HashSet;

use crate::error::DomainError;
use crate::generated::flow_v1 as pb;
use crate::sql::{transaction, Row, Session, Sql, Value};
use crate::values;

type DResult<T> = Result<T, DomainError>;

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

/// SQLite-backed store, generic over the [`Sql`] driver. Cheap to clone.
#[derive(Clone)]
pub struct SqliteStore<S: Sql> {
    sql: S,
}

impl<S: Sql> SqliteStore<S> {
    /// Wrap a driver.
    pub fn new(sql: S) -> Self {
        Self { sql }
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

/// Native convenience: build a store from a rusqlite connection.
#[cfg(not(target_arch = "wasm32"))]
impl SqliteStore<crate::sql::RusqliteSql> {
    pub fn open(conn: rusqlite::Connection) -> Self {
        Self::new(crate::sql::RusqliteSql::new(conn))
    }
}

impl<S: Sql> SqliteStore<S> {
    pub fn list_projects_locked(&self, include_archived: bool) -> DResult<Vec<pb::Project>> {
        let mut s = self.sql.session()?;
        s.query(
            "SELECT id, name, description, COALESCE(archived_at,0) AS archived_at, created_at
             FROM projects
             WHERE ?1 = 1 OR archived_at IS NULL
             ORDER BY created_at",
            values![include_archived],
        )?
        .iter()
        .map(row_to_project)
        .collect()
    }

    pub fn get_snapshot_locked(&self, project_id: &str) -> DResult<pb::GetSnapshotResponse> {
        let mut s = self.sql.session()?;

        let project = s
            .query_opt(
                "SELECT id, name, description, COALESCE(archived_at,0) AS archived_at, created_at
                 FROM projects WHERE id = ?1",
                values![project_id],
            )?
            .map(|r| row_to_project(&r))
            .transpose()?;

        let mut nodes: Vec<pb::Node> = s
            .query(
                "SELECT n.id, n.project_id, COALESCE(n.parent_id,'') AS parent_id,
                        n.kind, n.title, n.description, n.condition, n.note, COALESCE(n.reference,'') AS reference,
                        n.declared_status, n.wp_state, n.position, n.created_at, n.updated_at,
                        n.status AS effective_status
                 FROM node_state n
                 WHERE n.project_id = ?1",
                values![project_id],
            )?
            .iter()
            .map(row_to_node)
            .collect::<DResult<Vec<_>>>()?;
        for node in &mut nodes {
            node.verification = verification_for(&mut *s, &node.id)?;
        }

        let dependencies: Vec<pb::Dependency> = s
            .query(
                "SELECT d.blocker_id, d.blocked_id
                 FROM dependencies d
                 JOIN nodes a ON a.id = d.blocker_id
                 JOIN nodes b ON b.id = d.blocked_id
                 WHERE a.project_id = ?1 AND b.project_id = ?1",
                values![project_id],
            )?
            .iter()
            .map(|r| {
                Ok(pb::Dependency {
                    blocker_id: r.get_str("blocker_id")?,
                    blocked_id: r.get_str("blocked_id")?,
                })
            })
            .collect::<DResult<Vec<_>>>()?;

        let progress = progress_for(&mut *s, project_id)?;

        let recent_events: Vec<pb::Event> = s
            .query(
                "SELECT seq, project_id, COALESCE(node_id,'') AS node_id, kind, author, summary, payload AS payload_json, created_at
                 FROM events WHERE project_id = ?1
                 ORDER BY seq DESC LIMIT 25",
                values![project_id],
            )?
            .iter()
            .map(proto_event)
            .collect::<DResult<Vec<_>>>()?;
        let seq = s
            .query_one("SELECT COALESCE(MAX(seq),0) FROM events", values![])?
            .get_i64_at(0)?;

        Ok(pb::GetSnapshotResponse {
            project,
            nodes,
            dependencies,
            progress,
            recent_events,
            seq,
        })
    }

    pub fn events_after_locked(&self, project_id: &str, from_seq: i64) -> DResult<Vec<pb::Event>> {
        let mut s = self.sql.session()?;
        s.query(
            "SELECT seq, project_id, COALESCE(node_id,'') AS node_id, kind, author, summary, payload AS payload_json, created_at
             FROM events WHERE project_id = ?1 AND seq > ?2
             ORDER BY seq ASC",
            values![project_id, from_seq],
        )?
        .iter()
        .map(proto_event)
        .collect()
    }

    pub fn poll_changes_locked(
        &self,
        project_id: &str,
        after_seq: i64,
        limit: i32,
    ) -> DResult<pb::PollChangesResponse> {
        let lim = if limit <= 0 { 1000 } else { limit };
        let mut s = self.sql.session()?;
        let events: Vec<pb::Event> = s
            .query(
                "SELECT seq, project_id, COALESCE(node_id,'') AS node_id, kind, author, summary, payload AS payload_json, created_at
                 FROM events WHERE project_id = ?1 AND seq > ?2
                 ORDER BY seq ASC LIMIT ?3",
                values![project_id, after_seq, lim],
            )?
            .iter()
            .map(proto_event)
            .collect::<DResult<Vec<_>>>()?;
        let seq = events.last().map(|e| e.seq).unwrap_or(after_seq);
        Ok(pb::PollChangesResponse { events, seq })
    }

    pub fn list_events_locked(
        &self,
        project_id: &str,
        node_id: &str,
        before_seq: i64,
        limit: i32,
    ) -> DResult<(Vec<pb::Event>, bool)> {
        let lim = limit.max(1);
        let mut s = self.sql.session()?;
        let mut rows: Vec<pb::Event> = s
            .query(
                "SELECT seq, project_id, COALESCE(node_id,'') AS node_id, kind, author, summary, payload AS payload_json, created_at
                 FROM events
                 WHERE project_id = ?1
                   AND (?2 = '' OR node_id = ?2)
                   AND (?3 = 0 OR seq < ?3)
                 ORDER BY seq DESC LIMIT ?4",
                values![project_id, node_id, before_seq, lim + 1],
            )?
            .iter()
            .map(proto_event)
            .collect::<DResult<Vec<_>>>()?;
        let has_more = rows.len() as i32 > lim;
        rows.truncate(lim as usize);
        Ok((rows, has_more))
    }

    pub fn search_locked(
        &self,
        project_id: &str,
        query: &str,
        limit: i32,
    ) -> DResult<Vec<pb::Node>> {
        let mut s = self.sql.session()?;
        let mut nodes: Vec<pb::Node> = s
            .query(
                "SELECT n.id, n.project_id, COALESCE(n.parent_id,'') AS parent_id,
                        n.kind, n.title, n.description, n.condition, n.note, COALESCE(n.reference,'') AS reference,
                        n.declared_status, n.wp_state, n.position, n.created_at, n.updated_at,
                        ns.status AS effective_status
                 FROM nodes n
                 JOIN node_state ns ON ns.id = n.id
                 JOIN nodes_fts f ON f.rowid = n.rowid
                 WHERE n.project_id = ?1 AND nodes_fts MATCH ?2
                 ORDER BY rank LIMIT ?3",
                values![project_id, query, limit.max(1)],
            )?
            .iter()
            .map(row_to_node)
            .collect::<DResult<Vec<_>>>()?;
        for node in &mut nodes {
            node.verification = verification_for(&mut *s, &node.id)?;
        }
        Ok(nodes)
    }

    pub fn create_node_locked(&self, req: pb::CreateNodeRequest) -> DResult<pb::Mutation> {
        let mut s = self.sql.session()?;
        if let Some(hit) =
            check_idempotency_created(&mut *s, &req.project_id, idem_key(req.meta.as_ref()))?
        {
            if let Some(eid) = &hit.entity_id {
                if let Some(n) = fetch_node(&mut *s, eid)? {
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
        let project_id = req.project_id.clone();
        let (event, node_id) = transaction(&mut *s, |s| {
            let node_id = new_id(s, "node")?;
            s.execute(
                "INSERT INTO nodes (id, project_id, parent_id, kind, title, description, condition, note, reference, declared_status, wp_state, position)
                 VALUES (?1, ?2, NULLIF(?3, ''), ?4, ?5, ?6, ?7, ?8, NULLIF(?9, ''), 'OPEN', ?10, ?11)",
                values![
                    &node_id, &req.project_id, &req.parent_id, kind_str(req.kind), &req.title,
                    &req.description, &req.condition, &req.note, &req.reference, wp, req.position
                ],
            )?;
            let payload =
                serde_json::json!({ "after": { "id": &node_id, "title": &req.title } }).to_string();
            let event = append_event(
                s,
                &req.project_id,
                &node_id,
                "NODE_CREATED",
                &author,
                &format!("created {}", node_id),
                &payload,
            )?;
            record_idempotency_created(
                s,
                &req.project_id,
                idem_key(req.meta.as_ref()),
                event.seq,
                &node_id,
            )?;
            Ok((event, node_id))
        })?;
        finish_mutation(&mut *s, &project_id, vec![event], vec![node_id])
    }

    pub fn update_node_locked(&self, req: pb::UpdateNodeRequest) -> DResult<pb::Mutation> {
        let mut s = self.sql.session()?;
        let current = fetch_node(&mut *s, &req.node_id)?
            .ok_or_else(|| DomainError::from_db_message("node not found"))?;
        let project_id = current.project_id.clone();
        if let Some(seq) = check_idempotency(&mut *s, &project_id, idem_key(req.meta.as_ref()))? {
            return Ok(Self::empty_mutation(seq));
        }
        if req.update_mask.is_empty() {
            return Err(DomainError::from_db_message("update_mask cannot be empty"));
        }
        let author = author_of(req.meta.as_ref());
        let event = transaction(&mut *s, |s| {
            for f in &req.update_mask {
                match f.as_str() {
                    "title" => {
                        s.execute(
                            "UPDATE nodes SET title = ?1 WHERE id = ?2",
                            values![&req.title, &req.node_id],
                        )?;
                    }
                    "description" => {
                        s.execute(
                            "UPDATE nodes SET description = ?1 WHERE id = ?2",
                            values![&req.description, &req.node_id],
                        )?;
                    }
                    "condition" => {
                        s.execute(
                            "UPDATE nodes SET condition = ?1 WHERE id = ?2",
                            values![&req.condition, &req.node_id],
                        )?;
                    }
                    "position" => {
                        s.execute(
                            "UPDATE nodes SET position = ?1 WHERE id = ?2",
                            values![req.position, &req.node_id],
                        )?;
                    }
                    "reference" => {
                        s.execute(
                            "UPDATE nodes SET reference = ?1 WHERE id = ?2",
                            values![&req.reference, &req.node_id],
                        )?;
                    }
                    "note" => {
                        s.execute(
                            "UPDATE nodes SET note = ?1 WHERE id = ?2",
                            values![&req.note, &req.node_id],
                        )?;
                    }
                    "wp_state" => {
                        s.execute(
                            "UPDATE nodes SET wp_state = ?1 WHERE id = ?2",
                            values![wp_str(req.wp_state), &req.node_id],
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
            let event = append_event(
                s,
                &project_id,
                &req.node_id,
                "NODE_UPDATED",
                &author,
                &format!("updated {}", req.node_id),
                "{}",
            )?;
            record_idempotency(s, &project_id, idem_key(req.meta.as_ref()), event.seq)?;
            Ok(event)
        })?;
        finish_mutation(&mut *s, &project_id, vec![event], vec![req.node_id.clone()])
    }

    pub fn delete_node_locked(&self, req: pb::DeleteNodeRequest) -> DResult<pb::Mutation> {
        let mut s = self.sql.session()?;
        let current = fetch_node(&mut *s, &req.node_id)?
            .ok_or_else(|| DomainError::from_db_message("node not found"))?;
        let project_id = current.project_id.clone();
        if let Some(seq) = check_idempotency(&mut *s, &project_id, idem_key(req.meta.as_ref()))? {
            return Ok(Self::empty_mutation(seq));
        }
        if req.fail_if_referenced {
            let deps = s
                .query_one(
                    "SELECT count(*) FROM dependencies WHERE blocker_id = ?1 OR blocked_id = ?1",
                    values![&req.node_id],
                )?
                .get_i64_at(0)?;
            if deps > 0 {
                return Err(DomainError::from_db_message("node has dependents"));
            }
        }
        let before = node_before_json(&current);
        // Snapshot the whole subtree up front: the cascade delete removes children
        // silently, so collect their EXTERNAL dependents and emit a delete event
        // per node before they vanish.
        let dependents = query_strings(
            &mut *s,
            "WITH RECURSIVE subtree(id) AS (
                 SELECT ?1 UNION ALL
                 SELECT n.id FROM nodes n JOIN subtree s ON n.parent_id = s.id
             )
             SELECT DISTINCT d.blocked_id FROM dependencies d
             JOIN subtree s ON d.blocker_id = s.id
             WHERE d.blocked_id NOT IN (SELECT id FROM subtree)",
            values![&req.node_id],
        )?;
        let descendants = query_strings(
            &mut *s,
            "WITH RECURSIVE subtree(id) AS (
                 SELECT ?1 UNION ALL
                 SELECT n.id FROM nodes n JOIN subtree s ON n.parent_id = s.id
             )
             SELECT id FROM subtree WHERE id <> ?1",
            values![&req.node_id],
        )?;

        let author = author_of(req.meta.as_ref());
        let events = transaction(&mut *s, |s| {
            s.execute("DELETE FROM nodes WHERE id = ?1", values![&req.node_id])?;
            let mut events = Vec::with_capacity(descendants.len() + 1);
            for cid in &descendants {
                let payload = serde_json::json!({ "before": { "id": cid } }).to_string();
                events.push(append_event(
                    s,
                    &project_id,
                    "",
                    "NODE_DELETED",
                    &author,
                    &format!("deleted {}", cid),
                    &payload,
                )?);
            }
            events.push(append_event(
                s,
                &project_id,
                "",
                "NODE_DELETED",
                &author,
                &format!("deleted {}", req.node_id),
                &before,
            )?);
            let last_seq = events.last().map(|e| e.seq).unwrap_or(0);
            record_idempotency(s, &project_id, idem_key(req.meta.as_ref()), last_seq)?;
            Ok(events)
        })?;
        finish_mutation(&mut *s, &project_id, events, dependents)
    }

    pub fn set_status_locked(&self, req: pb::SetStatusRequest) -> DResult<pb::Mutation> {
        let mut s = self.sql.session()?;
        let current = fetch_node(&mut *s, &req.node_id)?
            .ok_or_else(|| DomainError::from_db_message("node not found"))?;
        let project_id = current.project_id.clone();
        if let Some(seq) = check_idempotency(&mut *s, &project_id, idem_key(req.meta.as_ref()))? {
            return Ok(Self::empty_mutation(seq));
        }
        let before = declared_str(current.declared_status);
        let new = declared_str(req.declared_status);
        let dependents = query_strings(
            &mut *s,
            "SELECT blocked_id FROM dependencies WHERE blocker_id = ?1",
            values![&req.node_id],
        )?;
        let author = author_of(req.meta.as_ref());
        let event = transaction(&mut *s, |s| {
            s.execute(
                "UPDATE nodes SET declared_status = ?1 WHERE id = ?2",
                values![new, &req.node_id],
            )?;
            let payload = serde_json::json!({ "before": before, "after": new }).to_string();
            let event = append_event(
                s,
                &project_id,
                &req.node_id,
                "STATUS_SET",
                &author,
                &format!("{} -> {}", req.node_id, new),
                &payload,
            )?;
            record_idempotency(s, &project_id, idem_key(req.meta.as_ref()), event.seq)?;
            Ok(event)
        })?;
        let mut affected = vec![req.node_id.clone()];
        affected.extend(dependents);
        finish_mutation(&mut *s, &project_id, vec![event], affected)
    }

    pub fn add_dependency_locked(&self, req: pb::AddDependencyRequest) -> DResult<pb::Mutation> {
        let mut s = self.sql.session()?;
        let project_id = project_of(&mut *s, &req.blocker_id)?;
        if let Some(seq) = check_idempotency(&mut *s, &project_id, idem_key(req.meta.as_ref()))? {
            return Ok(Self::empty_mutation(seq));
        }
        let author = author_of(req.meta.as_ref());
        let event = transaction(&mut *s, |s| {
            s.execute(
                "INSERT OR IGNORE INTO dependencies (blocker_id, blocked_id) VALUES (?1, ?2)",
                values![&req.blocker_id, &req.blocked_id],
            )?;
            let payload =
                serde_json::json!({ "blocker_id": &req.blocker_id, "blocked_id": &req.blocked_id })
                    .to_string();
            let event = append_event(
                s,
                &project_id,
                "",
                "DEP_ADDED",
                &author,
                &format!("{} blocks {}", req.blocker_id, req.blocked_id),
                &payload,
            )?;
            record_idempotency(s, &project_id, idem_key(req.meta.as_ref()), event.seq)?;
            Ok(event)
        })?;
        finish_mutation(
            &mut *s,
            &project_id,
            vec![event],
            vec![req.blocked_id.clone()],
        )
    }

    pub fn remove_dependency_locked(
        &self,
        req: pb::RemoveDependencyRequest,
    ) -> DResult<pb::Mutation> {
        let mut s = self.sql.session()?;
        let project_id = project_of(&mut *s, &req.blocker_id)?;
        if let Some(seq) = check_idempotency(&mut *s, &project_id, idem_key(req.meta.as_ref()))? {
            return Ok(Self::empty_mutation(seq));
        }
        let author = author_of(req.meta.as_ref());
        let event = transaction(&mut *s, |s| {
            s.execute(
                "DELETE FROM dependencies WHERE blocker_id = ?1 AND blocked_id = ?2",
                values![&req.blocker_id, &req.blocked_id],
            )?;
            let payload =
                serde_json::json!({ "blocker_id": &req.blocker_id, "blocked_id": &req.blocked_id })
                    .to_string();
            let event = append_event(
                s,
                &project_id,
                "",
                "DEP_REMOVED",
                &author,
                &format!("{} no longer blocks {}", req.blocker_id, req.blocked_id),
                &payload,
            )?;
            record_idempotency(s, &project_id, idem_key(req.meta.as_ref()), event.seq)?;
            Ok(event)
        })?;
        finish_mutation(
            &mut *s,
            &project_id,
            vec![event],
            vec![req.blocked_id.clone()],
        )
    }

    pub fn report_condition_locked(
        &self,
        req: pb::ReportConditionRequest,
    ) -> DResult<pb::Mutation> {
        let mut s = self.sql.session()?;
        let current = fetch_node(&mut *s, &req.node_id)?
            .ok_or_else(|| DomainError::from_db_message("node not found"))?;
        let project_id = current.project_id.clone();
        if let Some(seq) = check_idempotency(&mut *s, &project_id, idem_key(req.meta.as_ref()))? {
            return Ok(Self::empty_mutation(seq));
        }
        let result = match req.result {
            1 => "PASS",
            2 => "FAIL",
            _ => return Err(DomainError::from_db_message("invalid agent result")),
        };
        let author = author_of(req.meta.as_ref());
        let event = transaction(&mut *s, |s| {
            s.execute(
                "INSERT INTO verifications (node_id, agent_result, agent_name, agent_at, agent_node_rev, agent_detail)
                 VALUES (?1, ?2, ?3, unixepoch(), (SELECT updated_at FROM nodes WHERE id = ?1), ?4)
                 ON CONFLICT(node_id) DO UPDATE SET
                   agent_result = excluded.agent_result,
                   agent_name   = excluded.agent_name,
                   agent_at     = excluded.agent_at,
                   agent_node_rev = excluded.agent_node_rev,
                   agent_detail = excluded.agent_detail",
                values![&req.node_id, result, &author, &req.detail],
            )?;
            let payload =
                serde_json::json!({ "result": result, "detail": &req.detail }).to_string();
            let event = append_event(
                s,
                &project_id,
                &req.node_id,
                "AGENT_REPORTED",
                &author,
                &format!("agent reported {} on {}", result, req.node_id),
                &payload,
            )?;
            record_idempotency(s, &project_id, idem_key(req.meta.as_ref()), event.seq)?;
            Ok(event)
        })?;
        finish_mutation(&mut *s, &project_id, vec![event], vec![req.node_id.clone()])
    }

    pub fn set_verdict_locked(&self, req: pb::SetVerdictRequest) -> DResult<pb::Mutation> {
        let mut s = self.sql.session()?;
        let current = fetch_node(&mut *s, &req.node_id)?
            .ok_or_else(|| DomainError::from_db_message("node not found"))?;
        let project_id = current.project_id.clone();
        if let Some(seq) = check_idempotency(&mut *s, &project_id, idem_key(req.meta.as_ref()))? {
            return Ok(Self::empty_mutation(seq));
        }
        let author = author_of(req.meta.as_ref());
        let event = transaction(&mut *s, |s| {
            match req.verdict {
                1 => {
                    s.execute(
                        "INSERT INTO verifications (node_id, human_verdict, human_at)
                         VALUES (?1, 'ACCEPTED', unixepoch())
                         ON CONFLICT(node_id) DO UPDATE SET human_verdict = 'ACCEPTED', human_at = unixepoch()",
                        values![&req.node_id],
                    )?;
                }
                2 => {
                    s.execute(
                        "INSERT INTO verifications (node_id, human_verdict, human_at)
                         VALUES (?1, 'REJECTED', unixepoch())
                         ON CONFLICT(node_id) DO UPDATE SET human_verdict = 'REJECTED', human_at = unixepoch()",
                        values![&req.node_id],
                    )?;
                }
                _ => {
                    s.execute("UPDATE verifications SET human_verdict = NULL, human_at = NULL WHERE node_id = ?1", values![&req.node_id])?;
                }
            }
            let event = append_event(
                s,
                &project_id,
                &req.node_id,
                "VERDICT_SET",
                &author,
                &format!("verdict set on {}", req.node_id),
                "{}",
            )?;
            record_idempotency(s, &project_id, idem_key(req.meta.as_ref()), event.seq)?;
            Ok(event)
        })?;
        finish_mutation(&mut *s, &project_id, vec![event], vec![req.node_id.clone()])
    }

    pub fn add_comment_locked(&self, req: pb::AddCommentRequest) -> DResult<pb::Mutation> {
        let mut s = self.sql.session()?;
        let current = fetch_node(&mut *s, &req.node_id)?
            .ok_or_else(|| DomainError::from_db_message("node not found"))?;
        let project_id = current.project_id.clone();
        if let Some(seq) = check_idempotency(&mut *s, &project_id, idem_key(req.meta.as_ref()))? {
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
        let event = transaction(&mut *s, |s| {
            let event = append_event(
                s,
                &project_id,
                &req.node_id,
                "COMMENT",
                &author,
                &summary,
                &payload,
            )?;
            record_idempotency(s, &project_id, idem_key(req.meta.as_ref()), event.seq)?;
            Ok(event)
        })?;
        finish_mutation(&mut *s, &project_id, vec![event], vec![req.node_id.clone()])
    }

    pub fn undo_locked(&self, req: pb::UndoRequest) -> DResult<pb::Mutation> {
        const EV: &str = "SELECT seq, project_id, COALESCE(node_id,'') AS node_id, kind, author, summary, payload AS payload_json, created_at FROM events WHERE project_id = ?1 ";
        let mut s = self.sql.session()?;
        let event = if req.seq > 0 {
            s.query_opt(
                &format!("{EV} AND seq = ?2"),
                values![&req.project_id, req.seq],
            )?
        } else {
            s.query_opt(
                &format!("{EV} ORDER BY seq DESC LIMIT 1"),
                values![&req.project_id],
            )?
        }
        .map(|r| proto_event(&r))
        .transpose()?
        .ok_or_else(|| DomainError::from_db_message("no event to undo"))?;
        let project_id = event.project_id.clone();
        let node_id = event.node_id.clone();
        let kind = event.kind;
        if let Some(seq) = check_idempotency(&mut *s, &project_id, idem_key(req.meta.as_ref()))? {
            return Ok(Self::empty_mutation(seq));
        }

        let author = author_of(req.meta.as_ref());
        let payload_json = event.payload_json.clone();
        let (inverse, affected) = transaction(&mut *s, |s| {
            // `inverse_kind` is the valid schema kind describing the reversal (the
            // events CHECK rejects a synthetic "UNDO" kind, so we record the reversed act).
            let mut inverse_node = node_id.clone();
            let (affected, inverse_kind, inverse_summary): (Vec<String>, &str, String) = match kind
            {
                k if k == pb::EventKind::StatusSet as i32 => {
                    let before =
                        payload_get(&payload_json, "before").unwrap_or_else(|| "OPEN".into());
                    s.execute(
                        "UPDATE nodes SET declared_status = ?1 WHERE id = ?2",
                        values![&before, &node_id],
                    )?;
                    (
                        downstream_of(s, &node_id)?,
                        "STATUS_SET",
                        format!("undid status set on {}", node_id),
                    )
                }
                k if k == pb::EventKind::DepAdded as i32 => {
                    let blocker = payload_get(&payload_json, "blocker_id").ok_or_else(|| {
                        DomainError::from_db_message("dep payload missing blocker")
                    })?;
                    let blocked = payload_get(&payload_json, "blocked_id").ok_or_else(|| {
                        DomainError::from_db_message("dep payload missing blocked")
                    })?;
                    s.execute(
                        "DELETE FROM dependencies WHERE blocker_id = ?1 AND blocked_id = ?2",
                        values![&blocker, &blocked],
                    )?;
                    (
                        vec![blocked.clone()],
                        "DEP_REMOVED",
                        format!("undid: {} no longer blocks {}", blocker, blocked),
                    )
                }
                k if k == pb::EventKind::DepRemoved as i32 => {
                    let blocker = payload_get(&payload_json, "blocker_id").ok_or_else(|| {
                        DomainError::from_db_message("dep payload missing blocker")
                    })?;
                    let blocked = payload_get(&payload_json, "blocked_id").ok_or_else(|| {
                        DomainError::from_db_message("dep payload missing blocked")
                    })?;
                    s.execute("INSERT OR IGNORE INTO dependencies (blocker_id, blocked_id) VALUES (?1, ?2)", values![&blocker, &blocked])?;
                    (
                        vec![blocked.clone()],
                        "DEP_ADDED",
                        format!("undid: {} blocks {}", blocker, blocked),
                    )
                }
                k if k == pb::EventKind::NodeCreated as i32 => {
                    inverse_node.clear();
                    s.execute("DELETE FROM nodes WHERE id = ?1", values![&node_id])?;
                    (
                        vec![],
                        "NODE_DELETED",
                        format!("undid creation of {}", node_id),
                    )
                }
                k if k == pb::EventKind::NodeDeleted as i32 => {
                    let before: serde_json::Value =
                        serde_json::from_str(&payload_json).map_err(|_| {
                            DomainError::from_db_message("cannot undo delete: payload missing")
                        })?;
                    let obj = before.get("before").cloned().ok_or_else(|| {
                        DomainError::from_db_message("cannot undo delete: payload missing")
                    })?;
                    restore_node(s, &obj)?;
                    (
                        downstream_of(s, &node_id)?,
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
            let inverse = append_event(
                s,
                &project_id,
                &inverse_node,
                inverse_kind,
                &author,
                &inverse_summary,
                "{}",
            )?;
            record_idempotency(s, &project_id, idem_key(req.meta.as_ref()), inverse.seq)?;
            Ok((inverse, affected))
        })?;
        finish_mutation(&mut *s, &project_id, vec![inverse], affected)
    }

    pub fn move_node_locked(&self, req: pb::MoveNodeRequest) -> DResult<pb::Mutation> {
        let mut s = self.sql.session()?;
        let current = fetch_node(&mut *s, &req.node_id)?
            .ok_or_else(|| DomainError::from_db_message("node not found"))?;
        let project_id = current.project_id.clone();
        if let Some(seq) = check_idempotency(&mut *s, &project_id, idem_key(req.meta.as_ref()))? {
            return Ok(Self::empty_mutation(seq));
        }
        let old_kind = current.kind;
        let new_kind = req.kind;
        let wp = pb::NodeKind::WorkPackage as i32;
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
        let parent_id_before = current.parent_id.clone();
        let (events, affected) = transaction(&mut *s, |s| {
            let mut events: Vec<pb::Event> = Vec::new();
            let mut affected = vec![req.node_id.clone()];

            // TASK -> STEP demote drops this node's step children (destructive).
            // Collect each child's dependents BEFORE deleting; delete events use node_id = ''.
            if old_kind == pb::NodeKind::Task as i32 && new_kind == pb::NodeKind::Step as i32 {
                let child_ids = query_strings(
                    s,
                    "SELECT id FROM nodes WHERE parent_id = ?1",
                    values![&req.node_id],
                )?;
                for cid in &child_ids {
                    let deps = query_strings(
                        s,
                        "SELECT blocked_id FROM dependencies WHERE blocker_id = ?1",
                        values![cid],
                    )?;
                    affected.extend(deps);
                    s.execute("DELETE FROM nodes WHERE id = ?1", values![cid])?;
                    let payload = serde_json::json!({ "before": { "id": cid } }).to_string();
                    events.push(append_event(
                        s,
                        &project_id,
                        "",
                        "NODE_DELETED",
                        &author,
                        &format!("deleted {}", cid),
                        &payload,
                    )?);
                }
            }

            // A kind change invalidates the verification.
            if old_kind != new_kind {
                s.execute(
                    "DELETE FROM verifications WHERE node_id = ?1",
                    values![&req.node_id],
                )?;
            }

            let new_pos = s
                .query_one(
                    "SELECT COALESCE(MAX(position), 0) + 100 FROM nodes WHERE parent_id = ?1",
                    values![&req.parent_id],
                )?
                .get_i64_at(0)?;
            s.execute(
                "UPDATE nodes SET parent_id = NULLIF(?1, ''), kind = ?2, position = ?3 WHERE id = ?4",
                values![&req.parent_id, kind_str(new_kind), new_pos as i32, &req.node_id],
            )?;

            let payload = serde_json::json!({
                "before": { "parent_id": &parent_id_before, "kind": kind_str(old_kind) },
                "after": { "parent_id": &req.parent_id, "kind": kind_str(new_kind) }
            })
            .to_string();
            events.push(append_event(
                s,
                &project_id,
                &req.node_id,
                "NODE_UPDATED",
                &author,
                &format!("moved {}", req.node_id),
                &payload,
            )?);

            let last_seq = events.last().map(|e| e.seq).unwrap_or(0);
            record_idempotency(s, &project_id, idem_key(req.meta.as_ref()), last_seq)?;
            Ok((events, affected))
        })?;
        finish_mutation(&mut *s, &project_id, events, affected)
    }

    pub fn create_project_locked(&self, req: pb::CreateProjectRequest) -> DResult<pb::Project> {
        let mut s = self.sql.session()?;
        // Sentinel-scoped idempotency ('' scope): projects log no event; a retry
        // dedups under '' and returns the created row.
        if let Some(hit) = check_idempotency_created(&mut *s, "", idem_key(req.meta.as_ref()))? {
            if let Some(pid) = &hit.entity_id {
                if let Some(p) = fetch_project(&mut *s, pid)? {
                    return Ok(p);
                }
            }
        }
        let id = transaction(&mut *s, |s| {
            let id = new_id(s, "prj")?;
            s.execute(
                "INSERT INTO projects (id, name, description) VALUES (?1, ?2, ?3)",
                values![&id, &req.name, &req.description],
            )?;
            record_idempotency_created(s, "", idem_key(req.meta.as_ref()), 0, &id)?;
            Ok(id)
        })?;
        fetch_project(&mut *s, &id)?
            .ok_or_else(|| DomainError::from_db_message("created project not found"))
    }

    pub fn update_project_locked(&self, req: pb::UpdateProjectRequest) -> DResult<pb::Project> {
        let mut s = self.sql.session()?;
        if fetch_project(&mut *s, &req.project_id)?.is_none() {
            return Err(DomainError::from_db_message("project not found"));
        }
        if req.update_mask.is_empty() {
            return Err(DomainError::from_db_message("update_mask cannot be empty"));
        }
        transaction(&mut *s, |s| {
            for f in &req.update_mask {
                match f.as_str() {
                    "name" => {
                        s.execute(
                            "UPDATE projects SET name = ?1 WHERE id = ?2",
                            values![&req.name, &req.project_id],
                        )?;
                    }
                    "description" => {
                        s.execute(
                            "UPDATE projects SET description = ?1 WHERE id = ?2",
                            values![&req.description, &req.project_id],
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
            s.execute(
                "UPDATE projects SET updated_at = unixepoch() WHERE id = ?1",
                values![&req.project_id],
            )?;
            Ok(())
        })?;
        fetch_project(&mut *s, &req.project_id)?
            .ok_or_else(|| DomainError::from_db_message("project not found"))
    }

    pub fn archive_project_locked(&self, req: pb::ArchiveProjectRequest) -> DResult<pb::Project> {
        let mut s = self.sql.session()?;
        if fetch_project(&mut *s, &req.project_id)?.is_none() {
            return Err(DomainError::from_db_message("project not found"));
        }
        transaction(&mut *s, |s| {
            if req.archived {
                s.execute("UPDATE projects SET archived_at = unixepoch(), updated_at = unixepoch() WHERE id = ?1", values![&req.project_id])?;
            } else {
                s.execute("UPDATE projects SET archived_at = NULL, updated_at = unixepoch() WHERE id = ?1", values![&req.project_id])?;
            }
            Ok(())
        })?;
        fetch_project(&mut *s, &req.project_id)?
            .ok_or_else(|| DomainError::from_db_message("project not found"))
    }
}

impl<S: Sql> SqliteStore<S> {
    /// Transport-agnostic entry point — "the service minus the socket". Decodes a
    /// protobuf request for `method`, runs the synchronous store op, and returns
    /// the encoded response. The tonic edge, the Node host, and the browser all
    /// funnel through this one call; the wasm `#[wasm_bindgen]` facade wraps it.
    /// Unary only — `Watch` stays a native-edge concern.
    pub fn dispatch(&self, method: &str, req: &[u8]) -> DResult<Vec<u8>> {
        use prost::Message;
        fn decode<T: Message + Default>(bytes: &[u8]) -> DResult<T> {
            T::decode(bytes).map_err(|e| DomainError::invalid_argument(format!("bad request: {e}")))
        }
        Ok(match method {
            "ListProjects" => {
                let r: pb::ListProjectsRequest = decode(req)?;
                pb::ListProjectsResponse {
                    projects: self.list_projects_locked(r.include_archived)?,
                }
                .encode_to_vec()
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
                pb::SearchResponse {
                    nodes: self.search_locked(&r.project_id, &r.query, r.limit)?,
                }
                .encode_to_vec()
            }
            "PollChanges" => {
                let r: pb::PollChangesRequest = decode(req)?;
                self.poll_changes_locked(&r.project_id, r.after_seq, r.limit)?
                    .encode_to_vec()
            }
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

// ── shared helpers (free fns over a &mut dyn Session) ────────────────────────

/// Build the shared mutation payload for a committed write. Watch fan-out is a
/// native-edge concern: `flowd` reads the returned `Mutation` and publishes it.
fn finish_mutation(
    s: &mut dyn Session,
    project_id: &str,
    events: Vec<pb::Event>,
    affected_ids: Vec<String>,
) -> DResult<pb::Mutation> {
    let seq = events.last().map(|e| e.seq).unwrap_or(0);
    let mut seen: HashSet<String> = HashSet::new();
    let mut changed_nodes = Vec::new();
    for id in affected_ids {
        if seen.insert(id.clone()) {
            if let Some(n) = fetch_node(&mut *s, &id)? {
                changed_nodes.push(n);
            }
        }
    }
    let changed_progress = progress_for(&mut *s, project_id)?;
    Ok(pb::Mutation {
        events,
        changed_nodes,
        changed_progress,
        seq,
    })
}

fn check_idempotency(
    s: &mut dyn Session,
    project_id: &str,
    key: Option<&str>,
) -> DResult<Option<i64>> {
    let Some(key) = key else { return Ok(None) };
    s.query_opt(
        "SELECT seq FROM idempotency WHERE project_id = ?1 AND idempotency_key = ?2",
        values![project_id, key],
    )?
    .map(|r| r.get_i64("seq"))
    .transpose()
}

fn record_idempotency(
    s: &mut dyn Session,
    project_id: &str,
    key: Option<&str>,
    seq: i64,
) -> DResult<()> {
    let Some(key) = key else { return Ok(()) };
    s.execute(
        "INSERT OR IGNORE INTO idempotency (project_id, idempotency_key, seq) VALUES (?1, ?2, ?3)",
        values![project_id, key, seq],
    )?;
    Ok(())
}

fn downstream_of(s: &mut dyn Session, node_id: &str) -> DResult<Vec<String>> {
    query_strings(
        s,
        "SELECT blocked_id FROM dependencies WHERE blocker_id = ?1",
        values![node_id],
    )
}

/// Progress per work package, counting leaves (steps where they exist, else the
/// task itself) with the effective status of each leaf.
fn progress_for(s: &mut dyn Session, project_id: &str) -> DResult<Vec<pb::Progress>> {
    s.query(
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
        values![project_id],
    )?
    .iter()
    .map(|r| {
        Ok(pb::Progress {
            work_package_id: r.get_str("wp_id")?,
            total: r.get_i32("total")?,
            done: r.get_i32("done")?,
            ready: r.get_i32("ready")?,
            blocked: r.get_i32("blocked")?,
            deferred: r.get_i32("deferred")?,
        })
    })
    .collect()
}

fn verification_for(s: &mut dyn Session, node_id: &str) -> DResult<Option<pb::Verification>> {
    s.query_opt(
        "SELECT v.agent_result, COALESCE(v.agent_name,'') AS agent_name,
                COALESCE(v.agent_at,0) AS agent_at, v.agent_detail,
                v.human_verdict, COALESCE(v.human_at,0) AS human_at,
                (v.agent_node_rev IS NOT NULL AND n.updated_at > v.agent_node_rev) AS stale
         FROM verifications v JOIN nodes n ON n.id = v.node_id
         WHERE v.node_id = ?1",
        values![node_id],
    )?
    .map(|r| row_to_verification(&r))
    .transpose()
}

/// Fetch one node by id as a proto Node (None if missing).
fn fetch_node(s: &mut dyn Session, id: &str) -> DResult<Option<pb::Node>> {
    let mut node = s
        .query_opt(
            "SELECT n.id, n.project_id, COALESCE(n.parent_id,'') AS parent_id,
                    n.kind, n.title, n.description, n.condition, n.note, COALESCE(n.reference,'') AS reference,
                    n.declared_status, n.wp_state, n.position, n.created_at, n.updated_at,
                    n.status AS effective_status
             FROM node_state n WHERE n.id = ?1",
            values![id],
        )?
        .map(|r| row_to_node(&r))
        .transpose()?;
    if let Some(n) = &mut node {
        n.verification = verification_for(&mut *s, &n.id)?;
    }
    Ok(node)
}

/// Owning project of a node ('' if the node is unknown).
fn project_of(s: &mut dyn Session, node_id: &str) -> DResult<String> {
    Ok(s.query_opt(
        "SELECT project_id FROM nodes WHERE id = ?1",
        values![node_id],
    )?
    .map(|r| r.get_str("project_id"))
    .transpose()?
    .unwrap_or_default())
}

/// Fetch one project as a proto Project (None if missing).
fn fetch_project(s: &mut dyn Session, id: &str) -> DResult<Option<pb::Project>> {
    s.query_opt(
        "SELECT id, name, description, COALESCE(archived_at,0) AS archived_at, created_at
         FROM projects WHERE id = ?1",
        values![id],
    )?
    .map(|r| row_to_project(&r))
    .transpose()
}

/// Idempotency lookup that also returns the id of the entity the original request
/// created (for create replays).
fn check_idempotency_created(
    s: &mut dyn Session,
    scope: &str,
    key: Option<&str>,
) -> DResult<Option<IdemHit>> {
    let Some(key) = key else { return Ok(None) };
    s.query_opt(
        "SELECT seq, entity_id FROM idempotency WHERE project_id = ?1 AND idempotency_key = ?2",
        values![scope, key],
    )?
    .map(|r| {
        Ok(IdemHit {
            seq: r.get_i64("seq")?,
            entity_id: r.get_opt_str("entity_id")?,
        })
    })
    .transpose()
}

/// Record an idempotency row that remembers the created entity's id.
fn record_idempotency_created(
    s: &mut dyn Session,
    scope: &str,
    key: Option<&str>,
    seq: i64,
    entity_id: &str,
) -> DResult<()> {
    let Some(key) = key else { return Ok(()) };
    s.execute(
        "INSERT OR IGNORE INTO idempotency (project_id, idempotency_key, seq, entity_id) VALUES (?1, ?2, ?3, ?4)",
        values![scope, key, seq, entity_id],
    )?;
    Ok(())
}

/// Run a query whose first column is text and collect it into a Vec.
fn query_strings(s: &mut dyn Session, sql: &str, params: &[Value]) -> DResult<Vec<String>> {
    s.query(sql, params)?
        .iter()
        .map(|r| r.get_str_at(0))
        .collect()
}

/// Mint a short, sortable, unique id (`<prefix>-<unix-millis>-<8 hex>`) via SQL —
/// no host-clock/entropy API needed (wasm-safe).
fn new_id(s: &mut dyn Session, prefix: &str) -> DResult<String> {
    s.query_one(
        "SELECT ?1 || '-' || CAST(unixepoch('subsec') * 1000 AS INTEGER) || '-' || lower(hex(randomblob(4)))",
        values![prefix],
    )?
    .get_str_at(0)
}

/// Insert an event row (via `RETURNING`); returns the materialised `pb::Event`.
fn append_event(
    s: &mut dyn Session,
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
    let row = s.query_one(
        "INSERT INTO events (project_id, node_id, kind, author, summary, payload)
         VALUES (?1, ?2, ?3, ?4, ?5, ?6)
         RETURNING seq, project_id, COALESCE(node_id,'') AS node_id, kind, author, summary, payload AS payload_json, created_at",
        values![project_id, node, kind, author, summary, payload],
    )?;
    proto_event(&row)
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
fn restore_node(s: &mut dyn Session, obj: &serde_json::Value) -> DResult<()> {
    let wp = if str_val(obj, "kind") == "WORK_PACKAGE" {
        wp_state_str(obj.get("wp_state").and_then(|v| v.as_str()))
    } else {
        None
    };
    s.execute(
        "INSERT OR IGNORE INTO nodes (id, project_id, parent_id, kind, title, description, condition, note, reference, declared_status, wp_state, position)
         VALUES (?1, ?2, NULLIF(?3, ''), ?4, ?5, ?6, ?7, ?8, NULLIF(?9, ''), ?10, ?11, ?12)",
        values![
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
            obj.get("position").and_then(|v| v.as_i64()).unwrap_or(0) as i32
        ],
    )?;
    Ok(())
}

fn str_val<'a>(obj: &'a serde_json::Value, key: &str) -> &'a str {
    obj.get(key).and_then(|v| v.as_str()).unwrap_or("")
}

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
fn row_to_project(r: &Row) -> DResult<pb::Project> {
    Ok(pb::Project {
        id: r.get_str("id")?,
        name: r.get_str("name")?,
        description: r.get_str("description")?,
        archived_at: r.get_i64("archived_at")?,
        created_at: r.get_i64("created_at")?,
    })
}

/// Convert a node_state row into a proto Node.
fn row_to_node(r: &Row) -> DResult<pb::Node> {
    let kind = match r.get_str("kind")?.as_str() {
        "WORK_PACKAGE" => pb::NodeKind::WorkPackage,
        "TASK" => pb::NodeKind::Task,
        _ => pb::NodeKind::Step,
    };
    let declared = match r.get_str("declared_status")?.as_str() {
        "DEFERRED" => pb::DeclaredStatus::Deferred,
        "DONE" => pb::DeclaredStatus::Done,
        _ => pb::DeclaredStatus::Open,
    };
    let wp_state = match r.get_opt_str("wp_state")?.as_deref() {
        Some("ACTIVE") => pb::WorkPackageState::Active,
        Some("DONE") => pb::WorkPackageState::Done,
        Some("ARCHIVED") => pb::WorkPackageState::Archived,
        _ => pb::WorkPackageState::Planned,
    };
    Ok(pb::Node {
        id: r.get_str("id")?,
        project_id: r.get_str("project_id")?,
        parent_id: r.get_str("parent_id")?,
        kind: kind as i32,
        title: r.get_str("title")?,
        description: r.get_str("description")?,
        condition: r.get_str("condition")?,
        note: r.get_str("note")?,
        reference: r.get_str("reference")?,
        declared_status: declared as i32,
        status: effective_status(&r.get_str("effective_status")?),
        wp_state: wp_state as i32,
        position: r.get_i32("position")?,
        verification: None,
        created_at: r.get_i64("created_at")?,
        updated_at: r.get_i64("updated_at")?,
    })
}

/// Convert a verification row into a proto Verification.
fn row_to_verification(r: &Row) -> DResult<pb::Verification> {
    let agent_result = match r.get_opt_str("agent_result")?.as_deref() {
        Some("PASS") => pb::AgentResult::Pass,
        Some("FAIL") => pb::AgentResult::Fail,
        _ => pb::AgentResult::Unspecified,
    };
    let human_verdict = match r.get_opt_str("human_verdict")?.as_deref() {
        Some("ACCEPTED") => pb::HumanVerdict::Accepted,
        Some("REJECTED") => pb::HumanVerdict::Rejected,
        _ => pb::HumanVerdict::Unspecified,
    };
    Ok(pb::Verification {
        agent_result: agent_result as i32,
        agent_name: r.get_str("agent_name")?,
        agent_at: r.get_i64("agent_at")?,
        agent_detail: r.get_opt_str("agent_detail")?.unwrap_or_default(),
        human_verdict: human_verdict as i32,
        human_at: r.get_i64("human_at")?,
        stale: r.get_i64("stale")? != 0,
    })
}

/// Map an events row to a proto `Event`.
fn proto_event(r: &Row) -> DResult<pb::Event> {
    let kind = match r.get_str("kind")?.as_str() {
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
        seq: r.get_i64("seq")?,
        project_id: r.get_str("project_id")?,
        node_id: r.get_str("node_id")?,
        kind: kind as i32,
        author: r.get_str("author")?,
        summary: r.get_str("summary")?,
        payload_json: r.get_str("payload_json")?,
        created_at: r.get_i64("created_at")?,
    })
}
