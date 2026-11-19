import { webContext } from "@/lib/context";
import type { AdminSpanKind } from "@/lib/queries/admin";

// The fill of a span or operation in charts. Git gets the accent, since it is
// where Gogs spends most of its time.
export const KIND_FILLS: Record<AdminSpanKind, string> = {
  request: "bg-(--color-muted-foreground)/50",
  git: "bg-(--color-primary)",
  db: "bg-(--color-merged)",
  other: "bg-(--color-muted-foreground)/50",
};

const CLOCK_SECONDS_FMT = new Intl.DateTimeFormat(webContext.lang, {
  hour: "2-digit",
  minute: "2-digit",
  second: "2-digit",
  hour12: false,
});

const DAY_TIME_FMT = new Intl.DateTimeFormat(webContext.lang, {
  month: "short",
  day: "numeric",
  hour: "2-digit",
  minute: "2-digit",
  hour12: false,
});

const DAY_FMT = new Intl.DateTimeFormat(webContext.lang, { dateStyle: "medium" });

// formatDayTime formats Unix milliseconds as the day and the time of day, e.g.,
// "Nov 18, 14:35", which tells the hours of several days apart.
export function formatDayTime(time: number) {
  return DAY_TIME_FMT.format(new Date(time));
}

// formatDay formats Unix milliseconds as the day, e.g., "Nov 18, 2026".
export function formatDay(time: number) {
  return DAY_FMT.format(new Date(time));
}

const DAY_SECONDS_FMT = new Intl.DateTimeFormat(webContext.lang, {
  month: "short",
  day: "numeric",
  hour: "2-digit",
  minute: "2-digit",
  second: "2-digit",
  hour12: false,
});

// formatTraceStart returns how to write the start of a trace: the time of day
// to the second, with the day once the server keeps traces for longer than one.
export function formatTraceStart(maxAgeMinutes: number): (time: string | number) => string {
  return maxAgeMinutes > 24 * 60 ? (time) => DAY_SECONDS_FMT.format(new Date(time)) : formatClockSeconds;
}

const SHARE_FMT = new Intl.NumberFormat(webContext.lang, { style: "percent", maximumFractionDigits: 0 });
const SMALL_SHARE_FMT = new Intl.NumberFormat(webContext.lang, {
  style: "percent",
  minimumFractionDigits: 1,
  maximumFractionDigits: 1,
});

// formatShare writes a share of a whole as a percentage, e.g., "4.2%", and
// tells a share too small to show apart from none.
export function formatShare(share: number) {
  if (share > 0 && share < 0.001) return `<${SMALL_SHARE_FMT.format(0.001)}`;
  return (share > 0 && share < 0.1 ? SMALL_SHARE_FMT : SHARE_FMT).format(share);
}

// formatClockSeconds formats an ISO time or Unix milliseconds as the time of
// day to the second, e.g., "14:35:07", which tells traces of one minute apart.
export function formatClockSeconds(time: string | number) {
  return CLOCK_SECONDS_FMT.format(new Date(time));
}
