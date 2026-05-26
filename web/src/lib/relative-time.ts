// Date formatting helpers that mirror Gogs's server-side conventions:
//   - Relative time matches `internal/tool/tool.go`'s `timeSince` thresholds
//     (now / seconds / minutes / hours / days / weeks / months / years ago).
//   - Absolute format matches `time.RFC1123`, which is Gogs's default
//     `[time].FORMAT` and what server templates put in the `title` attribute
//     of `<span class="time-since">` elements.
//
// English-only for now: the Go side runs these strings through i18n, but the
// SPA's other date renderings (none yet) don't, and the source INI uses printf
// placeholders that don't round-trip through i18next. Wiring i18n through this
// helper is a follow-up once there's a second caller to motivate the shape.

const MINUTE = 60;
const HOUR = 60 * MINUTE;
const DAY = 24 * HOUR;
const WEEK = 7 * DAY;
const MONTH = 30 * DAY;
const YEAR = 12 * MONTH;

export function formatRelativeTime(iso: string, nowMs: number = Date.now()): string {
  const then = Date.parse(iso);
  if (Number.isNaN(then)) return iso;

  let diff = Math.floor((nowMs - then) / 1000);
  let suffix = "ago";
  if (diff < 0) {
    diff = -diff;
    suffix = "from now";
  }

  if (diff <= 0) return "now";
  if (diff <= 2) return `1 second ${suffix}`;
  if (diff < MINUTE) return `${diff} seconds ${suffix}`;
  if (diff < 2 * MINUTE) return `1 minute ${suffix}`;
  if (diff < HOUR) return `${Math.floor(diff / MINUTE)} minutes ${suffix}`;
  if (diff < 2 * HOUR) return `1 hour ${suffix}`;
  if (diff < DAY) return `${Math.floor(diff / HOUR)} hours ${suffix}`;
  if (diff < 2 * DAY) return `1 day ${suffix}`;
  if (diff < WEEK) return `${Math.floor(diff / DAY)} days ${suffix}`;
  if (diff < 2 * WEEK) return `1 week ${suffix}`;
  if (diff < MONTH) return `${Math.floor(diff / WEEK)} weeks ${suffix}`;
  if (diff < 2 * MONTH) return `1 month ${suffix}`;
  if (diff < YEAR) return `${Math.floor(diff / MONTH)} months ${suffix}`;
  if (diff < 2 * YEAR) return `1 year ${suffix}`;
  return `${Math.floor(diff / YEAR)} years ${suffix}`;
}

const DAY_NAMES = ["Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"];
const MONTH_NAMES = ["Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"];

function pad2(n: number): string {
  return n < 10 ? `0${n}` : String(n);
}

// Mirrors Go's `time.RFC1123` layout (`Mon, 02 Jan 2006 15:04:05 MST`) in the
// viewer's local timezone, so the tooltip reads the same as what server
// templates emit via `t.Format(conf.Time.FormatLayout)`.
export function formatAbsoluteTime(iso: string, now: Date = new Date(Date.parse(iso))): string {
  if (Number.isNaN(now.getTime())) return iso;
  const tz =
    new Intl.DateTimeFormat("en-US", { timeZoneName: "short" })
      .formatToParts(now)
      .find((p) => p.type === "timeZoneName")?.value ?? "";
  return (
    `${DAY_NAMES[now.getDay()]}, ${pad2(now.getDate())} ${MONTH_NAMES[now.getMonth()]} ${now.getFullYear()} ` +
    `${pad2(now.getHours())}:${pad2(now.getMinutes())}:${pad2(now.getSeconds())} ${tz}`
  );
}
