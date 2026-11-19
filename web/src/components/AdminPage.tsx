import { Link, type LinkProps } from "@tanstack/react-router";
import { ArrowLeft } from "lucide-react";
import { type ReactNode, use } from "react";
import { createPortal } from "react-dom";

import { Card } from "@/components/ui/card";
import { AdminCrumbSlot } from "@/lib/admin-crumb-context";
import { cn } from "@/lib/utils";

// AdminPageHeader heads a page of the admin console with its title and what
// the page is about, and holds the actions of the page at its end. A page
// below a page of the navigation links back to that page, and names itself in
// the breadcrumb with `crumb`.
export function AdminPageHeader({
  title,
  description,
  back,
  crumb,
  children,
}: {
  title: ReactNode;
  description?: ReactNode;
  back?: { to: LinkProps["to"]; label: string };
  crumb?: string;
  children?: ReactNode;
}) {
  const slot = use(AdminCrumbSlot);
  return (
    <div className="grid gap-3">
      {crumb && slot ? createPortal(crumb, slot) : null}
      {back ? (
        <Link
          to={back.to}
          className="inline-flex items-center gap-1.5 justify-self-start text-sm text-(--color-primary) hover:underline"
        >
          <ArrowLeft className="size-4 shrink-0" aria-hidden />
          {back.label}
        </Link>
      ) : null}
      <div className="flex flex-wrap items-end justify-between gap-x-4 gap-y-3">
        <div className="min-w-0">
          <h1 className="text-xl font-semibold wrap-anywhere">{title}</h1>
          {description ? <p className="mt-1 text-sm text-(--color-muted-foreground)">{description}</p> : null}
        </div>
        {children ? <div className="flex flex-wrap items-center gap-2">{children}</div> : null}
      </div>
    </div>
  );
}

// Segmented holds links that switch between views of the same page, e.g.,
// between time ranges. Give each link SEGMENTED_ITEM and `aria-current="page"`
// when it is the view shown.
export function Segmented({ label, className, children }: { label: string; className?: string; children: ReactNode }) {
  return (
    <nav
      aria-label={label}
      className={cn(
        "inline-flex max-w-full flex-wrap items-center gap-0.5 rounded-lg bg-(--color-surface) p-0.5",
        className,
      )}
    >
      {children}
    </nav>
  );
}

// The view shown is a raised pill with a border, so it does not rely on its
// color alone.
export const SEGMENTED_ITEM =
  "inline-flex h-8 items-center rounded-md border border-transparent px-3 text-sm font-medium whitespace-nowrap text-(--color-muted-foreground) outline-none hover:text-(--color-foreground) focus-visible:ring-1 focus-visible:ring-(--color-ring) aria-[current=page]:border-(--color-border) aria-[current=page]:bg-(--color-background) aria-[current=page]:text-(--color-foreground) aria-[current=page]:shadow-xs";

// StatCard heads a number with what it measures. The chip says what stands
// out about it, and the footer holds, e.g., the link to the page that tells
// more.
export function StatCard({
  label,
  chip,
  footer,
  className,
  children,
}: {
  label: string;
  chip?: ReactNode;
  footer?: ReactNode;
  className?: string;
  children: ReactNode;
}) {
  return (
    <Card className={cn("flex min-w-0 flex-col gap-2 p-5", className)}>
      <div className="flex min-h-6 flex-wrap items-center justify-between gap-x-2 gap-y-1">
        <h2 className="text-sm text-(--color-muted-foreground)">{label}</h2>
        {chip}
      </div>
      {children}
      {footer ? <div className="mt-auto pt-1">{footer}</div> : null}
    </Card>
  );
}

export const STAT_VALUE = "text-2xl/8 font-semibold tabular-nums";

// Rows lists labeled values in a card, with each value at the end of the line
// of its label, or below it when they do not fit one line.
export function Rows({ className, children }: { className?: string; children: ReactNode }) {
  return <dl className={cn("divide-y divide-(--color-border) text-sm", className)}>{children}</dl>;
}

export function Row({ label, children }: { label: ReactNode; children: ReactNode }) {
  return (
    <div className="flex flex-wrap items-baseline justify-between gap-x-4 gap-y-0.5 py-2 first:pt-0 last:pb-0">
      <dt className="font-medium">{label}</dt>
      <dd className="min-w-0 wrap-anywhere text-(--color-muted-foreground)">{children}</dd>
    </div>
  );
}
