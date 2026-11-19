import type { TFunction } from "i18next";

import { formatMs, formatNumber } from "@/lib/format";
import type { AdminHealthKey } from "@/lib/queries/admin";
import { formatAbsoluteTime } from "@/lib/relative-time";

type HealthSection = "traffic" | "runtime" | "resources";

// METRICS lists the measurements in the order they are shown. Metrics that
// follow traffic, e.g., requests, rise and fall every day, so only their spikes
// are flagged, not their level.
export const METRICS: {
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
// the range: its recent level stands out, or it spiked. A spike is reported at
// its start.
export function flag(points: [number, number][], followsTraffic: boolean): { up: number } | { spikeAt: number } | null {
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

// flagText words a flag, e.g., "Spike from 14:32". Pass formatTime to shorten
// the time where the range is known, e.g., to the time of day.
export function flagText(t: TFunction, f: ReturnType<typeof flag>, formatTime = formatAbsoluteTime) {
  if (!f) return "";
  return "up" in f
    ? t("admin.health.up_recently", { factor: f.up.toFixed(1) })
    : t("admin.health.spike_from", { time: formatTime(f.spikeAt) });
}
