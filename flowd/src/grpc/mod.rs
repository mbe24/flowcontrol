//! The tonic `FlowService` gRPC implementation.
//!
//! Read handlers are implemented in this iteration; writes and Watch return
//! `UNIMPLEMENTED` and land in a later milestone. Backed by the `Store` seam,
//! so the handlers are reusable against any future store.

use crate::generated::flow_v1 as pb;
use crate::generated::flow_v1::flow_service_server::FlowService;
use crate::store::DynStore;

#[derive(Clone)]
pub struct FlowServiceServer {
    store: DynStore,
}

/// Convert a boxed store error into a tonic Status.
fn into_status(e: Box<dyn std::error::Error + Send + Sync>) -> tonic::Status {
    tonic::Status::internal(e.to_string())
}

impl FlowServiceServer {
    pub fn new(store: DynStore) -> Self {
        Self { store }
    }
}

#[tonic::async_trait]
impl FlowService for FlowServiceServer {
    type WatchStream = tokio_stream::wrappers::ReceiverStream<Result<pb::WatchResponse, tonic::Status>>;

    async fn list_projects(
        &self,
        request: tonic::Request<pb::ListProjectsRequest>,
    ) -> Result<tonic::Response<pb::ListProjectsResponse>, tonic::Status> {
        let req = request.into_inner();
        let projects = self
            .store
            .list_projects(req.include_archived)
            .await
            .map_err(|e| tonic::Status::internal(e.to_string()))?;
        Ok(tonic::Response::new(pb::ListProjectsResponse { projects }))
    }

    async fn get_snapshot(
        &self,
        request: tonic::Request<pb::GetSnapshotRequest>,
    ) -> Result<tonic::Response<pb::GetSnapshotResponse>, tonic::Status> {
        let req = request.into_inner();
        self.store
            .get_snapshot(&req.project_id)
            .await
            .map(tonic::Response::new)
            .map_err(|e| tonic::Status::internal(e.to_string()))
    }

    async fn list_events(
        &self,
        request: tonic::Request<pb::ListEventsRequest>,
    ) -> Result<tonic::Response<pb::ListEventsResponse>, tonic::Status> {
        let req = request.into_inner();
        let events = self
            .store
            .list_events(&req.project_id, req.limit)
            .await
            .map_err(|e| tonic::Status::internal(e.to_string()))?;
        Ok(tonic::Response::new(pb::ListEventsResponse {
            events,
            has_more: false,
        }))
    }

    async fn search(
        &self,
        request: tonic::Request<pb::SearchRequest>,
    ) -> Result<tonic::Response<pb::SearchResponse>, tonic::Status> {
        let req = request.into_inner();
        let nodes = self
            .store
            .search(&req.project_id, &req.query, req.limit)
            .await
            .map_err(|e| tonic::Status::internal(e.to_string()))?;
        Ok(tonic::Response::new(pb::SearchResponse { nodes }))
    }

    async fn watch(
        &self,
        _request: tonic::Request<pb::WatchRequest>,
    ) -> Result<tonic::Response<Self::WatchStream>, tonic::Status> {
        Err(tonic::Status::unimplemented("watch not implemented yet"))
    }

    async fn create_node(
        &self,
        request: tonic::Request<pb::CreateNodeRequest>,
    ) -> Result<tonic::Response<pb::CreateNodeResponse>, tonic::Status> {
        let req = request.into_inner();
        let m = self.store.create_node(req).await.map_err(into_status)?;
        Ok(tonic::Response::new(pb::CreateNodeResponse { mutation: Some(m) }))
    }

    async fn update_node(
        &self,
        request: tonic::Request<pb::UpdateNodeRequest>,
    ) -> Result<tonic::Response<pb::UpdateNodeResponse>, tonic::Status> {
        let req = request.into_inner();
        let m = self.store.update_node(req).await.map_err(into_status)?;
        Ok(tonic::Response::new(pb::UpdateNodeResponse { mutation: Some(m) }))
    }

    async fn delete_node(
        &self,
        request: tonic::Request<pb::DeleteNodeRequest>,
    ) -> Result<tonic::Response<pb::DeleteNodeResponse>, tonic::Status> {
        let req = request.into_inner();
        let m = self.store.delete_node(req).await.map_err(into_status)?;
        Ok(tonic::Response::new(pb::DeleteNodeResponse { mutation: Some(m) }))
    }

    async fn set_status(
        &self,
        request: tonic::Request<pb::SetStatusRequest>,
    ) -> Result<tonic::Response<pb::SetStatusResponse>, tonic::Status> {
        let req = request.into_inner();
        let m = self.store.set_status(req).await.map_err(into_status)?;
        Ok(tonic::Response::new(pb::SetStatusResponse { mutation: Some(m) }))
    }

    async fn report_condition(
        &self,
        _request: tonic::Request<pb::ReportConditionRequest>,
    ) -> Result<tonic::Response<pb::ReportConditionResponse>, tonic::Status> {
        Err(tonic::Status::unimplemented("report_condition not implemented yet"))
    }

    async fn set_verdict(
        &self,
        _request: tonic::Request<pb::SetVerdictRequest>,
    ) -> Result<tonic::Response<pb::SetVerdictResponse>, tonic::Status> {
        Err(tonic::Status::unimplemented("set_verdict not implemented yet"))
    }

    async fn add_comment(
        &self,
        _request: tonic::Request<pb::AddCommentRequest>,
    ) -> Result<tonic::Response<pb::AddCommentResponse>, tonic::Status> {
        Err(tonic::Status::unimplemented("add_comment not implemented yet"))
    }

    async fn add_dependency(
        &self,
        request: tonic::Request<pb::AddDependencyRequest>,
    ) -> Result<tonic::Response<pb::AddDependencyResponse>, tonic::Status> {
        let req = request.into_inner();
        let m = self.store.add_dependency(req).await.map_err(into_status)?;
        Ok(tonic::Response::new(pb::AddDependencyResponse { mutation: Some(m) }))
    }

    async fn remove_dependency(
        &self,
        request: tonic::Request<pb::RemoveDependencyRequest>,
    ) -> Result<tonic::Response<pb::RemoveDependencyResponse>, tonic::Status> {
        let req = request.into_inner();
        let m = self.store.remove_dependency(req).await.map_err(into_status)?;
        Ok(tonic::Response::new(pb::RemoveDependencyResponse { mutation: Some(m) }))
    }

    async fn undo(
        &self,
        _request: tonic::Request<pb::UndoRequest>,
    ) -> Result<tonic::Response<pb::UndoResponse>, tonic::Status> {
        Err(tonic::Status::unimplemented("undo not implemented yet"))
    }
}
