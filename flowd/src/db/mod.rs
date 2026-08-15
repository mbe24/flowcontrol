//! Database setup: open a connection, run migrations, and seed dev fixtures.
//!
//! Synchronous (rusqlite) — SQLite is a synchronous, in-process library. The
//! async lives at the gRPC edge, not here.

use rusqlite::Connection;

use crate::error::DomainError;

/// The embedded schema. `sqlx::migrate!` had no rusqlite analog, so we run it
/// ourselves guarded by `PRAGMA user_version` (a one-migration ratchet for now).
const SCHEMA: &str = include_str!("../../migrations/0001_initial_schema.sql");

/// Open a connection and bring it up to the current schema. `url` is `:memory:`
/// for tests or a file path (an optional `sqlite://` prefix is stripped) for a
/// persistent DB.
pub fn open(url: &str) -> Result<Connection, DomainError> {
    let conn = match url {
        ":memory:" => Connection::open_in_memory(),
        path => Connection::open(path.strip_prefix("sqlite://").unwrap_or(path)),
    }
    .map_err(|e| DomainError::internal(format!("open db: {e}")))?;

    // Per-connection pragmas. FK enforcement is required for the schema's cascade
    // deletes; WAL + a busy timeout smooth concurrent reads on a file DB (both are
    // no-ops on `:memory:`). The browser host cannot WAL — that pragma is a Host
    // concern there, not part of the shared schema.
    conn.execute_batch(
        "PRAGMA foreign_keys = ON;
         PRAGMA journal_mode = WAL;
         PRAGMA busy_timeout = 5000;",
    )
    .map_err(|e| DomainError::internal(format!("pragmas: {e}")))?;

    let version: i64 = conn
        .query_row("PRAGMA user_version", [], |r| r.get(0))
        .map_err(|e| DomainError::internal(format!("read user_version: {e}")))?;
    if version < 1 {
        conn.execute_batch(SCHEMA)
            .map_err(|e| DomainError::internal(format!("apply schema: {e}")))?;
        conn.execute_batch("PRAGMA user_version = 1;")
            .map_err(|e| DomainError::internal(format!("set user_version: {e}")))?;
    }
    Ok(conn)
}

/// Seed a small fixture project so read handlers return real data without a
/// server round-trip. Mirrors the fixture data used by the TUI/web demo.
pub fn seed(conn: &Connection) -> Result<(), DomainError> {
    conn.execute_batch(
        "INSERT INTO projects (id, name, description) VALUES ('prj-travel', 'Travel Webapp', 'Booking flow, auth and payments.');
         INSERT INTO projects (id, name, description, archived_at) VALUES ('prj-docs', 'Developer Docs', 'Public API reference.', unixepoch());

         INSERT INTO nodes (id, project_id, parent_id, kind, title, description, condition, declared_status, wp_state, position) VALUES
             ('WP-AUTH', 'prj-travel', NULL, 'WORK_PACKAGE', 'Authentication Infrastructure', '', '', 'OPEN', 'ACTIVE', 100);
         INSERT INTO nodes (id, project_id, parent_id, kind, title, description, condition, declared_status, position) VALUES
             ('T-1042', 'prj-travel', 'WP-AUTH', 'TASK', 'OAuth2 device-code flow for the CLI', 'The TUI and MCP server both authenticate headlessly.', 'pnpm test:auth --grep device', 'OPEN', 100);
         INSERT INTO nodes (id, project_id, parent_id, kind, title, description, condition, declared_status, position) VALUES
             ('T-1043', 'prj-travel', 'WP-AUTH', 'TASK', 'Refresh-token rotation', 'Rotate on every refresh.', '', 'DONE', 200);
         INSERT INTO dependencies (blocker_id, blocked_id) VALUES ('T-1042', 'T-1043');

         INSERT INTO events (project_id, node_id, kind, author, summary, payload) VALUES
             ('prj-travel', 'T-1042', 'NODE_CREATED', 'seed', 'created T-1042', '{\"before\":null,\"after\":{}}');",
    )
    .map_err(|e| DomainError::internal(format!("seed: {e}")))?;
    Ok(())
}
