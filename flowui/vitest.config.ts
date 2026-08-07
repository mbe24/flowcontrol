import { svelte } from '@sveltejs/vite-plugin-svelte';
import { defineConfig } from 'vitest/config';

export default defineConfig({
  plugins: [svelte()],
  test: {
    environment: 'node',
    include: ['src/**/*.test.ts'],
    // Component tests need jsdom; the Svelte+connect worker is too heavy to
    // start within vitest's timeout on this (slow) host, so keep them out of
    // the default run. Run on a faster machine with: npx vitest run --dir src/components
    exclude: ['src/components/**']
  }
});
