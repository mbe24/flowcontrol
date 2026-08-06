//! The tonic `FlowService` gRPC implementation. Backed by the `Store` seam, so
//! the handlers are reusable against any future store.

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
    type WatchStream =
        tokio_stream::wrappers::ReceiverStream<Result<pb::WatchResponse, tonic::Status>>;

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
        request: tonic::Request<pb::WatchRequest>,
    ) -> Result<tonic::Response<Self::WatchStream>, tonic::Status> {
        // A replay window larger than this is treated as a gap the client must
        // recover from via GetSnapshot (a proxy for event-log retention).
        const REPLAY_LIMIT: usize = 1000;
        let req = request.into_inner();
        let project_id = req.project_id;
        let from_seq = req.from_seq;

        let mut rx = self.store.subscribe();
        let store = self.store.clone();
        let (tx, rx_stream) =
            tokio::sync::mpsc::channel::<Result<pb::WatchResponse, tonic::Status>>(16);

        tokio::spawn(async move {
            // Replay anything the client missed since its last seq.
            if from_seq > 0 {
                if let Ok(events) = store.events_after(&project_id, from_seq).await {
                    // Contiguous log: ask for a resync only when the window is
                    // unrealistic or the first event is not directly after the
                    // requested cursor.
                    let resync = (!events.is_empty() && events.len() > REPLAY_LIMIT)
                        || events
                            .first()
                            .map(|e| e.seq != from_seq + 1)
                            .unwrap_or(false);
                    if !events.is_empty() {
                        let seq = events.last().map(|e| e.seq).unwrap_or(from_seq);
                        let _ = tx
                            .send(Ok(pb::WatchResponse {
                                events,
                                changed_nodes: vec![],
                                changed_progress: vec![],
                                seq,
                                resync_required: resync,
                                heartbeat: false,
                            }))
                            .await;
                    }
                }
            }
            // Then stream live changes for this project, with a 30s heartbeat so
            // a client can tell a quiet server from a dead one.
            let mut interval = tokio::time::interval(std::time::Duration::from_secs(30));
            loop {
                tokio::select! {
                    _ = interval.tick() => {
                        if tx
                            .send(Ok(pb::WatchResponse {
                                events: vec![],
                                changed_nodes: vec![],
                                changed_progress: vec![],
                                seq: 0,
                                resync_required: false,
                                heartbeat: true,
                            }))
                            .await
                            .is_err()
                        {
                            break;
                        }
                    }
                    n = rx.recv() => match n {
                        Ok(m) if m.project_id == project_id => {
                            if tx
                                .send(Ok(pb::WatchResponse {
                                    events: m.events,
                                    changed_nodes: m.changed_nodes,
                                    changed_progress: m.changed_progress,
                                    seq: m.seq,
                                    resync_required: false,
                                    heartbeat: false,
                                }))
                                .await
                                .is_err()
                            {
                                break;
                            }
                        }
                        Ok(_) => {} // other project; skip
                        Err(tokio::sync::broadcast::error::RecvError::Lagged(_)) => {
                            let _ = tx
                                .send(Ok(pb::WatchResponse {
                                    events: vec![],
                                    changed_nodes: vec![],
                                    changed_progress: vec![],
                                    seq: 0,
                                    resync_required: true,
                                    heartbeat: false,
                                }))
                                .await;
                            break;
                        }
                        Err(_) => break,
                    }
                }
            }
        });

        Ok(tonic::Response::new(
            tokio_stream::wrappers::ReceiverStream::new(rx_stream),
        ))
    }

    async fn create_node(
        &self,
        request: tonic::Request<pb::CreateNodeRequest>,
    ) -> Result<tonic::Response<pb::CreateNodeResponse>, tonic::Status> {
        let req = request.into_inner();
        let m = self.store.create_node(req).await.map_err(into_status)?;
        Ok(tonic::Response::new(pb::CreateNodeResponse {
            mutation: Some(m),
        }))
    }

    async fn update_node(
        &self,
        request: tonic::Request<pb::UpdateNodeRequest>,
    ) -> Result<tonic::Response<pb::UpdateNodeResponse>, tonic::Status> {
        let req = request.into_inner();
        let m = self.store.update_node(req).await.map_err(into_status)?;
        Ok(tonic::Response::new(pb::UpdateNodeResponse {
            mutation: Some(m),
        }))
    }

    async fn delete_node(
        &self,
        request: tonic::Request<pb::DeleteNodeRequest>,
    ) -> Result<tonic::Response<pb::DeleteNodeResponse>, tonic::Status> {
        let req = request.into_inner();
        let m = self.store.delete_node(req).await.map_err(into_status)?;
        Ok(tonic::Response::new(pb::DeleteNodeResponse {
            mutation: Some(m),
        }))
    }

    async fn set_status(
        &self,
        request: tonic::Request<pb::SetStatusRequest>,
    ) -> Result<tonic::Response<pb::SetStatusResponse>, tonic::Status> {
        let req = request.into_inner();
        let m = self.store.set_status(req).await.map_err(into_status)?;
        Ok(tonic::Response::new(pb::SetStatusResponse {
            mutation: Some(m),
        }))
    }

    async fn report_condition(
        &self,
        request: tonic::Request<pb::ReportConditionRequest>,
    ) -> Result<tonic::Response<pb::ReportConditionResponse>, tonic::Status> {
        let req = request.into_inner();
        let m = self
            .store
            .report_condition(req)
            .await
            .map_err(into_status)?;
        Ok(tonic::Response::new(pb::ReportConditionResponse {
            mutation: Some(m),
        }))
    }

    async fn set_verdict(
        &self,
        request: tonic::Request<pb::SetVerdictRequest>,
    ) -> Result<tonic::Response<pb::SetVerdictResponse>, tonic::Status> {
        let req = request.into_inner();
        let m = self.store.set_verdict(req).await.map_err(into_status)?;
        Ok(tonic::Response::new(pb::SetVerdictResponse {
            mutation: Some(m),
        }))
    }

    async fn add_comment(
        &self,
        request: tonic::Request<pb::AddCommentRequest>,
    ) -> Result<tonic::Response<pb::AddCommentResponse>, tonic::Status> {
        let req = request.into_inner();
        let m = self.store.add_comment(req).await.map_err(into_status)?;
        Ok(tonic::Response::new(pb::AddCommentResponse {
            mutation: Some(m),
        }))
    }

    async fn add_dependency(
        &self,
        request: tonic::Request<pb::AddDependencyRequest>,
    ) -> Result<tonic::Response<pb::AddDependencyResponse>, tonic::Status> {
        let req = request.into_inner();
        let m = self.store.add_dependency(req).await.map_err(into_status)?;
        Ok(tonic::Response::new(pb::AddDependencyResponse {
            mutation: Some(m),
        }))
    }

    async fn remove_dependency(
        &self,
        request: tonic::Request<pb::RemoveDependencyRequest>,
    ) -> Result<tonic::Response<pb::RemoveDependencyResponse>, tonic::Status> {
        let req = request.into_inner();
        let m = self
            .store
            .remove_dependency(req)
            .await
            .map_err(into_status)?;
        Ok(tonic::Response::new(pb::RemoveDependencyResponse {
            mutation: Some(m),
        }))
    }

    async fn undo(
        &self,
        request: tonic::Request<pb::UndoRequest>,
    ) -> Result<tonic::Response<pb::UndoResponse>, tonic::Status> {
        let req = request.into_inner();
        let m = self.store.undo(req).await.map_err(into_status)?;
        Ok(tonic::Response::new(pb::UndoResponse { mutation: Some(m) }))
    }
}
