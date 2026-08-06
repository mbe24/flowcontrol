//! Database setup: pool creation, migrations, and dev fixtures.

use sqlx::sqlite::SqlitePool;

/// Open a pool and run the embedded migrations. `url` is `:memory:` for tests
/// or `sqlite://<path>` for a persistent file.
pub async fn open(url: &str) -> Result<SqlitePool, sqlx::Error> {
    let pool = SqlitePool::connect(url).await?;
    sqlx::migrate!("./migrations").run(&pool).await?;
    Ok(pool)
}

/// Seed a small fixture project so read handlers return real data without a
/// server round-trip. Mirrors the fixture data used by the TUI/web demo.
pub async fn seed(pool: &SqlitePool) -> Result<(), sqlx::Error> {
    sqlx::query("INSERT INTO projects (id, name, description) VALUES ('prj-travel', 'Travel Webapp', 'Booking flow, auth and payments.')")
        .execute(pool).await?;
    sqlx::query("INSERT INTO projects (id, name, description, archived_at) VALUES ('prj-docs', 'Developer Docs', 'Public API reference.', unixepoch())")
        .execute(pool).await?;

    sqlx::query("INSERT INTO nodes (id, project_id, parent_id, kind, title, description, condition, declared_status, wp_state, position) VALUES
        ('WP-AUTH', 'prj-travel', NULL, 'WORK_PACKAGE', 'Authentication Infrastructure', '', '', 'OPEN', 'ACTIVE', 100)")
        .execute(pool).await?;
    sqlx::query("INSERT INTO nodes (id, project_id, parent_id, kind, title, description, condition, declared_status, position) VALUES
        ('T-1042', 'prj-travel', 'WP-AUTH', 'TASK', 'OAuth2 device-code flow for the CLI', 'The TUI and MCP server both authenticate headlessly.', 'pnpm test:auth --grep device', 'OPEN', 100)")
        .execute(pool).await?;
    sqlx::query("INSERT INTO nodes (id, project_id, parent_id, kind, title, description, condition, declared_status, position) VALUES
        ('T-1043', 'prj-travel', 'WP-AUTH', 'TASK', 'Refresh-token rotation', 'Rotate on every refresh.', '', 'DONE', 200)")
        .execute(pool).await?;
    sqlx::query("INSERT INTO dependencies (blocker_id, blocked_id) VALUES ('T-1042', 'T-1043')")
        .execute(pool).await?;

    sqlx::query("INSERT INTO events (project_id, node_id, kind, author, summary, payload) VALUES
        ('prj-travel', 'T-1042', 'NODE_CREATED', 'seed', 'created T-1042', '{\"before\":null,\"after\":{}}')")
        .execute(pool).await?;
    Ok(())
}
