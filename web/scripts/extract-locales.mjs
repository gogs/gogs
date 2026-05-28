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
  "internal_server_error",
  "theme",
  "theme_light",
  "theme_dark",
  "theme_system",
  "username",
  "username_placeholder",
  "new_username_placeholder",
  "email",
  "email_placeholder",
  "password",
  "password_placeholder",
  "captcha",
  "captcha_placeholder",
  "captcha_image_alt",
  "refresh_captcha",
  "click_to_refresh_captcha",
  "auth_source",
  "local",
  "forget_password",
  "send_reset_email",
  "reset_password_email_submitting",
  "reset_password_email_failed",
  "reset_password_email_sent",
  "disable_register_mail",
  "disable_register_prompt",
  "reset_password_resend_limited",
  "non_local_account",
  "create_new_account",
  "register_hepler_msg",
  "sign_up",
  "sign_up_now",
  "sign_up_submitting",
  "sign_up_failed",
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
  "confirm_password",
  "confirm_password_placeholder",
  "confirm_new_password",
  "confirm_new_password_placeholder",
  "password_mismatch",
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
  "activate_your_account",
  "resend_rate_limited",
  "send_activation_email",
  "check_activation_email",
  "activation_email_pending",
  "activation_email_sent",
  "sending_activation_email",
  "send_activation_email_failed",
  "activating_account",
  // Commit diff page chrome (web/src/pages/repo/Commit.tsx and friends).
  "diff.showing_changed_files",
  "diff.additions",
  "diff.deletions",
  "diff.unified",
  "diff.split",
  "diff.diff_settings",
  "diff.whitespace",
  "diff.show_whitespace",
  "diff.ignore_whitespace_changes",
  "diff.ignore_all_whitespace",
  "diff.display",
  "diff.wrap_long_lines",
  "diff.expand_all_files",
  "diff.collapse_all_files",
  "diff.show_file_tree",
  "diff.hide_file_tree",
  "diff.expand_all_directories",
  "diff.collapse_all_directories",
  "diff.search_files",
  "diff.hide_search",
  "diff.search_in_diff",
  "diff.previous_match",
  "diff.next_match",
  "diff.expand_file",
  "diff.collapse_file",
  "diff.copy_file_path",
  "diff.expand_all_lines",
  "diff.all_lines_expanded",
  "diff.more_actions",
  "diff.view_history",
  "diff.view_raw",
  "diff.renamed_from",
  "diff.authored",
  "diff.copy_full_sha",
  "diff.diff",
  "diff.patch",
  "diff.browse_files",
  "diff.parents",
  "diff.parent",
  "diff.commit",
  // Reused from existing locale entries.
  "diff.view_file",
  "editor.edit_file",
  "editor.delete_this_file",
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
