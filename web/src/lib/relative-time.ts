// Date formatting helpers used by commit metadata and similar timestamps.
//   - Relative time matches `internal/tool/tool.go`'s `timeSince` thresholds
//     (now / seconds / minutes / hours / days / weeks / months / years ago).
//     Strings come from Gogs's `[tool]` section so the SPA reuses the existing
//     community translations. Quantity templates carry `%d` (count) and `%s`
//     (suffix) printf placeholders, substituted manually since i18next uses
//     `{count}` style interpolation.
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

// Substitute Go-style printf placeholders. Quantity templates may use either
// the simple `%d` / `%s` form (English, most locales) or the positional
// `%[1]d` / `%[2]s` form used by community translations that need to reorder
// the count and suffix (e.g. German: `%[2]s %[1]d Jahren`).
function fillQuantity(template: string, count: number, suffix: string): string {
  return template.replace(/%(?:\[1\])?d/g, String(count)).replace(/%(?:\[2\])?s/g, suffix);
}

function fillSuffix(template: string, suffix: string): string {
  return template.replace(/%(?:\[1\])?s/g, suffix);
}

export function formatRelativeTime(t: TFunction, iso: string, nowMs: number = Date.now()): string {
  const then = Date.parse(iso);
  if (Number.isNaN(then)) return iso;

  let diff = Math.floor((nowMs - then) / 1000);
  const suffix = diff < 0 ? t("tool.from_now") : t("tool.ago");
  if (diff < 0) diff = -diff;

  if (diff <= 0) return t("tool.now");
  if (diff <= 2) return fillSuffix(t("tool.1s"), suffix);
  if (diff < MINUTE) return fillQuantity(t("tool.seconds"), diff, suffix);
  if (diff < 2 * MINUTE) return fillSuffix(t("tool.1m"), suffix);
  if (diff < HOUR) return fillQuantity(t("tool.minutes"), Math.floor(diff / MINUTE), suffix);
  if (diff < 2 * HOUR) return fillSuffix(t("tool.1h"), suffix);
  if (diff < DAY) return fillQuantity(t("tool.hours"), Math.floor(diff / HOUR), suffix);
  if (diff < 2 * DAY) return fillSuffix(t("tool.1d"), suffix);
  if (diff < WEEK) return fillQuantity(t("tool.days"), Math.floor(diff / DAY), suffix);
  if (diff < 2 * WEEK) return fillSuffix(t("tool.1w"), suffix);
  if (diff < MONTH) return fillQuantity(t("tool.weeks"), Math.floor(diff / WEEK), suffix);
  if (diff < 2 * MONTH) return fillSuffix(t("tool.1mon"), suffix);
  if (diff < YEAR) return fillQuantity(t("tool.months"), Math.floor(diff / MONTH), suffix);
  if (diff < 2 * YEAR) return fillSuffix(t("tool.1y"), suffix);
  return fillQuantity(t("tool.years"), Math.floor(diff / YEAR), suffix);
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

export function formatAbsoluteTime(iso: string): string {
  const d = new Date(Date.parse(iso));
  if (Number.isNaN(d.getTime())) return iso;
  return ABSOLUTE_TIME_FMT.format(d);
}
