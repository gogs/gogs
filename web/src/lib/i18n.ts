import { i18n as diffsI18n } from "@pierre/diffs";
import i18n, { t } from "i18next";
import { initReactI18next } from "react-i18next";

import { webContext } from "@/lib/context";

// Toggle via `?i18n_debug=1` to wrap every translated string with its key
// for visual inspection. Read once at module load so it survives navigation
// within the SPA.
const showKeys = typeof window !== "undefined" && new URLSearchParams(window.location.search).has("i18n_debug");

if (showKeys) {
  // eslint-disable-next-line import/no-named-as-default-member
  i18n.use({
    type: "postProcessor",
    name: "i18n-debug",
    process: (value: string, keys: string[]) => `「${keys.join(",")}」${value}`,
  });
}

// The server builds the translations from its locale files and preloads them
// from the page, so this fetch is usually answered from that preload or the
// cache. Without them, the app still renders and shows the keys.
async function loadMessages(): Promise<Record<string, string>> {
  try {
    const resp = await fetch(webContext.localeURL);
    if (!resp.ok) throw new Error(`HTTP ${resp.status}`);
    return (await resp.json()) as Record<string, string>;
  } catch (err) {
    console.error("Failed to load translations:", err);
    return {};
  }
}

// eslint-disable-next-line import/no-named-as-default-member
await i18n.use(initReactI18next).init({
  resources: { [webContext.lang]: { translation: await loadMessages() } },
  lng: webContext.lang,
  // The server already fills keys missing from the active locale with en-US.
  fallbackLng: false,
  interpolation: { escapeValue: false, prefix: "{", suffix: "}" },
  returnNull: false,
  postProcess: showKeys ? ["i18n-debug"] : undefined,
});

// Pierre's diff view reads its text from a package-wide store of ICU
// MessageFormat templates, so it is filled once here rather than per page.
// `#` is ICU's placeholder for the plural count.
try {
  diffsI18n.set({
    locale: webContext.lang,
    translations: {
      unmodifiedLines: `{count, plural, one {${t("repo.diff.unmodified_line", { count: "#" })}} other {${t("repo.diff.unmodified_lines", { count: "#" })}}}`,
      // Intentionally blank. The `[data-unmodified-lines]:empty` rule in the
      // commit page's diff CSS keeps the separator clickable without a label.
      unknownUnmodifiedLines: "",
      expandAll: t("repo.diff.expand_all"),
      noNewlineAtEndOfFile: t("repo.diff.no_newline_at_eof"),
    },
  });
} catch (err) {
  // Pierre rejects templates with braces outside its ICU subset. Fall back to
  // its English text rather than failing app startup over one translation.
  console.error("Failed to set diff translations:", err);
}

export default i18n;
