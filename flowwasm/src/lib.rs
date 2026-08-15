//! The wasm face of `flowcore`: a `wasm-bindgen` binding that JS hosts (the Node
//! daemon and the browser) call. It is NOT a server — no socket, no gRPC. It
//! exposes `flowcore`'s synchronous `dispatch` ("the service minus the socket")
//! so a JS host can drive the store; the host owns the transport (connect-node in
//! Node) and the Watch fan-out. SQLite lives inside the wasm (sqlite-wasm-rs).

use flowcore::store::SqliteStore;
use wasm_bindgen::prelude::*;

/// An opened store, handed to JS. Holds `flowcore`'s sync `SqliteStore`.
#[wasm_bindgen]
pub struct Store {
    inner: SqliteStore,
}

#[wasm_bindgen]
impl Store {
    /// Open a store. `path` is `":memory:"` or a file path (persistence is the
    /// host's VFS concern); `seed` loads the demo fixture.
    #[wasm_bindgen(constructor)]
    pub fn new(path: &str, seed: bool) -> Result<Store, JsError> {
        let conn = flowcore::db::open(path).map_err(to_js)?;
        if seed {
            flowcore::db::seed(&conn).map_err(to_js)?;
        }
        Ok(Store {
            inner: SqliteStore::new(conn),
        })
    }

    /// Run one unary RPC: protobuf request bytes in, protobuf response bytes out.
    /// `method` is the proto RPC name (e.g. "CreateNode", "GetSnapshot").
    pub fn dispatch(&self, method: &str, req: &[u8]) -> Result<Vec<u8>, JsError> {
        self.inner.dispatch(method, req).map_err(to_js)
    }
}

/// Map a `DomainError` to a JS error, prefixing the transport-neutral code so the
/// host can recover it (`"<code>: <message>"`, split on the first `": "`).
fn to_js(e: flowcore::error::DomainError) -> JsError {
    JsError::new(&format!("{}: {}", e.code.as_str(), e.message))
}
