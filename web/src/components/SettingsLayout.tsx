import { Slot } from "@radix-ui/react-slot";
import { type ReactElement, type ReactNode, useEffect, useRef } from "react";

import { cn } from "@/lib/utils";

// SettingsLayout puts the links between the settings pages of a user or an
// organization beside the page, or above it on narrow screens, where they
// scroll sideways like tabs.
export function SettingsLayout({ label, nav, children }: { label: string; nav: ReactNode; children: ReactNode }) {
  const navRef = useRef<HTMLElement>(null);
  useEffect(() => {
    navRef.current?.querySelector('[aria-current="page"]')?.scrollIntoView({ block: "nearest", inline: "nearest" });
  }, []);
  return (
    <div className="mx-auto grid w-full max-w-6xl content-start gap-6 px-4 py-6 sm:px-6 md:grid-cols-[200px_minmax(0,1fr)]">
      <nav ref={navRef} aria-label={label} className="flex gap-1 overflow-x-auto md:flex-col md:overflow-visible">
        {nav}
      </nav>
      <section className="min-w-0 max-w-xl">{children}</section>
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
        "rounded-md px-3 py-1.5 text-sm whitespace-nowrap hover:bg-(--color-surface)",
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
