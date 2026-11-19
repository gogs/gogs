import { useState } from "react";

// REFRESH_SECONDS are the intervals a page of the admin console can refresh
// itself at. 0 turns that off.
export const REFRESH_SECONDS = [0, 10, 30, 60, 300, 900, 1800, 3600];

const DEFAULT_SECONDS = 60;
const STORAGE_KEY = "gogs-admin-refresh";

function readSeconds() {
  try {
    const stored = window.localStorage.getItem(STORAGE_KEY);
    if (stored !== null && REFRESH_SECONDS.map(String).includes(stored)) return Number(stored);
  } catch {
    // Storage may be unavailable, e.g., in a private window.
  }
  return DEFAULT_SECONDS;
}

// useRefreshSeconds holds how often the pages of the admin console refresh
// themselves, which the browser remembers.
export function useRefreshSeconds() {
  const [seconds, setSeconds] = useState(readSeconds);
  const change = (next: number) => {
    setSeconds(next);
    try {
      window.localStorage.setItem(STORAGE_KEY, String(next));
    } catch {
      // The choice lasts until the page reloads.
    }
  };
  return [seconds, change] as const;
}
