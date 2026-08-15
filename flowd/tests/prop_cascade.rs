//! Property test for the engine's core invariant: the effective-status
//! derivation (`node_state`) must match a straightforward reference for random
//! DAGs of tasks and random declared statuses.
//!
//! Rule (per the `node_state` view): a node is DONE/DEFERRED from its own
//! declared status; otherwise BLOCKED if any DIRECT blocker is not declared
//! DONE; otherwise READY. Only direct blockers matter (no recursion).

use flowd::db;
use flowd::generated::flow_v1 as pb;
use flowd::store::{NativeStore, SqliteStore, Store};
use proptest::prelude::*;

fn meta() -> Option<pb::WriteMeta> {
    Some(pb::WriteMeta {
        author: "prop".into(),
        idempotency_key: String::new(),
    })
}

/// Build a project of `decls.len()` tasks under one work package, set their
/// declared statuses, and add each edge as blocker=min→blocked=max (acyclic).
/// Returns (store, project_id, task_ids).
async fn build(decls: &[u8], edges: &[(usize, usize)]) -> (NativeStore, String, Vec<String>) {
    let conn = db::open(":memory:").unwrap();
    let store = NativeStore::new(SqliteStore::open(conn));
    let proj = store
        .create_project(pb::CreateProjectRequest {
            meta: meta(),
            name: "P".into(),
            description: String::new(),
        })
        .await
        .unwrap();
    let wp = store
        .create_node(pb::CreateNodeRequest {
            meta: meta(),
            project_id: proj.id.clone(),
            kind: pb::NodeKind::WorkPackage as i32,
            title: "WP".into(),
            ..Default::default()
        })
        .await
        .unwrap()
        .changed_nodes[0]
        .id
        .clone();

    let mut ids = Vec::new();
    for i in 0..decls.len() {
        let id = store
            .create_node(pb::CreateNodeRequest {
                meta: meta(),
                project_id: proj.id.clone(),
                parent_id: wp.clone(),
                kind: pb::NodeKind::Task as i32,
                title: format!("T{i}"),
                ..Default::default()
            })
            .await
            .unwrap()
            .changed_nodes[0]
            .id
            .clone();
        ids.push(id);
    }
    for (i, &d) in decls.iter().enumerate() {
        if d != 0 {
            let s = if d == 1 {
                pb::DeclaredStatus::Deferred
            } else {
                pb::DeclaredStatus::Done
            };
            store
                .set_status(pb::SetStatusRequest {
                    meta: meta(),
                    node_id: ids[i].clone(),
                    declared_status: s as i32,
                })
                .await
                .unwrap();
        }
    }
    for &(a, b) in edges {
        let (lo, hi) = match a.cmp(&b) {
            std::cmp::Ordering::Less => (a, b),
            std::cmp::Ordering::Greater => (b, a),
            std::cmp::Ordering::Equal => continue,
        };
        if hi < ids.len() {
            let _ = store
                .add_dependency(pb::AddDependencyRequest {
                    meta: meta(),
                    blocker_id: ids[lo].clone(),
                    blocked_id: ids[hi].clone(),
                })
                .await;
        }
    }
    (store, proj.id, ids)
}

/// Reference effective status per task, mirroring the `node_state` rule.
fn reference(decls: &[u8], edges: &[(usize, usize)]) -> Vec<i32> {
    let n = decls.len();
    let mut blockers: Vec<Vec<usize>> = vec![Vec::new(); n];
    for &(a, b) in edges {
        let (lo, hi) = match a.cmp(&b) {
            std::cmp::Ordering::Less => (a, b),
            std::cmp::Ordering::Greater => (b, a),
            std::cmp::Ordering::Equal => continue,
        };
        if hi < n {
            blockers[hi].push(lo);
        }
    }
    (0..n)
        .map(|i| {
            let e = match decls[i] {
                2 => pb::EffectiveStatus::Done,
                1 => pb::EffectiveStatus::Deferred,
                _ if blockers[i].iter().any(|&j| decls[j] != 2) => pb::EffectiveStatus::Blocked,
                _ => pb::EffectiveStatus::Ready,
            };
            e as i32
        })
        .collect()
}

proptest! {
    #![proptest_config(ProptestConfig::with_cases(24))]
    #[test]
    fn effective_status_matches_reference(
        decls in prop::collection::vec(0u8..3u8, 1..7),
        edges in prop::collection::vec((0usize..6, 0usize..6), 0..8),
    ) {
        let rt = tokio::runtime::Runtime::new().unwrap();
        rt.block_on(async {
            let (store, project_id, ids) = build(&decls, &edges).await;
            let snap = store.get_snapshot(&project_id).await.unwrap();
            let want = reference(&decls, &edges);
            for (i, id) in ids.iter().enumerate() {
                let node = snap.nodes.iter().find(|n| &n.id == id).unwrap();
                prop_assert_eq!(
                    node.status, want[i],
                    "task {} decls={:?} edges={:?}", i, decls, edges
                );
            }
            Ok(())
        })?;
    }
}
