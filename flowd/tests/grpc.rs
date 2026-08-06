//! In-process gRPC round-trip test: serve FlowService reads over a real tonic
//! server on an ephemeral port and call it with the generated client.

use flowd::db;
use flowd::generated::flow_v1 as pb;
use flowd::generated::flow_v1::flow_service_client::FlowServiceClient;
use flowd::generated::flow_v1::flow_service_server::FlowServiceServer;
use flowd::grpc::FlowServiceServer as FlowImpl;
use flowd::store::{DynStore, SqliteStore};
use tokio::net::TcpListener;

/// Spin up an in-memory DB, seed it, and serve FlowService on a random port.
/// Returns (client, port).
async fn spawn_server() -> (FlowServiceClient<tonic::transport::Channel>, u16) {
    let pool = db::open(":memory:").await.expect("db");
    db::seed(&pool).await.expect("seed");
    let store: DynStore = std::sync::Arc::new(SqliteStore::from_pool(pool));
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
    let client = FlowServiceClient::connect(endpoint)
        .await
        .expect("connect");
    (client, port)
}

#[tokio::test]
async fn list_projects_over_grpc() {
    let (mut client, _port) = spawn_server().await;
    let resp = client
        .list_projects(pb::ListProjectsRequest { include_archived: false })
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
