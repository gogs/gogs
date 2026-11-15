import { Link } from "@tanstack/react-router";
import { type KeyboardEvent, type MouseEvent, type ReactNode, useState } from "react";
import { useTranslation } from "react-i18next";

import { fill, formatMs, formatNumber } from "@/lib/format";
import type { AdminHealth, AdminHealthKey, AdminHealthRange, AdminHealthSeries } from "@/lib/queries/admin";
import { formatAbsoluteTime } from "@/lib/relative-time";
import { cn } from "@/lib/utils";

type HealthSection = "traffic" | "runtime" | "resources";

// METRICS lists the measurements in the order they are shown. Metrics that
// follow traffic, e.g., requests, rise and fall every day, so only their spikes
// are flagged, not their level.
const METRICS: {
  key: AdminHealthKey;
  section: HealthSection;
  label: string;
  followsTraffic?: true;
  format: (v: number) => string;
}[] = [
  {
    key: "requests",
    section: "traffic",
    label: "admin.health.requests",
    followsTraffic: true,
    format: (v) => `${v.toFixed(1)} /s`,
  },
  { key: "p95", section: "traffic", label: "admin.health.p95", format: formatMs },
  { key: "heap", section: "runtime", label: "admin.health.heap", format: formatBytes },
  { key: "goroutines", section: "runtime", label: "admin.health.goroutines", format: formatCount },
  { key: "gcPause", section: "runtime", label: "admin.health.gc_pause", format: formatMs },
  { key: "dbConnections", section: "resources", label: "admin.health.db_connections", format: formatCount },
  {
    key: "gitProcesses",
    section: "resources",
    label: "admin.health.git_processes",
    followsTraffic: true,
    format: formatCount,
  },
  { key: "openFiles", section: "resources", label: "admin.health.open_files", format: formatCount },
  { key: "mirrorQueue", section: "resources", label: "admin.health.mirror_queue", format: formatCount },
];

const SECTIONS: Record<HealthSection, string> = {
  traffic: "admin.health.traffic",
  runtime: "admin.health.runtime",
  resources: "admin.health.resources",
};

function formatCount(v: number) {
  return formatNumber(Math.round(v));
}

function formatBytes(v: number) {
  if (v >= 1 << 30) return `${(v / (1 << 30)).toFixed(2)} GB`;
  return `${Math.round(v / (1 << 20))} MB`;
}

function median(values: number[]) {
  return [...values].sort((a, b) => a - b)[Math.floor(values.length / 2)];
}

// flag tells when a metric left its usual range, which is the first half of
// the range: its recent level stands out, or it spiked. A spike is reported at
// its start.
function flag(points: [number, number][], followsTraffic: boolean): { up: number } | { spikeAt: number } | null {
  if (points.length < 20) return null;
  const values = points.map(([, v]) => v);
  const first = values.slice(0, Math.floor(values.length / 2));
  const base = median(first);
  const spread = Math.max(median(first.map((v) => Math.abs(v - base))), 0.5);
  const tail = values.slice(-Math.max(4, Math.round(values.length / 15)));
  const tailMean = tail.reduce((a, b) => a + b, 0) / tail.length;
  const peak = Math.max(...values);
  if (!followsTraffic && base > 0 && tailMean / base >= 1.5) return { up: tailMean / base };
  if (peak > base + 6 * spread && peak >= 3 * base) {
    const start = values.findIndex((v) => v > base + (peak - base) / 2);
    return { spikeAt: points[start][0] };
  }
  return null;
}

// valueAt returns the first value measured at or after the time, which sums
// up the step that holds the time, or the last one when the time is after
// every point.
function valueAt(points: [number, number][], at: number) {
  return (points.find(([t]) => t >= at) ?? points.at(-1))?.[1];
}

function Sparkline({
  series,
  start,
  end,
  cursor,
  flagged,
  label,
  onCursor,
  onPin,
  onClear,
  stepMs,
}: {
  series: AdminHealthSeries;
  start: number;
  end: number;
  cursor: number | null;
  flagged: boolean;
  label: string;
  onCursor: (at: number | null) => void;
  onPin: (at: number) => void;
  onClear: () => void;
  stepMs: number;
}) {
  const W = 300;
  const H = 100;
  const values = series.points.map(([, v]) => v);
  const peak = Math.max(0, ...values);
  // A limit far above the values, e.g., of open files, would flatten the line.
  const scaleToLimit = series.limit > 0 && series.limit <= peak * 10;
  const max = scaleToLimit ? series.limit : peak * 1.1 || 1;
  const x = (t: number) => ((t - start) / (end - start)) * W;
  const y = (v: number) => H - (Math.min(v, max) / max) * (H - 4) - 2;
  const line = series.points.map(([t, v]) => `${x(t).toFixed(1)},${y(v).toFixed(1)}`).join(" ");

  const timeAt = (e: MouseEvent<HTMLDivElement>) => {
    const rect = e.currentTarget.getBoundingClientRect();
    const ratio = Math.min(Math.max(0, (e.clientX - rect.left) / rect.width), 1);
    return start + ratio * (end - start);
  };
  const onKeyDown = (e: KeyboardEvent<HTMLDivElement>) => {
    const at = cursor ?? end;
    const step = e.shiftKey ? stepMs * 10 : stepMs;
    if (e.key === "ArrowLeft") onPin(Math.max(start, at - step));
    else if (e.key === "ArrowRight") onPin(Math.min(end, at + step));
    else if (e.key === "Home") onPin(start);
    else if (e.key === "End") onPin(end);
    else if (e.key === "Escape") onClear();
    else return;
    e.preventDefault();
  };

  return (
    <div
      role="slider"
      tabIndex={0}
      aria-label={label}
      aria-valuemin={start}
      aria-valuemax={end}
      aria-valuenow={cursor ?? end}
      aria-valuetext={formatAbsoluteTime(cursor ?? end)}
      onPointerMove={(e) => {
        if (e.pointerType === "mouse") onCursor(timeAt(e));
      }}
      onPointerLeave={() => onCursor(null)}
      onClick={(e) => onPin(timeAt(e))}
      onKeyDown={onKeyDown}
      className="relative h-9 min-w-0 cursor-crosshair rounded-sm outline-none focus-visible:ring-1 focus-visible:ring-(--color-ring)"
    >
      <svg viewBox={`0 0 ${W} ${H}`} preserveAspectRatio="none" className="size-full" aria-hidden>
        {scaleToLimit ? (
          <line
            x1={0}
            x2={W}
            y1={y(series.limit)}
            y2={y(series.limit)}
            className="stroke-(--color-border)"
            strokeDasharray="3 3"
            vectorEffect="non-scaling-stroke"
          />
        ) : null}
        <polyline
          points={line}
          fill="none"
          className={flagged ? "stroke-(--color-primary)" : "stroke-(--color-muted-foreground)"}
          strokeWidth={1.5}
          strokeLinejoin="round"
          vectorEffect="non-scaling-stroke"
        />
      </svg>
      {cursor !== null ? (
        <span
          aria-hidden
          className="pointer-events-none absolute inset-y-0 w-px bg-(--color-foreground)"
          style={{ left: `${((cursor - start) / (end - start)) * 100}%` }}
        />
      ) : null}
    </div>
  );
}

// ServerHealth shows how the server did over the last hour or day, one row per
// measurement. Moving over a chart, or along it with the arrow keys, reads
// every measurement at that moment, and links to the traces from then.
export function ServerHealth({
  health,
  range,
  children,
}: {
  health: AdminHealth;
  range: AdminHealthRange;
  // The numbers of right now, in a disclosure below the charts.
  children?: ReactNode;
}) {
  const { t } = useTranslation();
  const [hover, setHover] = useState<number | null>(null);
  const [pinned, setPinned] = useState<number | null>(null);
  const cursor = hover ?? pinned;
  const end = Date.parse(health.end);
  // Start at the first measurement, so that a server that started recently
  // spreads its measurements over the charts.
  const first = Math.min(end, ...health.series.flatMap((s) => (s.points.length > 0 ? [s.points[0][0]] : [])));
  const start = Math.max(Date.parse(health.start), Math.min(first, end - 5 * 60_000));
  const stepMs = health.stepSeconds * 1000;
  const rangeLabel = t(range === "1h" ? "admin.traces.last_hour" : "admin.health.last_24h");

  const rows = METRICS.flatMap((metric) => {
    const series = health.series.find((s) => s.key === metric.key);
    return series ? [{ metric, series, flag: flag(series.points, metric.followsTraffic ?? false) }] : [];
  });
  const empty = rows.every((row) => row.series.points.length === 0);

  const format = (row: (typeof rows)[number], v: number | undefined) => {
    if (v === undefined) return "N/A";
    const value = row.metric.format(v);
    return row.metric.key === "dbConnections" && row.series.limit > 0
      ? fill(t("admin.health.of"), value, formatCount(row.series.limit))
      : value;
  };
  const flagText = (f: ReturnType<typeof flag>) => {
    if (!f) return "";
    return "up" in f
      ? fill(t("admin.health.up_recently"), f.up.toFixed(1))
      : fill(t("admin.health.spike_from"), formatAbsoluteTime(f.spikeAt));
  };

  let readout: ReactNode = t(empty ? "admin.health.collecting" : "admin.health.help");
  if (cursor !== null) {
    const clock = formatAbsoluteTime(cursor);
    // Traces start before they end, and a point of the 24 hours sums up the 5
    // minutes before it, so open a window of at least a step around the moment.
    const around = Math.max(60_000, stepMs);
    const from = Math.round(cursor - around);
    const to = Math.round(cursor + around);
    readout = (
      <>
        <span className="font-medium text-(--color-foreground)">
          {hover === null ? fill(t("admin.health.pinned"), clock) : clock}
        </span>
        <Link
          to="/admin/traces"
          search={{ view: "search", from, to }}
          className="text-(--color-primary) hover:underline"
        >
          {fill(t("admin.health.open_traces"), formatAbsoluteTime(from), formatAbsoluteTime(to))}
        </Link>
        {pinned !== null ? (
          <button
            type="button"
            onClick={() => {
              setPinned(null);
              setHover(null);
            }}
            className="cursor-pointer text-(--color-primary) hover:underline"
          >
            {t("admin.health.clear")}
          </button>
        ) : null}
      </>
    );
  }

  return (
    <section aria-labelledby="server-health-title" className="mb-8">
      <div className="mb-2 flex flex-wrap items-center justify-between gap-2">
        <h2 id="server-health-title" className="text-lg font-semibold">
          {t("admin.health")}
        </h2>
        <RangeLinks
          label={t("admin.traces.range")}
          active={range}
          options={[
            { value: "1h", label: t("admin.traces.last_hour"), search: {} },
            { value: "24h", label: t("admin.health.last_24h"), search: { health: "24h" } },
          ]}
          to="/admin"
        />
      </div>
      <p
        aria-live="polite"
        className="mb-2 flex min-h-6 flex-wrap items-center gap-x-3 gap-y-1 text-sm text-(--color-muted-foreground)"
      >
        {readout}
      </p>
      {rows.map((row, i) => {
        const heading =
          i === 0 || rows[i - 1].metric.section !== row.metric.section ? (
            <div className="mt-4 flex items-center justify-between text-sm text-(--color-muted-foreground)">
              <h3>{t(SECTIONS[row.metric.section])}</h3>
              {row.metric.section === "traffic" ? (
                <Link to="/admin/traces" className="text-(--color-primary) hover:underline">
                  {t("admin.traces")}
                </Link>
              ) : null}
            </div>
          ) : null;
        const points = row.series.points;
        const now = points.at(-1)?.[1];
        const shown = cursor === null ? now : valueAt(points, cursor);
        const flagged = flagText(row.flag);
        const label = t(row.metric.label);
        return (
          <div key={row.metric.key}>
            {heading}
            {/* Below `sm`, the chart moves under the label and value. */}
            <div className="grid grid-cols-[minmax(0,1fr)_auto] items-center gap-x-4 gap-y-1 border-b border-(--color-border) py-2 text-sm sm:grid-cols-[minmax(0,13rem)_6.5rem_minmax(0,1fr)]">
              <div className="min-w-0">
                <div className="font-medium wrap-anywhere">{label}</div>
                {flagged ? (
                  <div className="flex items-center gap-1.5 text-xs text-(--color-muted-foreground)">
                    <span aria-hidden className="size-1.5 shrink-0 rounded-full bg-(--color-primary)" />
                    {flagged}
                  </div>
                ) : null}
              </div>
              <div className="text-right tabular-nums">{format(row, shown)}</div>
              <div className="col-span-2 min-w-0 sm:col-span-1">
                <Sparkline
                  series={row.series}
                  start={start}
                  end={end}
                  cursor={cursor}
                  flagged={flagged !== ""}
                  label={
                    fill(t("admin.health.chart"), label, rangeLabel, format(row, now)) + (flagged ? `, ${flagged}` : "")
                  }
                  onCursor={(at) => {
                    if (pinned === null) setHover(at);
                  }}
                  onPin={(at) => {
                    setPinned(at);
                    setHover(null);
                  }}
                  onClear={() => {
                    setPinned(null);
                    setHover(null);
                  }}
                  stepMs={stepMs}
                />
              </div>
            </div>
          </div>
        );
      })}
      {children}
    </section>
  );
}

// RangeLinks switches a page between ranges, e.g., the last hour and the last
// day, through its search params, so that the loader fetches the range.
export function RangeLinks<V extends string>({
  label,
  active,
  options,
  to,
}: {
  label: string;
  active: V;
  options: { value: V; label: string; search: Record<string, unknown> }[];
  to: string;
}) {
  return (
    <nav aria-label={label} className="flex gap-1">
      {options.map((option) => (
        <Link
          key={option.value}
          to={to}
          search={option.search}
          aria-current={option.value === active ? "page" : undefined}
          className={cn(
            "inline-flex h-8 items-center rounded-md border px-3 text-sm",
            option.value === active
              ? "border-(--color-ring) bg-(--color-surface) font-medium text-(--color-foreground)"
              : "border-(--color-input) text-(--color-muted-foreground) hover:bg-(--color-surface) hover:text-(--color-foreground)",
          )}
        >
          {option.label}
        </Link>
      ))}
    </nav>
  );
}
