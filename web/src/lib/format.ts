import { webContext } from "@/lib/context";

// fill puts values into a translated message in the order its Go
// placeholders, such as `%s`, `%d` or `%[1]s`, appear.
export function fill(message: string, ...values: (string | number)[]) {
  let i = 0;
  return message.replace(/%(?:\[\d+\])?[sdv]/g, () => String(values[i++] ?? ""));
}

const NUMBER_FMT = new Intl.NumberFormat(webContext.lang);

export function formatNumber(n: number) {
  return NUMBER_FMT.format(n);
}

// formatMs formats a duration in milliseconds with about three significant
// digits, e.g., "0.42 ms", "97 ms" or "2.52 s".
export function formatMs(ms: number) {
  // The bounds are where rounding reaches the next unit, e.g., 999.6 ms is "1.00 s".
  if (ms >= 999.5) return `${(ms / 1000).toFixed(ms >= 9995 ? 1 : 2)} s`;
  if (ms >= 9.95) return `${Math.round(ms)} ms`;
  if (ms >= 0.995) return `${ms.toFixed(1)} ms`;
  return `${ms.toFixed(2)} ms`;
}
