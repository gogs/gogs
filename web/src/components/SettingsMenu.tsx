import { Bug, Check, Monitor, Moon, Settings, Sun } from "lucide-react";
import { useState } from "react";
import { Trans, useTranslation } from "react-i18next";

import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import { webContext } from "@/lib/context";
import { type Theme, useTheme } from "@/lib/theme-context";
import { cn } from "@/lib/utils";

const LANGUAGES: { code: string; name: string }[] = [
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

// Dev-only feature flag: toggles a `?i18n_debug=1` query param that makes
// every translated string render with its key wrapped around it. `i18n.ts`
// reads the param at module load, so toggling forces a full reload.
const i18nDebugEnabled = typeof window !== "undefined" && new URLSearchParams(window.location.search).has("i18n_debug");

// Navigate to the current page with new query params, preserving the hash so
// deep-linked anchors (PR comment IDs, headings, file rows) stay intact.
// Assigning to `window.location.search` directly would drop the hash.
function reloadWithParams(params: URLSearchParams) {
  const qs = params.toString();
  const search = qs ? `?${qs}` : "";
  window.location.href = `${window.location.pathname}${search}${window.location.hash}`;
}

function toggleI18nDebug() {
  const params = new URLSearchParams(window.location.search);
  if (params.has("i18n_debug")) params.delete("i18n_debug");
  else params.set("i18n_debug", "1");
  reloadWithParams(params);
}

export function SettingsMenu() {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const [currentLang] = useState(() => webContext.lang);
  const { theme, setTheme } = useTheme();
  const currentLangName = LANGUAGES.find((l) => l.code === currentLang)?.name ?? currentLang;
  const otherLanguages = LANGUAGES.filter((l) => l.code !== currentLang);

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger
        aria-label={t("settings")}
        className="inline-flex size-9 cursor-pointer items-center justify-center rounded-md text-(--color-muted-foreground) hover:bg-(--color-surface) hover:text-(--color-foreground)"
      >
        <Settings className="size-4" />
      </PopoverTrigger>
      <PopoverContent
        align="end"
        className="w-64 p-0 focus:outline-none"
        // Focus the popover container itself rather than the first focusable
        // child (the Light theme toggle) so an Enter/Space press right after
        // opening the menu does not accidentally change the theme. Keyboard
        // users can Tab forward to reach the controls.
        onOpenAutoFocus={(e) => {
          e.preventDefault();
          (e.currentTarget as HTMLElement | null)?.focus();
        }}
        tabIndex={-1}
      >
        <div className="px-2 pt-2 pb-1 text-xs font-medium text-(--color-muted-foreground)">{t("theme")}</div>
        <div className="p-1">
          <ToggleGroup
            type="single"
            value={theme}
            onValueChange={(v) => v && setTheme(v as Theme)}
            size="tile"
            className="grid grid-cols-3 gap-1"
          >
            <ToggleGroupItem value="light" aria-label={t("theme_light")}>
              <Sun className="size-4" aria-hidden />
              {t("theme_light")}
            </ToggleGroupItem>
            <ToggleGroupItem value="dark" aria-label={t("theme_dark")}>
              <Moon className="size-4" aria-hidden />
              {t("theme_dark")}
            </ToggleGroupItem>
            <ToggleGroupItem value="system" aria-label={t("theme_system")}>
              <Monitor className="size-4" aria-hidden />
              {t("theme_system")}
            </ToggleGroupItem>
          </ToggleGroup>
        </div>

        <div className="my-1 h-px bg-(--color-border)" />

        <div className="px-2 pt-2 pb-1 text-xs font-medium text-(--color-muted-foreground)">
          <Trans
            i18nKey="language_current"
            values={{ name: currentLangName }}
            components={{ name: <span className="font-semibold text-(--color-foreground)" /> }}
          />
        </div>
        <ul className="max-h-60 overflow-y-auto p-1 text-sm">
          {otherLanguages.map((lang) => (
            <li key={lang.code}>
              <button
                type="button"
                onClick={() => {
                  const params = new URLSearchParams(window.location.search);
                  params.set("lang", lang.code);
                  reloadWithParams(params);
                }}
                className="flex w-full cursor-pointer items-center rounded-sm px-2 py-1.5 text-left hover:bg-(--color-surface) hover:text-(--color-foreground)"
              >
                {lang.name}
              </button>
            </li>
          ))}
        </ul>

        {import.meta.env.DEV ? (
          <>
            <div className="my-1 h-px bg-(--color-border)" />
            <div className="p-1">
              <button
                type="button"
                onClick={toggleI18nDebug}
                className="flex w-full cursor-pointer items-center gap-2 rounded-sm px-2 py-1.5 text-left text-sm hover:bg-(--color-surface) hover:text-(--color-foreground)"
              >
                <Bug className="size-4" aria-hidden />
                <span className="flex-1">i18n debug</span>
                <Check className={cn("size-4", i18nDebugEnabled ? "opacity-100" : "opacity-0")} />
              </button>
            </div>
          </>
        ) : null}
      </PopoverContent>
    </Popover>
  );
}
