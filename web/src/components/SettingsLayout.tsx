import { Slot } from "@radix-ui/react-slot";
import { ChevronDown } from "lucide-react";
import { type MouseEvent, type ReactElement, type ReactNode, useState } from "react";

import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import { cn } from "@/lib/utils";

// SettingsLayout puts the links between the settings pages of a user or an
// organization, or the pages of the admin panel, beside the page. On narrow
// screens they move into a dropdown above the page, named after the current
// page.
export function SettingsLayout({
  label,
  current,
  nav,
  wide = false,
  children,
}: {
  label: string;
  current: string;
  nav: ReactNode;
  // wide lets the page use the full width, e.g., for a list, instead of the
  // width of a form.
  wide?: boolean;
  children: ReactNode;
}) {
  const [open, setOpen] = useState(false);
  // The dropdown stays open after a click on a link that stays in the app.
  const closeOnLink = (e: MouseEvent) => {
    if ((e.target as Element).closest("a")) setOpen(false);
  };
  return (
    <div className="mx-auto grid w-full max-w-6xl content-start gap-6 px-4 py-6 sm:px-6 md:grid-cols-[200px_minmax(0,1fr)]">
      <nav aria-label={label} className="md:hidden">
        <Popover open={open} onOpenChange={setOpen}>
          <PopoverTrigger
            aria-label={`${label}: ${current}`}
            className="flex w-full cursor-pointer items-center justify-between gap-2 rounded-md border border-(--color-input) bg-(--color-background) px-3 py-2 text-sm font-semibold outline-none hover:bg-(--color-surface) focus-visible:ring-2 focus-visible:ring-(--color-ring)"
          >
            <span className="truncate">{current}</span>
            <ChevronDown className="size-4 shrink-0 text-(--color-muted-foreground)" aria-hidden />
          </PopoverTrigger>
          <PopoverContent
            align="start"
            className="flex w-(--radix-popover-trigger-width) flex-col gap-0.5"
            onClick={closeOnLink}
          >
            {nav}
          </PopoverContent>
        </Popover>
      </nav>
      <nav aria-label={label} className="hidden flex-col gap-1 md:flex">
        {nav}
      </nav>
      <section className={cn("min-w-0", !wide && "max-w-xl")}>{children}</section>
    </div>
  );
}

// SettingsNavItem styles its link as an item of the settings navigation.
// Links to destructive pages are red until they are the current page.
export function SettingsNavItem({
  active,
  destructive = false,
  children,
}: {
  active: boolean;
  destructive?: boolean;
  children: ReactElement;
}) {
  return (
    <Slot
      aria-current={active ? "page" : undefined}
      className={cn(
        "rounded-md px-3 py-1.5 text-sm hover:bg-(--color-surface)",
        active
          ? "bg-(--color-surface) font-semibold text-(--color-foreground)"
          : destructive
            ? "text-(--color-destructive)"
            : "text-(--color-muted-foreground)",
      )}
    >
      {children}
    </Slot>
  );
}
