import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

// Only the environment is needed from Node here; avoids pulling @types/node.
declare const process: { env: Record<string, string | undefined> };

// The build goes straight into the Go embed directory so `go build` ships the UI.
// For `npm run dev`, set TORRE_TOKEN (from ~/.torre/run/server.json) to proxy the API.
const target = process.env.TORRE_URL ?? 'http://127.0.0.1:7420';
const auth = process.env.TORRE_TOKEN ? { Authorization: `Bearer ${process.env.TORRE_TOKEN}` } : undefined;

export default defineConfig({
  plugins: [react()],
  build: {
    outDir: '../internal/webui/dist',
    emptyOutDir: true,
    chunkSizeWarningLimit: 2000,
  },
  server: {
    proxy: {
      '/api': { target, changeOrigin: true, headers: auth },
      '/ws': { target, changeOrigin: true, ws: true, headers: auth },
    },
  },
});
