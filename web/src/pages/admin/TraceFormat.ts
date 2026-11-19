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

// formatClockSeconds formats an ISO time or Unix milliseconds as the time of
// day to the second, e.g., "14:35:07", which tells traces of one minute apart.
export function formatClockSeconds(time: string | number) {
  return CLOCK_SECONDS_FMT.format(new Date(time));
}
