//! `flowcore` — the target-agnostic core of the flow system.
//!
//! Proto messages, the domain-error taxonomy, the SQLite schema, and the
//! synchronous store + [`store::SqliteStore::dispatch`] ("the service minus the
//! socket"). No tokio, no tonic in the default build, so it compiles to wasm32.
//! The native `flowd` daemon and a future wasm cdylib both build on it.

pub mod db;
pub mod error;
pub mod store;

pub mod generated {
    pub mod flow_v1 {
        // Prost message types (always). The tonic service stubs are gated: only
        // the native edge (`flowd`) enables `grpc`; the wasm build stays
        // tonic-free. Both files share this module so the service's `super::Msg`
        // references resolve.
        include!("generated/flow.v1.rs");
        #[cfg(feature = "grpc")]
        include!("generated/flow.v1.tonic.rs");
    }
}
