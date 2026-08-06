//! FlowControl core daemon (`flowd`).

pub mod db;
pub mod generated {
    pub mod flow_v1 {
        // flow.v1.rs ends with `include!("flow.v1.tonic.rs")`, so this single
        // include pulls in both the message types and the tonic service stubs.
        include!("generated/flow.v1.rs");
    }
}
pub mod grpc;
pub mod store;
