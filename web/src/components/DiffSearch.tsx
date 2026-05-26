import type { CodeViewHandle, CodeViewItem } from "@pierre/diffs/react";
import { ChevronDown, ChevronUp, Search, X } from "lucide-react";
import { type RefObject, useCallback, useEffect, useMemo, useRef, useState } from "react";

interface Match {
  itemId: string;
  side: "additions" | "deletions";
  lineNumber: number;
}

// Walk `hunkContent` so context lines (which exist on both `additionLines` and
// `deletionLines`) are counted once, not twice. Changes are counted on each
// side they actually appear.
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
            if (additionLines[c.additionLineIndex + k].toLowerCase().includes(needle)) {
              out.push({
                itemId: item.id,
                side: "additions",
                lineNumber: h.additionStart + (c.additionLineIndex + k - h.additionLineIndex),
              });
            }
          }
        } else {
          for (let k = 0; k < c.deletions; k++) {
            if (deletionLines[c.deletionLineIndex + k].toLowerCase().includes(needle)) {
              out.push({
                itemId: item.id,
                side: "deletions",
                lineNumber: h.deletionStart + (c.deletionLineIndex + k - h.deletionLineIndex),
              });
            }
          }
          for (let k = 0; k < c.additions; k++) {
            if (additionLines[c.additionLineIndex + k].toLowerCase().includes(needle)) {
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
  const [open, setOpen] = useState(false);
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

  // Window-level Cmd/Ctrl-F intercept. Opens the overlay and focuses the
  // input. Esc closes and clears selection.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const isFind = (e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "f";
      if (isFind) {
        e.preventDefault();
        setOpen(true);
        queueMicrotask(() => {
          inputRef.current?.focus();
          inputRef.current?.select();
        });
        return;
      }
      if (e.key === "Escape" && open) {
        e.preventDefault();
        setOpen(false);
        viewRef.current?.setSelectedLines(null);
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [open, viewRef]);

  if (!open) return null;

  return (
    <div
      role="search"
      className="absolute top-1 right-4 z-10 flex h-8 items-center gap-1 rounded-md border border-(--color-border) bg-(--color-background) px-1 shadow-md"
    >
      <Search className="ml-1 size-4 text-(--color-muted-foreground)" aria-hidden />
      <input
        ref={inputRef}
        type="search"
        value={query}
        onChange={(e) => updateQuery(e.target.value)}
        onKeyDown={(e) => {
          if (e.key === "Enter") {
            e.preventDefault();
            navigate(e.shiftKey ? -1 : 1);
          }
        }}
        placeholder="Search in diff"
        aria-label="Search in diff"
        className="w-48 bg-transparent px-1 py-0.5 text-sm outline-none placeholder:text-(--color-muted-foreground)"
      />
      <span className="px-1 text-xs tabular-nums text-(--color-muted-foreground)">
        {matches.length === 0 ? "0/0" : `${activeIndex + 1}/${matches.length}`}
      </span>
      <button
        type="button"
        onClick={() => navigate(-1)}
        disabled={matches.length === 0}
        aria-label="Previous match"
        className="cursor-pointer rounded p-1 hover:bg-(--color-surface) disabled:cursor-not-allowed disabled:opacity-40"
      >
        <ChevronUp className="size-4" aria-hidden />
      </button>
      <button
        type="button"
        onClick={() => navigate(1)}
        disabled={matches.length === 0}
        aria-label="Next match"
        className="cursor-pointer rounded p-1 hover:bg-(--color-surface) disabled:cursor-not-allowed disabled:opacity-40"
      >
        <ChevronDown className="size-4" aria-hidden />
      </button>
      <button
        type="button"
        onClick={() => {
          setOpen(false);
          viewRef.current?.setSelectedLines(null);
        }}
        aria-label="Close search"
        className="cursor-pointer rounded p-1 hover:bg-(--color-surface)"
      >
        <X className="size-4" aria-hidden />
      </button>
    </div>
  );
}
