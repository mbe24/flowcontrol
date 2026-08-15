-- fctrl core schema — SQLite (WAL), designed for sqlx migrations.
--
-- Why SQLite and plain SQL rather than an ORM: the interesting queries here are
-- recursive graph walks (critical path, hops-from-ready, cycle detection).
-- Diesel and SeaORM both make you drop to raw SQL for those, so the ORM buys
-- nothing but a second place for the schema to live. (flowd uses runtime-checked
-- sqlx::query — no compile-time query macros, no .sqlx offline cache — so this
-- schema file is the single source of truth for the store.)
--
--   sqlx migrate add -r initial_schema
--   sqlx migrate run
--
-- Run once per connection:
--   PRAGMA journal_mode = WAL;
--   PRAGMA foreign_keys = ON;
--   PRAGMA synchronous = NORMAL;
--   PRAGMA busy_timeout = 5000;

-- ─────────────────────────────────────────────────────────────────────────────
-- projects
-- ─────────────────────────────────────────────────────────────────────────────

CREATE TABLE projects (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    archived_at INTEGER,
    created_at  INTEGER NOT NULL DEFAULT (unixepoch()),
    updated_at  INTEGER NOT NULL DEFAULT (unixepoch())
) STRICT;

-- ─────────────────────────────────────────────────────────────────────────────
-- nodes — the flat-node model from datamodel.md v1.0
--
-- One table for work packages, tasks and steps. `kind` and `parent_id` carry
-- the hierarchy; a CHECK enforces that the hierarchy stays three deep and that
-- each kind sits under the right parent.
--
-- IMPORTANT: `declared_status` is what a human or agent SET. READY and BLOCKED
-- are DERIVED from dependencies and are never stored here — see node_state.
-- Storing them would give you two sources of truth for the same fact, and the
-- cascade would have to keep them in sync on every write.
-- ─────────────────────────────────────────────────────────────────────────────

CREATE TABLE nodes (
    id          TEXT PRIMARY KEY,
    project_id  TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    parent_id   TEXT REFERENCES nodes(id) ON DELETE CASCADE,
    kind        TEXT NOT NULL CHECK (kind IN ('WORK_PACKAGE', 'TASK', 'STEP')),

    title       TEXT NOT NULL,
    -- Markdown. Rendered by the clients, never by the core.
    description TEXT NOT NULL DEFAULT '',
    -- Free text the agent is expected to satisfy and verify. The core NEVER
    -- executes this.
    condition   TEXT NOT NULL DEFAULT '',
    -- External reference (plain text, e.g. JIRA-123 or a URL). Nullable.
    reference   TEXT,
    -- STEP body — a few sentences, shown on expand. '' for WP/TASK (mirrors
    -- `description`: "no note" and "empty note" are not a meaningful distinction).
    note        TEXT NOT NULL DEFAULT '',

    -- OPEN is the neutral state; the engine decides READY vs BLOCKED from it.
    declared_status TEXT NOT NULL DEFAULT 'OPEN'
        CHECK (declared_status IN ('OPEN', 'DEFERRED', 'DONE')),

    -- WORK_PACKAGE only. Set explicitly, not derived — an addition to v1.0.
    wp_state    TEXT CHECK (wp_state IN ('PLANNED', 'ACTIVE', 'DONE', 'ARCHIVED')),

    -- Sibling ordering. Sparse (100, 200, 300…) so a reorder is one UPDATE.
    position    INTEGER NOT NULL DEFAULT 0,

    created_at  INTEGER NOT NULL DEFAULT (unixepoch()),
    updated_at  INTEGER NOT NULL DEFAULT (unixepoch()),

    CHECK (
        (kind = 'WORK_PACKAGE' AND parent_id IS NULL     AND wp_state IS NOT NULL) OR
        (kind = 'TASK'         AND parent_id IS NOT NULL AND wp_state IS NULL)     OR
        (kind = 'STEP'         AND parent_id IS NOT NULL AND wp_state IS NULL)
    )
) STRICT;

CREATE INDEX nodes_project_idx ON nodes(project_id, kind);
CREATE INDEX nodes_parent_idx  ON nodes(parent_id, position);

-- Enforce that a TASK's parent is a WORK_PACKAGE and a STEP's parent is a TASK.
-- A CHECK cannot see another row, so this is a trigger.
CREATE TRIGGER nodes_parent_kind_insert
BEFORE INSERT ON nodes
WHEN NEW.parent_id IS NOT NULL
BEGIN
    SELECT RAISE(ABORT, 'invalid parent kind for node')
    WHERE (SELECT kind FROM nodes WHERE id = NEW.parent_id)
          IS NOT (CASE NEW.kind WHEN 'TASK' THEN 'WORK_PACKAGE' WHEN 'STEP' THEN 'TASK' END);
END;

-- The UPDATE counterpart, for MoveNode (reparent / kind change). Closes three
-- holes: (0) a node cannot be its own parent — in a BEFORE UPDATE the parent-kind
-- subquery reads the pre-update row, so MoveNode(X, parent=X) would otherwise
-- validate and create a 1-node cycle; (1) parent must be the right kind; (2) no
-- cross-project reparenting; (3) children must remain valid under the new kind
-- (runs unconditionally so it also catches a promotion to WORK_PACKAGE that would
-- strand STEP children). The move handler deletes step children before a demote,
-- so in the sanctioned path clause 3 sees no child; it is a backstop.
CREATE TRIGGER nodes_parent_kind_update
BEFORE UPDATE OF parent_id, kind ON nodes
BEGIN
    SELECT RAISE(ABORT, 'node cannot be its own parent')
    WHERE NEW.parent_id = NEW.id;

    SELECT RAISE(ABORT, 'invalid parent kind for node')
    WHERE NEW.parent_id IS NOT NULL
      AND (SELECT kind FROM nodes WHERE id = NEW.parent_id)
          IS NOT (CASE NEW.kind WHEN 'TASK' THEN 'WORK_PACKAGE' WHEN 'STEP' THEN 'TASK' END);

    SELECT RAISE(ABORT, 'cross-project move')
    WHERE NEW.parent_id IS NOT NULL
      AND (SELECT project_id FROM nodes WHERE id = NEW.parent_id) <> NEW.project_id;

    SELECT RAISE(ABORT, 'children invalid for new kind')
    WHERE EXISTS (
        SELECT 1 FROM nodes c
        WHERE c.parent_id = NEW.id
          AND c.kind IS NOT (CASE NEW.kind WHEN 'WORK_PACKAGE' THEN 'TASK'
                                           WHEN 'TASK'         THEN 'STEP' END)
        -- CASE is NULL for NEW.kind='STEP' ⇒ any child trips the guard.
    );
END;

CREATE TRIGGER nodes_touch
AFTER UPDATE ON nodes
BEGIN
    UPDATE nodes SET updated_at = unixepoch() WHERE id = NEW.id;
END;

-- ─────────────────────────────────────────────────────────────────────────────
-- verifications — 1:1 with a node, but its own table
--
-- Separate because it churns on a completely different schedule from the node
-- (agents write it constantly, the node barely changes), and because a null row
-- says "never reported" more honestly than five nullable columns.
--
-- agent_result is what the agent REPORTED. human_verdict is the operator's
-- acceptance. They are independent on purpose: the interesting case is an
-- operator accepting over a reported failure, and both halves must survive.
-- ─────────────────────────────────────────────────────────────────────────────

CREATE TABLE verifications (
    node_id       TEXT PRIMARY KEY REFERENCES nodes(id) ON DELETE CASCADE,

    agent_result  TEXT CHECK (agent_result IN ('PASS', 'FAIL')),
    agent_name    TEXT,
    agent_at      INTEGER,
    -- The node's updated_at when the report landed. If the node has changed
    -- since, the client shows the report as stale rather than trusting it.
    agent_node_rev INTEGER,
    -- Whatever the agent wants to keep: exit code, failing assertions, log tail.
    agent_detail  TEXT NOT NULL DEFAULT '',

    human_verdict TEXT CHECK (human_verdict IN ('ACCEPTED', 'REJECTED')),
    human_at      INTEGER,

    CHECK (agent_result IS NULL OR agent_at IS NOT NULL),
    CHECK (human_verdict IS NULL OR human_at IS NOT NULL)
) STRICT;

-- ─────────────────────────────────────────────────────────────────────────────
-- dependencies — the DAG
--
-- Cross-level edges are legal (a STEP may block a WORK_PACKAGE); the clients
-- roll those up for display. The core stores them as authored.
-- ─────────────────────────────────────────────────────────────────────────────

CREATE TABLE dependencies (
    blocker_id TEXT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
    blocked_id TEXT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
    created_at INTEGER NOT NULL DEFAULT (unixepoch()),
    PRIMARY KEY (blocker_id, blocked_id),
    CHECK (blocker_id <> blocked_id)
) STRICT;

CREATE INDEX deps_blocked_idx ON dependencies(blocked_id);

-- Reject an edge that would close a cycle: if the new blocker is already
-- reachable downstream of the new blocked node, adding this edge makes a loop.
CREATE TRIGGER dependencies_no_cycle
BEFORE INSERT ON dependencies
BEGIN
    SELECT RAISE(ABORT, 'dependency would create a cycle')
    WHERE EXISTS (
        WITH RECURSIVE downstream(id) AS (
            SELECT NEW.blocked_id
            UNION
            SELECT d.blocked_id FROM dependencies d JOIN downstream x ON d.blocker_id = x.id
        )
        SELECT 1 FROM downstream WHERE id = NEW.blocker_id
    );
END;

-- ─────────────────────────────────────────────────────────────────────────────
-- events — append-only, and the spine of the whole system
--
-- This one table is doing three jobs at once, which is why it earns its place:
--   1. the activity feed the clients render
--   2. the change stream the clients subscribe to (Watch resumes from `seq`)
--   3. the undo log
--
-- `seq` is monotonic per database. A client that reconnects sends its last seq
-- and gets everything after it; if the gap is too large the server answers with
-- a Resync instead. That is the whole catch-up protocol.
-- ─────────────────────────────────────────────────────────────────────────────

CREATE TABLE events (
    seq        INTEGER PRIMARY KEY AUTOINCREMENT,
    project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    -- Null for node-less events (e.g. dependency edges, deletes).
    node_id    TEXT REFERENCES nodes(id) ON DELETE CASCADE,

    kind       TEXT NOT NULL CHECK (kind IN (
                   'NODE_CREATED', 'NODE_UPDATED', 'NODE_DELETED',
                   'STATUS_SET', 'DEP_ADDED', 'DEP_REMOVED',
                   'AGENT_REPORTED', 'VERDICT_SET', 'COMMENT'
               )),

    -- A plain name. Agents get no special treatment in the UI, so none here.
    author     TEXT NOT NULL,
    -- Human-readable line for the activity feed.
    summary    TEXT NOT NULL DEFAULT '',
    -- Machine-readable payload: the before/after that makes undo possible.
    payload    TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(payload)),

    created_at INTEGER NOT NULL DEFAULT (unixepoch())
) STRICT;

CREATE INDEX events_project_seq_idx ON events(project_id, seq);
CREATE INDEX events_node_idx        ON events(node_id, seq DESC);

-- ─────────────────────────────────────────────────────────────────────────────
-- node_state — the derived view the clients actually read
--
-- READY vs BLOCKED is computed here, once, rather than being written by every
-- code path that touches a node. Note it is NOT recursive: a node is blocked if
-- any direct blocker is not DONE, and that blocker's own blockers are its
-- problem. One join, no fixpoint.
-- ─────────────────────────────────────────────────────────────────────────────

CREATE VIEW node_state AS
SELECT
    n.*,
    CASE
        WHEN n.declared_status = 'DONE'     THEN 'DONE'
        WHEN n.declared_status = 'DEFERRED' THEN 'DEFERRED'
        WHEN EXISTS (
            SELECT 1 FROM dependencies d
            JOIN nodes b ON b.id = d.blocker_id
            WHERE d.blocked_id = n.id AND b.declared_status <> 'DONE'
        ) THEN 'BLOCKED'
        ELSE 'READY'
    END AS status,
    (SELECT count(*) FROM nodes c WHERE c.parent_id = n.id) AS child_count,
    (SELECT count(*) FROM nodes c WHERE c.parent_id = n.id AND c.declared_status = 'DONE') AS child_done_count
FROM nodes n;

-- NOTE: the wp_progress and node_depth views were removed here. They were unused
-- by flowd (progress_for inlines its own query), and wp_progress counted DECLARED
-- status while the live code counts EFFECTIVE status — a latent trap. Reintroduce
-- correctly (effective status; wired to a real read) when a query actually needs
-- them.

-- ─────────────────────────────────────────────────────────────────────────────
-- search — backs the ⌘K palette and the TUI finder
-- ─────────────────────────────────────────────────────────────────────────────

CREATE VIRTUAL TABLE nodes_fts USING fts5(
    id UNINDEXED,
    title,
    description,
    note,
    content = 'nodes',
    content_rowid = 'rowid',
    tokenize = 'unicode61'
);

CREATE TRIGGER nodes_fts_insert AFTER INSERT ON nodes BEGIN
    INSERT INTO nodes_fts(rowid, id, title, description, note)
    VALUES (NEW.rowid, NEW.id, NEW.title, NEW.description, NEW.note);
END;

CREATE TRIGGER nodes_fts_delete AFTER DELETE ON nodes BEGIN
    INSERT INTO nodes_fts(nodes_fts, rowid, id, title, description, note)
    VALUES ('delete', OLD.rowid, OLD.id, OLD.title, OLD.description, OLD.note);
END;

CREATE TRIGGER nodes_fts_update AFTER UPDATE ON nodes BEGIN
    INSERT INTO nodes_fts(nodes_fts, rowid, id, title, description, note)
    VALUES ('delete', OLD.rowid, OLD.id, OLD.title, OLD.description, OLD.note);
    INSERT INTO nodes_fts(rowid, id, title, description, note)
    VALUES (NEW.rowid, NEW.id, NEW.title, NEW.description, NEW.note);
END;

-- ─────────────────────────────────────────────────────────────────────────────
-- idempotency — write-retry ledger
--
-- WriteMeta carries an idempotency_key so an agent that retries a write (network
-- timeout, process restart) records exactly one event. Every mutation first
-- looks up (project_id, idempotency_key); if a row exists the request is a retry
-- and the store returns the recorded seq instead of writing again.
--
-- Keys are scoped per project because `seq` is unique per database, not per
-- project, and we want a retried `SetStatus` to return the same cursor.
-- ─────────────────────────────────────────────────────────────────────────────

CREATE TABLE idempotency (
    -- Dedup scope. A real project id for node writes; the empty string '' is a
    -- reserved sentinel scope for pre-creation writes (CreateProject, whose id is
    -- minted by the request itself). No FK to projects — the sentinel scope has no
    -- project row, and projects are never hard-deleted (there is no DeleteProject),
    -- so the ON DELETE CASCADE it used to carry bought nothing.
    project_id      TEXT NOT NULL,
    idempotency_key TEXT NOT NULL,
    seq             INTEGER NOT NULL,
    -- Id of the entity the original request minted (node or project), so a
    -- replayed retry can return it instead of an empty payload. NULL for
    -- non-create mutations.
    entity_id       TEXT,
    created_at      INTEGER NOT NULL DEFAULT (unixepoch()),
    PRIMARY KEY (project_id, idempotency_key)
) STRICT;
