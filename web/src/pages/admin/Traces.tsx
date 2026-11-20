import { Link, useNavigate } from "@tanstack/react-router";
import { AlertCircle, ChevronDown, ChevronRight, Search as SearchIcon, X } from "lucide-react";
import { type FormEvent, type ReactNode, useMemo, useState } from "react";
import { Trans, useTranslation } from "react-i18next";

import { AdminPageHeader, RefreshControl, STAT_VALUE, SegmentedLinks, StatCard } from "@/components/AdminPage";
import { InlineCode } from "@/components/InlineCode";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardFooter, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow, TableSortHead } from "@/components/ui/table";
import { useRefreshed } from "@/lib/admin-refresh";
import { formatMinutes, formatMs, formatNumber } from "@/lib/format";
import type { TraceSearch } from "@/lib/page-search";
import {
  type AdminSpan,
  type AdminTrace,
  type AdminTraceOverview as AdminTraceOverviewData,
  type AdminTraceSearch as AdminTraceSearchData,
  adminTraceOverviewQuery,
  adminTraceSearchQuery,
} from "@/lib/queries/admin";
import { formatAbsoluteTime, formatRelativeTime } from "@/lib/relative-time";
import { cn } from "@/lib/utils";
import { TraceScatterChart, TraceTrafficChart } from "@/pages/admin/TraceCharts";
import { KIND_FILLS, formatShare, formatTraceStart } from "@/pages/admin/TraceFormat";
import { KeptLabel, KindChip, RouteName } from "@/pages/admin/TraceParts";

const CARD_TITLE = "text-base";
const CARD_BODY = "pt-4 sm:pt-4";

const LINK = "text-(--color-primary) hover:underline";

const NUMBER_HEAD = "text-right";
const NUMBER_CELL = "w-px text-right whitespace-nowrap tabular-nums";
const MUTED = "text-(--color-muted-foreground)";

// The last column a narrow card shows takes the padding of the last column of
// the table, which is hidden then.
const LAST_SHOWN = "pr-5";

// Facts holds what the hidden columns of a table would tell, below the name
// of a row, until the card is wide enough for the columns.
function Facts({ className, children }: { className: string; children: ReactNode }) {
  return <span className={cn("mt-1 flex flex-wrap gap-x-3 text-xs tabular-nums", MUTED, className)}>{children}</span>;
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
  toolbar,
  children,
}: {
  view: "overview" | "search";
  q?: string;
  maxAgeMinutes: number;
  // Controls of the view shown, at the end of the row that switches views.
  toolbar?: ReactNode;
  children: ReactNode;
}) {
  const { t } = useTranslation();
  return (
    <div className="grid gap-4">
      <AdminPageHeader
        title={t("admin.traces")}
        description={t("admin.traces.desc", { duration: formatMinutes(maxAgeMinutes) })}
      >
        <TraceIDForm q={q} />
      </AdminPageHeader>
      <div className="flex min-w-0 flex-wrap items-center justify-between gap-2">
        <SegmentedLinks
          to="/admin/traces"
          label={t("admin.traces")}
          active={view}
          options={[
            { value: "overview", label: t("admin.traces.overview"), search: {} },
            { value: "search", label: t("admin.traces.search"), search: { view: "search" } },
          ]}
        />
        {toolbar ? <div className="flex min-w-0 flex-wrap items-center gap-2">{toolbar}</div> : null}
      </div>
      {children}
    </div>
  );
}

// TracingDisabledNote tells how to turn tracing on.
export function TracingDisabledNote() {
  return (
    <Trans
      i18nKey="admin.traces.disabled"
      values={{ setting: "ENABLED = true", section: "[tracing]" }}
      components={{ code: <InlineCode /> }}
    />
  );
}

// Disabled shows only the title and how to turn tracing on, since there is
// nothing to find or look at.
function Disabled() {
  const { t } = useTranslation();
  return <AdminPageHeader title={t("admin.traces")} description={<TracingDisabledNote />} />;
}

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
    <CardFooter className="border-t border-(--color-border) py-3 sm:py-3">
      <button type="button" onClick={onClick} className={cn("min-h-6 cursor-pointer text-sm", LINK)}>
        {t("admin.traces.show_all", { count })}
      </button>
    </CardFooter>
  );
}

type RouteStat = AdminTraceOverviewData["routes"][number];
type OperationStat = AdminTraceOverviewData["operations"][number];

// What a column of the routes sorts by. Errors sort by their share of the
// calls, which is what the column shows, then by how many there are.
const ROUTE_ORDERS = {
  calls: (a, b) => a.requests - b.requests,
  errors: (a, b) => a.errors / a.requests - b.errors / b.requests || a.errors - b.errors,
  p50: (a, b) => a.p50Ms - b.p50Ms,
  p95: (a, b) => a.p95Ms - b.p95Ms,
  p99: (a, b) => a.p99Ms - b.p99Ms,
} satisfies Record<string, (a: RouteStat, b: RouteStat) => number>;

const OPERATION_ORDERS = {
  calls: (a, b) => a.calls - b.calls,
  p50: (a, b) => a.p50Ms - b.p50Ms,
  p95: (a, b) => a.p95Ms - b.p95Ms,
  share: (a, b) => a.selfMs - b.selfMs,
} satisfies Record<string, (a: OperationStat, b: OperationStat) => number>;

interface Sort<Column> {
  column: Column;
  descending: boolean;
}

const OPERATIONS_BY_SHARE: Sort<keyof typeof OPERATION_ORDERS> = { column: "share", descending: true };

// useSorted sorts the rows by the column whose header was clicked: the largest
// first, the smallest first on a second click, and back to the first order of
// the table on a third. Rows that compare equal keep the order they came in.
function useSorted<Row, Column extends string>(
  rows: Row[],
  orders: Record<Column, (a: Row, b: Row) => number>,
  initial: Sort<Column> | null = null,
) {
  const [sort, setSort] = useState(initial);
  const sorted = useMemo(() => {
    if (!sort) return rows;
    const order = orders[sort.column];
    return [...rows].sort(sort.descending ? (a, b) => order(b, a) : order);
  }, [rows, orders, sort]);
  // head returns the props of the header of a column.
  const head = (column: Column) => ({
    sorted: sort?.column !== column ? undefined : sort.descending ? ("descending" as const) : ("ascending" as const),
    onSort: () =>
      setSort(
        sort?.column !== column
          ? { column, descending: true }
          : sort.descending
            ? { column, descending: false }
            : initial,
      ),
  });
  // by sorts by a column, the largest first, or in the first order of the table.
  const by = (column: Column | null) => setSort(column === null ? initial : { column, descending: true });
  return { sorted, head, column: sort?.column ?? null, by };
}

const FIRST_ORDER = "first";

// SortMenu sorts a table by any of its columns while the card is too narrow to
// show them all, and so their headers.
function SortMenu<Column extends string>({
  column,
  options,
  onChange,
}: {
  column: Column | null;
  // A null column is the first order of the table.
  options: { column: Column | null; label: string }[];
  onChange: (column: Column | null) => void;
}) {
  const { t } = useTranslation();
  return (
    <Select
      value={column ?? FIRST_ORDER}
      onValueChange={(v) => onChange(options.find((option) => (option.column ?? FIRST_ORDER) === v)?.column ?? null)}
    >
      <SelectTrigger aria-label={t("admin.traces.sort")} className="glass-control mt-2 h-9 w-auto gap-2 @lg:hidden">
        <span className={MUTED}>{t("admin.traces.sort")}</span>
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        {options.map((option) => (
          <SelectItem key={option.column ?? FIRST_ORDER} value={option.column ?? FIRST_ORDER}>
            {option.label}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}

// TableCard heads a table with what it lists, how to read it and the menu
// that sorts it. The table runs to the edges of the card.
function TableCard({
  title,
  description,
  sortMenu,
  children,
}: {
  title: string;
  description: string;
  sortMenu: ReactNode;
  children: ReactNode;
}) {
  return (
    <Card className="@container min-w-0 overflow-hidden">
      <CardHeader className="items-start pb-4">
        <CardTitle className={CARD_TITLE}>{title}</CardTitle>
        <CardDescription>{description}</CardDescription>
        {sortMenu}
      </CardHeader>
      {children}
    </Card>
  );
}

// AdminTraceOverview aggregates every request of the last 15 minutes or hour:
// how many, how fast, and where their time went. The page refreshes itself
// while its tab is in view.
export function AdminTraceOverview({ overview: loaded }: { overview: AdminTraceOverviewData }) {
  const { t } = useTranslation();
  const { data: overview, refresh } = useRefreshed(
    { ...adminTraceOverviewQuery(loaded.range), enabled: loaded.enabled },
    loaded,
  );
  const [allRoutes, setAllRoutes] = useState(false);
  const [allOperations, setAllOperations] = useState(false);
  // The server lists routes by their total time, which no column shows.
  const routes = useSorted(overview.routes, ROUTE_ORDERS);
  // The server lists operations by their share of the time.
  const operations = useSorted(overview.operations, OPERATION_ORDERS, OPERATIONS_BY_SHARE);
  if (!overview.enabled) {
    return <Disabled />;
  }
  const errorRate = overview.requests > 0 ? overview.errors / overview.requests : 0;
  const stats: { label: string; value: string; alert?: boolean }[] = [
    { label: t("admin.traces.requests"), value: formatNumber(overview.requests) },
    { label: t("admin.traces.errors"), value: formatShare(errorRate), alert: overview.errors > 0 },
    { label: "p50", value: formatMs(overview.p50Ms) },
    { label: "p95", value: formatMs(overview.p95Ms) },
    { label: "p99", value: formatMs(overview.p99Ms) },
  ];

  return (
    <TracesLayout
      view="overview"
      maxAgeMinutes={overview.maxAgeMinutes}
      toolbar={
        <>
          <SegmentedLinks
            to="/admin/traces"
            label={t("admin.traces.range")}
            active={overview.range}
            options={[
              { value: "15m", label: t("admin.traces.last_15m"), search: { range: "15m" } },
              { value: "1h", label: t("admin.traces.last_hour"), search: {} },
            ]}
          />
          <RefreshControl {...refresh} />
        </>
      }
    >
      <div className="grid grid-cols-2 gap-4 lg:grid-cols-5">
        {stats.map((stat) => (
          <StatCard key={stat.label} label={stat.label} className="last:col-span-2 lg:last:col-span-1">
            <p className={cn(STAT_VALUE, stat.alert && "text-(--color-destructive)")}>{stat.value}</p>
          </StatCard>
        ))}
      </div>
      <Card className="min-w-0">
        <CardHeader>
          <CardTitle className={CARD_TITLE}>{t("admin.overview.traffic")}</CardTitle>
          <CardDescription>{t("admin.traces.per_5m")}</CardDescription>
        </CardHeader>
        <CardContent className={CARD_BODY}>
          {overview.requests > 0 ? (
            <TraceTrafficChart overview={overview} />
          ) : (
            <p className={cn("py-10 text-center text-sm", MUTED)}>{t("admin.traces.no_requests")}</p>
          )}
        </CardContent>
      </Card>

      {overview.routes.length > 0 || overview.operations.length > 0 ? (
        <div className="grid items-start gap-4 xl:grid-cols-2">
          {overview.routes.length > 0 ? (
            <TableCard
              title={t("admin.traces.routes")}
              description={t("admin.traces.routes_help")}
              sortMenu={
                <SortMenu
                  column={routes.column}
                  onChange={routes.by}
                  options={[
                    { column: null, label: t("admin.traces.total_time") },
                    { column: "calls", label: t("admin.traces.calls") },
                    { column: "errors", label: t("admin.traces.errors") },
                    { column: "p50", label: "p50" },
                    { column: "p95", label: "p95" },
                    { column: "p99", label: "p99" },
                  ]}
                />
              }
            >
              <Table className="border-t border-(--color-border)">
                <TableHeader>
                  <TableRow className="hover:bg-transparent">
                    <TableHead>{t("admin.traces.route")}</TableHead>
                    <TableSortHead align="right" {...routes.head("calls")}>
                      {t("admin.traces.calls")}
                    </TableSortHead>
                    <TableSortHead align="right" className="hidden @lg:table-cell" {...routes.head("errors")}>
                      {t("admin.traces.errors")}
                    </TableSortHead>
                    <TableSortHead align="right" className="hidden @lg:table-cell" {...routes.head("p50")}>
                      p50
                    </TableSortHead>
                    <TableSortHead align="right" className={cn(LAST_SHOWN, "@lg:pr-3")} {...routes.head("p95")}>
                      p95
                    </TableSortHead>
                    <TableSortHead align="right" className="hidden @lg:table-cell" {...routes.head("p99")}>
                      p99
                    </TableSortHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {routes.sorted.slice(0, allRoutes ? undefined : COLLAPSED_ROWS).map((route) => {
                    const errors = route.errors > 0 ? formatShare(route.errors / route.requests) : "0";
                    return (
                      <TableRow key={route.name}>
                        <TableCell>
                          <Link
                            to="/admin/traces"
                            search={{ view: "search", route: route.name }}
                            className={cn("inline-block py-0.5", LINK)}
                          >
                            <RouteName route={route} />
                          </Link>
                          <ShareBar
                            share={overview.totalMs > 0 ? route.totalMs / overview.totalMs : 0}
                            className="bg-(--color-muted-foreground)/60"
                          />
                          <Facts className="@lg:hidden">
                            <span className={cn(route.errors > 0 && "text-(--color-destructive)")}>
                              {t("admin.traces.errors")} {errors}
                            </span>
                            <span>p50 {formatMs(route.p50Ms)}</span>
                            <span>p99 {formatMs(route.p99Ms)}</span>
                          </Facts>
                        </TableCell>
                        <TableCell className={NUMBER_CELL}>{formatNumber(route.requests)}</TableCell>
                        <TableCell
                          className={cn(
                            NUMBER_CELL,
                            "hidden @lg:table-cell",
                            route.errors > 0 ? "text-(--color-destructive)" : MUTED,
                          )}
                        >
                          {errors}
                        </TableCell>
                        <TableCell className={cn(NUMBER_CELL, MUTED, "hidden @lg:table-cell")}>
                          {formatMs(route.p50Ms)}
                        </TableCell>
                        <TableCell className={cn(NUMBER_CELL, LAST_SHOWN, "@lg:pr-3")}>
                          {formatMs(route.p95Ms)}
                        </TableCell>
                        <TableCell className={cn(NUMBER_CELL, MUTED, "hidden @lg:table-cell")}>
                          {formatMs(route.p99Ms)}
                        </TableCell>
                      </TableRow>
                    );
                  })}
                </TableBody>
              </Table>
              {allRoutes ? null : <ShowAll count={overview.routes.length} onClick={() => setAllRoutes(true)} />}
            </TableCard>
          ) : null}

          {overview.operations.length > 0 ? (
            <TableCard
              title={t("admin.traces.where_time_goes")}
              description={t("admin.traces.operations_help")}
              sortMenu={
                <SortMenu
                  column={operations.column}
                  onChange={operations.by}
                  options={[
                    { column: "share", label: t("admin.traces.share") },
                    { column: "calls", label: t("admin.traces.calls") },
                    { column: "p50", label: "p50" },
                    { column: "p95", label: "p95" },
                  ]}
                />
              }
            >
              <Table className="border-t border-(--color-border)">
                <TableHeader>
                  <TableRow className="hover:bg-transparent">
                    <TableHead>{t("admin.traces.operation")}</TableHead>
                    <TableSortHead align="right" {...operations.head("calls")}>
                      {t("admin.traces.calls")}
                    </TableSortHead>
                    <TableSortHead align="right" className="hidden @lg:table-cell" {...operations.head("p50")}>
                      p50
                    </TableSortHead>
                    <TableSortHead align="right" className="hidden @lg:table-cell" {...operations.head("p95")}>
                      p95
                    </TableSortHead>
                    <TableSortHead align="right" {...operations.head("share")}>
                      {t("admin.traces.share")}
                    </TableSortHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {operations.sorted.slice(0, allOperations ? undefined : COLLAPSED_ROWS).map((op) => {
                    const share = overview.spanSelfMs > 0 ? op.selfMs / overview.spanSelfMs : 0;
                    return (
                      <TableRow key={op.name}>
                        <TableCell>
                          <span className="inline-flex max-w-full items-baseline gap-1.5">
                            <KindChip kind={op.kind} />
                            <span className="min-w-0 font-mono wrap-anywhere">{op.name}</span>
                          </span>
                          <Facts className="@lg:hidden">
                            <span>p50 {formatMs(op.p50Ms)}</span>
                            <span>p95 {formatMs(op.p95Ms)}</span>
                          </Facts>
                        </TableCell>
                        <TableCell className={NUMBER_CELL}>{formatNumber(op.calls)}</TableCell>
                        <TableCell className={cn(NUMBER_CELL, MUTED, "hidden @lg:table-cell")}>
                          {formatMs(op.p50Ms)}
                        </TableCell>
                        <TableCell className={cn(NUMBER_CELL, "hidden @lg:table-cell")}>{formatMs(op.p95Ms)}</TableCell>
                        <TableCell className={cn(NUMBER_CELL, "min-w-16")}>
                          {formatShare(share)}
                          <ShareBar share={share} className={KIND_FILLS[op.kind]} />
                        </TableCell>
                      </TableRow>
                    );
                  })}
                </TableBody>
              </Table>
              {allOperations ? null : (
                <ShowAll count={overview.operations.length} onClick={() => setAllOperations(true)} />
              )}
            </TableCard>
          ) : null}
        </div>
      ) : null}
    </TracesLayout>
  );
}

const DURATION_FILTERS = [100, 500, 1000, 5000];

// AdminTraceSearch finds kept traces by ID, or filters them by route, duration
// and status. The matches of the filters refresh themselves while the tab of
// the page is in view. The answer for a trace ID stays as it is.
export function AdminTraceSearch({ result: loaded, search }: { result: AdminTraceSearchData; search: TraceSearch }) {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const { data: result, refresh } = useRefreshed(
    { ...adminTraceSearchQuery(search), enabled: loaded.enabled && !search.q },
    loaded,
  );
  const now = Date.parse(result.now);
  if (!result.enabled) {
    return <Disabled />;
  }
  const refine = (change: Partial<TraceSearch>) =>
    void navigate({ to: "/admin/traces", search: { ...search, view: "search", ...change } });
  const ALL = "*";

  if (search.q && result.total === 0) {
    return (
      <TracesLayout view="search" q={search.q} maxAgeMinutes={result.maxAgeMinutes}>
        <Card className="min-w-0">
          <CardContent className="text-sm">
            <p>
              <Trans
                i18nKey="admin.traces.not_found"
                values={{ id: search.q }}
                components={{ id: <code className="font-mono wrap-anywhere" /> }}
              />
            </p>
            <p className={cn("mt-2", MUTED)}>
              {t("admin.traces.not_found_desc", { duration: formatMinutes(result.maxAgeMinutes) })}
              {result.oldest
                ? ` ${t("admin.traces.oldest", { when: formatRelativeTime(t, result.oldest, now) })}`
                : null}
            </p>
            <Link to="/admin/traces" search={{ view: "search" }} className={cn("mt-3 inline-block py-0.5", LINK)}>
              {t("admin.traces.clear_search")}
            </Link>
          </CardContent>
        </Card>
      </TracesLayout>
    );
  }

  return (
    <TracesLayout
      view="search"
      q={search.q}
      maxAgeMinutes={result.maxAgeMinutes}
      toolbar={search.q ? undefined : <RefreshControl {...refresh} />}
    >
      {search.q ? null : (
        <div role="group" aria-label={t("admin.traces.filters")} className="flex flex-wrap items-center gap-2">
          <Select value={search.route ?? ALL} onValueChange={(v) => refine({ route: v === ALL ? undefined : v })}>
            <SelectTrigger
              aria-label={t("admin.traces.route")}
              className="glass-control h-9 w-full min-w-0 sm:w-auto sm:max-w-sm sm:min-w-48"
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
            <SelectTrigger aria-label={t("admin.traces.duration")} className="glass-control h-9 w-auto gap-2">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value={ALL}>{t("admin.traces.any_duration")}</SelectItem>
              {(search.minMs && !DURATION_FILTERS.includes(search.minMs)
                ? [...DURATION_FILTERS, search.minMs].sort((a, b) => a - b)
                : DURATION_FILTERS
              ).map((ms) => (
                <SelectItem key={ms} value={String(ms)}>
                  {t("admin.traces.at_least", { duration: formatMs(ms) })}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          {search.from || search.to ? (
            <span className="glass-control inline-flex min-h-9 max-w-full items-center gap-1 rounded-md border !border-(--color-ring) bg-(--color-surface) py-1 pr-1 pl-3 text-sm">
              {search.from && search.to
                ? t("admin.traces.window", {
                    from: formatAbsoluteTime(search.from),
                    to: formatAbsoluteTime(search.to),
                  })
                : search.from
                  ? t("admin.traces.window_from", { from: formatAbsoluteTime(search.from) })
                  : t("admin.traces.window_until", { to: formatAbsoluteTime(search.to ?? 0) })}
              <Link
                to="/admin/traces"
                search={{ ...search, from: undefined, to: undefined }}
                aria-label={t("admin.traces.clear_window")}
                className="flex size-6 shrink-0 items-center justify-center rounded text-(--color-muted-foreground) hover:bg-(--color-background) hover:text-(--color-foreground) glass-ui:hover:bg-(--color-surface)"
              >
                <X className="size-4" aria-hidden />
              </Link>
            </span>
          ) : null}
          <SegmentedLinks
            to="/admin/traces"
            label={t("admin.traces.status")}
            active={search.status ?? "all"}
            options={[
              { value: "all", label: t("admin.traces.all"), search: { ...search, view: "search", status: undefined } },
              { value: "5xx", label: "5xx", search: { ...search, view: "search", status: "5xx" } },
              { value: "4xx", label: "4xx", search: { ...search, view: "search", status: "4xx" } },
            ]}
          />
          <Select
            value={search.sort ?? "recent"}
            onValueChange={(v) => refine({ sort: v === "slowest" ? "slowest" : undefined })}
          >
            <SelectTrigger aria-label={t("admin.traces.sort")} className="glass-control h-9 w-auto gap-2">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="recent">{t("admin.traces.most_recent")}</SelectItem>
              <SelectItem value="slowest">{t("admin.traces.slowest")}</SelectItem>
            </SelectContent>
          </Select>
        </div>
      )}

      <Card className="min-w-0">
        <CardHeader>
          <CardTitle className={CARD_TITLE}>{t("admin.traces.kept_traces")}</CardTitle>
          <CardDescription>
            {t("admin.traces.count", { total: formatNumber(result.total), shown: result.traces.length })}
          </CardDescription>
        </CardHeader>
        <CardContent className={CARD_BODY}>
          {result.points.length > 0 ? (
            <TraceScatterChart result={result} search={search} />
          ) : (
            <p className={cn("py-10 text-center text-sm", MUTED)}>{t("admin.traces.no_match")}</p>
          )}
        </CardContent>
      </Card>

      {result.traces.length > 0 ? (
        <Card className="@container min-w-0 overflow-hidden">
          <Table>
            <TableHeader className="bg-(--color-surface)">
              <TableRow className="hover:bg-transparent">
                <TableHead>{t("admin.traces.trace_id")}</TableHead>
                <TableHead className="hidden @xl:table-cell">{t("admin.traces.request")}</TableHead>
                <TableHead className={cn(NUMBER_HEAD, LAST_SHOWN, "@3xl:pr-3")}>{t("admin.traces.status")}</TableHead>
                <TableHead className="hidden @3xl:table-cell">{t("admin.traces.start")}</TableHead>
                <TableHead className={cn(NUMBER_HEAD, "hidden @3xl:table-cell")}>
                  {t("admin.traces.duration")}
                </TableHead>
                <TableHead className={cn(NUMBER_HEAD, "hidden @3xl:table-cell")}>
                  {t("admin.traces.span_count")}
                </TableHead>
                <TableHead className="hidden @3xl:table-cell">{t("admin.traces.kept")}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {result.traces.map((trace) => {
                const request = (
                  <>
                    {trace.method ? <span className={MUTED}>{trace.method} </span> : null}
                    <span className="font-mono">{trace.path || trace.route.name}</span>
                  </>
                );
                const started = formatRelativeTime(t, trace.start, now);
                return (
                  <TableRow key={trace.id}>
                    <TableCell className="@xl:w-px @xl:whitespace-nowrap">
                      <Link
                        to="/admin/traces/$id"
                        params={{ id: trace.id }}
                        className={cn("inline-flex min-h-6 items-center font-mono", LINK)}
                      >
                        {trace.id.slice(0, 16)}
                      </Link>
                      <span className="block wrap-anywhere @xl:hidden">{request}</span>
                      <Facts className="@3xl:hidden">
                        <span>{started}</span>
                        <span>{formatMs(trace.durationMs)}</span>
                        <span>{t("admin.traces.spans", { count: trace.spanCount })}</span>
                        <KeptLabel kept={trace.kept} />
                      </Facts>
                    </TableCell>
                    <TableCell className="hidden wrap-anywhere @xl:table-cell">{request}</TableCell>
                    <TableCell
                      className={cn(
                        NUMBER_CELL,
                        LAST_SHOWN,
                        "@3xl:pr-3",
                        trace.status >= 500 ? "text-(--color-destructive)" : trace.status > 0 ? "" : MUTED,
                      )}
                    >
                      {trace.status > 0 ? trace.status : t("admin.traces.background")}
                    </TableCell>
                    <TableCell className="hidden w-px whitespace-nowrap tabular-nums @3xl:table-cell">
                      {formatTraceStart(result.maxAgeMinutes)(trace.start)}
                      <span className={cn("ml-2 hidden text-xs @5xl:inline", MUTED)}>{started}</span>
                    </TableCell>
                    <TableCell className={cn(NUMBER_CELL, "hidden @3xl:table-cell")}>
                      {formatMs(trace.durationMs)}
                    </TableCell>
                    <TableCell className={cn(NUMBER_CELL, "hidden @3xl:table-cell")}>
                      {formatNumber(trace.spanCount)}
                    </TableCell>
                    <TableCell className="hidden w-px whitespace-nowrap @3xl:table-cell">
                      <KeptLabel kept={trace.kept} />
                    </TableCell>
                  </TableRow>
                );
              })}
            </TableBody>
          </Table>
        </Card>
      ) : null}
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
          <div key={row.key} className="grid gap-x-4 py-0.5 @xl:grid-cols-[minmax(0,12rem)_minmax(0,1fr)]">
            <dt className={cn("text-xs/5 text-(--color-muted-foreground) wrap-anywhere", row.code && "font-mono")}>
              {row.key}
            </dt>
            <dd className={cn("text-xs/5 wrap-anywhere", row.code && "font-mono")}>{row.value}</dd>
          </div>
        ))}
      </dl>
      {typical && node.depth > 0 ? (
        <p className="mt-2 text-xs text-(--color-muted-foreground)">
          {t("admin.traces.typical", {
            name: span.name,
            p50: formatMs(typical.p50Ms),
            p95: formatMs(typical.p95Ms),
            count: typical.calls,
          })}
          {span.durationMs > typical.p95Ms ? (
            <span className="text-(--color-foreground)"> {t("admin.traces.slower_than_p95")}</span>
          ) : null}
        </p>
      ) : null}
    </div>
  );
}

// Waterfall lists every span on the timeline of the trace, below the span
// that started it. It needs a `@container` around it: a narrow one puts the
// bar of a span below its name.
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
  const grid = "grid grid-cols-[minmax(0,1fr)_auto] gap-x-3 px-2 @xl:grid-cols-[minmax(0,5fr)_minmax(0,6fr)_4.5rem]";
  const track = (children?: ReactNode) => (
    <div className="relative col-span-2 h-5 @xl:col-span-1">
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
    <div className="-mx-2 text-sm">
      <div className={cn(grid, "border-b border-(--color-border) pb-1 text-xs text-(--color-muted-foreground)")}>
        <span className="hidden @xl:block">{t("admin.traces.span")}</span>
        <div className="relative col-span-2 h-4 @xl:col-span-1">
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
        <span className="hidden text-right @xl:block">{t("admin.traces.time")}</span>
      </div>
      <ul>
        {rows.map((node) => {
          const { span } = node;
          const isSelected = selected === span.id;
          const hasChildren = node.children.length > 0;
          const isCollapsed = collapsed.has(span.id);
          return (
            <li key={span.id}>
              <div className={cn(grid, "items-center rounded-md py-1", isSelected && "bg-(--color-foreground)/5")}>
                <div className="flex min-w-0 items-center gap-1" style={{ paddingLeft: `${node.depth * 0.75}rem` }}>
                  {hasChildren ? (
                    <button
                      type="button"
                      onClick={() => toggle(span.id)}
                      aria-expanded={!isCollapsed}
                      aria-label={t(isCollapsed ? "admin.traces.expand" : "admin.traces.collapse", { name: span.name })}
                      className="flex size-6 shrink-0 cursor-pointer items-center justify-center rounded text-(--color-muted-foreground) hover:bg-(--color-foreground)/5 hover:text-(--color-foreground)"
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
                      "flex min-h-6 min-w-6 cursor-pointer items-center gap-1.5 rounded text-left hover:underline",
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
                <span className="text-right tabular-nums @xl:order-last">{formatMs(span.durationMs)}</span>
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
              {isSelected ? (
                <div className="px-2">
                  <SpanDetails node={node} trace={trace} />
                </div>
              ) : null}
            </li>
          );
        })}
      </ul>
      {trace.droppedSpans > 0 ? (
        <p className={cn("mt-2 px-2 text-xs", MUTED)}>
          {t("admin.traces.dropped_spans", { count: trace.droppedSpans })}
        </p>
      ) : null}
    </div>
  );
}

function SideCard({ title, children }: { title: string; children: ReactNode }) {
  return (
    <Card className="min-w-0">
      <CardHeader>
        <CardTitle className={CARD_TITLE}>{title}</CardTitle>
      </CardHeader>
      <CardContent className={cn(CARD_BODY, "text-sm")}>{children}</CardContent>
    </Card>
  );
}

function Breakdown({ trace }: { trace: AdminTrace }) {
  const { t } = useTranslation();
  const totals = { go: 0, git: 0, db: 0 };
  const visit = (node: SpanNode) => {
    const kind = node.span.kind;
    totals[kind === "git" || kind === "db" ? kind : "go"] += node.selfMs;
    node.children.forEach(visit);
  };
  buildTree(trace.spans).forEach(visit);
  const sum = totals.go + totals.git + totals.db || 1;
  const parts = [
    { key: "go", label: t("admin.traces.go_code"), ms: totals.go, fill: KIND_FILLS.other },
    { key: "git", label: t("admin.traces.git"), ms: totals.git, fill: KIND_FILLS.git },
    { key: "db", label: t("admin.traces.database"), ms: totals.db, fill: KIND_FILLS.db },
  ].filter((part) => part.ms > 0);
  return (
    <SideCard title={t("admin.traces.where_time_went")}>
      <div aria-hidden className="flex h-2 gap-0.5 overflow-hidden rounded-full">
        {parts.map((part) => (
          <span key={part.key} className={part.fill} style={{ width: `${(part.ms / sum) * 100}%` }} />
        ))}
      </div>
      <p className="mt-3 flex flex-wrap gap-x-4 gap-y-1">
        {parts.map((part) => (
          <span key={part.key} className="inline-flex items-center gap-1.5">
            <span aria-hidden className={cn("size-2.5 rounded-sm", part.fill)} />
            {part.label}
            <span className="text-(--color-muted-foreground) tabular-nums">{formatMs(part.ms)}</span>
          </span>
        ))}
      </p>
    </SideCard>
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
    <SideCard title={t("admin.traces.repeated_title")}>
      <ul className="space-y-1">
        {runs.slice(0, 3).map((run, i) => {
          return (
            // A name can repeat under more than one parent.
            <li key={`${i}-${run.name}`}>
              <Trans
                i18nKey="admin.traces.repeated"
                values={{ name: run.name, count: run.count, duration: formatMs(run.ms) }}
                components={{ name: <code className="font-mono wrap-anywhere" /> }}
              />
            </li>
          );
        })}
      </ul>
    </SideCard>
  );
}

function RouteComparison({ trace }: { trace: AdminTrace }) {
  const { t } = useTranslation();
  const stats = trace.routeStats;
  if (!stats) return null;
  const max = Math.max(stats.p95Ms * 1.25, trace.durationMs * 1.02) || 1;
  const at = (ms: number) => `${Math.min(100, (ms / max) * 100)}%`;
  return (
    <SideCard title={t("admin.traces.compared_to_route")}>
      <p>
        <Trans
          i18nKey="admin.traces.compare"
          values={{ percent: `${Math.floor(stats.fasterPercent)}%` }}
          components={{ route: <RouteName route={trace.route} /> }}
        />
      </p>
      <div aria-hidden className="relative mt-3 h-4 rounded-sm bg-(--color-border)/60">
        <span
          className="absolute inset-y-0 bg-(--color-muted-foreground)/40"
          style={{ left: at(stats.p50Ms), width: `calc(${at(stats.p95Ms)} - ${at(stats.p50Ms)})` }}
        />
        <span className="absolute -inset-y-1 w-0.5 bg-(--color-foreground)" style={{ left: at(trace.durationMs) }} />
      </div>
      <p className="mt-3 text-xs text-(--color-muted-foreground)">
        {t("admin.traces.compare_band", {
          p50: formatMs(stats.p50Ms),
          p95: formatMs(stats.p95Ms),
          requests: formatNumber(stats.requests),
        })}
      </p>
    </SideCard>
  );
}

// AdminTraceView shows one trace: every span on a timeline, and beside it
// where its time went and how it compares to the other requests of its route.
export function AdminTraceView({ trace }: { trace: AdminTrace }) {
  const { t } = useTranslation();
  return (
    <div className="grid gap-4">
      <AdminPageHeader
        back={{ to: "/admin/traces", label: t("admin.traces.back") }}
        crumb={trace.id.slice(0, 8)}
        title={
          <span className="font-normal">
            {trace.method ? <span className={MUTED}>{trace.method} </span> : null}
            <span className="font-mono">{trace.path || trace.route.name}</span>
          </span>
        }
        description={
          <span className="mt-1 flex flex-wrap items-center gap-x-4 gap-y-1">
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
            <span>{t("admin.traces.spans", { count: trace.spans.length + trace.droppedSpans })}</span>
            <span>{formatAbsoluteTime(trace.start)}</span>
            <KeptLabel kept={trace.kept} />
            <code className="font-mono text-xs wrap-anywhere">{trace.id}</code>
          </span>
        }
      />
      {/* The summary comes before the timeline, and sits beside it on a wide screen. */}
      <div className="grid items-start gap-4 xl:grid-cols-[minmax(0,1fr)_22rem]">
        <div className="grid min-w-0 gap-4 xl:col-start-2 xl:row-start-1">
          <Breakdown trace={trace} />
          <RouteComparison trace={trace} />
          <RepeatedSpans trace={trace} />
        </div>
        <Card className="@container min-w-0 xl:col-start-1 xl:row-start-1">
          <CardHeader>
            <CardTitle className={CARD_TITLE}>{t("admin.traces.timeline")}</CardTitle>
            <CardDescription>{t("admin.traces.timeline_desc")}</CardDescription>
          </CardHeader>
          <CardContent className={CARD_BODY}>
            <Waterfall trace={trace} />
          </CardContent>
        </Card>
      </div>
    </div>
  );
}
