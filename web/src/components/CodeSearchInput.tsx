import { useQuery } from "@tanstack/react-query";
import { BookMarked, Building2, FileCode, Search, User, X } from "lucide-react";
import { type KeyboardEvent, type ReactNode, useId, useLayoutEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";

import { fetchJSON } from "@/lib/queries/request";
import { subUrl } from "@/lib/url";
import { useDebounced } from "@/lib/use-debounced";
import { cn } from "@/lib/utils";

type QualifierKey = "org" | "user" | "repo" | "path";

const GLOBAL_KEYS: readonly QualifierKey[] = ["org", "user", "repo", "path"];
const REPO_KEYS: readonly QualifierKey[] = ["path"];

// splitQuery splits the qualifiers of the keys out of a query, as chips, and
// leaves the rest of it as text.
function splitQuery(query: string, keys: readonly QualifierKey[]) {
  const chips: string[] = [];
  const words: string[] = [];
  for (const word of query.split(/\s+/).filter(Boolean)) {
    if (isQualifier(word, keys)) chips.push(word);
    else words.push(word);
  }
  return { chips, text: words.join(" ") };
}

// isQualifier reports whether the word is a qualifier of one of the keys with
// a value, like the server reads it.
function isQualifier(word: string, keys: readonly QualifierKey[]) {
  const at = word.indexOf(":");
  return at > 0 && at < word.length - 1 && (keys as readonly string[]).includes(word.slice(0, at).toLowerCase());
}

interface Suggestion {
  // value replaces the word being typed.
  value: string;
  // chip is set when the value is a whole qualifier, which becomes a chip.
  chip: boolean;
  label: ReactNode;
  hint?: string;
  icon: ReactNode;
}

interface ExploreList<T> {
  items: T[];
}

// useValueSuggestions suggests the values of a qualifier from the owners and
// repositories the viewer can see.
function useValueSuggestions(key: string | null, value: string): Suggestion[] {
  const lookup = useDebounced({ key, value });
  const owner = lookup.key === "repo" && lookup.value.includes("/") ? lookup.value.split("/")[0] : "";
  const keyword = lookup.key === "repo" ? lookup.value.slice(owner ? owner.length + 1 : 0) : lookup.value;
  const kind =
    lookup.key === "org" ? "organizations" : lookup.key === "user" ? "users" : lookup.key === "repo" ? "repos" : null;
  const { data } = useQuery({
    queryKey: ["code-search-suggest", kind, keyword],
    queryFn: () =>
      fetchJSON<ExploreList<{ name: string; owner?: string; avatarURL?: string }>>(
        subUrl(`/api/web/explore/${kind}${keyword ? `?${new URLSearchParams({ q: keyword }).toString()}` : ""}`),
      ),
    enabled: kind !== null,
    staleTime: 30_000,
    retry: false,
  });
  if (!kind || !data || lookup.key !== key) return [];

  return data.items
    .map((item) => (kind === "repos" ? `${item.owner}/${item.name}` : item.name))
    .filter((name) => name.toLowerCase().startsWith(owner ? `${owner.toLowerCase()}/` : ""))
    .filter((name) => name.toLowerCase().includes(value.toLowerCase()) || kind !== "repos")
    .slice(0, 8)
    .map((name) => ({
      value: `${key}:${name}`,
      chip: true,
      label: name,
      icon:
        kind === "repos" ? (
          <BookMarked className="size-4" aria-hidden />
        ) : kind === "organizations" ? (
          <Building2 className="size-4" aria-hidden />
        ) : (
          <User className="size-4" aria-hidden />
        ),
    }));
}

// CodeSearchInput is a search box that shows the qualifiers of a query as
// chips, and suggests qualifiers and their values while typing. It goes in a
// form, which submits the whole query as "q".
export function CodeSearchInput({
  query,
  global,
  autoFocus,
}: {
  query: string;
  // global is set for a search of all repositories, which takes org:, user:
  // and repo: besides path:.
  global?: boolean;
  autoFocus?: boolean;
}) {
  const { t } = useTranslation();
  const listID = useId();
  const keys = global ? GLOBAL_KEYS : REPO_KEYS;
  const inputRef = useRef<HTMLInputElement>(null);
  const [initial] = useState(() => splitQuery(query, keys));
  const [chips, setChips] = useState(initial.chips);
  const [text, setText] = useState(initial.text);
  const [caret, setCaret] = useState(initial.text.length);
  const [open, setOpen] = useState(false);
  const [active, setActive] = useState(-1);

  // The word being typed is the one that ends at the caret.
  const wordStart = text.slice(0, caret).search(/\S*$/);
  const word = text.slice(wordStart, caret);
  const colon = word.indexOf(":");
  const wordKey = colon > 0 ? word.slice(0, colon).toLowerCase() : null;
  const keyHints: Record<QualifierKey, string> = {
    org: t("repo.code_search.qualifier_owner"),
    user: t("repo.code_search.qualifier_owner"),
    repo: t("repo.code_search.qualifier_repo"),
    path: t("repo.code_search.qualifier_path"),
  };
  const keySuggestions: Suggestion[] =
    colon < 0
      ? keys
          .filter((key) => key.startsWith(word.toLowerCase()))
          .map((key) => ({
            value: `${key}:`,
            chip: false,
            label: `${key}:`,
            hint: keyHints[key],
            icon:
              key === "path" ? <FileCode className="size-4" aria-hidden /> : <Search className="size-4" aria-hidden />,
          }))
      : [];
  const valueSuggestions = useValueSuggestions(
    wordKey && (keys as readonly string[]).includes(wordKey) ? wordKey : null,
    colon > 0 ? word.slice(colon + 1) : "",
  );
  const suggestions = colon < 0 ? keySuggestions : valueSuggestions;
  const expanded = open && suggestions.length > 0;
  const activeIndex = active < suggestions.length ? active : -1;

  const queryOf = (nextChips: string[], nextText: string) => [...nextChips, nextText.trim()].filter(Boolean).join(" ");

  // pendingCaret is where to put the caret once the text it belongs to is
  // rendered, after a suggestion or a chip replaced part of it.
  const pendingCaret = useRef<number | null>(null);
  useLayoutEffect(() => {
    if (pendingCaret.current === null) return;
    inputRef.current?.setSelectionRange(pendingCaret.current, pendingCaret.current);
    pendingCaret.current = null;
  }, [text]);

  const setTextAt = (next: string, at: number) => {
    pendingCaret.current = at;
    setText(next);
    setCaret(at);
    setActive(-1);
  };

  const accept = (suggestion: Suggestion) => {
    const before = text.slice(0, wordStart);
    const after = text.slice(caret);
    if (suggestion.chip) {
      setChips([...chips.filter((chip) => chip.toLowerCase() !== suggestion.value.toLowerCase()), suggestion.value]);
      const rest = `${before.trimEnd()}${before.trim() && after.trim() ? " " : ""}${after.trimStart()}`;
      setTextAt(rest, before.trimEnd().length + (before.trim() && after.trim() ? 1 : 0));
    } else {
      setTextAt(before + suggestion.value + after, before.length + suggestion.value.length);
    }
    setOpen(true);
  };

  // chipWords turns the whole qualifiers typed before the caret into chips.
  const chipWords = (next: string) => {
    const words = next.split(" ");
    const typed = words.slice(0, -1);
    const qualifiers = typed.filter((w) => isQualifier(w, keys));
    if (!qualifiers.length) return false;
    setChips([...chips, ...qualifiers]);
    const rest = [...typed.filter((w) => !isQualifier(w, keys)), words.at(-1) ?? ""].join(" ").trimStart();
    setTextAt(rest, rest.length);
    return true;
  };

  const removeChip = (index: number) => {
    setChips(chips.filter((_, i) => i !== index));
    inputRef.current?.focus();
  };

  const onKeyDown = (e: KeyboardEvent<HTMLInputElement>) => {
    if (e.key === "ArrowDown" && suggestions.length) {
      e.preventDefault();
      setOpen(true);
      setActive((activeIndex + 1) % suggestions.length);
    } else if (e.key === "ArrowUp" && suggestions.length) {
      e.preventDefault();
      setOpen(true);
      setActive(activeIndex <= 0 ? suggestions.length - 1 : activeIndex - 1);
    } else if (e.key === "Escape" && expanded) {
      e.preventDefault();
      setOpen(false);
    } else if (e.key === "Tab" && expanded && !e.shiftKey && word) {
      e.preventDefault();
      accept(suggestions[Math.max(activeIndex, 0)]);
    } else if (e.key === "Enter" && expanded && activeIndex >= 0) {
      e.preventDefault();
      accept(suggestions[activeIndex]);
    } else if (e.key === "Enter") {
      setOpen(false);
    } else if (e.key === "Backspace" && caret === 0 && e.currentTarget.selectionEnd === 0 && chips.length) {
      e.preventDefault();
      setChips(chips.slice(0, -1));
    }
  };

  return (
    <div className="relative min-w-0 flex-1">
      <div
        className={cn(
          "flex min-h-8 w-full cursor-text flex-wrap items-center gap-1 rounded-md border border-(--color-input) bg-(--color-background) py-0.5 pr-1 pl-2.5 text-sm",
          "focus-within:border-(--color-ring) focus-within:ring-1 focus-within:ring-(--color-ring)",
        )}
        onClick={() => inputRef.current?.focus()}
      >
        <Search className="size-4 shrink-0 text-(--color-muted-foreground)" aria-hidden />
        {chips.map((chip, i) => {
          const at = chip.indexOf(":");
          return (
            <span
              key={`${chip}-${i}`}
              className="inline-flex max-w-full items-center gap-0.5 rounded bg-(--color-secondary) py-0.5 pl-1.5 font-mono text-xs text-(--color-secondary-foreground)"
            >
              <span className="truncate">
                <span className="text-(--color-muted-foreground)">{chip.slice(0, at + 1)}</span>
                {chip.slice(at + 1)}
              </span>
              <button
                type="button"
                aria-label={t("repo.code_search.remove_filter", { filter: chip })}
                onClick={(e) => {
                  e.stopPropagation();
                  removeChip(i);
                }}
                className="grid size-6 shrink-0 cursor-pointer place-items-center rounded text-(--color-muted-foreground) hover:bg-(--color-surface) hover:text-(--color-foreground)"
              >
                <X className="size-3" aria-hidden />
              </button>
            </span>
          );
        })}
        <input type="hidden" name="q" value={queryOf(chips, text)} />
        <input
          ref={inputRef}
          type="search"
          role="combobox"
          aria-label={t("repo.code_search")}
          aria-expanded={expanded}
          aria-controls={listID}
          aria-autocomplete="list"
          aria-activedescendant={expanded && activeIndex >= 0 ? `${listID}-${activeIndex}` : undefined}
          placeholder={chips.length ? undefined : t("repo.code_search")}
          autoFocus={autoFocus}
          autoComplete="off"
          spellCheck={false}
          value={text}
          onChange={(e) => {
            const next = e.target.value;
            if (next.endsWith(" ") && chipWords(next)) return;
            setText(next);
            setCaret(e.target.selectionStart ?? next.length);
            setActive(-1);
            setOpen(true);
          }}
          onSelect={(e) => setCaret(e.currentTarget.selectionStart ?? 0)}
          onFocus={() => setOpen(true)}
          onBlur={() => setOpen(false)}
          onKeyDown={onKeyDown}
          className="h-7 min-w-24 flex-1 bg-transparent font-mono outline-none placeholder:font-sans placeholder:text-(--color-muted-foreground) [&::-webkit-search-cancel-button]:hidden"
        />
      </div>
      <ul
        id={listID}
        role="listbox"
        aria-label={t("repo.code_search.suggestions")}
        hidden={!expanded}
        className="absolute top-full left-0 z-20 mt-1 w-full overflow-hidden rounded-md border border-(--color-border) bg-(--color-popover) p-1 text-sm text-(--color-popover-foreground) shadow-md"
      >
        {suggestions.map((suggestion, i) => (
          <li
            key={suggestion.value}
            id={`${listID}-${i}`}
            role="option"
            aria-selected={i === activeIndex}
            // Keep the focus in the input when a suggestion is clicked.
            onMouseDown={(e) => e.preventDefault()}
            onClick={() => accept(suggestion)}
            onMouseMove={() => setActive(i)}
            className={cn(
              "flex cursor-pointer items-center gap-2 rounded-sm px-2 py-1.5",
              i === activeIndex && "bg-(--color-surface)",
            )}
          >
            <span className="shrink-0 text-(--color-muted-foreground)">{suggestion.icon}</span>
            <span className="min-w-0 truncate font-mono">{suggestion.label}</span>
            {suggestion.hint ? (
              <span className="ml-auto min-w-0 truncate pl-2 text-xs text-(--color-muted-foreground)">
                {suggestion.hint}
              </span>
            ) : null}
          </li>
        ))}
      </ul>
    </div>
  );
}
