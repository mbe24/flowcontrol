//! The wasm face of `flowcore`: a `wasm-bindgen` binding that JS hosts (the Node
//! daemon `flowd.js` and the browser) call. It is NOT a server — no socket, no
//! gRPC. It exposes `flowcore`'s synchronous `dispatch` ("the service minus the
//! socket") so a JS host can drive the store.
//!
//! SQLite does NOT live inside the wasm. The store runs over the [`Sql`] seam, and
//! this module implements it as [`HostSql`] — a bridge to two synchronous host
//! functions (`__flowHostExec` / `__flowHostQuery`). The host owns a real SQLite
//! (Node `node:sqlite` with a durable WAL file; the browser worker with
//! `@sqlite.org/sqlite-wasm` + OPFS), so the wasm itself is pure Rust: no C
//! toolchain, no sqlite-wasm-rs. Params and rows cross the boundary as JSON.

use flowcore::error::DomainError;
use flowcore::sql::{Exec, Row, Session, Sql, Value};
use flowcore::store::SqliteStore;
use wasm_bindgen::prelude::*;

type DResult<T> = Result<T, DomainError>;

// The host-provided SQLite bridge. Synchronous (single-threaded JS). `params` is a
// JSON array; the return is a JSON envelope:
//   exec  → {"changes": N, "lastInsertRowid": M}
//   query → {"rows": [ {col: val}, … ]}
// either may instead be {"error": "message"} — the message is fed back through
// `from_db_message` so SQLite's own text (trigger `RAISE`, FTS `MATCH`) keeps its
// domain `Code` across the boundary.
#[wasm_bindgen]
extern "C" {
    #[wasm_bindgen(js_name = "__flowHostExec")]
    fn host_exec(sql: &str, params: &str) -> String;
    #[wasm_bindgen(js_name = "__flowHostQuery")]
    fn host_query(sql: &str, params: &str) -> String;
}

/// The wasm [`Sql`] driver: every session is the one host SQLite connection.
/// Cheap (zero-sized); the host serializes access (JS is single-threaded).
#[derive(Clone)]
pub struct HostSql;

impl Sql for HostSql {
    fn session(&self) -> DResult<Box<dyn Session + '_>> {
        Ok(Box::new(HostSession))
    }
}

struct HostSession;

impl Session for HostSession {
    fn execute(&mut self, sql: &str, params: &[Value]) -> DResult<Exec> {
        let out = host_exec(sql, &params_to_json(params));
        let v = parse_envelope(&out, "host_exec")?;
        Ok(Exec {
            changes: v.get("changes").and_then(|x| x.as_u64()).unwrap_or(0),
            last_insert_rowid: v
                .get("lastInsertRowid")
                .and_then(|x| x.as_i64())
                .unwrap_or(0),
        })
    }

    fn query(&mut self, sql: &str, params: &[Value]) -> DResult<Vec<Row>> {
        let out = host_query(sql, &params_to_json(params));
        let v = parse_envelope(&out, "host_query")?;
        let rows = v
            .get("rows")
            .and_then(|r| r.as_array())
            .ok_or_else(|| DomainError::internal("host_query: missing rows"))?;
        rows.iter().map(json_row_to_row).collect()
    }
}

/// Parse a host JSON envelope, surfacing `{"error": …}` as a domain error.
fn parse_envelope(out: &str, who: &str) -> DResult<serde_json::Value> {
    let v: serde_json::Value = serde_json::from_str(out)
        .map_err(|e| DomainError::internal(format!("{who} bad json: {e}")))?;
    if let Some(msg) = v.get("error").and_then(|e| e.as_str()) {
        return Err(DomainError::from_db_message(msg));
    }
    Ok(v)
}

fn params_to_json(params: &[Value]) -> String {
    let arr: Vec<serde_json::Value> = params.iter().map(value_to_json).collect();
    serde_json::Value::Array(arr).to_string()
}

fn value_to_json(v: &Value) -> serde_json::Value {
    match v {
        Value::Null => serde_json::Value::Null,
        Value::Int(n) => serde_json::Value::from(*n),
        Value::Real(f) => serde_json::json!(f),
        Value::Text(s) => serde_json::Value::from(s.clone()),
        Value::Blob(b) => serde_json::Value::from(b.clone()),
    }
}

fn json_row_to_row(row: &serde_json::Value) -> DResult<Row> {
    let obj = row
        .as_object()
        .ok_or_else(|| DomainError::internal("host row is not an object"))?;
    let cells = obj
        .iter()
        .map(|(k, v)| (k.clone(), json_to_value(v)))
        .collect();
    Ok(Row::from_cells(cells))
}

fn json_to_value(v: &serde_json::Value) -> Value {
    match v {
        serde_json::Value::Null => Value::Null,
        serde_json::Value::Bool(b) => Value::Int(*b as i64),
        serde_json::Value::Number(n) => n
            .as_i64()
            .map(Value::Int)
            .unwrap_or_else(|| Value::Real(n.as_f64().unwrap_or(0.0))),
        serde_json::Value::String(s) => Value::Text(s.clone()),
        // Arrays/objects don't occur in our result cells; treat as NULL.
        _ => Value::Null,
    }
}

/// An opened store, handed to JS. Drives `flowcore`'s sync store over [`HostSql`].
#[wasm_bindgen]
pub struct Store {
    inner: SqliteStore<HostSql>,
}

#[wasm_bindgen]
impl Store {
    /// Bind to the host's SQLite. The host must have applied [`schema_sql`] (and
    /// optionally [`seed_sql`]) to its connection before dispatching.
    #[wasm_bindgen(constructor)]
    pub fn new() -> Store {
        Store {
            inner: SqliteStore::new(HostSql),
        }
    }

    /// Run one unary RPC: protobuf request bytes in, protobuf response bytes out.
    /// `method` is the proto RPC name (e.g. "CreateNode", "GetSnapshot").
    pub fn dispatch(&self, method: &str, req: &[u8]) -> Result<Vec<u8>, JsError> {
        self.inner.dispatch(method, req).map_err(to_js)
    }
}

impl Default for Store {
    fn default() -> Self {
        Self::new()
    }
}

/// The schema SQL the host must apply to its SQLite before dispatching. Single
/// source of truth (shared with the native daemon).
#[wasm_bindgen]
pub fn schema_sql() -> String {
    flowcore::db::SCHEMA_SQL.to_string()
}

/// The dev-fixture seed SQL, for demos.
#[wasm_bindgen]
pub fn seed_sql() -> String {
    flowcore::db::SEED_SQL.to_string()
}

/// Map a `DomainError` to a JS error, prefixing the transport-neutral code so the
/// host can recover it (`"<code>: <message>"`, split on the first `": "`).
fn to_js(e: DomainError) -> JsError {
    JsError::new(&format!("{}: {}", e.code.as_str(), e.message))
}
