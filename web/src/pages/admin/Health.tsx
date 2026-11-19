import { Link } from "@tanstack/react-router";
import { type KeyboardEvent, memo, useId, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import {
  Area,
  AreaChart,
  CartesianGrid,
  type MouseHandlerDataParam,
  ReferenceDot,
  ReferenceLine,
  type TickItem,
  XAxis,
  YAxis,
} from "recharts";

import {
  AdminPageHeader,
  RefreshControl,
  STAT_VALUE,
  SegmentedLinks,
  StatCard,
  TrendChip,
} from "@/components/AdminPage";
import { type ChartConfig, ChartContainer, ChartTooltip } from "@/components/ui/chart";
import { METRICS, SECTIONS, flag, flagText, formatCount } from "@/lib/admin-health";
import { useRefreshed } from "@/lib/admin-refresh";
import { type AdminHealth, type AdminHealthRange, type AdminHealthSeries, adminHealthQuery } from "@/lib/queries/admin";
import { formatAbsoluteTime, formatClockTime } from "@/lib/relative-time";
import { cn } from "@/lib/utils";

const LINK = "inline-flex min-h-6 items-center text-(--color-primary) hover:underline";

const SYNC_ID = "admin-health";

// valueAt returns the first value measured at or after the time, which sums
// up the step that holds the time, or the last one when the time is after
// every point.
function valueAt(points: [number, number][], at: number) {
  return (points.find(([t]) => t >= at) ?? points.at(-1))?.[1];
}

// The series do not all hold the same times, e.g., one can start a sample
// later than the others, so the charts follow each other by time instead of by
// index, the same way valueAt reads a value.
function syncByTime(ticks: readonly TickItem[], data: MouseHandlerDataParam) {
  const at = Number(data.activeLabel);
  const index = ticks.findIndex((tick) => Number(tick.value) >= at);
  return index === -1 ? ticks.length - 1 : index;
}

// nearestTime snaps a time to the closest measurement, the way the cursor of
// a hover does, or keeps it when there are no measurements.
function nearestTime(points: [number, number][], at: number) {
  return points.reduce(
    (best, [time]) => (Math.abs(time - at) < Math.abs(best - at) ? time : best),
    points[0]?.[0] ?? at,
  );
}

function activeTime(state: MouseHandlerDataParam) {
  const at = Number(state.activeLabel);
  return state.activeLabel != null && Number.isFinite(at) ? at : null;
}

// MetricChart draws a measurement over the range. It only renders again when
// the pinned moment changes, since the charts move the cursor of a hover
// between themselves.
const MetricChart = memo(function MetricChart({
  series,
  start,
  end,
  pinned,
  flagged,
  onHover,
  onPin,
}: {
  series: AdminHealthSeries;
  start: number;
  end: number;
  pinned: number | null;
  flagged: boolean;
  onHover: (at: number | null) => void;
  onPin: (at: number) => void;
}) {
  const { t } = useTranslation();
  const gradient = `health-${useId().replace(/\W/g, "")}`;
  const data = useMemo(
    () =>
      series.points.length > 0
        ? series.points.map(([time, value]) => ({ time, value }))
        : // Recharts draws no axis without data, so a chart without measurements holds its two ends.
          [{ time: start }, { time: end }],
    [series.points, start, end],
  );
  const peak = Math.max(0, ...series.points.map(([, v]) => v));
  // A limit far above the values, e.g., of open files, would flatten the line.
  const scaleToLimit = series.limit > 0 && series.limit <= peak * 10;
  const max = scaleToLimit ? series.limit : peak * 1.1 || 1;
  const pinnedPoint = pinned === null ? undefined : series.points.find(([time]) => time === pinned);
  const config = {
    value: { color: flagged ? "var(--color-primary)" : "var(--color-muted-foreground)" },
  } satisfies ChartConfig;
  const dot = { r: 4, stroke: "var(--color-card)", strokeWidth: 2 };
  return (
    <ChartContainer
      config={config}
      // Chromium lets Tab and clicks focus the SVG groups that Recharts renders its layers into,
      // since React listens for focus events on them. The slider around the chart takes the focus.
      className="aspect-auto h-32 w-full [&_.recharts-curve.recharts-tooltip-cursor]:stroke-(--color-foreground) [&_.recharts-surface]:pointer-events-none [&_.recharts-surface]:[interactivity:inert]"
    >
      <AreaChart
        data={data}
        syncId={SYNC_ID}
        syncMethod={syncByTime}
        // The top margin holds the label of the limit.
        margin={{ top: 16, right: 0, bottom: 0, left: 0 }}
        accessibilityLayer={false}
        style={{ cursor: "crosshair" }}
        onMouseMove={(state) => onHover(state.isTooltipActive ? activeTime(state) : null)}
        onTouchMove={(state) => onHover(state.isTooltipActive ? activeTime(state) : null)}
        onMouseLeave={() => onHover(null)}
        // Recharts moves its cursor a frame after the pointer, so a click reads its own position.
        // Otherwise the first tap on a touch screen, which has no cursor yet, would pin nothing.
        onClick={(_, e) => {
          const box = e.currentTarget.getBoundingClientRect();
          const ratio = Math.min(Math.max(0, (e.clientX - box.left) / box.width), 1);
          onPin(nearestTime(series.points, start + ratio * (end - start)));
        }}
      >
        <defs>
          <linearGradient id={gradient} x1="0" y1="0" x2="0" y2="1">
            <stop offset="0%" stopColor="var(--color-value)" stopOpacity={0.25} />
            <stop offset="100%" stopColor="var(--color-value)" stopOpacity={0} />
          </linearGradient>
        </defs>
        <CartesianGrid vertical={false} strokeDasharray="3 3" horizontalValues={[0, max / 2, max]} />
        <XAxis
          dataKey="time"
          type="number"
          domain={[start, end]}
          ticks={[start, (start + end) / 2, end]}
          interval="preserveStartEnd"
          tickFormatter={formatClockTime}
          tickLine={false}
          axisLine={false}
          tickMargin={8}
        />
        <YAxis domain={[0, max]} allowDataOverflow hide />
        {/* A pinned moment holds the values, so a hover shows no cursor then. */}
        <ChartTooltip content={() => null} cursor={pinned === null} />
        <Area
          dataKey="value"
          type="monotone"
          stroke="var(--color-value)"
          strokeWidth={1.5}
          fill={`url(#${gradient})`}
          activeDot={pinned === null ? dot : false}
          isAnimationActive={false}
        />
        {scaleToLimit ? (
          <ReferenceLine
            y={series.limit}
            stroke="var(--color-muted-foreground)"
            strokeDasharray="3 3"
            label={{
              value: t("admin.health.limit", { limit: formatCount(series.limit) }),
              position: "insideBottomRight",
              offset: 4,
              fill: "var(--color-muted-foreground)",
            }}
          />
        ) : null}
        {pinned !== null ? <ReferenceLine x={pinned} stroke="var(--color-foreground)" /> : null}
        {pinnedPoint ? (
          <ReferenceDot x={pinnedPoint[0]} y={Math.min(pinnedPoint[1], max)} fill="var(--color-value)" {...dot} />
        ) : null}
      </AreaChart>
    </ChartContainer>
  );
});

// AdminServerHealth shows how the server did over the last hour or day, one
// card per measurement. Moving over a chart, or along it with the arrow keys,
// reads every measurement at that moment, and links to the traces from then.
// The page refreshes itself while its tab is in view.
export function AdminServerHealth({ health: loaded, range }: { health: AdminHealth; range: AdminHealthRange }) {
  const { t } = useTranslation();
  const { data: health, refresh } = useRefreshed(adminHealthQuery(range), loaded);
  const [hover, setHover] = useState<number | null>(null);
  const [pinned, setPinned] = useState<number | null>(null);
  const cursor = pinned ?? hover;
  const end = Date.parse(health.end);
  // The moments that were measured, oldest first.
  const times = useMemo(
    () => [...new Set(health.series.flatMap((s) => s.points.map(([time]) => time)))].sort((a, b) => a - b),
    [health],
  );
  // Start at the first measurement, so that a server that started recently
  // spreads its measurements over the charts.
  const start = Math.max(Date.parse(health.start), Math.min(times[0] ?? end, end - 5 * 60_000));
  const first = times[0] ?? start;
  const last = times.at(-1) ?? end;
  // The range moves on with each refresh, until it leaves a pinned moment behind.
  if (pinned !== null && pinned < first) setPinned(null);
  const stepMs = health.stepSeconds * 1000;
  const rangeLabel = t(range === "1h" ? "admin.traces.last_hour" : "admin.health.last_24h");

  const rows = METRICS.flatMap((metric) => {
    const series = health.series.find((s) => s.key === metric.key);
    return series ? [{ metric, series, flag: flag(series.points, metric, health.range) }] : [];
  });
  const empty = rows.every((row) => row.series.points.length === 0);

  const format = (row: (typeof rows)[number], v: number | undefined) => {
    if (v === undefined) return "N/A";
    const value = row.metric.format(v);
    return row.metric.key === "dbConnections" && row.series.limit > 0
      ? t("admin.health.of", { value, limit: formatCount(row.series.limit) })
      : value;
  };

  const onKeyDown = (e: KeyboardEvent<HTMLDivElement>) => {
    const at = cursor ?? last;
    // The cursor steps between the measurements, so it never rests on a moment
    // without one, e.g., while the server was paused.
    const move = (by: number) => {
      const nearest = times.reduce((best, time, i) => (Math.abs(time - at) < Math.abs(times[best] - at) ? i : best), 0);
      setPinned(times[Math.min(Math.max(0, nearest + by), times.length - 1)] ?? at);
    };
    const by = e.shiftKey ? 10 : 1;
    if (e.key === "ArrowLeft") move(-by);
    else if (e.key === "ArrowRight") move(by);
    else if (e.key === "Home") setPinned(first);
    else if (e.key === "End") setPinned(last);
    else if (e.key === "Escape") setPinned(null);
    else return;
    e.preventDefault();
  };

  // Traces start before they end, and a point of the 24 hours sums up the 5
  // minutes before it, so open a window of at least a step around the moment.
  const around = Math.max(60_000, stepMs);

  return (
    <div className="grid gap-4">
      <AdminPageHeader title={t("admin.health")}>
        <SegmentedLinks
          label={t("admin.traces.range")}
          active={range}
          options={[
            { value: "1h", label: t("admin.traces.last_hour"), search: {} },
            { value: "24h", label: t("admin.health.last_24h"), search: { health: "24h" } },
          ]}
          to="/admin/health"
        />
        <RefreshControl {...refresh} />
      </AdminPageHeader>
      {/* The help stays in the layout while a moment shows, so the charts do not jump under the pointer. */}
      <div aria-live="polite" className="grid text-sm/6 text-(--color-muted-foreground)">
        <p className={cn("col-start-1 row-start-1", cursor !== null && "invisible")}>
          {t(empty ? "admin.health.collecting" : "admin.health.help")}
        </p>
        {cursor !== null ? (
          <p className="col-start-1 row-start-1 flex flex-wrap items-center gap-x-3 self-start">
            <span className="font-medium text-(--color-foreground)">
              {pinned === null
                ? formatAbsoluteTime(cursor)
                : t("admin.health.pinned", { time: formatAbsoluteTime(cursor) })}
            </span>
            <Link
              to="/admin/traces"
              search={{ view: "search", from: Math.round(cursor - around), to: Math.round(cursor + around) }}
              className={LINK}
            >
              {t("admin.health.open_traces", {
                from: formatClockTime(cursor - around),
                to: formatClockTime(cursor + around),
              })}
            </Link>
            {pinned !== null ? (
              <button
                type="button"
                onClick={() => {
                  setPinned(null);
                  setHover(null);
                }}
                className={cn(LINK, "cursor-pointer")}
              >
                {t("admin.health.clear")}
              </button>
            ) : null}
          </p>
        ) : null}
      </div>
      {(Object.keys(SECTIONS) as (keyof typeof SECTIONS)[]).map((section) => {
        const metrics = rows.filter((row) => row.metric.section === section);
        if (metrics.length === 0) return null;
        return (
          <section key={section} aria-labelledby={`health-${section}`} className="grid gap-3">
            <div className="flex min-h-6 items-center justify-between gap-4 text-sm">
              <h2 id={`health-${section}`} className="font-medium">
                {t(SECTIONS[section])}
              </h2>
              {section === "traffic" ? (
                <Link to="/admin/traces" className={LINK}>
                  {t("admin.traces")}
                </Link>
              ) : null}
            </div>
            <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
              {metrics.map((row) => {
                const points = row.series.points;
                const now = points.at(-1)?.[1];
                const shown = cursor === null ? now : valueAt(points, cursor);
                const flagged = flagText(t, row.flag, formatClockTime);
                const label = t(row.metric.label);
                return (
                  <StatCard
                    key={row.metric.key}
                    as="h3"
                    label={label}
                    chip={flagged ? <TrendChip>{flagged}</TrendChip> : null}
                  >
                    <p className={cn(STAT_VALUE, shown === undefined && "text-(--color-muted-foreground)")}>
                      {format(row, shown)}
                    </p>
                    <div
                      role="slider"
                      tabIndex={0}
                      aria-label={
                        t("admin.health.chart", { label, range: rangeLabel, value: format(row, now) }) +
                        (flagged ? `, ${flagged}` : "")
                      }
                      aria-valuemin={first}
                      aria-valuemax={last}
                      aria-valuenow={cursor ?? last}
                      aria-valuetext={formatAbsoluteTime(cursor ?? last)}
                      onKeyDown={onKeyDown}
                      className="min-w-0 rounded-sm outline-none focus-visible:ring-1 focus-visible:ring-(--color-ring)"
                    >
                      <MetricChart
                        series={row.series}
                        start={start}
                        end={end}
                        pinned={pinned}
                        flagged={flagged !== ""}
                        onHover={setHover}
                        onPin={setPinned}
                      />
                    </div>
                  </StatCard>
                );
              })}
            </div>
          </section>
        );
      })}
    </div>
  );
}
