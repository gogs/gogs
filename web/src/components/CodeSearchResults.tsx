import { type DiffsHighlighter, type ThemedToken, getFiletypeFromFileName, getSharedHighlighter } from "@pierre/diffs";
import { useQuery } from "@tanstack/react-query";
import { FileCode } from "lucide-react";
import { type ReactNode, useMemo } from "react";
import { useTranslation } from "react-i18next";

import { CodeSearchInput } from "@/components/CodeSearchInput";
import { Button } from "@/components/ui/button";
import type { RepoSearchFile, RepoSearchLine } from "@/lib/queries/repo";
import { resolveTheme, useTheme } from "@/lib/theme-context";
import { cn } from "@/lib/utils";

const THEMES = { light: "pierre-light", dark: "pierre-dark" } as const;

// useHighlighter loads the highlighter of the file view with the grammars of
// the languages. It is undefined until they load, or when they fail to.
function useHighlighter(langs: string[]) {
  const key = [...new Set(langs)].sort();
  return useQuery({
    queryKey: ["highlighter", ...key],
    queryFn: () => getSharedHighlighter({ themes: [THEMES.light, THEMES.dark], langs: key }),
    staleTime: Infinity,
    retry: false,
  }).data;
}

function escapeRegExp(text: string) {
  return text.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

// matchRanges returns where the keyword is in the text, ignoring case like the
// search.
function matchRanges(text: string, keyword: string) {
  const ranges: [number, number][] = [];
  if (!keyword) return ranges;
  for (const match of text.matchAll(new RegExp(escapeRegExp(keyword), "giu"))) {
    ranges.push([match.index, match.index + match[0].length]);
  }
  return ranges;
}

interface Piece {
  start: number;
  text: string;
  color?: string;
  italic?: boolean;
}

// renderLine colors the pieces of a line and marks the parts that match the
// keyword, splitting pieces where a match starts or ends.
function renderLine(text: string, pieces: Piece[], keyword: string) {
  const ranges = matchRanges(text, keyword);
  const out: ReactNode[] = [];
  for (const piece of pieces) {
    const cuts = new Set([0, piece.text.length]);
    for (const [start, end] of ranges) {
      for (const at of [start - piece.start, end - piece.start]) {
        if (at > 0 && at < piece.text.length) cuts.add(at);
      }
    }
    const points = [...cuts].sort((a, b) => a - b);
    for (let i = 0; i < points.length - 1; i++) {
      const from = piece.start + points[i];
      const part = piece.text.slice(points[i], points[i + 1]);
      const style = { color: piece.color, fontStyle: piece.italic ? "italic" : undefined };
      const marked = ranges.some(([start, end]) => from >= start && from < end);
      out.push(
        marked ? (
          <mark
            key={from}
            style={style}
            className="rounded-sm bg-(--color-warning)/30 font-semibold outline outline-(--color-warning)/60"
          >
            {part}
          </mark>
        ) : (
          <span key={from} style={style}>
            {part}
          </span>
        ),
      );
    }
  }
  return out;
}

// snippetsOf splits the lines of a file into runs of consecutive lines.
function snippetsOf(lines: RepoSearchLine[]) {
  const snippets: RepoSearchLine[][] = [];
  for (const line of lines) {
    const last = snippets.at(-1);
    if (last && last[last.length - 1].line + 1 === line.line) last.push(line);
    else snippets.push([line]);
  }
  return snippets;
}

// CodeSearchResults lists the files that matched a search, each with its
// matching lines and the lines around them, highlighted like the file view.
export function CodeSearchResults({
  files,
  keyword,
  urlOf,
  headerOf,
}: {
  files: RepoSearchFile[];
  keyword: string;
  // urlOf links a file, or a line of it.
  urlOf: (file: RepoSearchFile, line?: number) => string;
  // headerOf shows what goes before the path in the header of a file.
  headerOf?: (file: RepoSearchFile) => ReactNode;
}) {
  const langs = useMemo(() => files.map((file) => getFiletypeFromFileName(file.path)), [files]);
  const highlighter = useHighlighter(langs);
  const { theme } = useTheme();
  const themeName = THEMES[resolveTheme(theme)];

  return (
    <div className="flex flex-col gap-4">
      {files.map((file, i) => (
        <section
          key={`${file.repo}/${file.path}`}
          aria-label={file.repo ? `${file.repo}/${file.path}` : file.path}
          className="overflow-hidden rounded-md border border-(--color-border)"
        >
          <h2 className="flex items-center gap-2 border-b border-(--color-border) bg-(--color-surface) px-4 py-2 text-sm">
            <FileCode className="size-4 shrink-0 text-(--color-muted-foreground)" aria-hidden />
            <span className="min-w-0 wrap-anywhere">
              {headerOf?.(file)}
              <a href={urlOf(file)} className="font-mono font-semibold text-(--color-primary) hover:underline">
                {file.path}
              </a>
            </span>
          </h2>
          <div className="divide-y divide-dashed divide-(--color-border) overflow-x-auto font-mono text-xs/5">
            {snippetsOf(file.lines).map((snippet) => (
              <Snippet
                key={snippet[0].line}
                lines={snippet}
                lang={langs[i]}
                keyword={keyword}
                highlighter={highlighter}
                themeName={themeName}
                lineURL={(line) => urlOf(file, line)}
              />
            ))}
          </div>
        </section>
      ))}
    </div>
  );
}

function Snippet({
  lines,
  lang,
  keyword,
  highlighter,
  themeName,
  lineURL,
}: {
  lines: RepoSearchLine[];
  lang: string;
  keyword: string;
  highlighter: DiffsHighlighter | undefined;
  themeName: string;
  lineURL: (line: number) => string;
}) {
  const { t } = useTranslation();
  // The lines are colored together, so that e.g. a comment that spans them
  // is colored as one.
  const tokens = useMemo<ThemedToken[][] | null>(() => {
    if (!highlighter) return null;
    try {
      return highlighter.codeToTokens(lines.map((line) => line.text).join("\n"), { lang, theme: themeName }).tokens;
    } catch {
      return null;
    }
  }, [highlighter, lang, themeName, lines]);
  // Token offsets count from the start of the snippet, the lines joined by
  // line feeds.
  const lineStarts = useMemo(
    () => lines.map((_, i) => lines.slice(0, i).reduce((n, line) => n + line.text.length + 1, 0)),
    [lines],
  );
  return (
    <ol className="min-w-max py-1" data-lang={lang}>
      {lines.map((line, i) => {
        const start = lineStarts[i];
        const pieces: Piece[] = tokens?.[i]
          ? tokens[i].map((token) => ({
              start: token.offset - start,
              text: token.content,
              color: token.color,
              italic: ((token.fontStyle ?? 0) & 1) === 1,
            }))
          : [{ start: 0, text: line.text }];
        return (
          <li key={line.line} className={cn("flex", line.isMatch && "bg-(--color-warning)/5")}>
            <a
              href={lineURL(line.line)}
              aria-label={t("repo.code_search.line", { line: line.line })}
              className="sticky left-0 w-14 shrink-0 bg-(--color-background) pr-3 text-right text-(--color-muted-foreground) tabular-nums hover:text-(--color-primary) hover:underline"
            >
              {line.line}
            </a>
            <span className="pr-4 whitespace-pre">
              {line.isMatch ? renderLine(line.text, pieces, keyword) : renderLine(line.text, pieces, "")}
            </span>
          </li>
        );
      })}
    </ol>
  );
}

// CodeSearchForm searches for the query, with qualifiers such as path: shown
// as chips. A global search also takes org:, user: and repo:.
export function CodeSearchForm({
  query,
  onSearch,
  autoFocus,
  global,
}: {
  query: string;
  onSearch: (query: string) => void;
  autoFocus?: boolean;
  global?: boolean;
}) {
  const { t } = useTranslation();
  return (
    <form
      role="search"
      onSubmit={(e) => {
        e.preventDefault();
        onSearch(((new FormData(e.currentTarget).get("q") as string | null) ?? "").trim());
      }}
      className="flex w-full items-start gap-2 sm:w-auto sm:flex-1"
    >
      <CodeSearchInput query={query} global={global} autoFocus={autoFocus} />
      <Button type="submit" variant="outline" size="sm" className="h-8">
        {t("explore.search")}
      </Button>
    </form>
  );
}

// CodeSearchHelp tells what to search for, and the qualifiers that narrow
// down the search.
export function CodeSearchHelp({ hint, global }: { hint: string; global?: boolean }) {
  const { t } = useTranslation();
  const qualifiers = [
    ...(global
      ? [
          { syntax: "org:gogs user:alice", label: t("repo.code_search.qualifier_owner") },
          { syntax: "repo:gogs/gogs", label: t("repo.code_search.qualifier_repo") },
        ]
      : []),
    { syntax: "path:internal/ path:*.go", label: t("repo.code_search.qualifier_path") },
  ];
  return (
    <div className="rounded-md border border-(--color-border) px-4 py-6 text-sm">
      <p className="text-(--color-muted-foreground)">{hint}</p>
      <h2 className="mt-4 mb-2 font-semibold">{t("repo.code_search.qualifiers")}</h2>
      <dl className="grid gap-x-4 gap-y-2 sm:grid-cols-[max-content_1fr]">
        {qualifiers.map(({ syntax, label }) => (
          <div key={syntax} className="contents">
            <dt className="flex flex-wrap gap-1">
              {syntax.split(" ").map((example) => (
                <code key={example} className="rounded bg-(--color-surface) px-1 font-mono text-(--color-foreground)">
                  {example}
                </code>
              ))}
            </dt>
            <dd className="text-(--color-muted-foreground)">{label}</dd>
          </div>
        ))}
      </dl>
    </div>
  );
}
