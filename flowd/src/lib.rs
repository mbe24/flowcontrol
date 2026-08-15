//! FlowControl core daemon (`flowd`) — the native gRPC edge over `flowcore`.

// Re-export the core's modules so `grpc`, `main`, and tests keep `crate::` paths
// (`crate::db`, `crate::error`, `crate::generated`). The tonic `FlowService` stubs
// under `generated` are present because this crate enables `flowcore/grpc`.
pub use flowcore::{db, error, generated};

pub mod grpc;
pub mod store;
