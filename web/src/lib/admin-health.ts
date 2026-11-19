import type { TFunction } from "i18next";

import { formatMs, formatNumber } from "@/lib/format";
import type { AdminHealthKey, AdminHealthRange } from "@/lib/queries/admin";
import { formatAbsoluteTime } from "@/lib/relative-time";

type HealthSection = "traffic" | "runtime" | "resources";

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
  format: (v: number) => string;
}

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
  // A single slow request sets the p95 latency of a quiet moment.
  {
    key: "p95",
    section: "traffic",
    label: "admin.health.p95",
    floor: 50,
    spikeFloor: 250,
    idleAtZero: true,
    format: formatMs,
  },
  { key: "heap", section: "runtime", label: "admin.health.heap", floor: 64 * (1 << 20), format: formatBytes },
  { key: "goroutines", section: "runtime", label: "admin.health.goroutines", floor: 100, format: formatCount },
  { key: "gcPause", section: "runtime", label: "admin.health.gc_pause", floor: 1, format: formatMs },
  {
    key: "dbConnections",
    section: "resources",
    label: "admin.health.db_connections",
    floor: 5,
    format: formatCount,
  },
  {
    key: "gitProcesses",
    section: "resources",
    label: "admin.health.git_processes",
    followsTraffic: true,
    floor: 5,
    format: formatCount,
  },
  { key: "openFiles", section: "resources", label: "admin.health.open_files", floor: 100, format: formatCount },
  { key: "mirrorQueue", section: "resources", label: "admin.health.mirror_queue", floor: 10, format: formatCount },
];

export const SECTIONS: Record<HealthSection, string> = {
  traffic: "admin.health.traffic",
  runtime: "admin.health.runtime",
  resources: "admin.health.resources",
};

export function formatCount(v: number) {
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
  const measured = idleAtZero ? points.filter(([, v]) => v > 0) : points;
  if (measured.length < 20) return null;
  const values = measured.map(([, v]) => v);
  const first = values.slice(0, Math.floor(values.length / 2));
  const base = median(first);
  const spread = Math.max(median(first.map((v) => Math.abs(v - base))), 0.5);
  const recent = median(values.slice(-Math.max(4, Math.round(values.length / 15))));
  if (!followsTraffic && base > 0 && recent >= floor && recent / base >= 1.5) return { up: recent / base };
  // A point of the last hour is one sample, which on a quiet server can be a
  // single request, so there a spike counts as far as a neighbor reaches it
  // too. A point of the last 24 hours is the peak or the mean rate of 5
  // minutes, which is worth a flag on its own.
  const held =
    range === "1h" ? values.map((v, i) => Math.min(v, Math.max(values[i - 1] ?? 0, values[i + 1] ?? 0))) : values;
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
