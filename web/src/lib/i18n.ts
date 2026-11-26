import { i18n as diffsI18n } from "@pierre/diffs";
import i18n, { exists, t } from "i18next";
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

const NUMBER_PARTS = new Intl.NumberFormat(webContext.lang).formatToParts(1234.5);
const GROUP = NUMBER_PARTS.find((p) => p.type === "group")?.value ?? ",";
const DECIMAL = NUMBER_PARTS.find((p) => p.type === "decimal")?.value ?? ".";

// The server turns each plural of the locale files, e.g., "${file, count}", into
// "{count, plural(noun: file; lang: en-US)}", where lang is the language that owns
// the message. This formatter picks the form of the noun by the plural rules of
// that language, so a message missing from the active language reads right in
// English. A count may also be a number already formatted for the active
// language, e.g., "1,234". `pluralForm` forces a form, e.g., to build ICU
// templates.
i18n.services.formatter?.add("plural", (value: unknown, _lng, options: Record<string, unknown>) => {
  const lang = String(options.lang);
  const count = typeof value === "number" ? value : Number(String(value).replaceAll(GROUP, "").replace(DECIMAL, "."));
  const form = (options.pluralForm as string | undefined) ?? new Intl.PluralRules(lang).select(count);
  const forms = `plurals.${lang}.${String(options.noun)}`;
  return t(`${forms}.${form}`, { defaultValue: t(`${forms}.other`) });
});

// hasPlurals reports whether the message of key picks its words by the plural
// rules of the active language. Only en-US has plurals for now, so the other
// languages keep the messages they translated before.
export function hasPlurals(key: string): boolean {
  const message = String(i18n.getDataByLanguage(webContext.lang)?.translation[key] ?? "");
  return webContext.lang === "en-US" || message.includes(`; lang: ${webContext.lang})`);
}

// tCount translates the message of key for count. Until the active language has
// plurals for it, the message comes from the keys the language translated for a
// count of one and for other counts, given as legacy, e.g.,
// ["repo.commits.n_commit"] when other counts use key itself. options may give
// count formatted, e.g., "1,234".
export function tCount(
  key: string,
  count: number,
  legacy: [one: string, other?: string],
  options?: Record<string, unknown>,
): string {
  let resolved = key;
  if (!hasPlurals(key)) {
    const [one, other = key] = legacy;
    const legacyKey = count === 1 ? one : other;
    if (exists(legacyKey)) resolved = legacyKey;
  }
  return t(resolved, { count, ...options });
}

// Pierre's diff view reads its text from a package-wide store of ICU
// MessageFormat templates, so it is filled once here rather than per page.
// `#` is ICU's placeholder for the plural count, and Pierre picks the branch
// by the rules of the language that owns the message.
const UNMODIFIED_LINES = "repo.diff.unmodified_lines";
const UNMODIFIED_LINE = "repo.diff.unmodified_line";
const legacyUnmodifiedLines = !hasPlurals(UNMODIFIED_LINES) && exists(UNMODIFIED_LINE);
const unmodifiedLinesLang = legacyUnmodifiedLines
  ? webContext.lang
  : (/lang: ([^)]+)\)/.exec(
      String(i18n.getDataByLanguage(webContext.lang)?.translation[UNMODIFIED_LINES] ?? ""),
    )?.[1] ?? webContext.lang);
const unmodifiedLinesBranches = legacyUnmodifiedLines
  ? `one {${t(UNMODIFIED_LINE, { count: "#" })}} other {${t(UNMODIFIED_LINES, { count: "#" })}}`
  : new Intl.PluralRules(unmodifiedLinesLang)
      .resolvedOptions()
      .pluralCategories.map((form) => `${form} {${t(UNMODIFIED_LINES, { count: "#", pluralForm: form })}}`)
      .join(" ");
try {
  diffsI18n.set({
    locale: unmodifiedLinesLang,
    translations: {
      unmodifiedLines: `{count, plural, ${unmodifiedLinesBranches}}`,
      // Intentionally blank. The `[data-unmodified-lines]:empty` rule in the
      // diff CSS of DiffWorkspace keeps the separator clickable without a label.
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
