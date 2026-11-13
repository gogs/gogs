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

// The license URLs of the bundled code by SPDX identifier, the ones LibreJS
// recognizes where it has one.
const LICENSE_URLS: Record<string, string> = {
  MIT: "http://www.jclark.com/xml/copying.txt",
  "Apache-2.0": "http://www.apache.org/licenses/LICENSE-2.0",
  ISC: "https://www.isc.org/downloads/software-support-policy/isc-license/",
  "BSD-2-Clause": "http://www.freebsd.org/copyright/freebsd-license.html",
  "BSD-3-Clause": "http://opensource.org/licenses/BSD-3-Clause",
  "MPL-2.0": "http://www.mozilla.org/MPL/2.0",
  "CC0-1.0": "http://creativecommons.org/publicdomain/zero/1.0/legalcode",
  Unlicense: "http://unlicense.org/UNLICENSE",
  "0BSD": "https://spdx.org/licenses/0BSD.html",
};

// licenseOf returns the license of the npm package that a bundled module is
// from, or Gogs's own license for a module of the web app.
function licenseOf(moduleID: string): { pkg: string; license: string } {
  const at = moduleID.lastIndexOf("/node_modules/");
  if (at < 0) return { pkg: "gogs", license: "MIT" };
  const parts = moduleID.slice(at + "/node_modules/".length).split("/");
  const name = parts[0].startsWith("@") ? `${parts[0]}/${parts[1]}` : parts[0];
  const manifest = JSON.parse(
    readFileSync(path.join(moduleID.slice(0, at), "node_modules", name, "package.json"), "utf8"),
  );
  return { pkg: name, license: manifest.license ?? "" };
}

// libreJSLabels writes the JavaScript license web labels of the bundle to
// "assets/librejs/librejs.html", which the footer links for LibreJS.
function libreJSLabels(): Plugin {
  return {
    name: "gogs:librejs-labels",
    apply: "build",
    generateBundle(_, bundle) {
      const rows: string[] = [];
      for (const file of Object.values(bundle).sort((a, b) => a.fileName.localeCompare(b.fileName))) {
        let licenses: Set<string>;
        if (file.type === "chunk") {
          licenses = new Set();
          for (const id of Object.keys(file.modules)) {
            const { pkg, license } = licenseOf(id.replace(/^\0/, "").split("?")[0]);
            // An SPDX expression like "(MIT OR Apache-2.0)" offers a choice,
            // so the first license LibreJS knows is enough.
            const known = Object.keys(LICENSE_URLS).find((l) =>
              license
                .replace(/[()]/g, "")
                .split(/ or /i)
                .some((choice) => choice.trim().toLowerCase() === l.toLowerCase()),
            );
            if (!known) throw new Error(`LibreJS: no license URL for "${license}" of ${pkg}`);
            licenses.add(known);
          }
        } else if (file.fileName.startsWith("assets/pdf.worker")) {
          licenses = new Set(["Apache-2.0"]);
        } else {
          continue;
        }
        const href = `../${file.fileName.slice("assets/".length)}`;
        const labels = [...licenses]
          .sort()
          .map((l) => `<a href="${LICENSE_URLS[l]}">${l}</a>`)
          .join("<br>");
        rows.push(
          `<tr><td><a href="${href}">${path.basename(file.fileName)}</a></td><td>${labels}</td><td><a href="https://github.com/gogs/gogs/tree/main/web">gogs/web</a></td></tr>`,
        );
      }
      this.emitFile({
        type: "asset",
        fileName: "assets/librejs/librejs.html",
        source: `<!DOCTYPE html>
<html lang="en">
<head><meta charset="UTF-8"><title>JavaScript license information</title></head>
<body>
<table id="jslicense-labels1">
${rows.join("\n")}
</table>
</body>
</html>
`,
      });
    },
  };
}

export default defineConfig({
  base: "./",
  plugins: [react(), tailwindcss(), pdfjsAssets(), libreJSLabels()],
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
