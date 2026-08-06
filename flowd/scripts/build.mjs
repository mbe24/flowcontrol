// scripts/build.mjs — cargo build via the Docker-fallback runner.
import { runCargo } from './native-runner.mjs';
const r = runCargo(['build', ...process.argv.slice(2)]);
process.exit(r.status ?? 1);
