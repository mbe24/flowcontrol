import { defineConfig, loadEnv } from 'vite';
import { svelte } from '@sveltejs/vite-plugin-svelte';

export default defineConfig(({ mode }) => {
  // Deployed origin for the social-preview meta, read from the mode's env files
  // (VITE_SITE_URL in .env.demo). Unset -> the meta falls back to a relative
  // path instead of leaking a literal token.
  const site = (loadEnv(mode, '.', '') as Record<string, string>).VITE_SITE_URL || '';

  return {
    plugins: [
      svelte(),
      {
        name: 'social-preview-meta',
        transformIndexHtml: (html) => html.replace(/%VITE_SITE_URL%/g, site)
      }
    ],
    server: { port: 5173 },
    // sqlite-wasm ships its own .wasm and must not be pre-bundled/transformed by
    // esbuild, or the OPFS worker glue breaks.
    optimizeDeps: { exclude: ['@sqlite.org/sqlite-wasm'] },
    worker: { format: 'es' },
    // Project site → base = /<repo>/; dev and preview = '' (root).
    base: process.env.BASE_PATH || ''
  };
});