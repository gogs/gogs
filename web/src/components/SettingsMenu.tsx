import { Check, Monitor, Moon, Settings, Sun } from "lucide-react";
import { useState } from "react";
import { useTranslation } from "react-i18next";

import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import { type Theme, useTheme } from "@/lib/theme";
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

function currentLangCode(): string {
  if (typeof document === "undefined") return "en-US";
  return document.documentElement.lang || "en-US";
}

export function SettingsMenu() {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const [currentLang] = useState(currentLangCode);
  const { theme, setTheme } = useTheme();

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger
        aria-label={t("settings")}
        className="inline-flex size-9 cursor-pointer items-center justify-center rounded-md text-(--color-muted-foreground) hover:bg-(--color-muted) hover:text-(--color-foreground)"
      >
        <Settings className="size-4" />
      </PopoverTrigger>
      <PopoverContent align="end" className="w-64 p-0" onOpenAutoFocus={(e) => e.preventDefault()}>
        <div className="px-2 pt-2 pb-1 text-xs font-medium text-(--color-muted-foreground)">{t("theme")}</div>
        <div className="grid grid-cols-3 gap-1 p-1">
          <ThemeOption
            value="light"
            current={theme}
            onSelect={setTheme}
            icon={<Sun className="size-4" />}
            label={t("theme_light")}
          />
          <ThemeOption
            value="dark"
            current={theme}
            onSelect={setTheme}
            icon={<Moon className="size-4" />}
            label={t("theme_dark")}
          />
          <ThemeOption
            value="system"
            current={theme}
            onSelect={setTheme}
            icon={<Monitor className="size-4" />}
            label={t("theme_system")}
          />
        </div>

        <div className="my-1 h-px bg-(--color-border)" />

        <div className="px-2 pt-2 pb-1 text-xs font-medium text-(--color-muted-foreground)">{t("language")}</div>
        <ul role="listbox" className="max-h-60 overflow-y-auto p-1 text-sm">
          {LANGUAGES.map((lang) => {
            const isActive = lang.code === currentLang;
            return (
              <li key={lang.code}>
                <button
                  type="button"
                  role="option"
                  aria-selected={isActive}
                  onClick={() => {
                    if (isActive) return;
                    const params = new URLSearchParams(window.location.search);
                    params.set("lang", lang.code);
                    window.location.search = "?" + params.toString();
                  }}
                  className={cn(
                    "flex w-full items-center rounded-sm px-2 py-1.5 text-left hover:bg-(--color-accent) hover:text-(--color-accent-foreground)",
                    isActive ? "cursor-default" : "cursor-pointer",
                  )}
                >
                  <Check className={cn("mr-2 size-4", isActive ? "opacity-100" : "opacity-0")} />
                  {lang.name}
                </button>
              </li>
            );
          })}
        </ul>
      </PopoverContent>
    </Popover>
  );
}

function ThemeOption({
  value,
  current,
  onSelect,
  icon,
  label,
}: {
  value: Theme;
  current: Theme;
  onSelect: (t: Theme) => void;
  icon: React.ReactNode;
  label: string;
}) {
  const isActive = current === value;
  return (
    <button
      type="button"
      onClick={() => onSelect(value)}
      aria-pressed={isActive}
      className={cn(
        "flex cursor-pointer flex-col items-center gap-1 rounded-md px-2 py-2 text-xs hover:bg-(--color-accent)",
        isActive
          ? "bg-(--color-accent) text-(--color-accent-foreground)"
          : "text-(--color-muted-foreground) hover:text-(--color-accent-foreground)",
      )}
    >
      {icon}
      {label}
    </button>
  );
}
