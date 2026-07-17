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
const KEYS = [
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
  "language_current",
  "repository",
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
  "close",
  "show_more",
  "show_less",
  "resize_sidebar",
  "copy_failed",
  "more",
  "more_tabs",
  "more_actions",
  "status.page_not_found",
  "status.internal_server_error",
  "auth.auth_source",
  "auth.local",
  "auth.forget_password",
  "auth.send_reset_email",
  "auth.reset_password_email_submitting",
  "auth.reset_password_email_failed",
  "auth.reset_password_email_sent",
  "auth.disable_register_mail",
  "auth.disable_register_prompt",
  "auth.reset_password_resend_limited",
  "auth.non_local_account",
  "auth.create_new_account",
  "auth.register_hepler_msg",
  "auth.sign_up_now",
  "auth.sign_up_submitting",
  "auth.sign_up_failed",
  "auth.sign_in_submitting",
  "auth.sign_in_failed",
  "auth.saml_sign_in_with",
  "auth.saml_sign_in_failed",
  "auth.saml_provisioning_failed",
  "auth.show_password",
  "auth.hide_password",
  "auth.back_to_sign_in",
  "auth.reset_password",
  "auth.invalid_code",
  "auth.reset_password_submit",
  "auth.reset_password_submitting",
  "auth.reset_password_failed",
  "auth.new_password",
  "auth.new_password_placeholder",
  "auth.confirm_password",
  "auth.confirm_password_placeholder",
  "auth.confirm_new_password",
  "auth.confirm_new_password_placeholder",
  "auth.password_mismatch",
  "auth.mfa_title",
  "auth.mfa_passcode",
  "auth.mfa_passcode_placeholder",
  "auth.mfa_recovery_code",
  "auth.mfa_recovery_code_placeholder",
  "auth.mfa_use_recovery_code",
  "auth.mfa_use_passcode",
  "auth.mfa_verify",
  "auth.mfa_verifying",
  "auth.mfa_session_expired",
  "auth.mfa_verify_failed",
  "auth.activate_your_account",
  "auth.resend_rate_limited",
  "auth.send_activation_email",
  "auth.check_activation_email",
  "auth.activation_email_pending",
  "auth.activation_email_sent",
  "auth.sending_activation_email",
  "auth.send_activation_email_failed",
  "auth.activating_account",
  "tool.now",
  "tool.ago",
  "tool.from_now",
  "tool.1s",
  "tool.1m",
  "tool.1h",
  "tool.1d",
  "tool.1w",
  "tool.1mon",
  "tool.1y",
  "tool.seconds",
  "tool.minutes",
  "tool.hours",
  "tool.days",
  "tool.weeks",
  "tool.months",
  "tool.years",
  "repo.diff.showing_changed_files",
  "repo.diff.additions",
  "repo.diff.deletions",
  "repo.diff.unified",
  "repo.diff.split",
  "repo.diff.diff_settings",
  "repo.diff.whitespace",
  "repo.diff.show_whitespace",
  "repo.diff.ignore_whitespace_changes",
  "repo.diff.ignore_all_whitespace",
  "repo.diff.display",
  "repo.diff.wrap_long_lines",
  "repo.diff.expand_all_files",
  "repo.diff.collapse_all_files",
  "repo.show_file_tree",
  "repo.hide_file_tree",
  "repo.expand_all_directories",
  "repo.collapse_all_directories",
  "repo.search_files",
  "repo.search_hide",
  "repo.search_diff",
  "repo.search_previous_match",
  "repo.search_next_match",
  "repo.diff.expand_file",
  "repo.diff.collapse_file",
  "repo.diff.expand_all_lines",
  "repo.diff.all_lines_expanded",
  "repo.commit_parent",
  "repo.commit_label",
  "repo.view_file",
  "repo.editor.edit_file",
  "repo.editor.delete_this_file",
  "repo.files",
  "repo.settings",
  "repo.wiki",
  "repo.watch",
  "repo.unwatch",
  "repo.star",
  "repo.starred",
  "repo.fork",
  "repo.mirror_of",
  "repo.sign_in_to_watch",
  "repo.sign_in_to_star",
  "repo.sign_in_to_fork",
  "repo.watch_this_repository",
  "repo.unwatch_this_repository",
  "repo.star_this_repository",
  "repo.unstar_this_repository",
  "repo.fork_this_repository",
  "repo.visibility_private",
  "repo.visibility_public",
  "repo.view_watchers",
  "repo.view_stargazers",
  "repo.view_forks",
  "repo.browse_files",
  "repo.view_history",
  "repo.view_raw",
  "repo.copy_file_path",
  "repo.copy_full_sha",
  "repo.renamed_from",
  "repo.authored",
  "repo.parents",
  "repo.diff_label",
  "repo.patch_label",
];

// Parse the INI into a single `section.key` to value map. Top-level keys
// (above any section header) are stored bare. First occurrence wins.
function parseIni(text) {
  const out = {};
  let section = "";
  for (const rawLine of text.split(/\r?\n/)) {
    const line = rawLine.trim();
    if (!line || line.startsWith(";") || line.startsWith("#")) continue;
    if (line.startsWith("[") && line.endsWith("]")) {
      section = line.slice(1, -1).trim();
      continue;
    }
    const eq = line.indexOf("=");
    if (eq < 0) continue;
    const key = line.slice(0, eq).trim();
    const value = line.slice(eq + 1).trim();
    if (!key) continue;
    const qualified = section ? `${section}.${key}` : key;
    if (!(qualified in out)) out[qualified] = value;
  }
  return out;
}

mkdirSync(outDir, { recursive: true });

const files = readdirSync(inDir).filter((f) => f.startsWith("locale_") && f.endsWith(".ini"));
for (const file of files) {
  const lang = file.slice("locale_".length, -".ini".length);
  const parsed = parseIni(readFileSync(join(inDir, file), "utf8"));
  const out = {};
  for (const key of KEYS) {
    if (parsed[key]) out[key] = parsed[key];
  }
  writeFileSync(join(outDir, `${lang}.json`), JSON.stringify(out, null, 2) + "\n", "utf8");
  console.log(`wrote ${lang}.json (${Object.keys(out).length} keys)`);
}
