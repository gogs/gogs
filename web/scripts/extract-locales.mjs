// Extracts the subset of keys the SPA needs from conf/locale/locale_*.ini and
// writes them as JSON under web/src/locales/. Run with `node scripts/extract-locales.mjs`
// after adding a new key or changing source translations.
import { mkdirSync, readFileSync, readdirSync, writeFileSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const repoRoot = resolve(here, "../..");
const inDir = join(repoRoot, "conf/locale");
const outDir = join(here, "..", "src/locales");

// Keys pulled from Gogs's INI files. Add new entries here when the SPA needs
// another existing translation. SPA-specific strings that don't exist in the
// INI (e.g. 404 CLI lines, theme picker labels) live in the supplements below
// and only ship in en-US; other locales fall back to en-US for those.
const REUSED_KEYS = [
  "app_desc",
  "home",
  "explore",
  "help",
  "register",
  "sign_in",
  "settings",
  "language",
  "page_not_found",
];

// SPA-specific strings that don't exist in Gogs's INI files. Add a key here,
// then optionally add per-language overrides in SPA_SUPPLEMENT_OVERRIDES below.
// Locales without an override fall back to en-US via react-i18next.
const SPA_SUPPLEMENTS = {
  theme: "Theme",
  theme_light: "Light",
  theme_dark: "Dark",
  theme_system: "System",
};

const SPA_SUPPLEMENT_OVERRIDES = {
  "zh-CN": {
    theme: "主题",
    theme_light: "浅色",
    theme_dark: "深色",
    theme_system: "跟随系统",
  },
  "zh-HK": {
    theme: "主題",
    theme_light: "淺色",
    theme_dark: "深色",
    theme_system: "跟隨系統",
  },
  "zh-TW": {
    theme: "主題",
    theme_light: "淺色",
    theme_dark: "深色",
    theme_system: "跟隨系統",
  },
};

// Lightweight INI parser: handles `key = value` and `key=value`, ignores
// comments, ignores sections (Gogs uses ini-sections for grouping but the
// keys we want all live at the top level).
function parseIni(text) {
  const out = {};
  for (const rawLine of text.split(/\r?\n/)) {
    const line = rawLine.trim();
    if (!line || line.startsWith(";") || line.startsWith("#") || line.startsWith("[")) continue;
    const eq = line.indexOf("=");
    if (eq < 0) continue;
    const key = line.slice(0, eq).trim();
    const value = line.slice(eq + 1).trim();
    if (key && !(key in out)) out[key] = value;
  }
  return out;
}

mkdirSync(outDir, { recursive: true });

const files = readdirSync(inDir).filter((f) => f.startsWith("locale_") && f.endsWith(".ini"));
for (const file of files) {
  const lang = file.slice("locale_".length, -".ini".length);
  const parsed = parseIni(readFileSync(join(inDir, file), "utf8"));
  const out = {};
  for (const key of REUSED_KEYS) {
    if (parsed[key]) out[key] = parsed[key];
  }
  if (lang === "en-US") {
    Object.assign(out, SPA_SUPPLEMENTS);
  } else if (SPA_SUPPLEMENT_OVERRIDES[lang]) {
    Object.assign(out, SPA_SUPPLEMENT_OVERRIDES[lang]);
  }
  writeFileSync(join(outDir, `${lang}.json`), JSON.stringify(out, null, 2) + "\n", "utf8");
  console.log(`wrote ${lang}.json (${Object.keys(out).length} keys)`);
}
