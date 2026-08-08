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
    // Project site → base = /<repo>/; dev and preview = '' (root).
    base: process.env.BASE_PATH || ''
  };
});