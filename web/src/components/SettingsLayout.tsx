import { Slot, Slottable } from "@radix-ui/react-slot";
import { ChevronDown, type LucideIcon } from "lucide-react";
import { type MouseEvent, type ReactElement, type ReactNode, useId, useState } from "react";

import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import { cn } from "@/lib/utils";

// SettingsLayout puts the links between the settings pages of a user, an
// organization or a repository, or the pages of the admin panel, beside the
// page. On narrow screens they move into a dropdown above the page, named
// after the current page.
export function SettingsLayout({
  label,
  current,
  nav,
  className,
  children,
}: {
  label: string;
  current: string;
  nav: ReactNode;
  // className changes the layout, e.g., the width to line up with a header.
  className?: string;
  children: ReactNode;
}) {
  const [open, setOpen] = useState(false);
  // The dropdown stays open after a click on a link that stays in the app.
  const closeOnLink = (e: MouseEvent) => {
    if ((e.target as Element).closest("a")) setOpen(false);
  };
  return (
    <div
      className={cn(
        "mx-auto grid w-full max-w-6xl content-start gap-6 px-4 glass-ui:gap-x-4 py-6 sm:px-6 md:grid-cols-[200px_minmax(0,1fr)]",
        className,
      )}
    >
      <nav aria-label={label} className="md:hidden">
        <Popover open={open} onOpenChange={setOpen}>
          <PopoverTrigger
            aria-label={`${label}: ${current}`}
            className="flex w-full cursor-pointer items-center justify-between gap-2 rounded-md border border-(--color-input) bg-(--color-field) px-3 py-2 text-sm font-semibold outline-none hover:bg-(--color-surface) focus-visible:ring-2 focus-visible:ring-(--color-ring)"
          >
            <span className="truncate">{current}</span>
            <ChevronDown className="size-4 shrink-0 text-(--color-muted-foreground)" aria-hidden />
          </PopoverTrigger>
          <PopoverContent
            align="start"
            className="flex w-(--radix-popover-trigger-width) flex-col gap-0.5 pl-3"
            onClick={closeOnLink}
          >
            {nav}
          </PopoverContent>
        </Popover>
      </nav>
      <nav
        aria-label={label}
        className="glass hidden flex-col gap-0.5 self-start pl-2 md:flex glass-ui:border glass-ui:p-2 glass-ui:pl-3"
      >
        {nav}
      </nav>
      <section className="glass min-w-0 glass-ui:border glass-ui:p-4 glass-ui:sm:p-6">{children}</section>
    </div>
  );
}

// SettingsNavItem styles its link as an item of the settings navigation,
// with a leading icon. The current page gets a bar in the primary color beside
// it, so it does not rely on the subtle surface fill alone. Links to
// destructive pages are red until they are the current page.
export function SettingsNavItem({
  active,
  icon: Icon,
  destructive = false,
  children,
}: {
  active: boolean;
  icon: LucideIcon;
  destructive?: boolean;
  children: ReactElement;
}) {
  return (
    <Slot
      aria-current={active ? "page" : undefined}
      className={cn(
        "relative flex items-center gap-2 rounded-md px-3 py-1.5 text-sm wrap-anywhere hover:bg-(--color-surface)",
        active
          ? "bg-(--color-surface) font-semibold text-(--color-foreground) before:absolute before:inset-y-1.5 before:-left-2 before:w-1 before:rounded-full before:bg-(--color-primary)"
          : destructive
            ? "text-(--color-destructive)"
            : "text-(--color-muted-foreground)",
      )}
    >
      <Icon className="size-4 shrink-0" aria-hidden />
      <Slottable>{children}</Slottable>
    </Slot>
  );
}

// SettingsNavGroup puts related items of the settings navigation under a
// heading. The first group of a menu goes without one.
export function SettingsNavGroup({ title, children }: { title?: string; children: ReactNode }) {
  const id = useId();
  return (
    <div role="group" aria-labelledby={title ? id : undefined} className="flex flex-col gap-0.5">
      {title ? (
        <div id={id} className="mt-4 mb-1 px-3 text-sm font-semibold text-(--color-foreground)">
          {title}
        </div>
      ) : null}
      {children}
    </div>
  );
}

// SettingsNavDivider sets the danger zone apart at the end of the menu.
export function SettingsNavDivider() {
  return <hr className="my-2 border-(--color-border)" />;
}
