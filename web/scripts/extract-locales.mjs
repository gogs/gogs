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
// another translation. Locales missing a key fall back to en-US via react-i18next.
const REUSED_KEYS = [
  "app_desc",
  "home",
  "dashboard",
  "issues",
  "pull_requests",
  "explore",
  "help",
  "register",
  "sign_in",
  "sign_out",
  "create_new",
  "new_repo",
  "new_migrate",
  "new_org",
  "signed_in_as",
  "user_profile_and_more",
  "your_profile",
  "your_settings",
  "admin_panel",
  "settings",
  "language",
  "page_not_found",
  "theme",
  "theme_light",
  "theme_dark",
  "theme_system",
  "username",
  "username_placeholder",
  "email",
  "password",
  "password_placeholder",
  "auth_source",
  "local",
  "remember_me",
  "forget_password",
  "send_reset_email",
  "reset_password_email_submitting",
  "reset_password_email_failed",
  "reset_password_email_sent",
  "disable_register_mail",
  "resent_limit_prompt",
  "non_local_account",
  "sign_up_now",
  "sign_in_submitting",
  "sign_in_failed",
  "show_password",
  "hide_password",
  "back_to_sign_in",
  "reset_password",
  "invalid_code",
  "reset_password_submit",
  "reset_password_submitting",
  "reset_password_failed",
  "new_password",
  "new_password_placeholder",
  "confirm_new_password",
  "confirm_new_password_placeholder",
  "reset_password_mismatch",
  "mfa_title",
  "mfa_passcode",
  "mfa_passcode_placeholder",
  "mfa_recovery_code",
  "mfa_recovery_code_placeholder",
  "mfa_use_recovery_code",
  "mfa_use_passcode",
  "mfa_verify",
  "mfa_verifying",
  "mfa_session_expired",
  "mfa_verify_failed",
];

// Lightweight INI parser: handles `key = value` and `key=value`, ignores
// comments, and flattens sections into a single namespace. Gogs's locale
// files group keys under sections like [status] (e.g. status.page_not_found
// resolves to a key named "page_not_found" inside [status]), but downstream
// callers reference keys by their bare name, so the section header is
// dropped here. First occurrence wins on collisions.
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
  writeFileSync(join(outDir, `${lang}.json`), JSON.stringify(out, null, 2) + "\n", "utf8");
  console.log(`wrote ${lang}.json (${Object.keys(out).length} keys)`);
}
