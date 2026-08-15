// `flow ui` — the human's route into the shared overview. Ensures a daemon is up
// (spawning one if needed) and opens flowui in the browser at its URL. Because a
// human runs this from an ordinary, un-sandboxed shell, the daemon it spawns
// PERSISTS across agent-session teardowns — this is the reliable way to pin the
// shared store (see plan/design.daemon-lifecycle.md). The daemon serves flowui
// same-origin, so the opened URL is already wired to that store.
import { spawn } from "node:child_process";

import { ensureDaemon } from "./session";

/** Ensure the shared daemon and open the browser at its URL. Returns the addr. */
export async function launchUi(): Promise<string> {
  const session = await ensureDaemon({ spawnedBy: "user" });
  openBrowser(session.addr);
  return session.addr;
}

/** Open the OS default browser, cross-platform. No-op under FLOW_NO_OPEN (tests/headless). */
function openBrowser(url: string): void {
  if (process.env.FLOW_NO_OPEN) return;
  let cmd: string;
  let args: string[];
  if (process.platform === "win32") {
    cmd = "cmd";
    args = ["/c", "start", "", url]; // empty "" is start's window-title slot
  } else if (process.platform === "darwin") {
    cmd = "open";
    args = [url];
  } else {
    cmd = "xdg-open";
    args = [url];
  }
  try {
    spawn(cmd, args, { stdio: "ignore", detached: true }).unref();
  } catch {
    /* no browser launcher available — the URL is printed below */
  }
}

const addr = await launchUi();
console.error(`[flow ui] shared daemon at ${addr} — open ${addr} in your browser`);
