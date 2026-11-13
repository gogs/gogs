import { readFileSync, readdirSync } from "node:fs";
import { createRequire } from "node:module";
import path from "node:path";

import tailwindcss from "@tailwindcss/vite";
import react from "@vitejs/plugin-react";
import { type Plugin, defineConfig } from "vite";

const pdfjsDir = path.dirname(createRequire(import.meta.url).resolve("pdfjs-dist/package.json"));
const PDFJS_ASSET_DIRS = ["cmaps", "iccs", "standard_fonts", "wasm"];

// pdfjsAssets serves the files that PDF.js loads while it renders, e.g.,
// character maps and fonts, under "assets/pdfjs/" from the installed
// pdfjs-dist, so they always match its version.
function pdfjsAssets(): Plugin {
  return {
    name: "gogs:pdfjs-assets",
    configureServer(server) {
      server.middlewares.use("/assets/pdfjs/", (req, res, next) => {
        const [dir, name, ...rest] = decodeURIComponent((req.url ?? "").split("?")[0])
          .split("/")
          .filter(Boolean);
        if (
          !PDFJS_ASSET_DIRS.includes(dir) ||
          !name ||
          rest.length > 0 ||
          !readdirSync(path.join(pdfjsDir, dir)).includes(name)
        ) {
          next();
          return;
        }
        res.end(readFileSync(path.join(pdfjsDir, dir, name)));
      });
    },
    generateBundle() {
      for (const dir of PDFJS_ASSET_DIRS) {
        for (const name of readdirSync(path.join(pdfjsDir, dir))) {
          this.emitFile({
            type: "asset",
            fileName: `assets/pdfjs/${dir}/${name}`,
            source: readFileSync(path.join(pdfjsDir, dir, name)),
          });
        }
      }
    },
  };
}

export default defineConfig({
  base: "./",
  plugins: [react(), tailwindcss(), pdfjsAssets()],
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
