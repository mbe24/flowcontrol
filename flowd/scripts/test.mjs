// scripts/test.mjs — cargo test via the Docker-fallback runner.
import { runCargo } from './native-runner.mjs';
process.exit((runCargo(['test', ...process.argv.slice(2)]) ?? {}).status ?? 1);
