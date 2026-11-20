import { type QueryKey, type UseQueryOptions, useQuery } from "@tanstack/react-query";
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

// refreshOptions are the options of a query that refreshes itself at the
// interval of useRefreshSeconds. Turning that off also stops React Query from
// asking again when the viewer comes back to the tab or goes online.
export function refreshOptions(seconds: number) {
  return {
    refetchInterval: seconds > 0 && seconds * 1000,
    refetchOnWindowFocus: seconds > 0,
    refetchOnReconnect: seconds > 0,
  };
}

// useRefreshed keeps what the loader of a page loaded fresh. The loader fills
// the query, and the page reads it again at the interval of useRefreshSeconds
// while its tab is in view. A refresh that fails keeps the last answer, which
// the RefreshControl tells. It returns what to show and the props of the
// RefreshControl of the page.
export function useRefreshed<T, K extends QueryKey>(query: UseQueryOptions<T, Error, T, K>, loaded: T) {
  const [seconds, setSeconds] = useRefreshSeconds();
  const result = useQuery({
    ...query,
    // The loader has just asked.
    refetchOnMount: false,
    ...refreshOptions(seconds),
  });
  return {
    data: result.data ?? loaded,
    refresh: {
      seconds,
      onSecondsChange: setSeconds,
      refreshing: result.isFetching,
      onRefresh: () => void result.refetch(),
      failedSince: result.isError ? result.dataUpdatedAt : undefined,
    },
  };
}
