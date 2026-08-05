import { defineConfig } from 'vite';
import { svelte } from '@sveltejs/vite-plugin-svelte';

export default defineConfig({
  plugins: [svelte()],
  server: { port: 5173 },
  // Project site → base = /<repo>/; dev and preview = '' (root).
  base: process.env.BASE_PATH || ''
});
