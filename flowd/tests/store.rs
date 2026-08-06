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
