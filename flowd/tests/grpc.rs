//! In-process gRPC round-trip test: serve FlowService reads over a real tonic
//! server on an ephemeral port and call it with the generated client.

use flowd::db;
use flowd::generated::flow_v1 as pb;
use flowd::generated::flow_v1::flow_service_client::FlowServiceClient;
use flowd::generated::flow_v1::flow_service_server::FlowServiceServer;
use flowd::grpc::FlowServiceServer as FlowImpl;
use flowd::store::{DynStore, NativeStore, SqliteStore};
use tokio::net::TcpListener;

/// Spin up an in-memory DB, seed it, and serve FlowService on a random port.
/// Returns (client, port).
async fn spawn_server() -> (FlowServiceClient<tonic::transport::Channel>, u16) {
    let conn = db::open(":memory:").expect("db");
    db::seed(&conn).expect("seed");
    let store: DynStore = std::sync::Arc::new(NativeStore::new(SqliteStore::new(conn)));
    let svc = FlowServiceServer::new(FlowImpl::new(store));

    let listener = TcpListener::bind("127.0.0.1:0").await.expect("bind");
    let addr = listener.local_addr().expect("addr");
    let port = addr.port();

    tokio::spawn(async move {
        tonic::transport::Server::builder()
            .add_service(svc)
            .serve_with_incoming(tokio_stream::wrappers::TcpListenerStream::new(listener))
            .await
            .expect("serve");
    });

    // Give the server a moment to start, then connect.
    tokio::time::sleep(std::time::Duration::from_millis(100)).await;
    let endpoint = format!("http://127.0.0.1:{}", port);
    let client = FlowServiceClient::connect(endpoint).await.expect("connect");
    (client, port)
}

#[tokio::test]
async fn list_projects_over_grpc() {
    let (mut client, _port) = spawn_server().await;
    let resp = client
        .list_projects(pb::ListProjectsRequest {
            include_archived: false,
        })
        .await
        .expect("rpc");
    let projects = resp.into_inner().projects;
    assert_eq!(projects.len(), 1);
    assert_eq!(projects[0].id, "prj-travel");
}

#[tokio::test]
async fn get_snapshot_over_grpc() {
    let (mut client, _port) = spawn_server().await;
    let resp = client
        .get_snapshot(pb::GetSnapshotRequest {
            project_id: "prj-travel".into(),
        })
        .await
        .expect("rpc");
    let snap = resp.into_inner();
    assert_eq!(snap.nodes.len(), 3);
    assert_eq!(snap.dependencies.len(), 1);
}

fn wm(author: &str, key: &str) -> pb::WriteMeta {
    pb::WriteMeta {
        author: author.into(),
        idempotency_key: key.into(),
    }
}

#[tokio::test]
async fn report_condition_over_grpc() {
    let (mut client, _port) = spawn_server().await;
    let resp = client
        .report_condition(pb::ReportConditionRequest {
            meta: Some(wm("agent-1", "")),
            node_id: "T-1042".into(),
            result: pb::AgentResult::Pass as i32,
            detail: "all good".into(),
        })
        .await
        .expect("rpc");
    let m = resp.into_inner().mutation.expect("mutation");
    assert_eq!(m.events.len(), 1);
    assert_eq!(m.events[0].kind, pb::EventKind::AgentReported as i32);
    // Verification round-trips through the snapshot.
    let snap = client
        .get_snapshot(pb::GetSnapshotRequest {
            project_id: "prj-travel".into(),
        })
        .await
        .expect("snap")
        .into_inner();
    let node = snap
        .nodes
        .iter()
        .find(|n| n.id == "T-1042")
        .expect("T-1042");
    assert_eq!(
        node.verification.as_ref().map(|v| v.agent_result),
        Some(pb::AgentResult::Pass as i32)
    );
}

#[tokio::test]
async fn set_verdict_over_grpc() {
    let (mut client, _port) = spawn_server().await;
    client
        .report_condition(pb::ReportConditionRequest {
            meta: Some(wm("agent-1", "")),
            node_id: "T-1042".into(),
            result: pb::AgentResult::Fail as i32,
            detail: String::new(),
        })
        .await
        .expect("report");
    let resp = client
        .set_verdict(pb::SetVerdictRequest {
            meta: Some(wm("operator", "")),
            node_id: "T-1042".into(),
            verdict: pb::HumanVerdict::Accepted as i32,
        })
        .await
        .expect("rpc");
    assert_eq!(
        resp.into_inner().mutation.expect("mutation").events.len(),
        1
    );
}

#[tokio::test]
async fn add_comment_over_grpc() {
    let (mut client, _port) = spawn_server().await;
    let resp = client
        .add_comment(pb::AddCommentRequest {
            meta: Some(wm("operator", "")),
            node_id: "T-1042".into(),
            text: "hi".into(),
        })
        .await
        .expect("rpc");
    let m = resp.into_inner().mutation.expect("mutation");
    assert_eq!(m.events[0].kind, pb::EventKind::Comment as i32);
}

#[tokio::test]
async fn undo_over_grpc() {
    let (mut client, _port) = spawn_server().await;
    let set = client
        .set_status(pb::SetStatusRequest {
            meta: Some(wm("tester", "")),
            node_id: "T-1042".into(),
            declared_status: pb::DeclaredStatus::Done as i32,
        })
        .await
        .expect("set")
        .into_inner()
        .mutation
        .expect("mutation");
    client
        .undo(pb::UndoRequest {
            meta: Some(wm("tester", "")),
            project_id: "prj-travel".into(),
            seq: set.seq,
        })
        .await
        .expect("undo");
    let snap = client
        .get_snapshot(pb::GetSnapshotRequest {
            project_id: "prj-travel".into(),
        })
        .await
        .expect("snap")
        .into_inner();
    let node = snap
        .nodes
        .iter()
        .find(|n| n.id == "T-1042")
        .expect("T-1042");
    assert_eq!(node.declared_status, pb::DeclaredStatus::Open as i32);
}

#[tokio::test]
async fn create_project_over_grpc() {
    let (mut client, _port) = spawn_server().await;
    let p = client
        .create_project(pb::CreateProjectRequest {
            meta: Some(wm("t", "")),
            name: "Grpc Proj".into(),
            description: "made over the wire".into(),
        })
        .await
        .expect("rpc")
        .into_inner()
        .project
        .expect("project");
    assert!(p.id.starts_with("prj-"));
    assert_eq!(p.name, "Grpc Proj");
}

#[tokio::test]
async fn move_wp_transition_maps_to_failed_precondition() {
    let (mut client, _port) = spawn_server().await;
    let status = client
        .move_node(pb::MoveNodeRequest {
            meta: Some(wm("t", "")),
            node_id: "T-1042".into(),
            parent_id: String::new(),
            kind: pb::NodeKind::WorkPackage as i32,
        })
        .await
        .expect_err("should be rejected");
    assert_eq!(status.code(), tonic::Code::FailedPrecondition);
}

#[tokio::test]
async fn move_unknown_node_maps_to_not_found() {
    let (mut client, _port) = spawn_server().await;
    let status = client
        .move_node(pb::MoveNodeRequest {
            meta: Some(wm("t", "")),
            node_id: "nope".into(),
            parent_id: "WP-AUTH".into(),
            kind: pb::NodeKind::Task as i32,
        })
        .await
        .expect_err("should be not found");
    assert_eq!(status.code(), tonic::Code::NotFound);
}

#[tokio::test]
async fn undo_non_undoable_maps_to_failed_precondition() {
    let (mut client, _port) = spawn_server().await;
    // update_node produces a NODE_UPDATED event, which undo cannot reverse.
    let m = client
        .update_node(pb::UpdateNodeRequest {
            meta: Some(wm("t", "")),
            node_id: "T-1042".into(),
            update_mask: vec!["title".into()],
            title: "Renamed".into(),
            ..Default::default()
        })
        .await
        .expect("update")
        .into_inner()
        .mutation
        .expect("mutation");
    let status = client
        .undo(pb::UndoRequest {
            meta: Some(wm("t", "")),
            project_id: "prj-travel".into(),
            seq: m.seq,
        })
        .await
        .expect_err("should be un-undoable");
    assert_eq!(status.code(), tonic::Code::FailedPrecondition);
}

#[tokio::test]
async fn search_bad_query_maps_to_invalid_argument() {
    let (mut client, _port) = spawn_server().await;
    let status = client
        .search(pb::SearchRequest {
            project_id: "prj-travel".into(),
            query: "(unbalanced".into(),
            limit: 10,
        })
        .await
        .expect_err("malformed fts query");
    assert_eq!(status.code(), tonic::Code::InvalidArgument);
}

#[tokio::test]
async fn watch_streams_live_events_over_grpc() {
    let (mut client, _port) = spawn_server().await;
    // from_seq = 0 -> "from now"; the seeded events are not replayed.
    let mut stream = client
        .watch(pb::WatchRequest {
            project_id: "prj-travel".into(),
            from_seq: 0,
        })
        .await
        .expect("watch")
        .into_inner();

    client
        .set_status(pb::SetStatusRequest {
            meta: Some(wm("tester", "")),
            node_id: "T-1042".into(),
            declared_status: pb::DeclaredStatus::Done as i32,
        })
        .await
        .expect("set");

    // Read messages (skipping heartsets / other events) until the STATUS_SET
    // produced by the mutation above arrives.
    let found = tokio::time::timeout(std::time::Duration::from_secs(5), async {
        loop {
            match stream.message().await.expect("msg") {
                Some(resp) => {
                    if resp
                        .events
                        .iter()
                        .any(|e| e.kind == pb::EventKind::StatusSet as i32)
                    {
                        return true;
                    }
                }
                None => return false,
            }
        }
    })
    .await
    .expect("watch timed out");
    assert!(found);
}
