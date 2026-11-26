// Date formatting helpers used by commit metadata and similar timestamps.
//   - Relative time matches the thresholds of `computeTimeDiff` in `cmd/gogs/internal/web/webapi_admin_system.go`
//     (now / seconds / minutes / hours / days / weeks / months / years ago).
//     Strings come from Gogs's `[tool]` section, with `{count}` and `{suffix}`
//     placeholders.
//   - Absolute time uses `Intl.DateTimeFormat` keyed on `webContext.lang`, so
//     each locale gets its own calendar conventions out of the box without
//     shipping per-locale day/month strings.
import type { TFunction } from "i18next";

import { webContext } from "@/lib/context";
import { tCount } from "@/lib/i18n";

const MINUTE = 60;
const HOUR = 60 * MINUTE;
const DAY = 24 * HOUR;
const WEEK = 7 * DAY;
const MONTH = 30 * DAY;
const YEAR = 12 * MONTH;

export function formatRelativeTime(t: TFunction, iso: string, nowMs: number = Date.now()): string {
  const then = Date.parse(iso);
  if (Number.isNaN(then)) return iso;

  let diff = Math.floor((nowMs - then) / 1000);
  const suffix = diff < 0 ? t("tool.from_now") : t("tool.ago");
  if (diff < 0) diff = -diff;

  if (diff <= 0) return t("tool.now");
  if (diff <= 2) return tCount("tool.seconds", 1, ["tool.1s"], { suffix });
  if (diff < MINUTE) return tCount("tool.seconds", diff, ["tool.1s"], { suffix });
  if (diff < HOUR) return tCount("tool.minutes", Math.floor(diff / MINUTE), ["tool.1m"], { suffix });
  if (diff < DAY) return tCount("tool.hours", Math.floor(diff / HOUR), ["tool.1h"], { suffix });
  if (diff < WEEK) return tCount("tool.days", Math.floor(diff / DAY), ["tool.1d"], { suffix });
  if (diff < MONTH) return tCount("tool.weeks", Math.floor(diff / WEEK), ["tool.1w"], { suffix });
  if (diff < YEAR) return tCount("tool.months", Math.floor(diff / MONTH), ["tool.1mon"], { suffix });
  return tCount("tool.years", Math.floor(diff / YEAR), ["tool.1y"], { suffix });
}

const ABSOLUTE_TIME_FMT = new Intl.DateTimeFormat(webContext.lang, {
  weekday: "short",
  day: "2-digit",
  month: "short",
  year: "numeric",
  hour: "2-digit",
  minute: "2-digit",
  second: "2-digit",
  hour12: false,
  timeZoneName: "short",
});

const CLOCK_TIME_FMT = new Intl.DateTimeFormat(webContext.lang, { hour: "2-digit", minute: "2-digit", hour12: false });

// formatClockTime formats an ISO time or Unix milliseconds as the time of day,
// e.g., "14:35", for the ticks of a chart that covers hours.
export function formatClockTime(time: string | number): string {
  return CLOCK_TIME_FMT.format(new Date(time));
}

// formatAbsoluteTime formats an ISO time or Unix milliseconds.
export function formatAbsoluteTime(time: string | number): string {
  const d = new Date(typeof time === "number" ? time : Date.parse(time));
  if (Number.isNaN(d.getTime())) return String(time);
  return ABSOLUTE_TIME_FMT.format(d);
}
