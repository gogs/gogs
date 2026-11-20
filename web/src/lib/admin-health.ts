import type { TFunction } from "i18next";

import { formatMs, formatNumber } from "@/lib/format";
import type { AdminHealthKey, AdminHealthRange } from "@/lib/queries/admin";
import { formatAbsoluteTime } from "@/lib/relative-time";

type HealthSection =
  | "traffic"
  | "time"
  | "process"
  | "runtime"
  | "database"
  | "work"
  | "background"
  | "storage"
  | "tracing";

interface Metric {
  key: AdminHealthKey;
  section: HealthSection;
  label: string;
  followsTraffic?: true;
  // The least value worth flagging, in the unit of the measurement. Below it
  // the server is quiet, however far the value is from its usual range, e.g.,
  // the first request after an idle hour.
  floor: number;
  // The least a spike reaches to be worth flagging, when that is more than
  // the floor.
  spikeFloor?: number;
  // A value of 0 tells that nothing was measured, e.g., no request arrived, so
  // the sample says nothing about the level.
  idleAtZero?: true;
  // The value reads against the limit of the measurement, e.g., "3 of 60".
  ofLimit?: true;
  format: (v: number) => string;
}

// The floor of a measurement that says nothing by leaving its usual range,
// e.g., the used space of a disk, which only matters against its size.
const NEVER = Infinity;

const MB = 1 << 20;

// METRICS lists the measurements in the order they are shown. Metrics that
// follow traffic, e.g., requests, rise and fall every day, so only their spikes
// are flagged, not their level.
export const METRICS: Metric[] = [
  {
    key: "requests",
    section: "traffic",
    label: "admin.health.requests",
    followsTraffic: true,
    floor: 5,
    format: (v) => `${v.toFixed(1)} /s`,
  },
  { key: "failedRequests", section: "traffic", label: "admin.health.failed_requests", floor: 1, format: perMinute },
  // A single slow request sets the latency of a quiet moment.
  {
    key: "p50",
    section: "traffic",
    label: "admin.health.p50",
    floor: 50,
    spikeFloor: 250,
    idleAtZero: true,
    format: formatMs,
  },
  {
    key: "p95",
    section: "traffic",
    label: "admin.health.p95",
    floor: 50,
    spikeFloor: 250,
    idleAtZero: true,
    format: formatMs,
  },
  {
    key: "p99",
    section: "traffic",
    label: "admin.health.p99",
    floor: 100,
    spikeFloor: 500,
    idleAtZero: true,
    format: formatMs,
  },
  {
    key: "dbTime",
    section: "time",
    label: "admin.health.db_time",
    floor: 20,
    spikeFloor: 100,
    idleAtZero: true,
    format: formatMs,
  },
  {
    key: "gitTime",
    section: "time",
    label: "admin.health.git_time",
    floor: 20,
    spikeFloor: 100,
    idleAtZero: true,
    format: formatMs,
  },
  {
    key: "otherTime",
    section: "time",
    label: "admin.health.other_time",
    floor: 20,
    spikeFloor: 100,
    idleAtZero: true,
    format: formatMs,
  },
  { key: "cpu", section: "process", label: "admin.health.cpu", floor: 50, format: formatPercent },
  {
    key: "residentMemory",
    section: "process",
    label: "admin.health.resident_memory",
    floor: 256 * MB,
    format: formatBytes,
  },
  { key: "threads", section: "process", label: "admin.health.threads", floor: 50, format: formatCount },
  { key: "openFiles", section: "process", label: "admin.health.open_files", floor: 100, format: formatCount },
  { key: "heap", section: "runtime", label: "admin.health.heap", floor: 64 * MB, format: formatBytes },
  { key: "goMemory", section: "runtime", label: "admin.health.go_memory", floor: 128 * MB, format: formatBytes },
  {
    key: "allocRate",
    section: "runtime",
    label: "admin.health.alloc_rate",
    followsTraffic: true,
    floor: 50 * MB,
    format: (v) => `${formatBytes(v)}/s`,
  },
  { key: "goroutines", section: "runtime", label: "admin.health.goroutines", floor: 100, format: formatCount },
  { key: "gcPause", section: "runtime", label: "admin.health.gc_pause", floor: 1, format: formatMs },
  {
    key: "gcCycles",
    section: "runtime",
    label: "admin.health.gc_cycles",
    followsTraffic: true,
    floor: 30,
    format: perMinute,
  },
  { key: "gcCPU", section: "runtime", label: "admin.health.gc_cpu", floor: 10, format: formatPercent },
  { key: "schedLatency", section: "runtime", label: "admin.health.sched_latency", floor: 5, format: formatMs },
  {
    key: "dbConnections",
    section: "database",
    label: "admin.health.db_connections",
    floor: 5,
    ofLimit: true,
    format: formatCount,
  },
  {
    key: "dbIdleConnections",
    section: "database",
    label: "admin.health.db_idle_connections",
    floor: NEVER,
    format: formatCount,
  },
  { key: "dbWaits", section: "database", label: "admin.health.db_waits", floor: 1, format: perMinute },
  {
    key: "dbWaitTime",
    section: "database",
    label: "admin.health.db_wait_time",
    floor: 10,
    idleAtZero: true,
    format: formatMs,
  },
  {
    key: "gitProcesses",
    section: "work",
    label: "admin.health.git_processes",
    followsTraffic: true,
    floor: 5,
    format: formatCount,
  },
  {
    key: "codeSearches",
    section: "work",
    label: "admin.health.code_searches",
    followsTraffic: true,
    floor: 2,
    ofLimit: true,
    format: formatCount,
  },
  {
    key: "sshConnections",
    section: "work",
    label: "admin.health.ssh_connections",
    followsTraffic: true,
    floor: 20,
    format: formatCount,
  },
  { key: "mirrorQueue", section: "background", label: "admin.health.mirror_queue", floor: 10, format: formatCount },
  { key: "hookQueue", section: "background", label: "admin.health.hook_queue", floor: 10, format: formatCount },
  {
    key: "failedHookDeliveries",
    section: "background",
    label: "admin.health.failed_hook_deliveries",
    floor: 1,
    format: perMinute,
  },
  {
    key: "pullRequestQueue",
    section: "background",
    label: "admin.health.pull_request_queue",
    floor: 10,
    format: formatCount,
  },
  { key: "mailQueue", section: "background", label: "admin.health.mail_queue", floor: 10, format: formatCount },
  { key: "notices", section: "background", label: "admin.health.notices", floor: 1, format: perMinute },
  {
    key: "diskRepositories",
    section: "storage",
    label: "admin.health.disk_repositories",
    floor: NEVER,
    ofLimit: true,
    format: formatBytes,
  },
  {
    key: "diskData",
    section: "storage",
    label: "admin.health.disk_data",
    floor: NEVER,
    ofLimit: true,
    format: formatBytes,
  },
  {
    key: "diskLFS",
    section: "storage",
    label: "admin.health.disk_lfs",
    floor: NEVER,
    ofLimit: true,
    format: formatBytes,
  },
  {
    key: "traceMemory",
    section: "tracing",
    label: "admin.health.trace_memory",
    floor: NEVER,
    ofLimit: true,
    format: formatBytes,
  },
  { key: "droppedSpans", section: "tracing", label: "admin.health.dropped_spans", floor: 1, format: perMinute },
];

export const SECTIONS: Record<HealthSection, string> = {
  traffic: "admin.health.traffic",
  time: "admin.health.time",
  process: "admin.health.process",
  runtime: "admin.health.runtime",
  database: "admin.health.database",
  work: "admin.health.work",
  background: "admin.health.background",
  storage: "admin.health.storage",
  tracing: "admin.health.tracing",
};

export function formatCount(v: number) {
  return formatNumber(Math.round(v));
}

function perMinute(v: number) {
  return `${formatNumber(Math.round(v * 10) / 10)} /min`;
}

function formatPercent(v: number) {
  return `${v.toFixed(1)}%`;
}

function formatBytes(v: number) {
  if (v >= 2 ** 40) return `${(v / 2 ** 40).toFixed(2)} TB`;
  if (v >= 1 << 30) return `${(v / (1 << 30)).toFixed(2)} GB`;
  if (v >= MB) return `${Math.round(v / MB)} MB`;
  return `${Math.round(v / 1024)} KB`;
}

function median(values: number[]) {
  return [...values].sort((a, b) => a - b)[Math.floor(values.length / 2)] ?? 0;
}

// RECENT_POINTS is how many of the last points tell the recent level: 4
// minutes of the last hour, and 30 minutes of the last 24 hours.
const RECENT_POINTS: Record<AdminHealthRange, number> = { "1h": 16, "24h": 6 };

// A level is not read off fewer samples than this.
const LEVEL_SAMPLES = 3;

// Samples further apart than this, with idle time between them, are not
// neighbors.
const NEIGHBOR_MS = 2 * 60_000;

// flag tells when a metric left its usual range, which is the first half of
// the range: its recent level stands out, or it spiked. Both levels are
// medians, which a few odd samples do not move. A spike is reported at its
// start. Values below the floor of the metric are never flagged.
export function flag(
  points: [number, number][],
  {
    followsTraffic,
    floor,
    spikeFloor = floor,
    idleAtZero,
  }: Pick<Metric, "followsTraffic" | "floor" | "spikeFloor" | "idleAtZero">,
  range: AdminHealthRange,
): { up: number } | { spikeAt: number } | null {
  if (points.length < 20) return null;
  // Both levels are read off a span of time, so that idle samples leave it
  // with fewer samples and not with older ones. A span with too few samples
  // has no level, and a metric without a usual level only flags spikes.
  const half = Math.floor(points.length / 2);
  const levelOf = (span: [number, number][]) => {
    const values = span.filter(([, v]) => !idleAtZero || v > 0).map(([, v]) => v);
    return values.length < LEVEL_SAMPLES ? [] : values;
  };
  const usual = levelOf(points.slice(0, half));
  const base = median(usual);
  const spread = Math.max(median(usual.map((v) => Math.abs(v - base))), 0.5);
  const recent = median(levelOf(points.slice(-Math.min(RECENT_POINTS[range], points.length - half))));
  if (!followsTraffic && base > 0 && recent >= floor && recent / base >= 1.5) return { up: recent / base };
  // A point of the last hour is one sample, which on a quiet server can be a
  // single request, so there a spike counts as far as a neighbor reaches it
  // too. A point of the last 24 hours is the peak or the mean rate of 5
  // minutes, which is worth a flag on its own.
  const measured = idleAtZero ? points.filter(([, v]) => v > 0) : points;
  const values = measured.map(([, v]) => v);
  const near = (i: number, j: number) =>
    measured[j] && Math.abs(measured[j][0] - measured[i][0]) <= NEIGHBOR_MS ? values[j] : 0;
  const held = range === "1h" ? values.map((v, i) => Math.min(v, Math.max(near(i, i - 1), near(i, i + 1)))) : values;
  const peak = Math.max(...held);
  if (peak >= spikeFloor && peak > base + 6 * spread && peak >= 3 * base) {
    const start = held.findIndex((v) => v > base + (peak - base) / 2);
    return { spikeAt: measured[start][0] };
  }
  return null;
}

// flagText words a flag, e.g., "Spike from 14:32". Pass formatTime to shorten
// the time where the range is known, e.g., to the time of day.
export function flagText(t: TFunction, f: ReturnType<typeof flag>, formatTime = formatAbsoluteTime) {
  if (!f) return "";
  return "up" in f
    ? t("admin.health.up_recently", { factor: f.up.toFixed(1) })
    : t("admin.health.spike_from", { time: formatTime(f.spikeAt) });
}
