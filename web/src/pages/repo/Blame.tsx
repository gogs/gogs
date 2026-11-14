import { type ThemedToken, getFiletypeFromFileName, getSharedHighlighter } from "@pierre/diffs";
import { useSuspenseQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { File, History } from "lucide-react";
import { type CSSProperties, useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";

import { RefPicker } from "@/components/RefPicker";
import { RepoHeader } from "@/components/RepoHeader";
import { Button } from "@/components/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { formatFileSize } from "@/lib/file-size";
import { usePageTitle } from "@/lib/page-title";
import { type RepoBlameCommit, type RepoBlameData, refPath, repoHeaderQuery } from "@/lib/queries/repo";
import { subUrl } from "@/lib/url";
import { cn } from "@/lib/utils";
import { FileModeNav, RelativeTime } from "@/pages/repo/Code";

const LINK_CLASS = "text-(--color-primary) hover:underline";
// AGE_STRENGTHS are the opacities of the bar of a hunk, from the oldest
// commit of the file to the newest.
const AGE_STRENGTHS = [0.18, 0.32, 0.5, 0.75, 1];

// splatOf is the route path of a ref, followed by a path when given.
function splatOf(ref: string, path?: string) {
  return path ? `${ref}/${path}` : ref;
}

// ageStrengthsOf returns the strength of the bar of each commit, by where its
// time falls between the oldest and the newest commit of the file.
function ageStrengthsOf(commits: Record<string, RepoBlameCommit>) {
  const times = Object.values(commits).map((c) => Date.parse(c.author.when));
  const oldest = Math.min(...times);
  const span = Math.max(...times) - oldest;
  const strengths: Record<string, number> = {};
  for (const [sha, commit] of Object.entries(commits)) {
    const age = span ? (Date.parse(commit.author.when) - oldest) / span : 1;
    strengths[sha] = AGE_STRENGTHS[Math.min(AGE_STRENGTHS.length - 1, Math.floor(age * AGE_STRENGTHS.length))];
  }
  return strengths;
}

// useHighlightedLines highlights the content like the files of the code
// browser, returning the tokens of each line once the highlighter loads.
function useHighlightedLines(content: string, fileName: string) {
  const [lines, setLines] = useState<ThemedToken[][] | null>(null);
  useEffect(() => {
    let cancelled = false;
    const lang = getFiletypeFromFileName(fileName);
    getSharedHighlighter({ themes: ["pierre-light", "pierre-dark"], langs: [lang] })
      .then((highlighter) => {
        if (cancelled) return;
        const result = highlighter.codeToTokens(content, {
          lang,
          themes: { light: "pierre-light", dark: "pierre-dark" },
          defaultColor: false,
          cssVariablePrefix: "--blame-",
          tokenizeMaxLineLength: 1000,
        });
        setLines(result.tokens);
      })
      // The lines stay plain text when the language fails to load.
      .catch(() => {});
    return () => {
      cancelled = true;
    };
  }, [content, fileName]);
  return lines;
}

// RepoBlame shows which commit last changed each line of a file, grouping the
// runs of lines under their commits.
export function RepoBlame({ owner, name, data }: { owner: string; name: string; data: RepoBlameData }) {
  const { t } = useTranslation();
  const { data: repo } = useSuspenseQuery(repoHeaderQuery(owner, name));
  const fullName = `${repo.owner}/${repo.name}`;
  usePageTitle(`${t("repo.blame")} ${data.path} - ${fullName}`);

  const params = { owner: repo.owner, repo: repo.name };
  const pathParts = data.path.split("/");
  const lines = useMemo(() => data.content.replace(/\n$/, "").split("\n"), [data.content]);
  const lineCount = data.kind === "text" ? lines.length : 0;
  const rawURL = subUrl(`/${repo.owner}/${repo.name}/raw/${refPath(splatOf(data.ref, data.path))}`);

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <RepoHeader repo={repo} activeTab="code" />
      <main className="mx-auto w-full max-w-7xl flex-1 px-4 py-6 sm:px-6">
        <h1 className="sr-only">
          {t("repo.blame")} {data.path}
        </h1>
        <div className="mb-4 flex flex-wrap items-center gap-2">
          <RefPicker
            current={data.ref}
            type={data.refType}
            branches={data.branches}
            tags={data.tags}
            renderLink={(ref, className, children) => (
              <Link
                to="/$owner/$repo/blame/$"
                params={{ ...params, _splat: splatOf(ref, data.path) }}
                className={className}
              >
                {children}
              </Link>
            )}
          />
          <nav aria-label={t("repo.files")} className="flex min-w-0 flex-wrap items-center gap-x-1 text-sm">
            <Link
              to="/$owner/$repo/src/$"
              params={{ ...params, _splat: data.ref }}
              className={cn(LINK_CLASS, "font-semibold wrap-anywhere")}
            >
              {repo.name}
            </Link>
            {pathParts.map((part, i) => (
              <span key={i} className="contents">
                <span className="text-(--color-muted-foreground)" aria-hidden>
                  /
                </span>
                {i === pathParts.length - 1 ? (
                  <span className="font-semibold wrap-anywhere" aria-current="page">
                    {part}
                  </span>
                ) : (
                  <Link
                    to="/$owner/$repo/src/$"
                    params={{ ...params, _splat: splatOf(data.ref, pathParts.slice(0, i + 1).join("/")) }}
                    className={cn(LINK_CLASS, "wrap-anywhere")}
                  >
                    {part}
                  </Link>
                )}
              </span>
            ))}
          </nav>
        </div>

        <section
          aria-labelledby="repo-file-heading"
          className="overflow-hidden rounded-md border border-(--color-border)"
        >
          <div className="flex flex-wrap items-center gap-x-3 gap-y-2 border-b border-(--color-border) bg-(--color-surface) px-4 py-2">
            <h2 id="repo-file-heading" className="flex min-w-0 items-center gap-2 text-sm">
              <File className="size-4 shrink-0 text-(--color-muted-foreground)" aria-hidden />
              <span className="font-semibold wrap-anywhere">{data.name}</span>
            </h2>
            <span className="text-xs text-(--color-muted-foreground)">
              {lineCount
                ? `${t(lineCount === 1 ? "repo.file_line" : "repo.file_lines", { count: lineCount })} · `
                : null}
              {formatFileSize(data.size)}
            </span>
            <div className="flex w-full flex-wrap items-center gap-2 sm:ml-auto sm:w-auto">
              <FileModeNav owner={repo.owner} name={repo.name} splat={splatOf(data.ref, data.path)} mode="blame" />
              <div className="flex overflow-hidden rounded-md border border-(--color-border) text-xs [&>*+*]:border-l [&>*+*]:border-(--color-border)">
                {data.refType !== "commit" ? (
                  <Link
                    to="/$owner/$repo/blame/$"
                    params={{ ...params, _splat: splatOf(data.commitID, data.path) }}
                    className="bg-(--color-background) px-2.5 py-1.5 hover:bg-(--color-surface)"
                  >
                    {t("repo.file_permalink")}
                  </Link>
                ) : null}
                <a href={rawURL} className="bg-(--color-background) px-2.5 py-1.5 hover:bg-(--color-surface)">
                  {t("repo.file_raw")}
                </a>
              </div>
            </div>
          </div>
          {data.kind === "text" ? (
            <BlameBody owner={repo.owner} name={repo.name} data={data} lines={lines} />
          ) : (
            <div className="flex flex-col items-center gap-3 px-4 py-10 text-center text-sm">
              <p className="font-semibold">
                {data.kind === "too_large" ? t("repo.file_too_large") : t("repo.blame_not_text")}
              </p>
              <Button asChild variant="outline" size="sm">
                <a href={rawURL} rel="nofollow">
                  {t("repo.file_view_raw")}
                </a>
              </Button>
            </div>
          )}
        </section>
      </main>
    </div>
  );
}

// newestOf returns the SHA of the newest commit of the file.
function newestOf(commits: Record<string, RepoBlameCommit>) {
  let newest = "";
  for (const [sha, commit] of Object.entries(commits)) {
    if (!newest || Date.parse(commit.author.when) > Date.parse(commits[newest].author.when)) newest = sha;
  }
  return newest;
}

function BlameBody({
  owner,
  name,
  data,
  lines,
}: {
  owner: string;
  name: string;
  data: RepoBlameData;
  lines: string[];
}) {
  const { t } = useTranslation();
  const [selected, setSelected] = useState(() => newestOf(data.commits));
  const tokens = useHighlightedLines(data.content.replace(/\n$/, ""), data.name);
  const strengths = useMemo(() => ageStrengthsOf(data.commits), [data.commits]);
  const selectedLines = useMemo(
    () => data.hunks.reduce((sum, hunk) => (hunk.sha === selected ? sum + hunk.lineCount : sum), 0),
    [data.hunks, selected],
  );

  const commit = data.commits[selected];
  const [committedBefore, committedAfter = ""] = t("repo.commits.committed").split("%s");
  const tabStyle = data.tabWidth ? ({ tabSize: data.tabWidth } as CSSProperties) : undefined;

  return (
    <>
      {commit ? (
        <div className="flex flex-wrap items-center gap-x-2 gap-y-1 border-b border-(--color-border) px-4 py-2 text-xs">
          <span
            className="font-semibold wrap-anywhere [&_a]:text-(--color-primary) [&_a]:hover:underline"
            dangerouslySetInnerHTML={{ __html: commit.subjectHTML }}
          />
          <span className="flex flex-wrap items-center gap-x-1 text-(--color-muted-foreground)">
            <img src={commit.author.avatarURL} alt="" className="size-4 shrink-0 rounded-full" />
            {commit.author.profileURL ? (
              <a href={commit.author.profileURL} className="font-semibold text-(--color-foreground) hover:underline">
                {commit.author.name}
              </a>
            ) : (
              <span className="font-semibold text-(--color-foreground)">{commit.author.name}</span>
            )}
            {committedBefore.trim()} <RelativeTime when={commit.author.when} />
            {committedAfter.trim()}
          </span>
          <Link
            to="/$owner/$repo/commit/$sha"
            params={{ owner, repo: name, sha: commit.sha }}
            className="font-mono text-(--color-primary) hover:underline"
          >
            {commit.sha.slice(0, 7)}
          </Link>
          <span className="text-(--color-muted-foreground)">
            {t("repo.blame_lines_of_commit", { count: selectedLines, total: lines.length })}
          </span>
          <span className="flex items-center gap-1 text-(--color-muted-foreground) sm:ml-auto" aria-hidden>
            {t("repo.blame_older")}
            {AGE_STRENGTHS.map((strength) => (
              <i
                key={strength}
                className="h-2.5 w-2 rounded-[1px] bg-(--color-primary)"
                style={{ opacity: strength }}
              />
            ))}
            {t("repo.blame_newer")}
          </span>
        </div>
      ) : null}
      <div className="divide-y divide-(--color-border) text-xs">
        {data.hunks.map((hunk) => {
          const hunkCommit = data.commits[hunk.sha];
          const isSelected = hunk.sha === selected;
          const shortSHA = hunk.sha.slice(0, 7);
          return (
            <div
              key={hunk.startLine}
              className={cn(
                "grid grid-cols-[3px_minmax(0,1fr)] sm:grid-cols-[3px_minmax(0,18.75rem)_minmax(0,1fr)]",
                isSelected && "bg-[color-mix(in_srgb,var(--color-primary)_10%,var(--color-background))]",
              )}
            >
              <div className="row-span-2 bg-(--color-primary) sm:row-span-1" style={{ opacity: strengths[hunk.sha] }} />
              <div className="flex min-w-0 items-start gap-1.5 px-2 pt-1 leading-[18px] sm:border-r sm:border-(--color-border) sm:pt-0">
                <span className="order-3 shrink-0 text-(--color-muted-foreground) sm:order-none sm:w-21">
                  <RelativeTime when={hunkCommit.author.when} />
                </span>
                <img src={hunkCommit.author.avatarURL} alt="" className="mt-0.5 size-3.5 shrink-0 rounded-full" />
                <button
                  type="button"
                  aria-pressed={isSelected}
                  onClick={() => setSelected(hunk.sha)}
                  className="min-w-0 flex-1 truncate text-left hover:text-(--color-primary) hover:underline"
                  title={hunkCommit.subject}
                >
                  {hunkCommit.subject}
                </button>
                {hunkCommit.previousSHA && hunkCommit.previousPath ? (
                  <Tooltip>
                    <TooltipTrigger asChild>
                      <Link
                        to="/$owner/$repo/blame/$"
                        params={{
                          owner,
                          repo: name,
                          _splat: splatOf(hunkCommit.previousSHA, hunkCommit.previousPath),
                        }}
                        hash={`L${hunk.startLine}`}
                        aria-label={t("repo.blame_prior", { sha: shortSHA })}
                        className="order-4 grid h-[18px] w-6 shrink-0 place-items-center rounded-sm text-(--color-muted-foreground) hover:bg-(--color-surface) hover:text-(--color-foreground) sm:order-none"
                      >
                        <History className="size-3.5" aria-hidden />
                      </Link>
                    </TooltipTrigger>
                    <TooltipContent>{t("repo.blame_prior", { sha: shortSHA })}</TooltipContent>
                  </Tooltip>
                ) : (
                  <span className="order-4 w-6 shrink-0 sm:order-none" />
                )}
              </div>
              <div className="col-start-2 min-w-0 pb-0.5 font-mono sm:col-start-auto sm:pb-0" style={tabStyle}>
                {lines.slice(hunk.startLine - 1, hunk.startLine - 1 + hunk.lineCount).map((line, i) => {
                  const number = hunk.startLine + i;
                  return (
                    <div
                      key={number}
                      id={`L${number}`}
                      className="grid scroll-mt-20 grid-cols-[3rem_minmax(0,1fr)] leading-[18px]"
                    >
                      <span className="pr-2.5 text-right text-(--color-muted-foreground) tabular-nums select-none">
                        {number}
                      </span>
                      <span className="pr-3 wrap-anywhere whitespace-pre-wrap">
                        <BlameLine tokens={tokens?.[number - 1]} text={line} />
                      </span>
                    </div>
                  );
                })}
              </div>
            </div>
          );
        })}
      </div>
    </>
  );
}

// BlameLine shows a line in the colors of the highlighter, which come as CSS
// variables for each theme, or as plain text before they load.
function BlameLine({ tokens, text }: { tokens?: ThemedToken[]; text: string }) {
  if (!tokens) return text || "\n";
  if (!tokens.length) return "\n";
  return tokens.map((token, i) => (
    <span key={i} style={token.htmlStyle as CSSProperties} className="text-(--blame-light) dark:text-(--blame-dark)">
      {token.content}
    </span>
  ));
}
