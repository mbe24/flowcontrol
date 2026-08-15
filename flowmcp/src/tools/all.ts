import { getProjectTool } from "./getProject";
import {
  listEventsTool,
  listProjectsTool,
  pollChangesTool,
  searchTool,
} from "./reads";
import type { ToolDef } from "./registry";
import {
  addCommentTool,
  createNodeTool,
  createProjectTool,
  deleteNodeTool,
  moveNodeTool,
  reportConditionTool,
  setDependencyTool,
  setStatusTool,
  undoTool,
  updateNodeTool,
  updateProjectTool,
} from "./writes";

/** The complete v1 tool set (plan.flowmcp.tools.md). Registry derives tools/list. */
export const tools: ToolDef[] = [
  // Wave 1 — the agent happy path
  listProjectsTool,
  getProjectTool,
  createProjectTool,
  createNodeTool,
  updateNodeTool,
  setStatusTool,
  setDependencyTool,
  reportConditionTool,
  addCommentTool,
  pollChangesTool,
  // Wave 2
  searchTool,
  moveNodeTool,
  deleteNodeTool,
  undoTool,
  updateProjectTool,
  listEventsTool,
];
