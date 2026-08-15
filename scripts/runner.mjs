// scripts/runner.mjs — pick native vs Docker for the Rust/Go tasks, honoring the
// FLOW_RUNNER env var (auto | local | docker).
//
//   local   — run the tool natively on the host (never Docker).
//   docker  — run inside a container (the Windows host blocks executing freshly
//             built binaries; a container's own execution isn't blocked).
//   auto    — the default: native on Linux/macOS (incl. WSL, where native exec
//             works); Docker on Windows. So in WSL, `auto` and `local` both stay
//             native — only FLOW_RUNNER=docker starts Docker.
//
// CI does NOT use this — it invokes cargo/go directly on native Linux runners.
import { spawnSync } from 'node:child_process';

const RUNNER = (process.env.FLOW_RUNNER || 'auto').toLowerCase();
const task = process.argv[2];

// Each task has a `native` command (run from `cwd`) and a `docker` command (run
// from the repo root, where docker-compose.yml lives).
const TASKS = {
  'test:flowd': {
    cwd: 'flowd',
    native: ['cargo', 'test'],
    docker: ['docker', 'compose', 'run', '--rm', 'flowd', 'cargo', 'test']
  },
  'build:flowd': {
    cwd: 'flowd',
    native: ['cargo', 'build'],
    docker: ['docker', 'compose', 'run', '--rm', 'flowd', 'cargo', 'build']
  },
  'check:flowd': {
    cwd: 'flowd',
    native: ['sh', '-c', 'cargo fmt --check && cargo clippy --all-targets -- -D warnings'],
    docker: [
      'docker', 'compose', 'run', '--rm', 'flowd', 'sh', '-c',
      'rustup component add rustfmt clippy >/dev/null 2>&1 && cargo fmt --check && cargo clippy --all-targets -- -D warnings'
    ]
  },
  'test:flowcli': {
    cwd: 'flowcli',
    native: ['go', 'test', './...'],
    docker: ['docker', 'compose', 'run', '--rm', 'flowcli-test', 'go', 'test', './...']
  }
};

const def = TASKS[task];
if (!def) {
  console.error(`[flow-runner] unknown task: ${task} (known: ${Object.keys(TASKS).join(', ')})`);
  process.exit(2);
}

let mode = RUNNER;
if (mode === 'auto') mode = process.platform === 'win32' ? 'docker' : 'local';
if (mode !== 'local' && mode !== 'docker') {
  console.error(`[flow-runner] FLOW_RUNNER must be auto|local|docker, got "${RUNNER}"`);
  process.exit(2);
}

const cmd = mode === 'docker' ? def.docker : def.native;
const cwd = mode === 'docker' ? undefined : def.cwd; // docker compose runs from repo root
console.error(`[flow-runner] ${task} via ${mode}${cwd ? ` (cwd ${cwd})` : ''}`);

const r = spawnSync(cmd[0], cmd.slice(1), { stdio: 'inherit', cwd });
if (r.error) {
  console.error(`[flow-runner] failed to start ${cmd[0]}: ${r.error.message}`);
  process.exit(1);
}
process.exit(r.status ?? 1);
