import path from "node:path";

import tailwindcss from "@tailwindcss/vite";
import react from "@vitejs/plugin-react";
import { defineConfig } from "vite";

export default defineConfig({
  base: "./",
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: {
      "@": path.resolve(import.meta.dirname, "src"),
    },
  },
  server: {
    port: 5173,
    // The dev page is served by the Go server (e.g., https://gogs.localhost)
    // which reverse-proxies HTTP to this Vite dev server. That proxy is
    // HTTP-only, so the HMR client's WebSocket can't tunnel through it. Point
    // HMR's WS directly at the Vite dev port instead, bypassing gogs entirely.
    hmr: {
      protocol: "ws",
      host: "localhost",
      port: 5173,
    },
  },
  build: {
    outDir: "../public/dist",
    emptyOutDir: true,
  },
});
