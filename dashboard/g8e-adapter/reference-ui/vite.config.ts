import { defineConfig } from 'vite';

export default defineConfig({
  root: 'reference-ui',
  build: {
    outDir: 'reference-ui/dist',
    emptyOutDir: true,
  },
  server: {
    port: 5174,
  },
});
