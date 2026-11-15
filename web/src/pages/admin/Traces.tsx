import { Link, useNavigate } from "@tanstack/react-router";
import { AlertCircle, ArrowLeft, ChevronDown, ChevronRight, Search as SearchIcon, X } from "lucide-react";
import { type FormEvent, Fragment, type ReactNode, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";

import { AdminLayout } from "@/components/AdminLayout";
import { PageTabs } from "@/components/PageTabs";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { webContext } from "@/lib/context";
import { fill, formatMinutes, formatMs, formatNumber } from "@/lib/format";
import type { TraceSearch } from "@/lib/page-search";
import type {
  AdminSpan,
  AdminSpanKind,
  AdminTrace,
  AdminTraceKept,
  AdminTraceOverview as AdminTraceOverviewData,
  AdminTraceRoute,
  AdminTraceSearch as AdminTraceSearchData,
} from "@/lib/queries/admin";
import { formatAbsoluteTime, formatRelativeTime } from "@/lib/relative-time";
import { cn } from "@/lib/utils";
import { RangeLinks } from "@/pages/admin/Health";

const KIND_CHIPS: Record<AdminSpanKind, string> = { request: "http", git: "git", other: "go" };

// The fill of a span or operation in charts. Git gets the accent, since it is
// where Gogs spends most of its time.
const KIND_FILLS: Record<AdminSpanKind, string> = {
  request: "bg-(--color-muted-foreground)/50",
  git: "bg-(--color-primary)",
  other: "bg-(--color-muted-foreground)/50",
};

// fillNodes is fill for values that are elements, e.g., a name set in code.
function fillNodes(message: string, ...values: ReactNode[]) {
  return message
    .split(/%(?:\[\d+\])?[sdv]/)
    .flatMap((part, i) => (i === 0 ? [part] : [<Fragment key={i}>{values[i - 1]}</Fragment>, part]));
}

function KindChip({ kind }: { kind: AdminSpanKind }) {
  return (
    <span className="shrink-0 rounded border border-(--color-border) px-1 font-mono text-[0.6875rem]/4 text-(--color-muted-foreground)">
      {KIND_CHIPS[kind]}
    </span>
  );
}

function RouteName({ route }: { route: AdminTraceRoute }) {
  const { t } = useTranslation();
  switch (route.kind) {
    case "other":
      return <span>{fill(t("admin.traces.other_pages"), route.name)}</span>;
    case "background":
      return (
        <span className="inline-flex flex-wrap items-baseline gap-x-1.5">
          <span className="font-mono wrap-anywhere">{route.name}</span>
          <span className="text-xs text-(--color-muted-foreground)">{t("admin.traces.background")}</span>
        </span>
      );
    default:
      return <span className="font-mono wrap-anywhere">{route.name}</span>;
  }
}

function Section({
  id,
  title,
  action,
  children,
}: {
  id: string;
  title: string;
  action?: ReactNode;
  children: ReactNode;
}) {
  return (
    <section aria-labelledby={`${id}-title`} className="mt-8">
      <div className="mb-2 flex flex-wrap items-center justify-between gap-2">
        <h2 id={`${id}-title`} className="text-lg font-semibold">
          {title}
        </h2>
        {action}
      </div>
      {children}
    </section>
  );
}

function Note({ children }: { children: ReactNode }) {
  return <p className="mt-2 text-xs text-(--color-muted-foreground)">{children}</p>;
}

function Panel({ children, className }: { children: ReactNode; className?: string }) {
  return (
    <div className={cn("rounded-md border border-(--color-border) bg-(--color-surface) p-4 text-sm", className)}>
      {children}
    </div>
  );
}

// TraceIDForm finds a trace by its ID, or the first 8 or more characters of
// it, e.g., one copied from a log line.
function TraceIDForm({ q }: { q?: string }) {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const onSubmit = (e: FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    const id = (new FormData(e.currentTarget).get("q") as string | null)?.trim() ?? "";
    void navigate({ to: "/admin/traces", search: id ? { view: "search", q: id } : {} });
  };
  return (
    <form role="search" onSubmit={onSubmit} className="flex w-full gap-2 sm:w-auto">
      <div className="relative min-w-0 flex-1">
        <SearchIcon
          className="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-(--color-muted-foreground)"
          aria-hidden
        />
        {/* Remount on navigation so the field always shows the ID of the current URL. */}
        <Input
          key={q ?? ""}
          name="q"
          defaultValue={q}
          minLength={8}
          placeholder={t("admin.traces.trace_id")}
          aria-label={t("admin.traces.trace_id")}
          autoComplete="off"
          spellCheck={false}
          className="h-9 pl-9 font-mono sm:w-72"
        />
      </div>
      <Button type="submit" variant="outline" size="sm" className="h-9">
        {t("admin.traces.find")}
      </Button>
    </form>
  );
}

function TracesLayout({
  view,
  q,
  maxAgeMinutes,
  children,
}: {
  view: "overview" | "search";
  q?: string;
  maxAgeMinutes: number;
  children: ReactNode;
}) {
  const { t } = useTranslation();
  return (
    <AdminLayout active="traces">
      <div className="mb-4 flex flex-wrap items-start justify-between gap-3">
        <div>
          <h1 className="text-xl font-semibold">{t("admin.traces")}</h1>
          {/* Tracing that is off keeps nothing. */}
          {maxAgeMinutes > 0 ? (
            <p className="mt-1 text-sm text-(--color-muted-foreground)">
              {fill(t("admin.traces.desc"), formatMinutes(maxAgeMinutes))}
            </p>
          ) : null}
        </div>
        <TraceIDForm q={q} />
      </div>
      <PageTabs
        label={t("admin.traces")}
        className="mb-4 border-b border-(--color-border)"
        active={view}
        tabs={[
          { key: "overview", label: t("admin.traces.overview") },
          { key: "search", label: t("admin.traces.search") },
        ]}
        renderLink={(tab, props) => (
          <Link to="/admin/traces" search={tab.key === "search" ? { view: "search" } : {}} {...props} />
        )}
      />
      {children}
    </AdminLayout>
  );
}

function Disabled() {
  const { t } = useTranslation();
  return <Panel>{t("admin.traces.disabled")}</Panel>;
}

function OverviewChart({ overview }: { overview: AdminTraceOverviewData }) {
  const { t } = useTranslation();
  const W = 600;
  const H = 120;
  const buckets = overview.buckets;
  const maxRequests = Math.max(1, ...buckets.map((b) => b.requests));
  const maxP95 = Math.max(0, ...buckets.map((b) => b.p95Ms));
  const slot = W / buckets.length;
  const x = (i: number) => i * slot + slot / 2;
  const y = (p95: number) => H - 6 - (p95 / (maxP95 || 1)) * (H - 24);
  const line = buckets
    .map((b, i) => (b.requests > 0 ? `${x(i).toFixed(1)},${y(b.p95Ms).toFixed(1)}` : null))
    .filter((p) => p !== null)
    .join(" ");
  const peak = buckets.findIndex((b) => b.p95Ms === maxP95 && maxP95 > 0);
  const minutes = overview.range === "15m" ? 15 : 60;
  const first = buckets[0]?.start;
  const last = buckets.at(-1)?.start;
  return (
    <figure className="mt-4">
      <div
        role="img"
        aria-label={
          first && last
            ? fill(t("admin.traces.chart_overview"), formatAbsoluteTime(first), formatAbsoluteTime(last))
            : undefined
        }
        className="relative h-32"
      >
        <svg viewBox={`0 0 ${W} ${H}`} preserveAspectRatio="none" className="size-full" aria-hidden>
          {buckets.map((b, i) => {
            const h = (b.requests / maxRequests) * (H - 24);
            return (
              <rect
                key={b.start}
                x={i * slot + 2}
                width={slot - 4}
                y={H - h}
                height={h}
                className="fill-(--color-surface) stroke-(--color-border)"
                vectorEffect="non-scaling-stroke"
              />
            );
          })}
          <polyline
            points={line}
            fill="none"
            className="stroke-(--color-primary)"
            strokeWidth={2}
            strokeLinejoin="round"
            vectorEffect="non-scaling-stroke"
          />
          {/* Zero-length lines with round caps draw dots that keep their shape
              while the chart stretches, and show a bucket without neighbors. */}
          {buckets.map((b, i) =>
            b.requests > 0 ? (
              <line
                key={b.start}
                x1={x(i)}
                x2={x(i)}
                y1={y(b.p95Ms)}
                y2={y(b.p95Ms)}
                strokeWidth={6}
                strokeLinecap="round"
                className="stroke-(--color-primary)"
                vectorEffect="non-scaling-stroke"
              />
            ) : null,
          )}
        </svg>
        {peak >= 0 ? (
          <span
            // Labels near an edge stay inside the chart.
            className={cn(
              "absolute -translate-y-full text-xs whitespace-nowrap tabular-nums",
              x(peak) / W > 0.8 ? "-translate-x-full" : x(peak) / W < 0.2 ? "" : "-translate-x-1/2",
            )}
            style={{ left: `${(x(peak) / W) * 100}%`, top: `${(y(maxP95) / H) * 100}%` }}
          >
            p95 {formatMs(maxP95)}
          </span>
        ) : null}
      </div>
      <div className="mt-1 flex justify-between text-xs text-(--color-muted-foreground)">
        <span>{fill(t("admin.traces.minutes_ago"), minutes)}</span>
        <span>{t("admin.traces.now")}</span>
      </div>
      <figcaption className="mt-2 flex flex-wrap gap-x-4 gap-y-1 text-xs text-(--color-muted-foreground)">
        <span className="inline-flex items-center gap-1.5">
          <span aria-hidden className="size-2.5 border border-(--color-border) bg-(--color-surface)" />
          {t("admin.traces.requests_per_5m")}
        </span>
        <span className="inline-flex items-center gap-1.5">
          <span aria-hidden className="h-0.5 w-3 bg-(--color-primary)" />
          {t("admin.traces.p95_latency")}
        </span>
      </figcaption>
    </figure>
  );
}

const TH = "py-2 pl-3 text-right font-normal text-(--color-muted-foreground) first:pl-0 first:text-left";
const TD = "py-2 pl-3 text-right align-top tabular-nums first:pl-0 first:text-left";

function ShareBar({ share, className }: { share: number; className?: string }) {
  return (
    <span aria-hidden className="mt-1 block h-1 w-full rounded-full">
      <span
        className={cn("block h-full rounded-full", className)}
        style={{ width: `${Math.min(100, Math.max(share * 100, 0.5))}%` }}
      />
    </span>
  );
}

// COLLAPSED_ROWS is how many routes and operations show before "Show all".
const COLLAPSED_ROWS = 10;

function ShowAll({ count, onClick }: { count: number; onClick: () => void }) {
  const { t } = useTranslation();
  if (count <= COLLAPSED_ROWS) return null;
  return (
    <button
      type="button"
      onClick={onClick}
      className="mt-2 cursor-pointer text-sm text-(--color-primary) hover:underline"
    >
      {fill(t("admin.traces.show_all"), count)}
    </button>
  );
}

// AdminTraceOverview aggregates every request of the last 15 minutes or hour:
// how many, how fast, and where their time went.
export function AdminTraceOverview({ overview }: { overview: AdminTraceOverviewData }) {
  const { t } = useTranslation();
  const [allRoutes, setAllRoutes] = useState(false);
  const [allOperations, setAllOperations] = useState(false);
  if (!overview.enabled) {
    return (
      <TracesLayout view="overview" maxAgeMinutes={overview.maxAgeMinutes}>
        <Disabled />
      </TracesLayout>
    );
  }
  const errorRate = overview.requests > 0 ? overview.errors / overview.requests : 0;
  const percent = (share: number) =>
    share > 0 && share < 0.001 ? "<0.1%" : `${(share * 100).toFixed(share > 0 && share < 0.1 ? 1 : 0)}%`;
  const stats: { label: string; value: string; alert?: boolean }[] = [
    { label: t("admin.traces.requests"), value: formatNumber(overview.requests) },
    { label: t("admin.traces.errors"), value: percent(errorRate), alert: overview.errors > 0 },
    { label: "p50", value: formatMs(overview.p50Ms) },
    { label: "p95", value: formatMs(overview.p95Ms) },
    { label: "p99", value: formatMs(overview.p99Ms) },
  ];

  return (
    <TracesLayout view="overview" maxAgeMinutes={overview.maxAgeMinutes}>
      <RangeLinks
        label={t("admin.traces.range")}
        active={overview.range}
        to="/admin/traces"
        options={[
          { value: "15m", label: t("admin.traces.last_15m"), search: { range: "15m" } },
          { value: "1h", label: t("admin.traces.last_hour"), search: {} },
        ]}
      />
      <dl className="mt-4 grid grid-cols-2 overflow-hidden rounded-md border border-(--color-border) sm:grid-cols-5">
        {stats.map((stat) => (
          <div
            key={stat.label}
            className="-mr-px -mb-px border-r border-b border-(--color-border) px-4 py-3 last:col-span-2 sm:last:col-span-1"
          >
            <dt className="text-xs text-(--color-muted-foreground)">{stat.label}</dt>
            <dd className={cn("mt-1 text-lg tabular-nums", stat.alert && "text-(--color-destructive)")}>
              {stat.value}
            </dd>
          </div>
        ))}
      </dl>
      {overview.requests > 0 ? <OverviewChart overview={overview} /> : <Note>{t("admin.traces.no_requests")}</Note>}

      {overview.routes.length > 0 ? (
        <Section id="routes" title={t("admin.traces.routes")}>
          <table className="w-full table-fixed text-sm">
            <thead className="border-b border-(--color-border)">
              <tr>
                <th className={cn(TH, "w-auto")}>{t("admin.traces.route")}</th>
                <th className={cn(TH, "w-16")}>{t("admin.traces.calls")}</th>
                <th className={cn(TH, "hidden w-16 sm:table-cell")}>{t("admin.traces.errors")}</th>
                <th className={cn(TH, "hidden w-18 sm:table-cell")}>p50</th>
                <th className={cn(TH, "w-18")}>p95</th>
                <th className={cn(TH, "hidden w-18 sm:table-cell")}>p99</th>
              </tr>
            </thead>
            <tbody>
              {overview.routes.slice(0, allRoutes ? undefined : COLLAPSED_ROWS).map((route) => (
                <tr key={route.name} className="border-b border-(--color-border)">
                  <td className={TD}>
                    <Link to="/admin/traces" search={{ view: "search", route: route.name }} className="hover:underline">
                      <RouteName route={route} />
                    </Link>
                    <ShareBar
                      share={overview.totalMs > 0 ? route.totalMs / overview.totalMs : 0}
                      className="bg-(--color-muted-foreground)/60"
                    />
                  </td>
                  <td className={TD}>{formatNumber(route.requests)}</td>
                  <td
                    className={cn(
                      TD,
                      "hidden sm:table-cell",
                      route.errors > 0 ? "text-(--color-destructive)" : "text-(--color-muted-foreground)",
                    )}
                  >
                    {route.errors > 0 ? percent(route.errors / route.requests) : "0"}
                  </td>
                  <td className={cn(TD, "hidden text-(--color-muted-foreground) sm:table-cell")}>
                    {formatMs(route.p50Ms)}
                  </td>
                  <td className={TD}>{formatMs(route.p95Ms)}</td>
                  <td className={cn(TD, "hidden text-(--color-muted-foreground) sm:table-cell")}>
                    {formatMs(route.p99Ms)}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
          {allRoutes ? null : <ShowAll count={overview.routes.length} onClick={() => setAllRoutes(true)} />}
          <Note>{t("admin.traces.routes_help")}</Note>
        </Section>
      ) : null}

      {overview.operations.length > 0 ? (
        <Section id="operations" title={t("admin.traces.where_time_goes")}>
          <table className="w-full table-fixed text-sm">
            <thead className="border-b border-(--color-border)">
              <tr>
                <th className={TH}>{t("admin.traces.operation")}</th>
                <th className={cn(TH, "w-16")}>{t("admin.traces.calls")}</th>
                <th className={cn(TH, "hidden w-18 sm:table-cell")}>p50</th>
                <th className={cn(TH, "hidden w-18 sm:table-cell")}>p95</th>
                <th className={cn(TH, "w-16")}>{t("admin.traces.share")}</th>
              </tr>
            </thead>
            <tbody>
              {overview.operations.slice(0, allOperations ? undefined : COLLAPSED_ROWS).map((op) => {
                const share = overview.spanSelfMs > 0 ? op.selfMs / overview.spanSelfMs : 0;
                return (
                  <tr key={op.name} className="border-b border-(--color-border)">
                    <td className={TD}>
                      <span className="inline-flex max-w-full items-baseline gap-1.5">
                        <KindChip kind={op.kind} />
                        <span className="min-w-0 font-mono wrap-anywhere">{op.name}</span>
                      </span>
                    </td>
                    <td className={TD}>{formatNumber(op.calls)}</td>
                    <td className={cn(TD, "hidden text-(--color-muted-foreground) sm:table-cell")}>
                      {formatMs(op.p50Ms)}
                    </td>
                    <td className={cn(TD, "hidden sm:table-cell")}>{formatMs(op.p95Ms)}</td>
                    <td className={TD}>
                      {percent(share)}
                      <ShareBar share={share} className={KIND_FILLS[op.kind]} />
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
          {allOperations ? null : <ShowAll count={overview.operations.length} onClick={() => setAllOperations(true)} />}
          <Note>{t("admin.traces.operations_help")}</Note>
        </Section>
      ) : null}
    </TracesLayout>
  );
}

const DURATION_FILTERS = [100, 500, 1000, 5000];

const CLOCK_FMT = new Intl.DateTimeFormat(webContext.lang, { hour: "2-digit", minute: "2-digit" });

function ScatterPlot({ result, search }: { result: AdminTraceSearchData; search: TraceSearch }) {
  const { t } = useTranslation();
  const W = 600;
  const H = 160;
  const now = Date.parse(result.now);
  // Start at the oldest kept trace, so that a server that started recently
  // spreads its traces over the plot, or at the window of the search.
  const oldest = result.oldest ? Date.parse(result.oldest) : now;
  const start = search.from ?? Math.max(now - result.maxAgeMinutes * 60_000, Math.min(oldest, now - 5 * 60_000));
  const end = Math.max(Math.min(search.to ?? now, now), start + 60_000);
  const windowed = search.from !== undefined || search.to !== undefined;
  const maxMs = Math.max(10, ...result.points.map((p) => p.durationMs));
  // Powers of ten from 1 ms up to the slowest trace.
  const top = Math.ceil(Math.log10(maxMs));
  const ticks = Array.from({ length: top + 1 }, (_, i) => 10 ** i);
  const x = (iso: string) => ((Date.parse(iso) - start) / (end - start)) * W;
  const y = (ms: number) => H - 4 - (Math.log10(Math.max(ms, 1)) / top) * (H - 8);
  const slow = result.slowThresholdMs;

  // Zero-length lines with round caps draw dots that keep their shape while
  // the plot stretches to its width.
  const dot = (cx: number, cy: number, size: number, className: string, key: string) => (
    <line
      key={key}
      x1={cx}
      x2={cx}
      y1={cy}
      y2={cy}
      strokeWidth={size}
      strokeLinecap="round"
      vectorEffect="non-scaling-stroke"
      className={className}
    />
  );

  return (
    <figure className="mt-4">
      <div className="grid grid-cols-[auto_minmax(0,1fr)] gap-x-2">
        <div aria-hidden className="relative h-40 w-12 text-right text-xs text-(--color-muted-foreground)">
          {ticks.map((tick) => (
            <span key={tick} className="absolute right-0 -translate-y-1/2" style={{ top: `${(y(tick) / H) * 100}%` }}>
              {formatMs(tick)}
            </span>
          ))}
        </div>
        <div
          role="img"
          aria-label={fill(t("admin.traces.chart_search"), result.points.length)}
          className="relative h-40 border-b border-l border-(--color-border)"
        >
          <svg viewBox={`0 0 ${W} ${H}`} preserveAspectRatio="none" className="size-full overflow-visible" aria-hidden>
            {ticks.map((tick) => (
              <line
                key={tick}
                x1={0}
                x2={W}
                y1={y(tick)}
                y2={y(tick)}
                className="stroke-(--color-border)"
                vectorEffect="non-scaling-stroke"
              />
            ))}
            {slow > 0 ? (
              <line
                x1={0}
                x2={W}
                y1={y(slow)}
                y2={y(slow)}
                strokeDasharray="4 3"
                className="stroke-(--color-primary)"
                vectorEffect="non-scaling-stroke"
              />
            ) : null}
            {result.points.map((p) =>
              p.error
                ? [
                    dot(x(p.start), y(p.durationMs), 9, "stroke-(--color-destructive)", `${p.id}-ring`),
                    dot(x(p.start), y(p.durationMs), 4, "stroke-(--color-background)", p.id),
                  ]
                : dot(
                    x(p.start),
                    y(p.durationMs),
                    6,
                    slow > 0 && p.durationMs >= slow
                      ? "stroke-(--color-primary)"
                      : "stroke-(--color-muted-foreground)/70",
                    p.id,
                  ),
            )}
          </svg>
        </div>
        <div />
        <div className="mt-1 flex justify-between text-xs text-(--color-muted-foreground)">
          <span>{windowed ? CLOCK_FMT.format(start) : formatRelativeTime(t, new Date(start).toISOString(), now)}</span>
          <span>{end < now ? CLOCK_FMT.format(end) : t("admin.traces.now")}</span>
        </div>
      </div>
      <figcaption className="mt-2 flex flex-wrap gap-x-4 gap-y-1 text-xs text-(--color-muted-foreground)">
        <span className="inline-flex items-center gap-1.5">
          <span aria-hidden className="size-2 rounded-full bg-(--color-muted-foreground)/70" />
          {t("admin.traces.request")}
        </span>
        {slow > 0 ? (
          <span className="inline-flex items-center gap-1.5">
            <span aria-hidden className="size-2 rounded-full bg-(--color-primary)" />
            {fill(t("admin.traces.slower_than"), formatMs(slow))}
            <span aria-hidden className="w-3 border-t border-dashed border-(--color-primary)" />
          </span>
        ) : null}
        <span className="inline-flex items-center gap-1.5">
          <span aria-hidden className="size-2.5 rounded-full border-2 border-(--color-destructive)" />
          {t("admin.traces.error_5xx")}
        </span>
      </figcaption>
    </figure>
  );
}

function KeptLabel({ kept }: { kept: AdminTraceKept }) {
  const { t } = useTranslation();
  if (kept === "failed") {
    return (
      <span className="inline-flex items-center gap-1 text-(--color-destructive)">
        <AlertCircle className="size-3.5" aria-hidden />
        {t("admin.traces.kept_failed")}
      </span>
    );
  }
  return <span>{t(kept === "slow" ? "admin.traces.kept_slow" : "admin.traces.kept_sampled")}</span>;
}

// AdminTraceSearch finds kept traces by ID, or filters them by route, duration
// and status.
export function AdminTraceSearch({ result, search }: { result: AdminTraceSearchData; search: TraceSearch }) {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const now = Date.parse(result.now);
  if (!result.enabled) {
    return (
      <TracesLayout view="search" q={search.q} maxAgeMinutes={result.maxAgeMinutes}>
        <Disabled />
      </TracesLayout>
    );
  }
  const refine = (change: Partial<TraceSearch>) =>
    void navigate({ to: "/admin/traces", search: { ...search, view: "search", ...change } });
  const ALL = "*";

  if (search.q && result.total === 0) {
    return (
      <TracesLayout view="search" q={search.q} maxAgeMinutes={result.maxAgeMinutes}>
        <Panel>
          <p>{fillNodes(t("admin.traces.not_found"), <code className="font-mono wrap-anywhere">{search.q}</code>)}</p>
          <p className="mt-2 text-(--color-muted-foreground)">
            {fill(t("admin.traces.not_found_desc"), formatMinutes(result.maxAgeMinutes))}
            {result.oldest ? ` ${fill(t("admin.traces.oldest"), formatRelativeTime(t, result.oldest, now))}` : null}
          </p>
          <Link
            to="/admin/traces"
            search={{ view: "search" }}
            className="mt-3 inline-block text-(--color-primary) hover:underline"
          >
            {t("admin.traces.clear_search")}
          </Link>
        </Panel>
      </TracesLayout>
    );
  }

  return (
    <TracesLayout view="search" q={search.q} maxAgeMinutes={result.maxAgeMinutes}>
      {search.q ? null : (
        <div role="group" aria-label={t("admin.traces.filters")} className="flex flex-wrap items-center gap-2">
          <Select value={search.route ?? ALL} onValueChange={(v) => refine({ route: v === ALL ? undefined : v })}>
            <SelectTrigger
              aria-label={t("admin.traces.route")}
              className="h-9 w-full min-w-0 sm:w-auto sm:max-w-sm sm:flex-1"
            >
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value={ALL}>{t("admin.traces.all_routes")}</SelectItem>
              {result.routes.map((route) => (
                <SelectItem key={route.name} value={route.name}>
                  <RouteName route={route} />
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <Select
            value={search.minMs ? String(search.minMs) : ALL}
            onValueChange={(v) => refine({ minMs: v === ALL ? undefined : Number(v) })}
          >
            <SelectTrigger aria-label={t("admin.traces.duration")} className="h-9 w-auto">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value={ALL}>{t("admin.traces.any_duration")}</SelectItem>
              {(search.minMs && !DURATION_FILTERS.includes(search.minMs)
                ? [...DURATION_FILTERS, search.minMs].sort((a, b) => a - b)
                : DURATION_FILTERS
              ).map((ms) => (
                <SelectItem key={ms} value={String(ms)}>
                  {fill(t("admin.traces.at_least"), formatMs(ms))}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          {search.from || search.to ? (
            <span className="inline-flex h-8 items-center gap-1 rounded-md border border-(--color-ring) bg-(--color-surface) pr-1 pl-3 text-sm">
              {fill(
                t("admin.traces.window"),
                search.from ? CLOCK_FMT.format(search.from) : "",
                search.to ? CLOCK_FMT.format(search.to) : "",
              )}
              <Link
                to="/admin/traces"
                search={{ ...search, from: undefined, to: undefined }}
                aria-label={t("admin.traces.clear_window")}
                className="flex size-6 items-center justify-center rounded text-(--color-muted-foreground) hover:bg-(--color-background) hover:text-(--color-foreground)"
              >
                <X className="size-4" aria-hidden />
              </Link>
            </span>
          ) : null}
          <RangeLinks
            label={t("admin.traces.status")}
            active={search.status ?? "all"}
            to="/admin/traces"
            options={[
              { value: "all", label: t("admin.traces.all"), search: { ...search, view: "search", status: undefined } },
              { value: "5xx", label: "5xx", search: { ...search, view: "search", status: "5xx" } },
              { value: "4xx", label: "4xx", search: { ...search, view: "search", status: "4xx" } },
            ]}
          />
        </div>
      )}
      {result.points.length > 0 ? <ScatterPlot result={result} search={search} /> : null}

      <div className="mt-6 mb-2 flex flex-wrap items-center justify-between gap-2">
        <p className="text-sm text-(--color-muted-foreground)">
          {fill(t("admin.traces.count"), formatNumber(result.total), result.traces.length)}
        </p>
        {search.q ? null : (
          <Select
            value={search.sort ?? "recent"}
            onValueChange={(v) => refine({ sort: v === "slowest" ? "slowest" : undefined })}
          >
            <SelectTrigger aria-label={t("admin.traces.sort")} className="h-9 w-auto gap-2">
              <SelectValue />
            </SelectTrigger>
            <SelectContent align="end">
              <SelectItem value="recent">{t("admin.traces.most_recent")}</SelectItem>
              <SelectItem value="slowest">{t("admin.traces.slowest")}</SelectItem>
            </SelectContent>
          </Select>
        )}
      </div>
      {result.traces.length > 0 ? (
        <ul className="border-t border-(--color-border)">
          {result.traces.map((trace) => (
            <li key={trace.id} className="border-b border-(--color-border)">
              <Link
                to="/admin/traces/$id"
                params={{ id: trace.id }}
                className="flex items-start gap-3 px-1 py-2 text-sm hover:bg-(--color-surface)"
              >
                <span className="min-w-0 flex-1">
                  <span className="block wrap-anywhere">
                    {trace.method ? <span className="text-(--color-muted-foreground)">{trace.method} </span> : null}
                    <span className="font-mono">{trace.path || trace.route.name}</span>
                  </span>
                  <span className="mt-0.5 flex flex-wrap gap-x-3 text-xs text-(--color-muted-foreground)">
                    <span>{formatRelativeTime(t, trace.start, now)}</span>
                    <KeptLabel kept={trace.kept} />
                  </span>
                </span>
                {trace.status > 0 ? (
                  <span
                    className={cn(
                      "shrink-0 tabular-nums",
                      trace.status >= 500 ? "text-(--color-destructive)" : "text-(--color-muted-foreground)",
                    )}
                  >
                    {trace.status}
                  </span>
                ) : null}
                <span className="w-18 shrink-0 text-right tabular-nums">{formatMs(trace.durationMs)}</span>
              </Link>
            </li>
          ))}
        </ul>
      ) : (
        <p className="border-t border-(--color-border) py-6 text-sm text-(--color-muted-foreground)">
          {t("admin.traces.no_match")}
        </p>
      )}
    </TracesLayout>
  );
}

interface SpanNode {
  span: AdminSpan;
  depth: number;
  children: SpanNode[];
  // The time spent in the span itself, without the spans it started.
  selfMs: number;
}

function buildTree(spans: AdminSpan[]): SpanNode[] {
  const nodes = new Map(
    spans.map((span): [string, SpanNode] => [span.id, { span, depth: 0, children: [], selfMs: 0 }]),
  );
  const roots: SpanNode[] = [];
  for (const node of nodes.values()) {
    const parent = node.span.parentId ? nodes.get(node.span.parentId) : undefined;
    if (parent) parent.children.push(node);
    else roots.push(node);
  }
  const visit = (node: SpanNode, depth: number) => {
    node.depth = depth;
    node.children.sort((a, b) => a.span.startMs - b.span.startMs);
    const childMs = node.children.reduce((sum, child) => sum + child.span.durationMs, 0);
    node.selfMs = Math.max(0, node.span.durationMs - childMs);
    node.children.forEach((child) => visit(child, depth + 1));
  };
  roots.forEach((root) => visit(root, 0));
  return roots;
}

// niceStep returns a round step for about three gridlines over the total.
function niceStep(total: number) {
  const rough = total / 3;
  const power = 10 ** Math.floor(Math.log10(rough));
  return ([1, 2, 5, 10].find((m) => m * power >= rough) ?? 10) * power;
}

function SpanDetails({ node, trace }: { node: SpanNode; trace: AdminTrace }) {
  const { t } = useTranslation();
  const { span } = node;
  const typical = trace.operations[span.name];
  // Attributes are set in code, the other rows are labels of the panel.
  const rows: { key: string; value: ReactNode; code?: true }[] = [
    { key: t("admin.traces.start"), value: `+${formatMs(span.startMs)}` },
    { key: t("admin.traces.duration"), value: formatMs(span.durationMs) },
    ...(span.error ? [{ key: t("admin.traces.error"), value: span.error }] : []),
    ...span.attributes.map((a) => ({ key: a.key, value: a.value, code: true as const })),
  ];
  return (
    <div className="my-1 rounded-md border border-(--color-border) bg-(--color-surface) p-3 text-sm">
      <dl>
        {rows.map((row) => (
          <div key={row.key} className="grid gap-x-4 py-0.5 sm:grid-cols-[minmax(0,12rem)_minmax(0,1fr)]">
            <dt className={cn("text-xs/5 text-(--color-muted-foreground) wrap-anywhere", row.code && "font-mono")}>
              {row.key}
            </dt>
            <dd className={cn("text-xs/5 wrap-anywhere", row.code && "font-mono")}>{row.value}</dd>
          </div>
        ))}
      </dl>
      {typical && node.depth > 0 ? (
        <p className="mt-2 text-xs text-(--color-muted-foreground)">
          {fill(t("admin.traces.typical"), span.name, formatMs(typical.p50Ms), formatMs(typical.p95Ms), typical.calls)}
          {span.durationMs > typical.p95Ms ? (
            <span className="text-(--color-foreground)"> {t("admin.traces.slower_than_p95")}</span>
          ) : null}
        </p>
      ) : null}
    </div>
  );
}

function Waterfall({ trace }: { trace: AdminTrace }) {
  const { t } = useTranslation();
  const roots = useMemo(() => buildTree(trace.spans), [trace.spans]);
  const [collapsed, setCollapsed] = useState<Set<string>>(new Set());
  // A failed trace opens with the details of its last failed span, which is
  // usually where the error came from. The root comes first, so it is picked
  // only when no other span failed.
  const [selected, setSelected] = useState<string | null>(
    () => [...trace.spans].reverse().find((span) => span.error)?.id ?? null,
  );
  const total = Math.max(trace.durationMs, ...trace.spans.map((s) => s.startMs + s.durationMs), 0.001);
  const step = niceStep(total);
  const ticks = Array.from({ length: Math.floor(total / step) + 1 }, (_, i) => i * step);

  const rows: SpanNode[] = [];
  const walk = (node: SpanNode) => {
    rows.push(node);
    if (!collapsed.has(node.span.id)) node.children.forEach(walk);
  };
  roots.forEach(walk);

  const toggle = (id: string) =>
    setCollapsed((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  const grid = "grid grid-cols-[minmax(0,1fr)_auto] gap-x-3 sm:grid-cols-[minmax(0,15rem)_minmax(0,1fr)_4.5rem]";
  const track = (children?: ReactNode) => (
    <div className="relative col-span-2 h-5 sm:col-span-1">
      {ticks.map((tick) => (
        <span
          key={tick}
          aria-hidden
          className="absolute inset-y-0 w-px bg-(--color-border)"
          style={{ left: `${(tick / total) * 100}%` }}
        />
      ))}
      {children}
    </div>
  );

  return (
    <div className="mt-6 text-sm">
      <div className={cn(grid, "border-b border-(--color-border) pb-1 text-xs text-(--color-muted-foreground)")}>
        <span className="hidden sm:block">{t("admin.traces.span")}</span>
        <div className="relative col-span-2 h-4 sm:col-span-1">
          {ticks.map((tick) => (
            <span
              key={tick}
              // Labels near the end stay inside the timeline.
              className={cn(
                "absolute whitespace-nowrap tabular-nums",
                tick === 0 ? "" : tick / total > 0.85 ? "-translate-x-full" : "-translate-x-1/2",
              )}
              style={{ left: `${(tick / total) * 100}%` }}
            >
              {tick === 0 ? "0" : formatMs(tick)}
            </span>
          ))}
        </div>
        <span className="hidden text-right sm:block">{t("admin.traces.time")}</span>
      </div>
      <ul>
        {rows.map((node) => {
          const { span } = node;
          const isSelected = selected === span.id;
          const hasChildren = node.children.length > 0;
          const isCollapsed = collapsed.has(span.id);
          return (
            <li key={span.id}>
              <div className={cn(grid, "items-center py-1", isSelected && "bg-(--color-surface)")}>
                <div className="flex min-w-0 items-center gap-1" style={{ paddingLeft: `${node.depth * 0.75}rem` }}>
                  {hasChildren ? (
                    <button
                      type="button"
                      onClick={() => toggle(span.id)}
                      aria-expanded={!isCollapsed}
                      aria-label={fill(t(isCollapsed ? "admin.traces.expand" : "admin.traces.collapse"), span.name)}
                      className="flex size-6 shrink-0 cursor-pointer items-center justify-center rounded text-(--color-muted-foreground) hover:bg-(--color-surface) hover:text-(--color-foreground)"
                    >
                      {isCollapsed ? (
                        <ChevronRight className="size-4" aria-hidden />
                      ) : (
                        <ChevronDown className="size-4" aria-hidden />
                      )}
                    </button>
                  ) : (
                    <span className="w-6 shrink-0" />
                  )}
                  <button
                    type="button"
                    onClick={() => setSelected(isSelected ? null : span.id)}
                    aria-expanded={isSelected}
                    className={cn(
                      "flex min-h-6 min-w-0 cursor-pointer items-center gap-1.5 rounded text-left hover:underline",
                      span.error && "text-(--color-destructive)",
                    )}
                  >
                    {span.error ? (
                      <AlertCircle className="size-3.5 shrink-0" aria-label={t("admin.traces.error")} />
                    ) : null}
                    <span className="truncate">{span.name}</span>
                    <KindChip kind={span.kind} />
                  </button>
                </div>
                <span className="text-right tabular-nums sm:order-last">{formatMs(span.durationMs)}</span>
                {track(
                  <span
                    aria-hidden
                    className={cn(
                      "absolute top-1/2 h-2 min-w-0.5 -translate-y-1/2 rounded-sm",
                      span.error ? "bg-(--color-destructive)" : KIND_FILLS[span.kind],
                    )}
                    style={{
                      left: `${(span.startMs / total) * 100}%`,
                      width: `${(span.durationMs / total) * 100}%`,
                    }}
                  />,
                )}
              </div>
              {isSelected ? <SpanDetails node={node} trace={trace} /> : null}
            </li>
          );
        })}
      </ul>
      {trace.droppedSpans > 0 ? <Note>{fill(t("admin.traces.dropped_spans"), trace.droppedSpans)}</Note> : null}
    </div>
  );
}

function Breakdown({ trace }: { trace: AdminTrace }) {
  const { t } = useTranslation();
  const totals = { go: 0, git: 0 };
  const visit = (node: SpanNode) => {
    totals[node.span.kind === "git" ? "git" : "go"] += node.selfMs;
    node.children.forEach(visit);
  };
  buildTree(trace.spans).forEach(visit);
  const sum = totals.go + totals.git || 1;
  const parts = [
    { key: "go", label: t("admin.traces.go_code"), ms: totals.go, fill: KIND_FILLS.other },
    { key: "git", label: t("admin.traces.git"), ms: totals.git, fill: KIND_FILLS.git },
  ].filter((part) => part.ms > 0);
  return (
    <div className="mt-4">
      <div aria-hidden className="flex h-2 gap-0.5 overflow-hidden rounded-full">
        {parts.map((part) => (
          <span key={part.key} className={part.fill} style={{ width: `${(part.ms / sum) * 100}%` }} />
        ))}
      </div>
      <p className="mt-2 flex flex-wrap gap-x-4 gap-y-1 text-sm">
        {parts.map((part) => (
          <span key={part.key} className="inline-flex items-center gap-1.5">
            <span aria-hidden className={cn("size-2.5 rounded-sm", part.fill)} />
            {part.label}
            <span className="text-(--color-muted-foreground) tabular-nums">{formatMs(part.ms)}</span>
          </span>
        ))}
      </p>
    </div>
  );
}

// REPEATED_RUN is how many spans of one name in a row, under one parent, make
// a hint, e.g., a query or Git command run once per item of a list.
const REPEATED_RUN = 5;

function RepeatedSpans({ trace }: { trace: AdminTrace }) {
  const { t } = useTranslation();
  const runs: { name: string; count: number; ms: number }[] = [];
  const visit = (node: SpanNode) => {
    let i = 0;
    while (i < node.children.length) {
      const name = node.children[i].span.name;
      let j = i;
      let ms = 0;
      while (j < node.children.length && node.children[j].span.name === name) {
        ms += node.children[j].span.durationMs;
        j++;
      }
      if (j - i >= REPEATED_RUN) runs.push({ name, count: j - i, ms });
      i = j;
    }
    node.children.forEach(visit);
  };
  buildTree(trace.spans).forEach(visit);
  if (runs.length === 0) return null;
  runs.sort((a, b) => b.ms - a.ms);
  return (
    <Panel className="mt-4">
      <ul className="space-y-1">
        {runs.slice(0, 3).map((run, i) => {
          return (
            // A name can repeat under more than one parent.
            <li key={`${i}-${run.name}`}>
              {fillNodes(
                t("admin.traces.repeated"),
                <code className="font-mono wrap-anywhere">{run.name}</code>,
                run.count,
                formatMs(run.ms),
              )}
            </li>
          );
        })}
      </ul>
    </Panel>
  );
}

function RouteComparison({ trace }: { trace: AdminTrace }) {
  const { t } = useTranslation();
  const stats = trace.routeStats;
  if (!stats) return null;
  const max = Math.max(stats.p95Ms * 1.25, trace.durationMs * 1.02) || 1;
  const at = (ms: number) => `${Math.min(100, (ms / max) * 100)}%`;
  return (
    <Panel className="mt-4">
      <p>
        {fillNodes(t("admin.traces.compare"), `${Math.floor(stats.fasterPercent)}%`, <RouteName route={trace.route} />)}
      </p>
      <div aria-hidden className="relative mt-2 h-4 rounded-sm bg-(--color-border)/60">
        <span
          className="absolute inset-y-0 bg-(--color-muted-foreground)/40"
          style={{ left: at(stats.p50Ms), width: `calc(${at(stats.p95Ms)} - ${at(stats.p50Ms)})` }}
        />
        <span className="absolute -inset-y-1 w-0.5 bg-(--color-foreground)" style={{ left: at(trace.durationMs) }} />
      </div>
      <p className="mt-2 text-xs text-(--color-muted-foreground)">
        {fill(
          t("admin.traces.compare_band"),
          formatMs(stats.p50Ms),
          formatMs(stats.p95Ms),
          formatNumber(stats.requests),
        )}
      </p>
    </Panel>
  );
}

// AdminTraceView shows one trace: where its time went, how it compares to the
// other requests of its route, and every span on a timeline.
export function AdminTraceView({ trace }: { trace: AdminTrace }) {
  const { t } = useTranslation();
  return (
    <AdminLayout active="traces">
      <Link
        to="/admin/traces"
        className="mb-3 inline-flex items-center gap-1 text-sm text-(--color-primary) hover:underline"
      >
        <ArrowLeft className="size-4" aria-hidden />
        {t("admin.traces.back")}
      </Link>
      <h1 className="text-xl wrap-anywhere">
        {trace.method ? <span className="text-(--color-muted-foreground)">{trace.method} </span> : null}
        <span className="font-mono">{trace.path || trace.route.name}</span>
      </h1>
      <p className="mt-2 flex flex-wrap items-center gap-x-4 gap-y-1 text-sm text-(--color-muted-foreground)">
        {trace.status > 0 ? (
          <span
            className={cn(
              "rounded border px-1.5 tabular-nums",
              trace.status >= 500
                ? "border-(--color-destructive) text-(--color-destructive)"
                : "border-(--color-border) text-(--color-foreground)",
            )}
          >
            {trace.status}
          </span>
        ) : null}
        <span className="tabular-nums">{formatMs(trace.durationMs)}</span>
        <span>{fill(t("admin.traces.spans"), trace.spans.length + trace.droppedSpans)}</span>
        <span>{formatAbsoluteTime(trace.start)}</span>
        <KeptLabel kept={trace.kept} />
        <code className="font-mono text-xs wrap-anywhere">{trace.id}</code>
      </p>
      <Breakdown trace={trace} />
      <RouteComparison trace={trace} />
      <RepeatedSpans trace={trace} />
      <Waterfall trace={trace} />
    </AdminLayout>
  );
}
