import { reloadWithParams } from "@/lib/url";

// LANGUAGES are the languages of the interface, by locale code.
export const LANGUAGES: { code: string; name: string }[] = [
  { code: "en-US", name: "English" },
  { code: "zh-CN", name: "简体中文" },
  { code: "zh-HK", name: "繁體中文（香港）" },
  { code: "zh-TW", name: "繁體中文（臺灣）" },
  { code: "de-DE", name: "Deutsch" },
  { code: "fr-FR", name: "français" },
  { code: "nl-NL", name: "Nederlands" },
  { code: "lv-LV", name: "latviešu" },
  { code: "ru-RU", name: "русский" },
  { code: "ja-JP", name: "日本語" },
  { code: "es-ES", name: "español" },
  { code: "pt-BR", name: "português do Brasil" },
  { code: "pl-PL", name: "polski" },
  { code: "bg-BG", name: "български" },
  { code: "it-IT", name: "italiano" },
  { code: "fi-FI", name: "suomi" },
  { code: "tr-TR", name: "Türkçe" },
  { code: "cs-CZ", name: "čeština" },
  { code: "sr-SP", name: "српски" },
  { code: "sv-SE", name: "svenska" },
  { code: "ko-KR", name: "한국어" },
  { code: "gl-ES", name: "galego" },
  { code: "uk-UA", name: "українська" },
  { code: "en-GB", name: "English (United Kingdom)" },
  { code: "hu-HU", name: "Magyar" },
  { code: "sk-SK", name: "Slovenčina" },
  { code: "id-ID", name: "Indonesian" },
  { code: "fa-IR", name: "Persian" },
  { code: "vi-VN", name: "Vietnamese" },
  { code: "pt-PT", name: "Português" },
  { code: "mn-MN", name: "Монгол" },
  { code: "ro-RO", name: "Română" },
];

// switchLanguage reloads the page in the language, which the server then
// remembers for the browser.
export function switchLanguage(code: string) {
  const params = new URLSearchParams(window.location.search);
  params.set("lang", code);
  reloadWithParams(params);
}
