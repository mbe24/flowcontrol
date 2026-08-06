//! Integration tests for the SQLite store against the real migration + seed.
//! Each test uses its own `:memory:` database so they are independent and fast.

use flowd::db;
use flowd::generated::flow_v1 as pb;
use flowd::store::{SqliteStore, Store};

async fn seeded_store() -> SqliteStore {
    let pool = db::open(":memory:").await.expect("db open");
    db::seed(&pool).await.expect("seed");
    SqliteStore::from_pool(pool)
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
    assert_eq!(snap.project.as_ref().map(|p| p.id.as_str()), Some("prj-travel"));
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
    let done = snap.nodes.iter().find(|n| n.id == "T-1043").expect("T-1043");
    assert_eq!(done.status, pb::EffectiveStatus::Done as i32);
    // T-1042 is OPEN but blocked by nothing done -> READY (no blockers not done).
    let open = snap.nodes.iter().find(|n| n.id == "T-1042").expect("T-1042");
    assert_eq!(open.status, pb::EffectiveStatus::Ready as i32);
}

#[tokio::test]
async fn list_events_returns_seeded_event() {
    let store = seeded_store().await;
    let events = store.list_events("prj-travel", 10).await.expect("events");
    assert_eq!(events.len(), 1);
    assert_eq!(events[0].node_id, "T-1042");
    assert_eq!(events[0].kind, pb::EventKind::NodeCreated as i32);
}

#[tokio::test]
async fn search_finds_matching_nodes() {
    let store = seeded_store().await;
    let nodes = store.search("prj-travel", "OAuth2", 10).await.expect("search");
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
            meta: Some(pb::WriteMeta { author: "tester".into(), idempotency_key: String::new() }),
            project_id: "prj-travel".into(),
            parent_id: "WP-AUTH".into(),
            kind: pb::NodeKind::Task as i32,
            title: "New task".into(),
            description: "desc".into(),
            condition: String::new(),
            position: 300,
            reference: String::new(),
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
    let ev = store.list_events("prj-travel", 10).await.expect("events");
    assert!(ev.iter().any(|e| e.node_id == id && e.kind == pb::EventKind::NodeCreated as i32));
}

#[tokio::test]
async fn set_status_updates_and_logs_event() {
    let store = seeded_store().await;
    let m = store
        .set_status(pb::SetStatusRequest {
            meta: Some(pb::WriteMeta { author: "tester".into(), idempotency_key: String::new() }),
            node_id: "T-1042".into(),
            declared_status: pb::DeclaredStatus::Done as i32,
        })
        .await
        .expect("set_status");
    assert_eq!(m.changed_nodes[0].id, "T-1042");
    // Effective status becomes DONE.
    assert_eq!(m.changed_nodes[0].status, pb::EffectiveStatus::Done as i32);
    let ev = store.list_events("prj-travel", 10).await.expect("events");
    let status_events: Vec<_> = ev.iter().filter(|e| e.kind == pb::EventKind::StatusSet as i32).collect();
    assert_eq!(status_events.len(), 1);
}

#[tokio::test]
async fn update_node_respects_mask() {
    let store = seeded_store().await;
    let m = store
        .update_node(pb::UpdateNodeRequest {
            meta: Some(pb::WriteMeta { author: "tester".into(), idempotency_key: String::new() }),
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
            meta: Some(pb::WriteMeta { author: "tester".into(), idempotency_key: String::new() }),
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
            meta: Some(pb::WriteMeta { author: "tester".into(), idempotency_key: String::new() }),
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
            meta: Some(pb::WriteMeta { author: "tester".into(), idempotency_key: String::new() }),
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
            meta: Some(pb::WriteMeta { author: "tester".into(), idempotency_key: String::new() }),
            node_id: "T-1042".into(),
            fail_if_referenced: true,
        })
        .await;
    assert!(res.is_err());
}
