import { useNavigate } from "@tanstack/react-router";
import { type KeyboardEvent, useCallback, useEffect, useRef } from "react";
import { useTranslation } from "react-i18next";
import {
  Bar,
  CartesianGrid,
  ComposedChart,
  Line,
  ReferenceLine,
  Scatter,
  ScatterChart,
  type ScatterShapeProps,
  XAxis,
  YAxis,
  ZAxis,
  useActiveTooltipDataPoints,
} from "recharts";

import {
  type ChartConfig,
  ChartContainer,
  ChartLegend,
  ChartLegendContent,
  ChartTooltip,
  ChartTooltipContent,
} from "@/components/ui/chart";
import { webContext } from "@/lib/context";
import { formatMs, formatNumber } from "@/lib/format";
import type { TraceSearch } from "@/lib/page-search";
import type { AdminTraceOverview, AdminTraceSearch } from "@/lib/queries/admin";
import { formatClockTime } from "@/lib/relative-time";
import { cn } from "@/lib/utils";
import { formatClockSeconds } from "@/pages/admin/TraceFormat";

const COMPACT_FMT = new Intl.NumberFormat(webContext.lang, { notation: "compact", maximumFractionDigits: 1 });

const BUCKET_MS = 5 * 60_000;

// formatMsTick writes a round duration without its trailing zeros, e.g.,
// "1 s" rather than "1.00 s", for the ticks of an axis.
function formatMsTick(ms: number) {
  return ms === 0
    ? "0"
    : formatMs(ms)
        .replace(/(\.\d*?)0+ /, "$1 ")
        .replace(". ", " ");
}

// TraceTrafficChart draws the requests of each 5 minutes as bars and their p95
// latency as a line, each on its own axis, and names the highest p95.
export function TraceTrafficChart({ overview, className }: { overview: AdminTraceOverview; className?: string }) {
  const { t } = useTranslation();
  const config = {
    requests: { label: t("admin.traces.requests_per_5m"), color: "var(--color-input)", format: formatNumber },
    p95Ms: { label: t("admin.traces.p95_latency"), color: "var(--color-primary)", format: formatMs },
  } satisfies ChartConfig;
  // A bucket without requests has no latency, so the line skips it.
  const data = overview.buckets.map((bucket) => ({ ...bucket, p95Ms: bucket.requests > 0 ? bucket.p95Ms : null }));
  const first = overview.buckets[0]?.start;
  const last = overview.buckets.at(-1)?.start;
  const maxP95 = Math.max(0, ...data.map((bucket) => bucket.p95Ms ?? 0));
  const peak = maxP95 > 0 ? data.findIndex((bucket) => bucket.p95Ms === maxP95) : -1;
  return (
    <ChartContainer config={config} className={cn("aspect-auto h-64 w-full", className)}>
      <ComposedChart
        data={data}
        // The top margin holds the label of a peak that reaches the top of the plot.
        margin={{ top: 20, right: 0, bottom: 0, left: 0 }}
        title={
          first && last
            ? t("admin.traces.chart_overview", { from: formatClockTime(first), to: formatClockTime(last) })
            : undefined
        }
      >
        <CartesianGrid vertical={false} strokeDasharray="3 3" />
        <XAxis
          dataKey="start"
          tickFormatter={formatClockTime}
          tickLine={false}
          axisLine={false}
          tickMargin={8}
          minTickGap={24}
        />
        <YAxis
          yAxisId="requests"
          width="auto"
          tickLine={false}
          axisLine={false}
          allowDecimals={false}
          tickFormatter={(value: number) => COMPACT_FMT.format(value)}
        />
        <YAxis
          yAxisId="p95"
          orientation="right"
          width="auto"
          tickLine={false}
          axisLine={false}
          tickFormatter={formatMsTick}
        />
        <ChartTooltip
          content={
            <ChartTooltipContent
              labelFormatter={(start) => {
                if (typeof start !== "string") return start;
                const from = Date.parse(start);
                return t("admin.traces.window", {
                  from: formatClockTime(from),
                  to: formatClockTime(from + BUCKET_MS),
                });
              }}
            />
          }
        />
        <ChartLegend content={<ChartLegendContent className="flex-wrap gap-y-1" />} itemSorter={null} />
        <Bar
          yAxisId="requests"
          dataKey="requests"
          fill="var(--color-requests)"
          radius={[3, 3, 0, 0]}
          isAnimationActive={false}
        />
        <Line
          yAxisId="p95"
          dataKey="p95Ms"
          type="monotone"
          stroke="var(--color-p95Ms)"
          strokeWidth={2}
          dot={{ r: 3, fill: "var(--color-p95Ms)" }}
          connectNulls
          isAnimationActive={false}
          label={({ x, y, index }) =>
            index === peak && typeof x === "number" && typeof y === "number" ? (
              <text
                x={x}
                y={y - 8}
                // Labels near an edge stay inside the chart.
                textAnchor={peak < 2 ? "start" : peak > data.length - 3 ? "end" : "middle"}
                // The outline keeps the label readable over a bar.
                className="fill-(--color-foreground) stroke-(--color-card) stroke-[3px] tabular-nums [paint-order:stroke]"
              >
                p95 {formatMs(maxP95)}
              </text>
            ) : (
              <g />
            )
          }
        />
      </ComposedChart>
    </ChartContainer>
  );
}

type TraceKind = "request" | "slow" | "error";

interface TracePoint {
  id: string;
  kind: TraceKind;
  time: number;
  // The duration on the log scale, which has no place for zero.
  ms: number;
  status: number;
}

// The floor of the log scale. No request is this fast.
const MIN_MS = 0.01;

// Marks are wider to the pointer than they look, so that a single one is not
// hard to hit.
const HIT_SIZE = 24;

// TraceMark draws a failed trace as a square and the others as dots, so that
// the shape tells them apart and not only the color.
function TraceMark({ cx, cy, isActive, payload }: Partial<ScatterShapeProps>) {
  if (cx === undefined || cy === undefined) return <g />;
  const kind = (payload as TracePoint | undefined)?.kind ?? "request";
  const r = isActive ? 6 : 4;
  const look = { fill: `var(--color-${kind})`, className: "stroke-(--color-card)", strokeWidth: isActive ? 2 : 1 };
  return (
    <g className="cursor-pointer">
      <rect x={cx - HIT_SIZE / 2} y={cy - HIT_SIZE / 2} width={HIT_SIZE} height={HIT_SIZE} fill="transparent" />
      {kind === "error" ? (
        <rect x={cx - r} y={cy - r} width={r * 2} height={r * 2} rx={1} {...look} />
      ) : (
        <circle cx={cx} cy={cy} r={r} {...look} />
      )}
    </g>
  );
}

function RoundIcon({ className }: { className: string }) {
  return <span aria-hidden className={cn("size-2 shrink-0 rounded-full", className)} />;
}

function RequestIcon() {
  return <RoundIcon className="bg-(--color-request)" />;
}

function SlowIcon() {
  return (
    <span aria-hidden className="inline-flex shrink-0 items-center gap-1">
      <RoundIcon className="bg-(--color-slow)" />
      <span className="w-3 border-t border-dashed border-(--color-slow)" />
    </span>
  );
}

function ErrorIcon() {
  return <span aria-hidden className="size-2 shrink-0 rounded-[1px] bg-(--color-error)" />;
}

// TICK_STEPS are the round steps between the ticks of a time axis, in seconds.
const TICK_STEPS = [10, 15, 30, 60, 120, 300, 600, 900, 1800];

function timeTicks(start: number, end: number) {
  // A window longer than the steps cover, e.g., from a link with an old start,
  // steps by as many whole hours as keep the ticks few.
  const hours = Math.ceil((end - start) / (6 * 3_600_000));
  const step = (TICK_STEPS.find((s) => (end - start) / (s * 1000) <= 6) ?? hours * 3600) * 1000;
  const ticks: number[] = [];
  for (let tick = Math.ceil(start / step) * step; tick <= end; tick += step) ticks.push(tick);
  return { ticks, format: step < 60_000 ? formatClockSeconds : formatClockTime };
}

// ActiveTrace reports the trace under the tooltip, which the arrow keys move
// between marks, so that Enter can open it.
function ActiveTrace({ onChange }: { onChange: (id: string | null) => void }) {
  const id = useActiveTooltipDataPoints<TracePoint>()?.[0]?.id ?? null;
  useEffect(() => onChange(id), [id, onChange]);
  return null;
}

// TraceScatterChart plots the duration of kept traces over time on a log
// scale. A mark opens its trace.
export function TraceScatterChart({ result, search }: { result: AdminTraceSearch; search: TraceSearch }) {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const active = useRef<string | null>(null);
  const now = Date.parse(result.now);
  // Start at the oldest kept trace, so that a server that started recently
  // spreads its traces over the plot, or at the window of the search.
  const oldest = result.oldest ? Date.parse(result.oldest) : now;
  const start = search.from ?? Math.max(now - result.maxAgeMinutes * 60_000, Math.min(oldest, now - 5 * 60_000));
  const end = Math.max(Math.min(search.to ?? now, now), start + 60_000);
  const slow = result.slowThresholdMs;

  const points = result.points
    .map(
      (p): TracePoint => ({
        id: p.id,
        kind: p.error ? "error" : slow > 0 && p.durationMs >= slow ? "slow" : "request",
        time: Date.parse(p.start),
        ms: Math.max(p.durationMs, MIN_MS),
        status: p.status,
      }),
    )
    .sort((a, b) => a.time - b.time);

  // Powers of ten from the fastest trace, at most 1 ms, up to the slowest
  // trace or the slow threshold, whichever is higher, so the threshold line
  // stays inside the plot.
  const durations = points.map((p) => p.ms);
  const bottom = Math.floor(Math.log10(Math.min(1, ...durations)));
  const top = Math.ceil(Math.log10(Math.max(10, slow, ...durations)));
  const yTicks = Array.from({ length: top - bottom + 1 }, (_, i) => 10 ** (bottom + i));
  const x = timeTicks(start, end);

  const config = {
    request: { label: t("admin.traces.request"), color: "var(--color-muted-foreground)", icon: RequestIcon },
    slow: {
      label: t("admin.traces.slower_than", { duration: formatMs(slow) }),
      color: "var(--color-primary)",
      icon: SlowIcon,
    },
    error: { label: t("admin.traces.error_5xx"), color: "var(--color-destructive)", icon: ErrorIcon },
    time: { label: t("admin.traces.start"), format: formatClockSeconds },
    ms: { label: t("admin.traces.duration"), format: formatMs },
    status: {
      label: t("admin.traces.status"),
      format: (status: number) => (status > 0 ? String(status) : t("admin.traces.background")),
    },
  } satisfies ChartConfig;

  const open = (id: string) => void navigate({ to: "/admin/traces/$id", params: { id } });
  const onKeyDown = (e: KeyboardEvent) => {
    if (e.key === "Enter" && active.current) open(active.current);
  };
  // The chart counts the marks drawn again as more steps after the last
  // trace, which would move the arrow keys onto nothing.
  const onKeyDownCapture = (e: KeyboardEvent) => {
    if (e.key === "ArrowRight" && active.current === points.at(-1)?.id) e.stopPropagation();
  };
  const setActive = useCallback((id: string | null) => {
    active.current = id;
  }, []);
  const marks = {
    shape: TraceMark,
    activeShape: TraceMark,
    isAnimationActive: false,
    onClick: (point: { payload?: TracePoint }) => {
      if (point.payload) open(point.payload.id);
    },
  };

  return (
    <ChartContainer
      config={config}
      className="aspect-auto h-56 w-full"
      onKeyDown={onKeyDown}
      onKeyDownCapture={onKeyDownCapture}
    >
      <ScatterChart
        margin={{ top: 8, right: 8, bottom: 0, left: 0 }}
        title={t("admin.traces.chart_search", { count: result.points.length })}
        desc={t("admin.traces.chart_search_keys")}
      >
        <CartesianGrid vertical={false} strokeDasharray="3 3" />
        <XAxis
          type="number"
          dataKey="time"
          name="time"
          domain={[start, end]}
          ticks={x.ticks}
          tickFormatter={x.format}
          tickLine={false}
          axisLine={false}
          tickMargin={8}
          minTickGap={24}
        />
        <YAxis
          type="number"
          dataKey="ms"
          name="ms"
          scale="log"
          domain={[10 ** bottom, 10 ** top]}
          ticks={yTicks}
          width="auto"
          tickLine={false}
          axisLine={false}
          tickFormatter={formatMsTick}
        />
        <ZAxis type="number" dataKey="status" name="status" range={[64, 64]} />
        <ChartTooltip
          cursor={false}
          content={
            <ChartTooltipContent
              hideIndicator
              labelFormatter={(_, payload) => {
                const point = payload[0]?.payload as TracePoint | undefined;
                return point ? <span className="font-mono">{point.id.slice(0, 16)}</span> : null;
              }}
            />
          }
        />
        <ChartLegend content={<ChartLegendContent nameKey="value" className="flex-wrap gap-y-1" />} itemSorter={null} />
        {slow > 0 ? <ReferenceLine y={slow} stroke="var(--color-slow)" strokeDasharray="4 3" /> : null}
        {/* The arrow keys move through the first series only, so it holds every
            trace, oldest first. The slow and the failed ones are drawn again
            above it, so that the marks around them do not cover them. */}
        <Scatter name="request" data={points} fill="var(--color-request)" {...marks} />
        {slow > 0 ? (
          <Scatter name="slow" data={points.filter((p) => p.kind === "slow")} fill="var(--color-slow)" {...marks} />
        ) : null}
        <Scatter name="error" data={points.filter((p) => p.kind === "error")} fill="var(--color-error)" {...marks} />
        <ActiveTrace onChange={setActive} />
      </ScatterChart>
    </ChartContainer>
  );
}
