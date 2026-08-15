//! Integration tests for the SQLite store against the real migration + seed.
//! Each test uses its own `:memory:` database so they are independent and fast.

use flowd::db;
use flowd::generated::flow_v1 as pb;
use flowd::store::{SqliteStore, Store};

async fn seeded_store() -> SqliteStore {
    let conn = db::open(":memory:").expect("db open");
    db::seed(&conn).expect("seed");
    SqliteStore::new(conn)
}

#[tokio::test]
async fn list_projects_excludes_archived_by_default() {
    let store = seeded_store().await;
    let projects = store.list_projects(false).await.expect("list");
    assert_eq!(projects.len(), 1);
    assert_eq!(projects[0].id, "prj-travel");
    assert_eq!(projects[0].name, "Travel Webapp");
}

#[tokio::test]
async fn list_projects_includes_archived_when_requested() {
    let store = seeded_store().await;
    let projects = store.list_projects(true).await.expect("list");
    assert_eq!(projects.len(), 2);
}

#[tokio::test]
async fn get_snapshot_returns_nodes_dependencies_and_progress() {
    let store = seeded_store().await;
    let snap = store.get_snapshot("prj-travel").await.expect("snapshot");
    assert_eq!(
        snap.project.as_ref().map(|p| p.id.as_str()),
        Some("prj-travel")
    );
    // 1 work package + 2 tasks.
    assert_eq!(snap.nodes.len(), 3);
    // One dependency edge.
    assert_eq!(snap.dependencies.len(), 1);
    // One work package has progress.
    assert_eq!(snap.progress.len(), 1);
    // Seeded event present.
    assert_eq!(snap.recent_events.len(), 1);
    assert!(snap.seq >= 1);
}

#[tokio::test]
async fn get_snapshot_maps_effective_status() {
    let store = seeded_store().await;
    let snap = store.get_snapshot("prj-travel").await.expect("snapshot");
    // T-1043 is declared DONE -> effective DONE.
    let done = snap
        .nodes
        .iter()
        .find(|n| n.id == "T-1043")
        .expect("T-1043");
    assert_eq!(done.status, pb::EffectiveStatus::Done as i32);
    // T-1042 is OPEN but blocked by nothing done -> READY (no blockers not done).
    let open = snap
        .nodes
        .iter()
        .find(|n| n.id == "T-1042")
        .expect("T-1042");
    assert_eq!(open.status, pb::EffectiveStatus::Ready as i32);
}

#[tokio::test]
async fn list_events_returns_seeded_event() {
    let store = seeded_store().await;
    let (events, has_more) = store
        .list_events("prj-travel", "", 0, 10)
        .await
        .expect("events");
    assert_eq!(events.len(), 1);
    assert!(!has_more);
    assert_eq!(events[0].node_id, "T-1042");
    assert_eq!(events[0].kind, pb::EventKind::NodeCreated as i32);
}

#[tokio::test]
async fn list_events_filters_by_node_and_paginates() {
    let store = seeded_store().await;
    for _ in 0..3 {
        store
            .add_comment(pb::AddCommentRequest {
                meta: meta("t"),
                node_id: "T-1042".into(),
                text: "c".into(),
            })
            .await
            .expect("comment");
    }
    store
        .add_comment(pb::AddCommentRequest {
            meta: meta("t"),
            node_id: "T-1043".into(),
            text: "other".into(),
        })
        .await
        .expect("comment");

    // Node filter: only T-1042's events (3 comments + the seeded NODE_CREATED).
    let (filtered, _) = store
        .list_events("prj-travel", "T-1042", 0, 100)
        .await
        .expect("filtered");
    assert!(filtered.iter().all(|e| e.node_id == "T-1042"));
    assert_eq!(filtered.len(), 4);

    // Pagination: newest-first, limit 2 → has_more; before_seq walks backwards.
    let (page1, more1) = store.list_events("prj-travel", "", 0, 2).await.expect("p1");
    assert_eq!(page1.len(), 2);
    assert!(more1);
    let oldest = page1.last().unwrap().seq;
    let (page2, _) = store
        .list_events("prj-travel", "", oldest, 2)
        .await
        .expect("p2");
    assert!(page2.iter().all(|e| e.seq < oldest));
}

#[tokio::test]
async fn search_finds_matching_nodes() {
    let store = seeded_store().await;
    let nodes = store
        .search("prj-travel", "OAuth2", 10)
        .await
        .expect("search");
    assert_eq!(nodes.len(), 1);
    assert_eq!(nodes[0].id, "T-1042");
}

#[tokio::test]
async fn get_snapshot_unknown_project_is_empty() {
    let store = seeded_store().await;
    let snap = store.get_snapshot("nope").await.expect("snapshot");
    assert!(snap.project.is_none());
    assert!(snap.nodes.is_empty());
}

// ── writes ────────────────────────────────────────────────────────────────

#[tokio::test]
async fn create_node_inserts_and_logs_event() {
    let store = seeded_store().await;
    let m = store
        .create_node(pb::CreateNodeRequest {
            meta: Some(pb::WriteMeta {
                author: "tester".into(),
                idempotency_key: String::new(),
            }),
            project_id: "prj-travel".into(),
            parent_id: "WP-AUTH".into(),
            kind: pb::NodeKind::Task as i32,
            title: "New task".into(),
            description: "desc".into(),
            condition: String::new(),
            position: 300,
            reference: String::new(),
            note: String::new(),
        })
        .await
        .expect("create");
    assert_eq!(m.changed_nodes.len(), 1);
    let id = m.changed_nodes[0].id.clone();
    assert!(id.starts_with("node-"));
    // The new node is visible in a snapshot.
    let snap = store.get_snapshot("prj-travel").await.expect("snap");
    assert_eq!(snap.nodes.len(), 4);
    // Events were logged.
    let (ev, _) = store
        .list_events("prj-travel", "", 0, 10)
        .await
        .expect("events");
    assert!(ev
        .iter()
        .any(|e| e.node_id == id && e.kind == pb::EventKind::NodeCreated as i32));
}

#[tokio::test]
async fn set_status_updates_and_logs_event() {
    let store = seeded_store().await;
    let m = store
        .set_status(pb::SetStatusRequest {
            meta: Some(pb::WriteMeta {
                author: "tester".into(),
                idempotency_key: String::new(),
            }),
            node_id: "T-1042".into(),
            declared_status: pb::DeclaredStatus::Done as i32,
        })
        .await
        .expect("set_status");
    assert_eq!(m.changed_nodes[0].id, "T-1042");
    // Effective status becomes DONE.
    assert_eq!(m.changed_nodes[0].status, pb::EffectiveStatus::Done as i32);
    let (ev, _) = store
        .list_events("prj-travel", "", 0, 10)
        .await
        .expect("events");
    let status_events: Vec<_> = ev
        .iter()
        .filter(|e| e.kind == pb::EventKind::StatusSet as i32)
        .collect();
    assert_eq!(status_events.len(), 1);
}

#[tokio::test]
async fn update_node_respects_mask() {
    let store = seeded_store().await;
    let m = store
        .update_node(pb::UpdateNodeRequest {
            meta: Some(pb::WriteMeta {
                author: "tester".into(),
                idempotency_key: String::new(),
            }),
            node_id: "T-1042".into(),
            update_mask: vec!["title".into(), "reference".into()],
            title: "Renamed".into(),
            reference: "JIRA-9".into(),
            ..Default::default()
        })
        .await
        .expect("update");
    assert_eq!(m.changed_nodes[0].title, "Renamed");
    assert_eq!(m.changed_nodes[0].reference, "JIRA-9");
}

#[tokio::test]
async fn add_and_remove_dependency() {
    let store = seeded_store().await;
    let created = store
        .create_node(pb::CreateNodeRequest {
            meta: Some(pb::WriteMeta {
                author: "tester".into(),
                idempotency_key: String::new(),
            }),
            project_id: "prj-travel".into(),
            parent_id: "WP-AUTH".into(),
            kind: pb::NodeKind::Task as i32,
            title: "leaf".into(),
            ..Default::default()
        })
        .await
        .expect("create");
    let leaf = created.changed_nodes[0].id.clone();
    let add = store
        .add_dependency(pb::AddDependencyRequest {
            meta: Some(pb::WriteMeta {
                author: "tester".into(),
                idempotency_key: String::new(),
            }),
            blocker_id: "T-1043".into(),
            blocked_id: leaf.clone(),
        })
        .await
        .expect("add_dep");
    assert!(add.seq > 0);
    let snap = store.get_snapshot("prj-travel").await.expect("snap");
    assert_eq!(snap.dependencies.len(), 2);

    store
        .remove_dependency(pb::RemoveDependencyRequest {
            meta: Some(pb::WriteMeta {
                author: "tester".into(),
                idempotency_key: String::new(),
            }),
            blocker_id: "T-1043".into(),
            blocked_id: leaf.clone(),
        })
        .await
        .expect("remove_dep");
    let snap2 = store.get_snapshot("prj-travel").await.expect("snap");
    assert_eq!(snap2.dependencies.len(), 1);
}

#[tokio::test]
async fn delete_node_with_fail_if_referenced_rejects() {
    let store = seeded_store().await;
    // T-1043 is referenced as a blocker of nothing but is itself a node; T-1042 blocks T-1043,
    // so T-1043 is blocked_by T-1042. Deleting T-1042 fails because it blocks T-1043.
    let res = store
        .delete_node(pb::DeleteNodeRequest {
            meta: Some(pb::WriteMeta {
                author: "tester".into(),
                idempotency_key: String::new(),
            }),
            node_id: "T-1042".into(),
            fail_if_referenced: true,
        })
        .await;
    assert!(res.is_err());
}

// ── M3: Watch / idempotency / verification / undo ───────────────────────────

fn meta(author: &str) -> Option<pb::WriteMeta> {
    Some(pb::WriteMeta {
        author: author.into(),
        idempotency_key: String::new(),
    })
}
fn meta_idem(author: &str, key: &str) -> Option<pb::WriteMeta> {
    Some(pb::WriteMeta {
        author: author.into(),
        idempotency_key: key.into(),
    })
}

#[tokio::test]
async fn events_after_returns_post_cursor_events_in_order() {
    let store = seeded_store().await;
    let seeded_seq = store.get_snapshot("prj-travel").await.expect("snap").seq;
    let m = store
        .set_status(pb::SetStatusRequest {
            meta: meta("tester"),
            node_id: "T-1042".into(),
            declared_status: pb::DeclaredStatus::Done as i32,
        })
        .await
        .expect("set_status");
    let after = store
        .events_after("prj-travel", seeded_seq)
        .await
        .expect("events_after");
    assert_eq!(after.len(), 1);
    assert_eq!(after[0].seq, m.seq);
    assert_eq!(after[0].kind, pb::EventKind::StatusSet as i32);
}

#[tokio::test]
async fn mutation_returns_the_events_it_produced() {
    let store = seeded_store().await;
    let m = store
        .set_status(pb::SetStatusRequest {
            meta: meta("tester"),
            node_id: "T-1042".into(),
            declared_status: pb::DeclaredStatus::Deferred as i32,
        })
        .await
        .expect("set_status");
    assert_eq!(m.events.len(), 1);
    assert_eq!(m.events[0].kind, pb::EventKind::StatusSet as i32);
    assert_eq!(m.events[0].node_id, "T-1042");
    // Changed node carries the new effective status.
    assert!(m
        .changed_nodes
        .iter()
        .any(|n| n.id == "T-1042" && n.status == pb::EffectiveStatus::Deferred as i32));
}

#[tokio::test]
async fn idempotent_retry_writes_one_event() {
    let store = seeded_store().await;
    let first = store
        .set_status(pb::SetStatusRequest {
            meta: meta_idem("agent", "op-1"),
            node_id: "T-1042".into(),
            declared_status: pb::DeclaredStatus::Done as i32,
        })
        .await
        .expect("first");
    let retry = store
        .set_status(pb::SetStatusRequest {
            meta: meta_idem("agent", "op-1"),
            node_id: "T-1042".into(),
            declared_status: pb::DeclaredStatus::Done as i32,
        })
        .await
        .expect("retry");
    assert_eq!(retry.seq, first.seq);
    assert!(retry.events.is_empty());
    let (events, _) = store
        .list_events("prj-travel", "", 0, 10)
        .await
        .expect("events");
    let status_events: Vec<_> = events
        .iter()
        .filter(|e| e.kind == pb::EventKind::StatusSet as i32)
        .collect();
    assert_eq!(status_events.len(), 1);
}

#[tokio::test]
async fn watch_receives_live_notifications() {
    let store = seeded_store().await;
    let mut rx = store.subscribe();
    let m = store
        .set_status(pb::SetStatusRequest {
            meta: meta("tester"),
            node_id: "T-1042".into(),
            declared_status: pb::DeclaredStatus::Done as i32,
        })
        .await
        .expect("set_status");
    let n = rx.recv().await.expect("notified");
    assert_eq!(n.project_id, "prj-travel");
    assert_eq!(n.seq, m.seq);
    assert!(!n.events.is_empty());
    // The cascade's changed node is present.
    assert!(n.changed_nodes.iter().any(|c| c.id == "T-1042"));
}

#[tokio::test]
async fn report_condition_and_verdict_round_trip() {
    let store = seeded_store().await;
    store
        .report_condition(pb::ReportConditionRequest {
            meta: meta("agent-1"),
            node_id: "T-1042".into(),
            result: pb::AgentResult::Fail as i32,
            detail: "assertion blew up".into(),
        })
        .await
        .expect("report");
    store
        .set_verdict(pb::SetVerdictRequest {
            meta: meta("operator"),
            node_id: "T-1042".into(),
            verdict: pb::HumanVerdict::Accepted as i32,
        })
        .await
        .expect("verdict");

    let snap = store.get_snapshot("prj-travel").await.expect("snap");
    let node = snap
        .nodes
        .iter()
        .find(|n| n.id == "T-1042")
        .expect("T-1042");
    let v = node.verification.as_ref().expect("verification present");
    assert_eq!(v.agent_result, pb::AgentResult::Fail as i32);
    assert_eq!(v.agent_name, "agent-1");
    assert_eq!(v.human_verdict, pb::HumanVerdict::Accepted as i32);
}

#[tokio::test]
async fn add_comment_logs_comment_event() {
    let store = seeded_store().await;
    let m = store
        .add_comment(pb::AddCommentRequest {
            meta: meta("operator"),
            node_id: "T-1042".into(),
            text: "please look at the auth flow".into(),
        })
        .await
        .expect("comment");
    assert_eq!(m.events.len(), 1);
    assert_eq!(m.events[0].kind, pb::EventKind::Comment as i32);
    let (events, _) = store
        .list_events("prj-travel", "", 0, 10)
        .await
        .expect("events");
    assert!(events
        .iter()
        .any(|e| e.kind == pb::EventKind::Comment as i32));
}

#[tokio::test]
async fn undo_reverses_a_status_set() {
    let store = seeded_store().await;
    let set = store
        .set_status(pb::SetStatusRequest {
            meta: meta("tester"),
            node_id: "T-1042".into(),
            declared_status: pb::DeclaredStatus::Done as i32,
        })
        .await
        .expect("set_status");
    store
        .undo(pb::UndoRequest {
            meta: meta("tester"),
            project_id: "prj-travel".into(),
            seq: set.seq,
        })
        .await
        .expect("undo");
    let snap = store.get_snapshot("prj-travel").await.expect("snap");
    let node = snap
        .nodes
        .iter()
        .find(|n| n.id == "T-1042")
        .expect("T-1042");
    assert_eq!(node.declared_status, pb::DeclaredStatus::Open as i32);
}

#[tokio::test]
async fn undo_reverses_a_dep_added() {
    let store = seeded_store().await;
    let created = store
        .create_node(pb::CreateNodeRequest {
            meta: meta("tester"),
            project_id: "prj-travel".into(),
            parent_id: "WP-AUTH".into(),
            kind: pb::NodeKind::Task as i32,
            title: "leaf".into(),
            ..Default::default()
        })
        .await
        .expect("create");
    let leaf = created.changed_nodes[0].id.clone();
    let add = store
        .add_dependency(pb::AddDependencyRequest {
            meta: meta("tester"),
            blocker_id: "T-1042".into(),
            blocked_id: leaf.clone(),
        })
        .await
        .expect("add_dep");
    let before = store
        .get_snapshot("prj-travel")
        .await
        .expect("snap")
        .dependencies
        .len();
    store
        .undo(pb::UndoRequest {
            meta: meta("tester"),
            project_id: "prj-travel".into(),
            seq: add.seq,
        })
        .await
        .expect("undo");
    let after = store
        .get_snapshot("prj-travel")
        .await
        .expect("snap")
        .dependencies
        .len();
    assert_eq!(after, before - 1);
}

#[tokio::test]
async fn undo_reverses_a_node_create() {
    let store = seeded_store().await;
    let m = store
        .create_node(pb::CreateNodeRequest {
            meta: meta("tester"),
            project_id: "prj-travel".into(),
            parent_id: "WP-AUTH".into(),
            kind: pb::NodeKind::Task as i32,
            title: "doomed".into(),
            ..Default::default()
        })
        .await
        .expect("create");
    let id = m.changed_nodes[0].id.clone();
    store
        .undo(pb::UndoRequest {
            meta: meta("tester"),
            project_id: "prj-travel".into(),
            seq: m.seq,
        })
        .await
        .expect("undo");
    let snap = store.get_snapshot("prj-travel").await.expect("snap");
    assert!(!snap.nodes.iter().any(|n| n.id == id));
}

#[tokio::test]
async fn undo_reverses_a_node_delete() {
    let store = seeded_store().await;
    let m = store
        .create_node(pb::CreateNodeRequest {
            meta: meta("tester"),
            project_id: "prj-travel".into(),
            parent_id: "WP-AUTH".into(),
            kind: pb::NodeKind::Task as i32,
            title: "restorable".into(),
            ..Default::default()
        })
        .await
        .expect("create");
    let id = m.changed_nodes[0].id.clone();
    let del = store
        .delete_node(pb::DeleteNodeRequest {
            meta: meta("tester"),
            node_id: id.clone(),
            fail_if_referenced: false,
        })
        .await
        .expect("delete");
    assert!(!store
        .get_snapshot("prj-travel")
        .await
        .expect("snap")
        .nodes
        .iter()
        .any(|n| n.id == id));
    store
        .undo(pb::UndoRequest {
            meta: meta("tester"),
            project_id: "prj-travel".into(),
            seq: del.seq,
        })
        .await
        .expect("undo");
    let snap = store.get_snapshot("prj-travel").await.expect("snap");
    assert!(snap
        .nodes
        .iter()
        .any(|n| n.id == id && n.title == "restorable"));
}

#[tokio::test]
async fn search_returns_effective_status() {
    let store = seeded_store().await;
    let nodes = store
        .search("prj-travel", "OAuth2", 10)
        .await
        .expect("search");
    assert_eq!(nodes.len(), 1);
    // T-1042 is OPEN with no blockers done -> READY.
    assert_eq!(nodes[0].status, pb::EffectiveStatus::Ready as i32);
}

// ── M2: note · project lifecycle · MoveNode · idempotent replay ─────────────

/// Create a node and return its id.
async fn add(
    store: &SqliteStore,
    project: &str,
    parent: &str,
    kind: pb::NodeKind,
    title: &str,
) -> String {
    store
        .create_node(pb::CreateNodeRequest {
            meta: meta("t"),
            project_id: project.into(),
            parent_id: parent.into(),
            kind: kind as i32,
            title: title.into(),
            ..Default::default()
        })
        .await
        .expect("create")
        .changed_nodes[0]
        .id
        .clone()
}

#[tokio::test]
async fn create_step_with_note_round_trips() {
    let store = seeded_store().await;
    let m = store
        .create_node(pb::CreateNodeRequest {
            meta: meta("t"),
            project_id: "prj-travel".into(),
            parent_id: "T-1042".into(),
            kind: pb::NodeKind::Step as i32,
            title: "step one".into(),
            note: "a few sentences of detail".into(),
            ..Default::default()
        })
        .await
        .expect("create");
    let id = m.changed_nodes[0].id.clone();
    assert_eq!(m.changed_nodes[0].note, "a few sentences of detail");
    let snap = store.get_snapshot("prj-travel").await.expect("snap");
    let s = snap.nodes.iter().find(|n| n.id == id).expect("step");
    assert_eq!(s.note, "a few sentences of detail");
}

#[tokio::test]
async fn update_node_note_via_mask() {
    let store = seeded_store().await;
    let id = add(&store, "prj-travel", "T-1042", pb::NodeKind::Step, "s").await;
    let m = store
        .update_node(pb::UpdateNodeRequest {
            meta: meta("t"),
            node_id: id,
            update_mask: vec!["note".into()],
            note: "edited body".into(),
            ..Default::default()
        })
        .await
        .expect("update");
    assert_eq!(m.changed_nodes[0].note, "edited body");
}

#[tokio::test]
async fn search_matches_step_note() {
    let store = seeded_store().await;
    store
        .create_node(pb::CreateNodeRequest {
            meta: meta("t"),
            project_id: "prj-travel".into(),
            parent_id: "T-1042".into(),
            kind: pb::NodeKind::Step as i32,
            title: "plain title".into(),
            note: "zzqltoken in the body".into(),
            ..Default::default()
        })
        .await
        .expect("create");
    let nodes = store
        .search("prj-travel", "zzqltoken", 10)
        .await
        .expect("search");
    assert_eq!(nodes.len(), 1);
    assert_eq!(nodes[0].note, "zzqltoken in the body");
}

#[tokio::test]
async fn undo_restore_preserves_note() {
    let store = seeded_store().await;
    let id = add(&store, "prj-travel", "T-1042", pb::NodeKind::Step, "s").await;
    store
        .update_node(pb::UpdateNodeRequest {
            meta: meta("t"),
            node_id: id.clone(),
            update_mask: vec!["note".into()],
            note: "keep me".into(),
            ..Default::default()
        })
        .await
        .expect("update");
    let del = store
        .delete_node(pb::DeleteNodeRequest {
            meta: meta("t"),
            node_id: id.clone(),
            fail_if_referenced: false,
        })
        .await
        .expect("delete");
    store
        .undo(pb::UndoRequest {
            meta: meta("t"),
            project_id: "prj-travel".into(),
            seq: del.seq,
        })
        .await
        .expect("undo");
    let snap = store.get_snapshot("prj-travel").await.expect("snap");
    let s = snap.nodes.iter().find(|n| n.id == id).expect("restored");
    assert_eq!(s.note, "keep me");
}

#[tokio::test]
async fn create_project_returns_new_project() {
    let store = seeded_store().await;
    let p = store
        .create_project(pb::CreateProjectRequest {
            meta: meta("t"),
            name: "New Proj".into(),
            description: "desc".into(),
        })
        .await
        .expect("create");
    assert!(p.id.starts_with("prj-"));
    assert_eq!(p.name, "New Proj");
    assert!(store
        .list_projects(false)
        .await
        .expect("list")
        .iter()
        .any(|x| x.id == p.id));
}

#[tokio::test]
async fn create_project_idempotent_retry_returns_same() {
    let store = seeded_store().await;
    let first = store
        .create_project(pb::CreateProjectRequest {
            meta: meta_idem("agent", "cp-1"),
            name: "X".into(),
            description: String::new(),
        })
        .await
        .expect("first");
    let retry = store
        .create_project(pb::CreateProjectRequest {
            meta: meta_idem("agent", "cp-1"),
            name: "X".into(),
            description: String::new(),
        })
        .await
        .expect("retry");
    assert_eq!(first.id, retry.id);
    // seeded travel (non-archived) + exactly one new project.
    assert_eq!(store.list_projects(false).await.expect("list").len(), 2);
}

#[tokio::test]
async fn update_project_name_and_description() {
    let store = seeded_store().await;
    let p = store
        .update_project(pb::UpdateProjectRequest {
            meta: meta("t"),
            project_id: "prj-travel".into(),
            update_mask: vec!["name".into(), "description".into()],
            name: "Renamed".into(),
            description: "new blurb".into(),
        })
        .await
        .expect("update");
    assert_eq!(p.name, "Renamed");
    assert_eq!(p.description, "new blurb");
}

#[tokio::test]
async fn archive_and_unarchive_project() {
    let store = seeded_store().await;
    let p = store
        .archive_project(pb::ArchiveProjectRequest {
            meta: meta("t"),
            project_id: "prj-travel".into(),
            archived: true,
        })
        .await
        .expect("archive");
    assert!(p.archived_at > 0);
    assert!(store
        .list_projects(false)
        .await
        .expect("list")
        .iter()
        .all(|x| x.id != "prj-travel"));
    let p2 = store
        .archive_project(pb::ArchiveProjectRequest {
            meta: meta("t"),
            project_id: "prj-travel".into(),
            archived: false,
        })
        .await
        .expect("unarchive");
    assert_eq!(p2.archived_at, 0);
    assert!(store
        .list_projects(false)
        .await
        .expect("list")
        .iter()
        .any(|x| x.id == "prj-travel"));
}

#[tokio::test]
async fn create_node_idempotent_retry_returns_same_node() {
    let store = seeded_store().await;
    let first = store
        .create_node(pb::CreateNodeRequest {
            meta: meta_idem("agent", "cn-1"),
            project_id: "prj-travel".into(),
            parent_id: "WP-AUTH".into(),
            kind: pb::NodeKind::Task as i32,
            title: "once".into(),
            ..Default::default()
        })
        .await
        .expect("first");
    let id = first.changed_nodes[0].id.clone();
    let retry = store
        .create_node(pb::CreateNodeRequest {
            meta: meta_idem("agent", "cn-1"),
            project_id: "prj-travel".into(),
            parent_id: "WP-AUTH".into(),
            kind: pb::NodeKind::Task as i32,
            title: "once".into(),
            ..Default::default()
        })
        .await
        .expect("retry");
    // Replay returns the SAME node (not an empty payload), no duplicate created.
    assert_eq!(retry.changed_nodes.len(), 1);
    assert_eq!(retry.changed_nodes[0].id, id);
    assert!(retry.events.is_empty());
    let snap = store.get_snapshot("prj-travel").await.expect("snap");
    assert_eq!(snap.nodes.iter().filter(|n| n.title == "once").count(), 1);
}

#[tokio::test]
async fn move_promote_step_to_task() {
    let store = seeded_store().await;
    let s = add(
        &store,
        "prj-travel",
        "T-1042",
        pb::NodeKind::Step,
        "promote me",
    )
    .await;
    let m = store
        .move_node(pb::MoveNodeRequest {
            meta: meta("t"),
            node_id: s.clone(),
            parent_id: "WP-AUTH".into(),
            kind: pb::NodeKind::Task as i32,
        })
        .await
        .expect("promote");
    let moved = m.changed_nodes.iter().find(|n| n.id == s).expect("moved");
    assert_eq!(moved.kind, pb::NodeKind::Task as i32);
    assert_eq!(moved.parent_id, "WP-AUTH");
    assert!(m
        .events
        .iter()
        .any(|e| e.kind == pb::EventKind::NodeUpdated as i32));
}

#[tokio::test]
async fn move_reparents_task_to_another_wp() {
    let store = seeded_store().await;
    let wp2 = add(&store, "prj-travel", "", pb::NodeKind::WorkPackage, "WP2").await;
    let m = store
        .move_node(pb::MoveNodeRequest {
            meta: meta("t"),
            node_id: "T-1042".into(),
            parent_id: wp2.clone(),
            kind: pb::NodeKind::Task as i32,
        })
        .await
        .expect("reparent");
    let moved = m
        .changed_nodes
        .iter()
        .find(|n| n.id == "T-1042")
        .expect("moved");
    assert_eq!(moved.parent_id, wp2);
    assert_eq!(moved.kind, pb::NodeKind::Task as i32);
}

#[tokio::test]
async fn move_demote_task_drops_its_steps() {
    let store = seeded_store().await;
    let task = add(
        &store,
        "prj-travel",
        "WP-AUTH",
        pb::NodeKind::Task,
        "demote me",
    )
    .await;
    let child = add(
        &store,
        "prj-travel",
        &task,
        pb::NodeKind::Step,
        "doomed step",
    )
    .await;
    let m = store
        .move_node(pb::MoveNodeRequest {
            meta: meta("t"),
            node_id: task.clone(),
            parent_id: "T-1043".into(),
            kind: pb::NodeKind::Step as i32,
        })
        .await
        .expect("demote");
    // A NODE_DELETED event was produced for the dropped child.
    assert!(m
        .events
        .iter()
        .any(|e| e.kind == pb::EventKind::NodeDeleted as i32));
    let snap = store.get_snapshot("prj-travel").await.expect("snap");
    let t = snap.nodes.iter().find(|n| n.id == task).expect("moved");
    assert_eq!(t.kind, pb::NodeKind::Step as i32);
    assert_eq!(t.parent_id, "T-1043");
    // The step child is gone.
    assert!(!snap.nodes.iter().any(|n| n.id == child));
}

#[tokio::test]
async fn move_kind_change_clears_verification() {
    let store = seeded_store().await;
    let task = add(
        &store,
        "prj-travel",
        "WP-AUTH",
        pb::NodeKind::Task,
        "verify me",
    )
    .await;
    store
        .report_condition(pb::ReportConditionRequest {
            meta: meta("agent"),
            node_id: task.clone(),
            result: pb::AgentResult::Pass as i32,
            detail: "ok".into(),
        })
        .await
        .expect("report");
    let m = store
        .move_node(pb::MoveNodeRequest {
            meta: meta("t"),
            node_id: task.clone(),
            parent_id: "T-1043".into(),
            kind: pb::NodeKind::Step as i32,
        })
        .await
        .expect("demote");
    let moved = m
        .changed_nodes
        .iter()
        .find(|n| n.id == task)
        .expect("moved");
    assert!(moved.verification.is_none());
}

#[tokio::test]
async fn move_rejects_work_package_transition() {
    let store = seeded_store().await;
    assert!(store
        .move_node(pb::MoveNodeRequest {
            meta: meta("t"),
            node_id: "WP-AUTH".into(),
            parent_id: String::new(),
            kind: pb::NodeKind::Task as i32,
        })
        .await
        .is_err());
    assert!(store
        .move_node(pb::MoveNodeRequest {
            meta: meta("t"),
            node_id: "T-1042".into(),
            parent_id: String::new(),
            kind: pb::NodeKind::WorkPackage as i32,
        })
        .await
        .is_err());
}

#[tokio::test]
async fn move_rejects_self_parent() {
    let store = seeded_store().await;
    assert!(store
        .move_node(pb::MoveNodeRequest {
            meta: meta("t"),
            node_id: "T-1042".into(),
            parent_id: "T-1042".into(),
            kind: pb::NodeKind::Task as i32,
        })
        .await
        .is_err());
}

#[tokio::test]
async fn move_rejects_cross_project() {
    let store = seeded_store().await;
    let wp2 = add(
        &store,
        "prj-docs",
        "",
        pb::NodeKind::WorkPackage,
        "other wp",
    )
    .await;
    assert!(store
        .move_node(pb::MoveNodeRequest {
            meta: meta("t"),
            node_id: "T-1042".into(),
            parent_id: wp2,
            kind: pb::NodeKind::Task as i32,
        })
        .await
        .is_err());
}

#[tokio::test]
async fn delete_subtree_emits_events_and_refreshes_external_dependents() {
    let store = seeded_store().await;
    // An external task (in a second WP) blocked by T-1042 inside WP-AUTH.
    let wp2 = add(&store, "prj-travel", "", pb::NodeKind::WorkPackage, "WP2").await;
    let ext = add(&store, "prj-travel", &wp2, pb::NodeKind::Task, "external").await;
    store
        .add_dependency(pb::AddDependencyRequest {
            meta: meta("t"),
            blocker_id: "T-1042".into(),
            blocked_id: ext.clone(),
        })
        .await
        .expect("dep");
    // Delete the whole WP-AUTH subtree (WP-AUTH + T-1042 + T-1043).
    let m = store
        .delete_node(pb::DeleteNodeRequest {
            meta: meta("t"),
            node_id: "WP-AUTH".into(),
            fail_if_referenced: false,
        })
        .await
        .expect("delete");
    // One NODE_DELETED per subtree node (root + 2 descendants).
    let deleted = m
        .events
        .iter()
        .filter(|e| e.kind == pb::EventKind::NodeDeleted as i32)
        .count();
    assert_eq!(deleted, 3);
    // The external dependent's blocker vanished, so it is refreshed in the cascade.
    assert!(m.changed_nodes.iter().any(|n| n.id == ext));
    // The subtree is gone.
    let snap = store.get_snapshot("prj-travel").await.expect("snap");
    assert!(!snap
        .nodes
        .iter()
        .any(|n| n.id == "WP-AUTH" || n.id == "T-1042" || n.id == "T-1043"));
}

#[tokio::test]
async fn poll_changes_pages_forward_from_cursor() {
    let store = seeded_store().await;
    let start = store.get_snapshot("prj-travel").await.expect("snap").seq;
    store
        .set_status(pb::SetStatusRequest {
            meta: meta("t"),
            node_id: "T-1042".into(),
            declared_status: pb::DeclaredStatus::Done as i32,
        })
        .await
        .expect("status");
    store
        .add_comment(pb::AddCommentRequest {
            meta: meta("t"),
            node_id: "T-1042".into(),
            text: "hi".into(),
        })
        .await
        .expect("comment");

    // Everything after the pre-change cursor, oldest-first.
    let resp = store
        .poll_changes("prj-travel", start, 0)
        .await
        .expect("poll");
    assert_eq!(resp.events.len(), 2);
    assert!(resp.events[0].seq < resp.events[1].seq);
    assert_eq!(resp.seq, resp.events.last().unwrap().seq);

    // Polling from the returned cursor yields nothing; cursor is unchanged.
    let empty = store
        .poll_changes("prj-travel", resp.seq, 0)
        .await
        .expect("poll2");
    assert!(empty.events.is_empty());
    assert_eq!(empty.seq, resp.seq);

    // `limit` is honored.
    let one = store
        .poll_changes("prj-travel", start, 1)
        .await
        .expect("poll3");
    assert_eq!(one.events.len(), 1);
    assert_eq!(one.seq, one.events[0].seq);
}
