import { useQuery } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { Search } from "lucide-react";
import { useEffect, useId, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";

import { FileIcon } from "@/components/FileIcon";
import { repoPathsQuery } from "@/lib/queries/repo";
import { cn } from "@/lib/utils";

const MAX_RESULTS = 50;

// rankOf orders a match like GitHub's file finder: a file name that starts
// with the query, then a file name that contains it, then a path that
// contains it, then a path with its letters in order, which only counts when
// nothing contains the query. It is -1 for no match.
function rankOf(path: string, query: string) {
  const lower = path.toLowerCase();
  const name = lower.slice(lower.lastIndexOf("/") + 1);
  if (name.startsWith(query)) return 0;
  if (name.includes(query)) return 1;
  if (lower.includes(query)) return 2;
  let at = 0;
  for (const ch of query) {
    at = lower.indexOf(ch, at) + 1;
    if (at === 0) return -1;
  }
  return 3;
}

function Highlighted({ path, query }: { path: string; query: string }) {
  const at = path.toLowerCase().lastIndexOf(query);
  if (at < 0) return <>{path}</>;
  return (
    <>
      {path.slice(0, at)}
      <mark className="bg-transparent font-semibold text-(--color-foreground)">
        {path.slice(at, at + query.length)}
      </mark>
      {path.slice(at + query.length)}
    </>
  );
}

// GoToFile finds a file of the commit by its path as the viewer types, and
// opens it at the ref. "t" focuses it, like on GitHub.
export function GoToFile({
  owner,
  name,
  gitRef: refName,
  commitID,
  className,
}: {
  owner: string;
  name: string;
  gitRef: string;
  commitID: string;
  className?: string;
}) {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const listID = useId();
  const input = useRef<HTMLInputElement>(null);
  const [query, setQuery] = useState("");
  const [open, setOpen] = useState(false);
  const [active, setActive] = useState(0);
  const [wanted, setWanted] = useState(false);
  const { data, isPending } = useQuery({ ...repoPathsQuery(owner, name, commitID), enabled: wanted });

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key !== "t" || e.metaKey || e.ctrlKey || e.altKey) return;
      const target = e.target as HTMLElement | null;
      if (target?.closest("input, textarea, select, [contenteditable='true']")) return;
      e.preventDefault();
      input.current?.focus();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);

  const q = query.trim().toLowerCase();
  const results = useMemo(() => {
    if (!q || !data) return [];
    const ranked: { path: string; rank: number }[] = [];
    for (const path of data.paths) {
      const rank = rankOf(path, q);
      if (rank >= 0) ranked.push({ path, rank });
    }
    ranked.sort((a, b) => a.rank - b.rank || a.path.length - b.path.length || a.path.localeCompare(b.path));
    const loose = ranked.length && ranked[0].rank < 3 ? ranked.filter((r) => r.rank < 3) : ranked;
    return loose.slice(0, MAX_RESULTS).map((r) => r.path);
  }, [data, q]);

  const go = (path: string) => {
    setOpen(false);
    setQuery("");
    input.current?.blur();
    void navigate({ to: "/$owner/$repo/src/$", params: { owner, repo: name, _splat: `${refName}/${path}` } });
  };

  const showList = open && q !== "";
  return (
    <div className={cn("relative min-w-0", className)}>
      <div className="flex h-8 items-center gap-2 rounded-md border border-(--color-border) bg-(--color-background) px-2 focus-within:border-(--color-ring) focus-within:ring-1 focus-within:ring-(--color-ring)">
        <Search className="size-4 shrink-0 text-(--color-muted-foreground)" aria-hidden />
        <input
          ref={input}
          type="text"
          role="combobox"
          aria-label={t("repo.go_to_file")}
          aria-expanded={showList}
          aria-controls={listID}
          aria-autocomplete="list"
          aria-activedescendant={showList && results.length ? `${listID}-${active}` : undefined}
          placeholder={t("repo.go_to_file")}
          value={query}
          onFocus={() => {
            setWanted(true);
            setOpen(true);
          }}
          onBlur={() => setOpen(false)}
          onChange={(e) => {
            setQuery(e.target.value);
            setActive(0);
            setOpen(true);
          }}
          onKeyDown={(e) => {
            if (e.key === "ArrowDown") {
              e.preventDefault();
              setActive((i) => Math.min(i + 1, results.length - 1));
            } else if (e.key === "ArrowUp") {
              e.preventDefault();
              setActive((i) => Math.max(i - 1, 0));
            } else if (e.key === "Enter" && results[active]) {
              e.preventDefault();
              go(results[active]);
            } else if (e.key === "Escape") {
              setQuery("");
              input.current?.blur();
            }
          }}
          className="min-w-0 flex-1 bg-transparent text-sm outline-none placeholder:text-(--color-muted-foreground)"
        />
        <kbd className="hidden rounded border border-(--color-border) px-1.5 font-mono text-xs text-(--color-muted-foreground) sm:inline">
          t
        </kbd>
      </div>
      {showList ? (
        <div className="absolute top-full right-0 left-0 z-20 mt-1 overflow-hidden rounded-md border border-(--color-border) bg-(--color-background) shadow-lg sm:left-auto sm:w-[28rem]">
          <ul id={listID} role="listbox" aria-label={t("repo.go_to_file")} className="max-h-80 overflow-y-auto p-1">
            {results.map((path, i) => (
              <li
                key={path}
                id={`${listID}-${i}`}
                role="option"
                aria-selected={i === active}
                onMouseDown={(e) => e.preventDefault()}
                onClick={() => go(path)}
                onMouseMove={() => setActive(i)}
                className={cn(
                  "flex cursor-pointer items-center gap-2 rounded-sm px-2 py-1.5 text-sm text-(--color-muted-foreground)",
                  i === active && "bg-(--color-surface)",
                )}
              >
                <FileIcon name={path.slice(path.lastIndexOf("/") + 1)} />
                <span className="min-w-0 truncate">
                  <Highlighted path={path} query={q} />
                </span>
              </li>
            ))}
          </ul>
          {!results.length ? (
            <p className="px-3 py-2 text-sm text-(--color-muted-foreground)">
              {isPending ? t("repo.go_to_file_loading") : t("repo.go_to_file_none")}
            </p>
          ) : null}
        </div>
      ) : null}
    </div>
  );
}
