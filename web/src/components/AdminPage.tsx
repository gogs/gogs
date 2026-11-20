import { Link, type LinkProps, useLinkProps } from "@tanstack/react-router";
import { ArrowLeft, CircleAlert, RefreshCw, TrendingUp } from "lucide-react";
import { type ReactNode, use } from "react";
import { createPortal } from "react-dom";
import { useTranslation } from "react-i18next";

import { FormAlert } from "@/components/FormField";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardFooter } from "@/components/ui/card";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { AdminCrumbSlot } from "@/lib/admin-crumb-context";
import { REFRESH_SECONDS } from "@/lib/admin-refresh";
import { formatInterval } from "@/lib/format";
import { formatClockTime } from "@/lib/relative-time";
import { cn } from "@/lib/utils";

// AdminPageHeader heads a page of the admin console with its title and what
// the page is about, and holds the actions of the page at its end. A page
// below a page of the navigation links back to that page, and names itself in
// the breadcrumb with `crumb`. A `note` is a line of its own below the header,
// e.g., a RefreshNote, which comes and goes without moving the actions.
export function AdminPageHeader({
  title,
  description,
  back,
  crumb,
  note,
  children,
}: {
  title: ReactNode;
  description?: ReactNode;
  back?: { to: LinkProps["to"]; label: string };
  crumb?: string;
  note?: ReactNode;
  children?: ReactNode;
}) {
  const slot = use(AdminCrumbSlot);
  return (
    <div className="grid gap-3">
      {crumb && slot ? createPortal(crumb, slot) : null}
      {back ? (
        <Link
          to={back.to}
          className="inline-flex min-h-6 items-center gap-1.5 justify-self-start text-sm text-(--color-primary) hover:underline"
        >
          <ArrowLeft className="size-4 shrink-0" aria-hidden />
          {back.label}
        </Link>
      ) : null}
      <div className="min-w-0">
        <div className="flex min-w-0 flex-wrap items-end justify-between gap-x-4 gap-y-3">
          <div className="min-w-0">
            <h1 className="text-xl font-semibold wrap-anywhere">{title}</h1>
            {description ? <p className="mt-1 text-sm text-(--color-muted-foreground)">{description}</p> : null}
          </div>
          {children ? <div className="flex min-w-0 flex-wrap items-center gap-2">{children}</div> : null}
        </div>
        {note}
      </div>
    </div>
  );
}

// RefreshNote tells from when a page shows its numbers while the latest
// refresh failed, so that old numbers do not pass for current ones. Render it
// in a block right below the controls of the page, not in a grid or a row with
// a gap: its status is on the page from the start, for screen readers to
// announce it, and takes no room while empty.
export function RefreshNote({
  failedSince,
}: {
  // When the page got what it shows, in Unix milliseconds, set while the
  // latest refresh failed.
  failedSince?: number;
}) {
  const { t } = useTranslation();
  return (
    <div role="status" className="min-w-0 text-sm">
      {failedSince ? (
        <p className="mt-3 flex items-start gap-1.5">
          <CircleAlert className="mt-0.5 size-4 shrink-0 text-(--color-destructive)" aria-hidden />
          {t("admin.refresh.failed", { time: formatClockTime(failedSince) })}
        </p>
      ) : null}
    </div>
  );
}

// RefreshControl refreshes what a page shows: right away with its button, and
// by itself at the interval picked beside it. The two wrap together. A
// RefreshNote below the controls tells when a refresh failed.
export function RefreshControl({
  seconds,
  onSecondsChange,
  refreshing,
  onRefresh,
}: {
  seconds: number;
  onSecondsChange: (seconds: number) => void;
  refreshing: boolean;
  onRefresh: () => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="flex items-center gap-2">
      <Button type="button" variant="outline" className="h-9 px-3" onClick={onRefresh}>
        <RefreshCw className={cn("size-4", refreshing && "animate-spin")} aria-hidden />
        {t("admin.refresh")}
      </Button>
      <Select value={String(seconds)} onValueChange={(value) => onSecondsChange(Number(value))}>
        <SelectTrigger aria-label={t("admin.refresh.auto")} className="glass-control h-9 w-auto gap-2">
          <SelectValue>
            {seconds === 0
              ? t("admin.refresh.auto_off")
              : t("admin.refresh.every", { interval: formatInterval(seconds) })}
          </SelectValue>
        </SelectTrigger>
        <SelectContent align="end">
          {REFRESH_SECONDS.map((option) => (
            <SelectItem key={option} value={String(option)}>
              {option === 0 ? t("admin.refresh.off") : formatInterval(option)}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
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
        "glass-control glass-tabs inline-flex max-w-full flex-wrap items-center gap-0.5 rounded-lg bg-(--color-surface) p-0.5 glass-ui:overflow-x-auto",
        className,
      )}
    >
      {children}
    </nav>
  );
}

// The view shown is a raised pill with a border, so it does not rely on its
// color alone. With glass on, `glass-tabs` makes the links the segments of one
// glass control, as in the view switchers of the site, and the view shown is
// the filled one in bold.
export const SEGMENTED_ITEM =
  "inline-flex h-8 items-center rounded-md border border-transparent px-3 text-sm font-medium whitespace-nowrap text-(--color-muted-foreground) outline-none hover:text-(--color-foreground) focus-visible:ring-1 focus-visible:ring-(--color-ring) aria-[current=page]:border-(--color-border) aria-[current=page]:bg-(--color-background) aria-[current=page]:text-(--color-foreground) aria-[current=page]:shadow-xs glass-ui:h-8.5 glass-ui:focus-visible:ring-inset glass-ui:aria-[current=page]:bg-(--color-surface) glass-ui:aria-[current=page]:font-semibold glass-ui:aria-[current=page]:shadow-none";

// The router marks a link as the current page from the URL, which changes
// before the loader of the new view answers, and whenever the search params of
// the link are among those of the URL, which the params of the first view
// always are. The page tells which view it shows instead.
function SegmentedLink({
  to,
  search,
  current,
  children,
}: {
  to: string;
  search: Record<string, unknown>;
  current: boolean;
  children: ReactNode;
}) {
  const props = useLinkProps({ to, search });
  return (
    <a {...props} aria-current={current ? "page" : undefined} className={SEGMENTED_ITEM}>
      {children}
    </a>
  );
}

// SegmentedLinks switches a page between views through its search params, so
// that the loader fetches the view, e.g., the last hour or the last day.
// `active` is the view the page shows, which stays marked while another loads.
export function SegmentedLinks<V extends string>({
  label,
  active,
  options,
  to,
  className,
}: {
  label: string;
  active: V;
  options: { value: V; label: string; search: Record<string, unknown> }[];
  to: string;
  className?: string;
}) {
  return (
    <Segmented label={label} className={className}>
      {options.map((option) => (
        <SegmentedLink key={option.value} to={to} search={option.search} current={option.value === active}>
          {option.label}
        </SegmentedLink>
      ))}
    </Segmented>
  );
}

// StatCard heads a number with what it measures. The chip says what stands
// out about it, and the footer holds, e.g., the link to the page that tells
// more. Cards below a section heading take `as="h3"`.
export function StatCard({
  label,
  as: Heading = "h2",
  chip,
  footer,
  className,
  children,
}: {
  label: string;
  as?: "h2" | "h3";
  chip?: ReactNode;
  footer?: ReactNode;
  className?: string;
  children: ReactNode;
}) {
  return (
    <Card className={cn("flex min-w-0 flex-col gap-2 p-5", className)}>
      <div className="flex min-h-6 flex-wrap items-center justify-between gap-x-2 gap-y-1">
        <Heading className="text-sm text-(--color-muted-foreground)">{label}</Heading>
        {chip}
      </div>
      {children}
      {footer ? <div className="mt-auto pt-1">{footer}</div> : null}
    </Card>
  );
}

// TrendChip says what stands out about a measurement, e.g., "Spike from
// 14:32". The icon and the text carry it, not a color.
export function TrendChip({ children }: { children: ReactNode }) {
  return (
    <span className="inline-flex max-w-full items-center gap-1 rounded-full border border-(--color-border) px-2 text-xs/5">
      <TrendingUp className="size-3 shrink-0" aria-hidden />
      <span className="truncate">{children}</span>
    </span>
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

// AdminFormCard lays a form of the admin console out in a card, with its
// actions in a bar at the foot of the card. Everything in it is disabled
// while a submit is `pending`.
export function AdminFormCard({
  pending,
  error,
  actions,
  onSubmit,
  children,
}: {
  pending: boolean;
  error: ReactNode;
  actions: ReactNode;
  onSubmit: () => void;
  children: ReactNode;
}) {
  return (
    <form
      noValidate
      className="min-w-0"
      onSubmit={(e) => {
        e.preventDefault();
        onSubmit();
      }}
    >
      <fieldset disabled={pending} className="contents">
        <Card className="@container max-w-5xl">
          <CardContent>
            <FormAlert message={error} />
            <div className="flex flex-col gap-5">{children}</div>
          </CardContent>
          <CardFooter className="flex-wrap gap-2 border-t border-(--color-border) py-4 sm:py-4">{actions}</CardFooter>
        </Card>
      </fieldset>
    </form>
  );
}

// FieldGrid puts the fields of a form in two columns once its card is wide
// enough for both.
export function FieldGrid({ className, children }: { className?: string; children: ReactNode }) {
  return <div className={cn("grid grid-cols-1 gap-5 @2xl:grid-cols-2", className)}>{children}</div>;
}

// Each label of a checkbox in the group is 24px tall, the least a touch
// target may be.
export const CHECKBOX_GROUP = "flex flex-col gap-2 [&>label]:min-h-6";
