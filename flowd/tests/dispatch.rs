//! Tests for the transport-agnostic `dispatch` entry point — the sync "service
//! minus the socket" that the wasm facade and Node host will call. Each test
//! encodes a proto request, runs it through `dispatch`, and decodes the response,
//! exercising the exact bytes-in/bytes-out path the non-tonic hosts use.

use flowd::db;
use flowd::error::Code;
use flowd::generated::flow_v1 as pb;
use flowd::store::{Core, SqliteStore};
use prost::Message;

fn seeded() -> Core {
    let conn = db::open(":memory:").expect("db open");
    db::seed(&conn).expect("seed");
    SqliteStore::open(conn)
}

fn meta() -> Option<pb::WriteMeta> {
    Some(pb::WriteMeta {
        author: "dispatch-test".into(),
        idempotency_key: String::new(),
    })
}

#[test]
fn list_projects_roundtrips() {
    let s = seeded();
    let req = pb::ListProjectsRequest {
        include_archived: false,
    }
    .encode_to_vec();
    let out = s.dispatch("ListProjects", &req).expect("dispatch");
    let resp = pb::ListProjectsResponse::decode(out.as_slice()).expect("decode");
    assert_eq!(resp.projects.len(), 1);
    assert_eq!(resp.projects[0].id, "prj-travel");
}

#[test]
fn create_node_then_snapshot_sees_it() {
    let s = seeded();
    let req = pb::CreateNodeRequest {
        meta: meta(),
        project_id: "prj-travel".into(),
        parent_id: "WP-AUTH".into(),
        kind: pb::NodeKind::Task as i32,
        title: "Created via dispatch".into(),
        ..Default::default()
    }
    .encode_to_vec();
    let out = s.dispatch("CreateNode", &req).expect("dispatch");
    let resp = pb::CreateNodeResponse::decode(out.as_slice()).expect("decode");
    let id = resp.mutation.expect("mutation").changed_nodes[0].id.clone();
    assert!(id.starts_with("node-"), "minted id: {id}");

    // The write is visible in a subsequent snapshot through the same path.
    let sreq = pb::GetSnapshotRequest {
        project_id: "prj-travel".into(),
    }
    .encode_to_vec();
    let sout = s.dispatch("GetSnapshot", &sreq).expect("snapshot");
    let snap = pb::GetSnapshotResponse::decode(sout.as_slice()).expect("decode");
    assert_eq!(snap.nodes.len(), 4); // 3 seeded + 1
    assert!(snap.nodes.iter().any(|n| n.id == id));
}

#[test]
fn search_works_through_dispatch_fts5() {
    // Proves FTS5 is reachable via the dispatch path — the same `dispatch` code the
    // wasm host drives over its own SQLite (node:sqlite), which also ships FTS5.
    let s = seeded();
    let req = pb::SearchRequest {
        project_id: "prj-travel".into(),
        query: "device".into(),
        limit: 10,
    }
    .encode_to_vec();
    let out = s.dispatch("Search", &req).expect("dispatch");
    let resp = pb::SearchResponse::decode(out.as_slice()).expect("decode");
    assert!(resp.nodes.iter().any(|n| n.id == "T-1042"));
}

#[test]
fn store_errors_propagate_with_their_code() {
    // A domain error (here: FailedPrecondition from a non-undoable state) survives
    // the dispatch boundary with its Code intact — this is what the Node/browser
    // hosts map to a gRPC/Connect status.
    let s = seeded();
    // Undo with nothing undoable on an unknown project → "no event to undo".
    let req = pb::UndoRequest {
        meta: meta(),
        project_id: "does-not-exist".into(),
        seq: 0,
    }
    .encode_to_vec();
    let err = s.dispatch("Undo", &req).unwrap_err();
    assert_eq!(err.code, Code::Internal); // "no event to undo" (unclassified)
    assert!(err.message.contains("no event to undo"));
}

#[test]
fn unknown_method_is_not_found() {
    let s = seeded();
    let err = s.dispatch("Nope", &[]).unwrap_err();
    assert_eq!(err.code, Code::NotFound);
}

#[test]
fn undecodable_request_is_invalid_argument() {
    let s = seeded();
    // Wire byte 0xff is an invalid protobuf tag → decode fails → InvalidArgument.
    let err = s.dispatch("GetSnapshot", &[0xff, 0xff, 0xff]).unwrap_err();
    assert_eq!(err.code, Code::InvalidArgument);
}
