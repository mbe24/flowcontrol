//! Database setup: the schema/seed SQL (all targets) plus native helpers to open
//! a rusqlite connection, run migrations, and seed dev fixtures.
//!
//! The SQL strings are target-agnostic so the wasm host (`flowd.js` over
//! `node:sqlite`, or the browser worker) can apply the SAME schema to its own
//! SQLite. The rusqlite `open`/`seed` helpers are native-only — on wasm the host
//! owns the connection.

/// The embedded schema. `sqlx::migrate!` had no rusqlite analog, so we run it
/// ourselves guarded by `PRAGMA user_version` (a one-migration ratchet for now).
/// Public so the wasm host can bring its own SQLite to the same schema.
pub const SCHEMA_SQL: &str = include_str!("../../migrations/0001_initial_schema.sql");

/// Dev-fixture seed: a small project so read handlers return real data without a
/// server round-trip. Mirrors the fixture used by the TUI/web demo. Public so the
/// wasm host can seed its own SQLite.
pub const SEED_SQL: &str = "INSERT INTO projects (id, name, description) VALUES ('prj-travel', 'Travel Webapp', 'Booking flow, auth and payments.');
     INSERT INTO projects (id, name, description, archived_at) VALUES ('prj-docs', 'Developer Docs', 'Public API reference.', unixepoch());

     INSERT INTO nodes (id, project_id, parent_id, kind, title, description, condition, declared_status, wp_state, position) VALUES
         ('WP-AUTH', 'prj-travel', NULL, 'WORK_PACKAGE', 'Authentication Infrastructure', '', '', 'OPEN', 'ACTIVE', 100);
     INSERT INTO nodes (id, project_id, parent_id, kind, title, description, condition, declared_status, position) VALUES
         ('T-1042', 'prj-travel', 'WP-AUTH', 'TASK', 'OAuth2 device-code flow for the CLI', 'The TUI and MCP server both authenticate headlessly.', 'pnpm test:auth --grep device', 'OPEN', 100);
     INSERT INTO nodes (id, project_id, parent_id, kind, title, description, condition, declared_status, position) VALUES
         ('T-1043', 'prj-travel', 'WP-AUTH', 'TASK', 'Refresh-token rotation', 'Rotate on every refresh.', '', 'DONE', 200);
     INSERT INTO dependencies (blocker_id, blocked_id) VALUES ('T-1042', 'T-1043');

     INSERT INTO events (project_id, node_id, kind, author, summary, payload) VALUES
         ('prj-travel', 'T-1042', 'NODE_CREATED', 'seed', 'created T-1042', '{\"before\":null,\"after\":{}}');";

/// The per-connection pragmas the native host applies. FK enforcement is required
/// for the schema's cascade deletes; WAL + a busy timeout smooth concurrent reads
/// on a file DB (both no-ops on `:memory:`). The browser host cannot WAL — pragmas
/// are a host concern there, not part of the shared schema.
pub const PRAGMAS_SQL: &str = "PRAGMA foreign_keys = ON;
     PRAGMA journal_mode = WAL;
     PRAGMA busy_timeout = 5000;";

// ── Native connection helpers (rusqlite) ─────────────────────────────────────
#[cfg(not(target_arch = "wasm32"))]
pub use native::{open, seed};

#[cfg(not(target_arch = "wasm32"))]
mod native {
    use rusqlite::Connection;

    use super::{PRAGMAS_SQL, SCHEMA_SQL, SEED_SQL};
    use crate::error::DomainError;

    /// Open a connection and bring it up to the current schema. `url` is `:memory:`
    /// for tests or a file path (an optional `sqlite://` prefix is stripped) for a
    /// persistent DB.
    pub fn open(url: &str) -> Result<Connection, DomainError> {
        let conn = match url {
            ":memory:" => Connection::open_in_memory(),
            path => Connection::open(path.strip_prefix("sqlite://").unwrap_or(path)),
        }
        .map_err(|e| DomainError::internal(format!("open db: {e}")))?;

        conn.execute_batch(PRAGMAS_SQL)
            .map_err(|e| DomainError::internal(format!("pragmas: {e}")))?;

        let version: i64 = conn
            .query_row("PRAGMA user_version", [], |r| r.get(0))
            .map_err(|e| DomainError::internal(format!("read user_version: {e}")))?;
        if version < 1 {
            conn.execute_batch(SCHEMA_SQL)
                .map_err(|e| DomainError::internal(format!("apply schema: {e}")))?;
            conn.execute_batch("PRAGMA user_version = 1;")
                .map_err(|e| DomainError::internal(format!("set user_version: {e}")))?;
        }
        Ok(conn)
    }

    /// Seed the dev fixture ([`SEED_SQL`]) into an open connection.
    pub fn seed(conn: &Connection) -> Result<(), DomainError> {
        conn.execute_batch(SEED_SQL)
            .map_err(|e| DomainError::internal(format!("seed: {e}")))?;
        Ok(())
    }
}
