// Renders the emails in web/src/emails/templates into the Go templates under
// templates/mail that the server sends: an HTML template (.tmpl) and a plain
// text twin (.txt) for each. Run with `moon run web:build-emails` after
// changing an email, and commit the output.
import { mkdirSync, readdirSync, writeFileSync } from "node:fs";
import { dirname, join, relative, resolve } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

import { type ComponentType, createElement } from "react";
import { render, toPlainText } from "react-email";

const here = dirname(fileURLToPath(import.meta.url));
const srcDir = resolve(here, "../src/emails/templates");
const outDir = resolve(here, "../../templates/mail");

function sources(dir: string): string[] {
  return readdirSync(dir, { withFileTypes: true }).flatMap((entry) => {
    const path = join(dir, entry.name);
    if (entry.isDirectory()) return sources(path);
    return entry.name.endsWith(".tsx") ? [path] : [];
  });
}

// toGoTemplate checks that React left the Go template actions intact and drops
// the HTML comments React writes, which html/template would strip anyway.
// Conditional comments for Outlook are kept by printing them from an action.
// Trailing spaces are dropped, too.
function toGoTemplate(source: string, html: string) {
  for (const [action] of html.matchAll(/\{\{.*?\}\}/gs)) {
    if (action.includes("&")) {
      throw new Error(`${source}: React escaped the Go template action ${action}, write it with v() or t()`);
    }
  }
  return html
    .replace(/<!--(\[if [\s\S]*?<!\[endif\])-->/g, (_, comment: string) => `{{rawComment \`${comment}\`}}`)
    .replace(/<!--[\s\S]*?-->/g, "")
    .replace(/[ \t]+$/gm, "");
}

for (const path of sources(srcDir)) {
  const name = relative(srcDir, path).replace(/\.tsx$/, "");
  const source = relative(resolve(here, "../.."), path);
  const { default: Email } = (await import(pathToFileURL(path).href)) as { default: ComponentType };
  const html = await render(createElement(Email), { pretty: true });
  const text = toPlainText(html, {
    selectors: ["h1", "h2", "h3", "h4", "h5", "h6"].map((selector) => ({ selector, options: { uppercase: false } })),
  });

  const header = `{{- /* Generated from ${source}. */ -}}\n`;
  const out = join(outDir, name);
  mkdirSync(dirname(out), { recursive: true });
  writeFileSync(`${out}.tmpl`, header + toGoTemplate(source, html).trim() + "\n");
  writeFileSync(`${out}.txt`, header + text.trim() + "\n");
  console.log(`Wrote templates/mail/${name}.{tmpl,txt}`);
}
