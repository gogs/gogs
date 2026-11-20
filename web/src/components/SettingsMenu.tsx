import { Blend, Bug, Check, Monitor, Moon, Settings, Sun } from "lucide-react";
import { useState } from "react";
import { Trans, useTranslation } from "react-i18next";

import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import { ScrollArea } from "@/components/ui/scroll-area";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import { webContext } from "@/lib/context";
import { readGlass, setGlass } from "@/lib/glass";
import { LANGUAGES, switchLanguage } from "@/lib/languages";
import { type Theme, useTheme } from "@/lib/theme-context";
import { reloadWithParams } from "@/lib/url";
import { cn } from "@/lib/utils";

// Dev-only feature flag: toggles a `?i18n_debug=1` query param that makes
// every translated string render with its key wrapped around it. `i18n.ts`
// reads the param at module load, so toggling forces a full reload.
const i18nDebugEnabled = typeof window !== "undefined" && new URLSearchParams(window.location.search).has("i18n_debug");

function toggleI18nDebug() {
  const params = new URLSearchParams(window.location.search);
  if (params.has("i18n_debug")) params.delete("i18n_debug");
  else params.set("i18n_debug", "1");
  reloadWithParams(params);
}

// The themes share one row, and a longer label, e.g., in another language,
// takes more of it than a shorter one. Labels that do not fit the row together
// wrap onto a second one. The theme in use is bold, so it does not rely on its
// fill alone.
const THEME_ITEM = "min-w-0 flex-auto gap-1.5 data-[state=on]:font-semibold glass-ui:gap-1 glass-ui:px-1.5";

export function SettingsMenu() {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const [currentLang] = useState(() => webContext.lang);
  const { theme, setTheme } = useTheme();
  const [glass, setGlassState] = useState(readGlass);
  const currentLangName = LANGUAGES.find((l) => l.code === currentLang)?.name ?? currentLang;
  const otherLanguages = LANGUAGES.filter((l) => l.code !== currentLang);

  return (
    <Popover
      open={open}
      onOpenChange={(next) => {
        // The command palette can toggle glass while this menu is closed.
        if (next) setGlassState(readGlass());
        setOpen(next);
      }}
    >
      <PopoverTrigger
        aria-label={t("settings")}
        className="inline-flex size-9 cursor-pointer items-center justify-center rounded-md text-(--color-muted-foreground) hover:bg-(--color-surface) hover:text-(--color-foreground)"
      >
        <Settings className="size-4" />
      </PopoverTrigger>
      <PopoverContent
        align="end"
        collisionPadding={16}
        className="flex max-h-(--radix-popover-content-available-height) flex-col w-64 p-0 focus:outline-none"
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
            size="sm"
            // With glass the themes are the segments of one control, which cannot wrap, so a label
            // that is too long for its share of the row is cut short, and the others keep their room.
            className="glass-control glass-tabs flex-wrap glass-ui:grid glass-ui:!w-full glass-ui:grid-cols-[repeat(3,minmax(0,auto))] glass-ui:justify-stretch"
          >
            <ToggleGroupItem
              value="light"
              aria-label={t("theme_light")}
              title={t("theme_light")}
              className={THEME_ITEM}
            >
              <Sun className="size-4 shrink-0" aria-hidden />
              <span className="truncate">{t("theme_light")}</span>
            </ToggleGroupItem>
            <ToggleGroupItem value="dark" aria-label={t("theme_dark")} title={t("theme_dark")} className={THEME_ITEM}>
              <Moon className="size-4 shrink-0" aria-hidden />
              <span className="truncate">{t("theme_dark")}</span>
            </ToggleGroupItem>
            <ToggleGroupItem
              value="system"
              aria-label={t("theme_system")}
              title={t("theme_system")}
              className={THEME_ITEM}
            >
              <Monitor className="size-4 shrink-0" aria-hidden />
              <span className="truncate">{t("theme_system")}</span>
            </ToggleGroupItem>
          </ToggleGroup>
        </div>

        <div className="my-1 h-px bg-(--color-border)" />

        <div className="px-2 pt-2 pb-1 text-xs font-medium text-(--color-muted-foreground)">{t("experiments")}</div>
        <div className="p-1">
          <button
            type="button"
            role="switch"
            aria-checked={glass}
            onClick={() => {
              setGlass(!glass);
              setGlassState(!glass);
            }}
            className="flex w-full cursor-pointer items-center gap-2 rounded-md px-2 py-2 text-left text-sm text-(--color-foreground) outline-none hover:bg-(--color-surface) focus-visible:ring-1 focus-visible:ring-(--color-ring)"
          >
            <Blend className="size-4 text-(--color-muted-foreground)" aria-hidden />
            <span className="flex-1">{t("glass_effect")}</span>
            <span
              aria-hidden
              className={cn(
                "relative h-5 w-9 shrink-0 rounded-full border transition-colors",
                glass ? "border-(--color-primary) bg-(--color-primary)" : "border-(--color-input) bg-(--color-field)",
              )}
            >
              <span
                className={cn(
                  "absolute top-0.5 left-0.5 size-3.5 rounded-full shadow-sm transition-transform motion-reduce:transition-none",
                  glass ? "translate-x-4 bg-(--color-primary-foreground)" : "bg-(--color-muted-foreground)",
                )}
              />
            </span>
          </button>
        </div>

        <div className="my-1 h-px bg-(--color-border)" />

        <div className="px-2 pt-2 pb-1 text-xs font-medium text-(--color-muted-foreground)">
          <Trans
            i18nKey="language_current"
            values={{ name: currentLangName }}
            components={{ name: <span className="font-semibold text-(--color-foreground)" /> }}
          />
        </div>
        <ScrollArea className="max-h-60">
          <ul className="p-1 text-sm">
            {otherLanguages.map((lang) => (
              <li key={lang.code}>
                <button
                  type="button"
                  onClick={() => switchLanguage(lang.code)}
                  className="flex w-full cursor-pointer items-center rounded-sm px-2 py-1.5 text-left hover:bg-(--color-surface) hover:text-(--color-foreground)"
                >
                  {lang.name}
                </button>
              </li>
            ))}
          </ul>
        </ScrollArea>

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
