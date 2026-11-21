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
  if (diff <= 2) return t("tool.1s", { suffix });
  if (diff < MINUTE) return t("tool.seconds", { count: diff, suffix });
  if (diff < 2 * MINUTE) return t("tool.1m", { suffix });
  if (diff < HOUR) return t("tool.minutes", { count: Math.floor(diff / MINUTE), suffix });
  if (diff < 2 * HOUR) return t("tool.1h", { suffix });
  if (diff < DAY) return t("tool.hours", { count: Math.floor(diff / HOUR), suffix });
  if (diff < 2 * DAY) return t("tool.1d", { suffix });
  if (diff < WEEK) return t("tool.days", { count: Math.floor(diff / DAY), suffix });
  if (diff < 2 * WEEK) return t("tool.1w", { suffix });
  if (diff < MONTH) return t("tool.weeks", { count: Math.floor(diff / WEEK), suffix });
  if (diff < 2 * MONTH) return t("tool.1mon", { suffix });
  if (diff < YEAR) return t("tool.months", { count: Math.floor(diff / MONTH), suffix });
  if (diff < 2 * YEAR) return t("tool.1y", { suffix });
  return t("tool.years", { count: Math.floor(diff / YEAR), suffix });
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

// formatAbsoluteTime formats an ISO time or Unix milliseconds.
export function formatAbsoluteTime(time: string | number): string {
  const d = new Date(typeof time === "number" ? time : Date.parse(time));
  if (Number.isNaN(d.getTime())) return String(time);
  return ABSOLUTE_TIME_FMT.format(d);
}
