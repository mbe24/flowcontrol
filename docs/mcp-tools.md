# MCP tools

`flowmcp` exposes the task graph to your agent as MCP tools. Start with `list_projects` to get
a `project_id`, then `get_project` to load the tree; everything else edits it.

## Projects

| Tool | What it does |
| --- | --- |
| `list_projects` | List projects — id, name, description, archived. Start here. |
| `create_project` | Create a project (the namespace all nodes live in). |
| `get_project` | Load a project's node tree with status, dependency edges, and the `seq` cursor. |
| `update_project` | Edit a project's name/description, or archive it. |

## Nodes

| Tool | What it does |
| --- | --- |
| `create_node` | Create a `WORK_PACKAGE`, `TASK`, or `STEP` (parent required for tasks and steps). |
| `update_node` | Edit a node — title, description, condition, note, position, reference. |
| `move_node` | Promote / demote / reparent / reorder a node. |
| `delete_node` | Remove a node. |

## Status & dependencies

| Tool | What it does |
| --- | --- |
| `set_status` | Set the declared status: `OPEN`, `DEFERRED`, or `DONE`. `READY`/`BLOCKED` are derived. |
| `set_dependency` | Add or remove a blocker → blocked edge (cycles rejected). |
| `report_condition` | Verify a task's free-text condition (the "how you know it's done" check). |

## Collaboration & history

| Tool | What it does |
| --- | --- |
| `add_comment` | Attach a comment to a node. |
| `list_events` | The append-only change log. |
| `poll_changes` | What changed since a `seq` cursor — the basis for live updates. |
| `search` | Full-text search across the graph. |
| `undo` | Revert the last change. |

!!! tip "Decompose into steps"
    Work packages and tasks are the skeleton; the real execution lives in **steps**. After
    creating a task, break it into verifiable steps — that is what makes the graph actionable
    for both agents and humans.
