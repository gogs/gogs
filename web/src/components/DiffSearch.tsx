import type { CodeViewHandle, CodeViewItem } from "@pierre/diffs/react";
import { ChevronDown, ChevronUp, Search, X } from "lucide-react";
import { type RefObject, useCallback, useEffect, useMemo, useRef, useState } from "react";

interface Match {
  itemId: string;
  side: "additions" | "deletions";
  lineNumber: number;
}

// `additionLines` and `deletionLines` on a partial (patch-only) FileDiffMetadata
// are flat arrays indexed in patch order, not new-/old-file line numbers. Each
// hunk declares `additionStart`/`deletionStart` (the line number in the
// respective file where the hunk begins) and `additionLineIndex`/
// `deletionLineIndex` (where in the flat array that hunk's lines start). Walk
// hunks to find which one contains a given flat index, then offset from the
// hunk's start to get the real file line number, which is what
// `CodeView.scrollTo`/`setSelectedLines` expect.
function flatIndexToLineNumber(
  hunks: readonly {
    additionStart: number;
    deletionStart: number;
    additionLineIndex: number;
    deletionLineIndex: number;
    additionLines: number;
    deletionLines: number;
  }[],
  flatIndex: number,
  side: "additions" | "deletions",
): number | null {
  for (const h of hunks) {
    if (side === "additions") {
      const startIdx = h.additionLineIndex;
      const endIdx = startIdx + h.additionLines;
      if (flatIndex >= startIdx && flatIndex < endIdx) {
        return h.additionStart + (flatIndex - startIdx);
      }
    } else {
      const startIdx = h.deletionLineIndex;
      const endIdx = startIdx + h.deletionLines;
      if (flatIndex >= startIdx && flatIndex < endIdx) {
        return h.deletionStart + (flatIndex - startIdx);
      }
    }
  }
  return null;
}

function buildMatches(items: readonly CodeViewItem[], query: string): Match[] {
  if (!query) return [];
  const needle = query.toLowerCase();
  const out: Match[] = [];
  for (const item of items) {
    if (item.type !== "diff") continue;
    const { additionLines, deletionLines, hunks } = item.fileDiff;
    for (let i = 0; i < additionLines.length; i++) {
      if (additionLines[i].toLowerCase().includes(needle)) {
        const lineNumber = flatIndexToLineNumber(hunks, i, "additions");
        if (lineNumber != null) {
          out.push({ itemId: item.id, side: "additions", lineNumber });
        }
      }
    }
    for (let i = 0; i < deletionLines.length; i++) {
      if (deletionLines[i].toLowerCase().includes(needle)) {
        const lineNumber = flatIndexToLineNumber(hunks, i, "deletions");
        if (lineNumber != null) {
          out.push({ itemId: item.id, side: "deletions", lineNumber });
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
      className="absolute top-3 right-4 z-10 flex h-8 items-center gap-1 rounded-md border border-(--color-border) bg-(--color-background) px-1 shadow-md"
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
        className="rounded p-1 hover:bg-(--color-surface) disabled:opacity-40"
      >
        <ChevronUp className="size-4" aria-hidden />
      </button>
      <button
        type="button"
        onClick={() => navigate(1)}
        disabled={matches.length === 0}
        aria-label="Next match"
        className="rounded p-1 hover:bg-(--color-surface) disabled:opacity-40"
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
        className="rounded p-1 hover:bg-(--color-surface)"
      >
        <X className="size-4" aria-hidden />
      </button>
    </div>
  );
}
