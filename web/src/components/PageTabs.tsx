import { Menu } from "lucide-react";
import type { ComponentType, ReactNode } from "react";
import { Fragment, useState } from "react";
import { useTranslation } from "react-i18next";

import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import { cn } from "@/lib/utils";

export interface PageTab<K extends string> {
  key: K;
  label: string;
  icon?: ComponentType<{ className?: string }>;
  // A number shown in a badge after the label.
  count?: number;
  // Shows the tab without a link, e.g., when there is nothing to show in it.
  disabled?: boolean;
}

// PageTabLinkProps are what renderLink passes on to the link of a tab.
export interface PageTabLinkProps {
  className: string;
  "aria-current"?: "page";
  onClick?: () => void;
  children: ReactNode;
}

// PageTabs is a row of page-level tabs. Below `sm`, only the first `inline`
// tabs stay in the row, always including the active one, and the rest fold
// into a "More" menu, so the row never scrolls sideways.
export function PageTabs<K extends string>({
  label,
  tabs,
  active,
  inline = tabs.length,
  renderLink,
  className,
}: {
  label: string;
  tabs: readonly PageTab<K>[];
  active: K | undefined;
  // Pick a number of tabs that fit in the row at 320px wide.
  inline?: number;
  renderLink: (tab: PageTab<K>, props: PageTabLinkProps) => ReactNode;
  className?: string;
}) {
  const { t } = useTranslation();
  const [menuOpen, setMenuOpen] = useState(false);

  const inlineKeys = tabs.slice(0, inline).map((tab) => tab.key);
  if (active !== undefined && !inlineKeys.includes(active) && inline > 0) {
    inlineKeys[inline - 1] = active;
  }
  const folded = tabs.filter((tab) => !inlineKeys.includes(tab.key));

  const content = (tab: PageTab<K>) => (
    <>
      {tab.icon ? <tab.icon className="size-4 shrink-0" aria-hidden /> : null}
      {tab.label}
      {tab.count !== undefined ? (
        <span className="rounded-full bg-(--color-secondary) px-2 text-xs/5 font-medium tabular-nums text-(--color-secondary-foreground)">
          {tab.count}
        </span>
      ) : null}
    </>
  );

  return (
    <nav aria-label={label} className={cn("flex", className)}>
      {tabs.map((tab) => {
        const isActive = tab.key === active;
        const tabClassName = cn(
          "-mb-px shrink-0 items-center gap-2 border-b-2 px-3 py-2 text-sm whitespace-nowrap",
          inlineKeys.includes(tab.key) ? "flex" : "hidden sm:flex",
          isActive
            ? "border-(--color-primary) font-semibold text-(--color-foreground)"
            : "border-transparent text-(--color-muted-foreground) hover:border-(--color-border) hover:text-(--color-foreground)",
        );
        if (tab.disabled) {
          return (
            <span key={tab.key} aria-disabled className={cn(tabClassName, "cursor-not-allowed opacity-60")}>
              {content(tab)}
            </span>
          );
        }
        return (
          <Fragment key={tab.key}>
            {renderLink(tab, {
              className: tabClassName,
              "aria-current": isActive ? "page" : undefined,
              children: content(tab),
            })}
          </Fragment>
        );
      })}
      {folded.length > 0 ? (
        <Popover open={menuOpen} onOpenChange={setMenuOpen}>
          <PopoverTrigger asChild>
            <button
              type="button"
              aria-label={t("more_tabs")}
              className="-mb-px flex shrink-0 cursor-pointer items-center gap-2 border-b-2 border-transparent px-3 py-2 text-sm whitespace-nowrap text-(--color-muted-foreground) hover:border-(--color-border) hover:text-(--color-foreground) sm:hidden"
            >
              <Menu className="size-4" aria-hidden />
              <span>{t("more")}</span>
            </button>
          </PopoverTrigger>
          <PopoverContent align="start" className="w-56 p-1">
            {folded.map((tab) => {
              const itemClassName = "flex items-center gap-2 rounded px-2 py-1.5 text-sm text-(--color-foreground)";
              if (tab.disabled) {
                return (
                  <span key={tab.key} aria-disabled className={cn(itemClassName, "cursor-not-allowed opacity-60")}>
                    {content(tab)}
                  </span>
                );
              }
              return (
                <Fragment key={tab.key}>
                  {renderLink(tab, {
                    className: cn(itemClassName, "hover:bg-(--color-surface)"),
                    onClick: () => setMenuOpen(false),
                    children: content(tab),
                  })}
                </Fragment>
              );
            })}
          </PopoverContent>
        </Popover>
      ) : null}
    </nav>
  );
}
