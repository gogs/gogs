import type { CodeViewHandle, CodeViewItem } from "@pierre/diffs/react";
import { ChevronDown, ChevronUp, Search } from "lucide-react";
import { type RefObject, useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";

interface Match {
  itemId: string;
  side: "additions" | "deletions";
  lineNumber: number;
}

// Walk `hunkContent` so context lines (which exist on both `additionLines` and
// `deletionLines`) are counted once, not twice. Changes are counted on each
// side they actually appear. Guard every array read because a malformed patch
// or stale Pierre indices would otherwise crash the whole search panel on
// `undefined.toLowerCase()`.
function buildMatches(items: readonly CodeViewItem[], query: string): Match[] {
  if (!query) return [];
  const needle = query.toLowerCase();
  const out: Match[] = [];
  for (const item of items) {
    if (item.type !== "diff") continue;
    const { additionLines, deletionLines, hunks } = item.fileDiff;
    for (const h of hunks) {
      for (const c of h.hunkContent) {
        if (c.type === "context") {
          for (let k = 0; k < c.lines; k++) {
            const line = additionLines[c.additionLineIndex + k];
            if (line === undefined) continue;
            if (line.toLowerCase().includes(needle)) {
              out.push({
                itemId: item.id,
                side: "additions",
                lineNumber: h.additionStart + (c.additionLineIndex + k - h.additionLineIndex),
              });
            }
          }
        } else {
          for (let k = 0; k < c.deletions; k++) {
            const line = deletionLines[c.deletionLineIndex + k];
            if (line === undefined) continue;
            if (line.toLowerCase().includes(needle)) {
              out.push({
                itemId: item.id,
                side: "deletions",
                lineNumber: h.deletionStart + (c.deletionLineIndex + k - h.deletionLineIndex),
              });
            }
          }
          for (let k = 0; k < c.additions; k++) {
            const line = additionLines[c.additionLineIndex + k];
            if (line === undefined) continue;
            if (line.toLowerCase().includes(needle)) {
              out.push({
                itemId: item.id,
                side: "additions",
                lineNumber: h.additionStart + (c.additionLineIndex + k - h.additionLineIndex),
              });
            }
          }
        }
      }
    }
  }
  return out;
}

interface Props<L> {
  items: readonly CodeViewItem[];
  viewRef: RefObject<CodeViewHandle<L> | null>;
}

export function DiffSearch<L>({ items, viewRef }: Props<L>) {
  const { t } = useTranslation();
  const [query, setQuery] = useState("");
  const [activeIndex, setActiveIndex] = useState(0);
  const inputRef = useRef<HTMLInputElement>(null);

  // Recompute matches whenever the query (or item set) changes. Debounce-free
  // because users expect instant feedback while typing.
  const matches = useMemo(() => buildMatches(items, query), [items, query]);

  // Surface a match by scrolling to it and emphasizing its line on the
  // underlying CodeView. Returns the clamped index so callers can sync state.
  const surface = useCallback(
    (index: number, list: Match[]): number => {
      const view = viewRef.current;
      if (!view || list.length === 0) return 0;
      const safe = ((index % list.length) + list.length) % list.length;
      const m = list[safe];
      view.scrollTo({
        type: "line",
        id: m.itemId,
        lineNumber: m.lineNumber,
        side: m.side,
        align: "center",
        behavior: "smooth",
      });
      view.setSelectedLines({
        id: m.itemId,
        range: { start: m.lineNumber, end: m.lineNumber, side: m.side, endSide: m.side },
      });
      return safe;
    },
    [viewRef],
  );

  // Update query + immediately surface the first match. Doing it inline (not
  // in an effect) avoids react-hooks/set-state-in-effect and keeps the search
  // UX feeling synchronous.
  const updateQuery = useCallback(
    (next: string) => {
      setQuery(next);
      const list = buildMatches(items, next);
      if (list.length === 0) {
        viewRef.current?.setSelectedLines(null);
        setActiveIndex(0);
        return;
      }
      setActiveIndex(surface(0, list));
    },
    [items, surface, viewRef],
  );

  const navigate = useCallback(
    (delta: number) => {
      if (matches.length === 0) return;
      setActiveIndex((prev) => surface(prev + delta, matches));
    },
    [matches, surface],
  );

  // Window-level Cmd/Ctrl-F intercept. First press focuses the diff search.
  // A second press within 500ms falls through to the browser's native
  // find-in-page, so users can still search outside the diff (e.g. their
  // own comment text) without having to remap muscle memory.
  useEffect(() => {
    let lastFindAt = 0;
    const onKey = (e: KeyboardEvent) => {
      if (!((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "f")) return;
      const now = Date.now();
      if (now - lastFindAt < 500) {
        lastFindAt = 0;
        return;
      }
      lastFindAt = now;
      e.preventDefault();
      inputRef.current?.focus();
      inputRef.current?.select();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);

  return (
    <search className="flex h-7 items-center gap-1 rounded-md border border-(--color-border) bg-(--color-background) px-1 focus-within:border-(--color-ring) focus-within:ring-2 focus-within:ring-(--color-ring)/30">
      <Search className="ml-1 size-3.5 text-(--color-muted-foreground)" aria-hidden />
      <input
        ref={inputRef}
        type="search"
        value={query}
        onChange={(e) => updateQuery(e.target.value)}
        onKeyDown={(e) => {
          if (e.key === "Enter") {
            e.preventDefault();
            navigate(e.shiftKey ? -1 : 1);
          } else if (e.key === "Escape") {
            e.preventDefault();
            updateQuery("");
            viewRef.current?.setSelectedLines(null);
            inputRef.current?.blur();
          }
        }}
        placeholder={t("repo.search_diff")}
        aria-label={t("repo.search_diff")}
        className="w-40 min-w-0 flex-1 bg-transparent px-1 py-0.5 text-sm outline-none placeholder:text-(--color-muted-foreground)"
      />
      <span className="px-1 text-xs tabular-nums text-(--color-muted-foreground)">
        {matches.length === 0 ? (query ? "0/0" : "") : `${activeIndex + 1}/${matches.length}`}
      </span>
      <button
        type="button"
        onClick={() => navigate(-1)}
        disabled={matches.length === 0}
        aria-label={t("repo.search_previous_match")}
        className="cursor-pointer rounded p-1 hover:bg-(--color-surface) disabled:cursor-not-allowed disabled:opacity-40"
      >
        <ChevronUp className="size-3.5" aria-hidden />
      </button>
      <button
        type="button"
        onClick={() => navigate(1)}
        disabled={matches.length === 0}
        aria-label={t("repo.search_next_match")}
        className="cursor-pointer rounded p-1 hover:bg-(--color-surface) disabled:cursor-not-allowed disabled:opacity-40"
      >
        <ChevronDown className="size-3.5" aria-hidden />
      </button>
    </search>
  );
}
