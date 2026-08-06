// scripts/native-runner.mjs — Docker-fallback runner for flowd, modelled on the
// pattern in C:\Users\MikaelBeyene\Development\chatletting\scripts\native-runner.mjs.
//
// Compiles and tests flowd locally when the host allows it, else falls back to
// Docker (the maintainer host blocks native build scripts — cargo fails with
// "Access is denied" on build-script-build). Mounts the repo at /work, uses
// named cache volumes for the cargo registry and target dir, and redirects
// CARGO_TARGET_DIR off the mount so container rebuilds are incremental.
import { spawnSync } from 'node:child_process';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

export const REPO = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..', '..');
export const CRATE = path.join(REPO, 'flowd');
export const RUNNER = (process.env.FLOWD_NATIVE_RUNNER || 'auto').toLowerCase();
export const IMAGE = process.env.FLOWD_NATIVE_IMAGE || 'rust:1-slim-bookworm';
export const REGISTRY_VOLUME = 'flowd-cargo';
export const TARGET_VOLUME = 'flowd-target';

export function dockerAvailable() {
  const r = spawnSync('docker', ['version', '--format', '{{.Server.Version}}'], { encoding: 'utf8' });
  return r.status === 0 && (r.stdout || '').trim().length > 0;
}

export function dockerRunArgs(cmd, { env = {}, args = [] } = {}) {
  const base = [
    'run', '--rm',
    '-v', `${REPO}:/work`,
    '-v', `${REGISTRY_VOLUME}:/usr/local/cargo/registry`,
    '-v', `${TARGET_VOLUME}:/tmp/target`,
    '-w', '/work/flowd',
    '-e', 'CARGO_TARGET_DIR=/tmp/target',
    IMAGE,
    cmd,
  ];
  for (const a of args) base.push(a);
  return base;
}

export function runCargo(cargoArgs) {
  const localRes = () => spawnSync('cargo', cargoArgs, { stdio: 'inherit' });
  const dockerRes = () => {
    const r = spawnSync('docker', dockerRunArgs('cargo', { args: cargoArgs }), { stdio: 'inherit' });
    return r;
  };
  if (RUNNER === 'docker') return dockerRes();
  if (RUNNER === 'local') return localRes();
  // auto: try local, fall back to Docker on failure
  try {
    const r = localRes();
    if (r.status === 0) return r;
  } catch {}
  if (dockerAvailable()) {
    console.error('[flowd runner=auto] local cargo failed; falling back to Docker');
    return dockerRes();
  }
  console.error('[flowd] cargo blocked and Docker unavailable — install Docker or run on Linux/WSL.');
  process.exit(1);
}
