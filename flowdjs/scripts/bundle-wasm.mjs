// Copy the wasm-bindgen (nodejs) glue into dist/wasm so the published package is
// self-contained — store.ts loads it from ./wasm at runtime. Runs after tsup.
// Generate the glue first with `npm run gen:wasm` (produces flowwasm/pkg).
import { cpSync, existsSync, mkdirSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const here = dirname(fileURLToPath(import.meta.url)); // flowdjs/scripts
const src = join(here, "..", "..", "flowwasm", "pkg"); // repo/flowwasm/pkg
const dest = join(here, "..", "dist", "wasm"); // flowdjs/dist/wasm

if (!existsSync(join(src, "flowwasm.js"))) {
  console.error(`bundle-wasm: missing ${src} — run \`npm run gen:wasm\` first`);
  process.exit(1);
}
mkdirSync(dest, { recursive: true });
cpSync(src, dest, { recursive: true });
// The wasm-bindgen nodejs glue is CommonJS. flowdjs is "type":"module", so mark
// this vendored dir as CommonJS or node parses the .js as ESM (exports is not defined).
writeFileSync(join(dest, "package.json"), JSON.stringify({ type: "commonjs" }) + "\n");
console.log("bundle-wasm: copied glue → dist/wasm (+ CommonJS marker)");
