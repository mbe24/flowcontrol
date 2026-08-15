// Serve the flowui SPA from the daemon itself — same origin as the FlowService
// API. That is the settled transport decision (design.transport.md): one origin
// removes mixed-content, CORS, certs and Private-Network-Access preflights in one
// move. Used as connect-node's `fallback`, so RPC paths hit the service and
// everything else is a static asset (with an index.html fallback for SPA routes).
import { createReadStream, existsSync, readFileSync, statSync } from "node:fs";
import type { IncomingMessage, ServerResponse } from "node:http";
import { basename, extname, join, normalize } from "node:path";

const MIME: Record<string, string> = {
  ".html": "text/html; charset=utf-8",
  ".js": "text/javascript",
  ".mjs": "text/javascript",
  ".css": "text/css",
  ".wasm": "application/wasm",
  ".json": "application/json",
  ".svg": "image/svg+xml",
  ".png": "image/png",
  ".ico": "image/x-icon",
  ".woff2": "font/woff2",
  ".map": "application/json"
};

/** A static-file request handler rooted at `dir`, with SPA index.html fallback. If
 *  `token` is given it is injected into index.html as a <meta> tag so the
 *  same-origin SPA can read it and authenticate its RPCs. */
export function staticHandler(dir: string, token?: string) {
  return (req: IncomingMessage, res: ServerResponse): void => {
    try {
      let p = decodeURIComponent(new URL(req.url ?? "/", "http://localhost").pathname);
      if (p === "/") p = "/index.html";
      // Strip any leading `../` so a crafted path can't escape the bundle dir.
      let file = join(dir, normalize(p).replace(/^(\.\.[/\\])+/, ""));
      if (!existsSync(file) || !statSync(file).isFile()) {
        file = join(dir, "index.html"); // SPA client-side route → shell
      }
      if (!existsSync(file)) {
        res.statusCode = 404;
        res.end("not found");
        return;
      }
      // index.html gets the token woven in; everything else streams as-is.
      if (token && basename(file) === "index.html") {
        const html = readFileSync(file, "utf8").replace(
          "</head>",
          `  <meta name="flow-token" content="${token}" />\n</head>`
        );
        res.setHeader("Content-Type", "text/html; charset=utf-8");
        res.end(html);
        return;
      }
      res.setHeader("Content-Type", MIME[extname(file)] ?? "application/octet-stream");
      createReadStream(file).pipe(res);
    } catch {
      res.statusCode = 500;
      res.end("static error");
    }
  };
}
