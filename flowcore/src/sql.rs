//! The SQL seam: the one capability the store depends on.
//!
//! The store logic is written against [`Session`] (execute / query / [`transaction`])
//! instead of a concrete driver, so the same code runs over:
//! - **rusqlite** natively and in a self-contained wasm (the [`Sql`] impl here),
//! - a **host-imported** driver on wasm (Node `node:sqlite`, browser
//!   `sqlite-wasm`) — a synchronous `host_exec`/`host_query` bridge added by
//!   `flowwasm`.
//!
//! Design notes (per review): the store is generic over `impl Sql` (static
//! dispatch), so `Session` need not be dyn-generic. A [`Session`] holds the
//! connection lock for one whole operation, so `BEGIN … COMMIT` cannot interleave
//! with another op. `transaction` is a free fn (works over `&mut dyn Session`)
//! that rolls back on any `Err`. `INSERT … RETURNING` goes through `query`, not
//! `execute` (execute must not return rows).

use crate::error::DomainError;

type DResult<T> = Result<T, DomainError>;

/// A dynamically-typed SQL value crossing the seam (params in, row cells out).
#[derive(Clone, Debug, PartialEq)]
pub enum Value {
    Null,
    Int(i64),
    Real(f64),
    Text(String),
    Blob(Vec<u8>),
}

impl From<i64> for Value {
    fn from(v: i64) -> Self {
        Value::Int(v)
    }
}
impl From<i32> for Value {
    fn from(v: i32) -> Self {
        Value::Int(v as i64)
    }
}
impl From<bool> for Value {
    fn from(v: bool) -> Self {
        Value::Int(v as i64)
    }
}
impl From<String> for Value {
    fn from(v: String) -> Self {
        Value::Text(v)
    }
}
impl From<&String> for Value {
    fn from(v: &String) -> Self {
        Value::Text(v.clone())
    }
}
impl From<&str> for Value {
    fn from(v: &str) -> Self {
        Value::Text(v.to_string())
    }
}
impl From<Option<&str>> for Value {
    fn from(v: Option<&str>) -> Self {
        v.map(|s| Value::Text(s.to_string())).unwrap_or(Value::Null)
    }
}

/// Build a `&[Value]` param slice from expressions convertible via `Value::from`.
#[macro_export]
macro_rules! values {
    ($($v:expr),* $(,)?) => {
        &[$($crate::sql::Value::from($v)),*][..]
    };
}

/// Result of a non-returning statement.
#[derive(Clone, Copy, Debug)]
pub struct Exec {
    pub changes: u64,
    pub last_insert_rowid: i64,
}

/// One result row: cells keyed by column name (small rows → a linear scan is fine).
#[derive(Clone, Debug, Default)]
pub struct Row {
    cells: Vec<(String, Value)>,
}

impl Row {
    pub fn from_cells(cells: Vec<(String, Value)>) -> Self {
        Self { cells }
    }

    fn cell(&self, col: &str) -> DResult<&Value> {
        self.cells
            .iter()
            .find(|(name, _)| name == col)
            .map(|(_, v)| v)
            .ok_or_else(|| DomainError::internal(format!("no such column: {col}")))
    }

    pub fn get_i64(&self, col: &str) -> DResult<i64> {
        match self.cell(col)? {
            Value::Int(i) => Ok(*i),
            Value::Null => Ok(0),
            v => Err(DomainError::internal(format!(
                "{col}: expected int, got {v:?}"
            ))),
        }
    }

    pub fn get_i32(&self, col: &str) -> DResult<i32> {
        Ok(self.get_i64(col)? as i32)
    }

    /// A required text column (NULL → error).
    pub fn get_str(&self, col: &str) -> DResult<String> {
        match self.cell(col)? {
            Value::Text(s) => Ok(s.clone()),
            v => Err(DomainError::internal(format!(
                "{col}: expected text, got {v:?}"
            ))),
        }
    }

    /// A nullable text column.
    pub fn get_opt_str(&self, col: &str) -> DResult<Option<String>> {
        match self.cell(col)? {
            Value::Text(s) => Ok(Some(s.clone())),
            Value::Null => Ok(None),
            v => Err(DomainError::internal(format!(
                "{col}: expected text/null, got {v:?}"
            ))),
        }
    }

    fn cell_at(&self, i: usize) -> DResult<&Value> {
        self.cells
            .get(i)
            .map(|(_, v)| v)
            .ok_or_else(|| DomainError::internal(format!("no column at index {i}")))
    }

    /// First-column helpers for scalar/`query_scalar`-style selects.
    pub fn get_i64_at(&self, i: usize) -> DResult<i64> {
        match self.cell_at(i)? {
            Value::Int(v) => Ok(*v),
            Value::Null => Ok(0),
            v => Err(DomainError::internal(format!(
                "col {i}: expected int, got {v:?}"
            ))),
        }
    }
    pub fn get_str_at(&self, i: usize) -> DResult<String> {
        match self.cell_at(i)? {
            Value::Text(s) => Ok(s.clone()),
            v => Err(DomainError::internal(format!(
                "col {i}: expected text, got {v:?}"
            ))),
        }
    }
}

/// The store's provider of connections. Generic in the store (`Store<impl Sql>`).
pub trait Sql: Clone + Send + Sync + 'static {
    /// Borrow an exclusive session that holds the connection lock for its lifetime.
    fn session(&self) -> DResult<Box<dyn Session + '_>>;
}

/// One exclusive database session: run statements, in or out of a transaction.
pub trait Session {
    /// A non-returning statement (INSERT/UPDATE/DELETE/BEGIN/…). Must NOT be used
    /// for statements that return rows — use [`Session::query`] for `RETURNING`.
    fn execute(&mut self, sql: &str, params: &[Value]) -> DResult<Exec>;
    /// A row-returning statement (SELECT, or `INSERT … RETURNING`).
    fn query(&mut self, sql: &str, params: &[Value]) -> DResult<Vec<Row>>;

    /// Convenience: at most one row.
    fn query_opt(&mut self, sql: &str, params: &[Value]) -> DResult<Option<Row>> {
        Ok(self.query(sql, params)?.into_iter().next())
    }

    /// Convenience: exactly one row (for aggregates / `SELECT <scalar>`).
    fn query_one(&mut self, sql: &str, params: &[Value]) -> DResult<Row> {
        self.query_opt(sql, params)?
            .ok_or_else(|| DomainError::internal("query returned no rows"))
    }
}

/// Run `f` inside a transaction on `s`: commit on `Ok`, roll back on `Err`.
/// Works over `&mut dyn Session`. (Panic-safety — rollback-on-unwind — is left to
/// the driver's session `Drop`; store logic returns `Err`, never panics on the
/// data path.)
pub fn transaction<S: Session + ?Sized, T>(
    s: &mut S,
    f: impl FnOnce(&mut S) -> DResult<T>,
) -> DResult<T> {
    s.execute("BEGIN", &[])?;
    match f(s) {
        Ok(v) => {
            s.execute("COMMIT", &[])?;
            Ok(v)
        }
        Err(e) => {
            let _ = s.execute("ROLLBACK", &[]);
            Err(e)
        }
    }
}

// ── Native driver: rusqlite ─────────────────────────────────────────────────
#[cfg(not(target_arch = "wasm32"))]
pub use native::RusqliteSql;

#[cfg(not(target_arch = "wasm32"))]
mod native {
    use super::*;
    use rusqlite::types::{ToSqlOutput, ValueRef};
    use rusqlite::{Connection, ToSql};
    use std::sync::{Arc, Mutex, MutexGuard};

    impl From<rusqlite::Error> for DomainError {
        fn from(e: rusqlite::Error) -> Self {
            // Carries SQLite's own text — trigger `RAISE(ABORT)` strings and FTS
            // `MATCH` syntax errors — which `classify` maps to the right code.
            DomainError::from_db_message(e.to_string())
        }
    }

    impl ToSql for Value {
        fn to_sql(&self) -> rusqlite::Result<ToSqlOutput<'_>> {
            Ok(match self {
                Value::Null => ToSqlOutput::Borrowed(ValueRef::Null),
                Value::Int(i) => ToSqlOutput::Borrowed(ValueRef::Integer(*i)),
                Value::Real(f) => ToSqlOutput::Borrowed(ValueRef::Real(*f)),
                Value::Text(s) => ToSqlOutput::Borrowed(ValueRef::Text(s.as_bytes())),
                Value::Blob(b) => ToSqlOutput::Borrowed(ValueRef::Blob(b)),
            })
        }
    }

    fn value_of(r: ValueRef<'_>) -> Value {
        match r {
            ValueRef::Null => Value::Null,
            ValueRef::Integer(i) => Value::Int(i),
            ValueRef::Real(f) => Value::Real(f),
            ValueRef::Text(t) => Value::Text(String::from_utf8_lossy(t).into_owned()),
            ValueRef::Blob(b) => Value::Blob(b.to_vec()),
        }
    }

    /// The native `Sql`: a shared, mutex-guarded rusqlite connection.
    #[derive(Clone)]
    pub struct RusqliteSql {
        conn: Arc<Mutex<Connection>>,
    }

    impl RusqliteSql {
        pub fn new(conn: Connection) -> Self {
            Self {
                conn: Arc::new(Mutex::new(conn)),
            }
        }
    }

    impl Sql for RusqliteSql {
        fn session(&self) -> DResult<Box<dyn Session + '_>> {
            let guard = self
                .conn
                .lock()
                .map_err(|_| DomainError::internal("store mutex poisoned"))?;
            Ok(Box::new(RusqliteSession { conn: guard }))
        }
    }

    struct RusqliteSession<'a> {
        conn: MutexGuard<'a, Connection>,
    }

    impl Session for RusqliteSession<'_> {
        fn execute(&mut self, sql: &str, params: &[Value]) -> DResult<Exec> {
            let changes = self
                .conn
                .execute(sql, rusqlite::params_from_iter(params.iter()))?;
            Ok(Exec {
                changes: changes as u64,
                last_insert_rowid: self.conn.last_insert_rowid(),
            })
        }

        fn query(&mut self, sql: &str, params: &[Value]) -> DResult<Vec<Row>> {
            let mut stmt = self.conn.prepare(sql)?;
            let names: Vec<String> = stmt.column_names().iter().map(|s| s.to_string()).collect();
            let rows = stmt.query_map(rusqlite::params_from_iter(params.iter()), |r| {
                let cells = names
                    .iter()
                    .enumerate()
                    .map(|(i, name)| Ok((name.clone(), value_of(r.get_ref(i)?))))
                    .collect::<rusqlite::Result<Vec<_>>>()?;
                Ok(Row::from_cells(cells))
            })?;
            Ok(rows.collect::<rusqlite::Result<Vec<_>>>()?)
        }
    }
}
