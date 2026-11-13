import { useSuspenseQuery } from "@tanstack/react-query";
import { Link, useNavigate } from "@tanstack/react-router";
import { FileCode, Search, TriangleAlert } from "lucide-react";
import { type FormEvent, useState } from "react";
import { useTranslation } from "react-i18next";

import { RefPicker } from "@/components/RefPicker";
import { RepoHeader } from "@/components/RepoHeader";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { usePageTitle } from "@/lib/page-title";
import { type RepoSearchData, refPath, repoHeaderQuery } from "@/lib/queries/repo";
import { subUrl } from "@/lib/url";

// highlight marks every occurrence of the keyword in the text, ignoring case
// like the search.
function highlight(text: string, keyword: string) {
  const pattern = new RegExp(`(${keyword.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")})`, "giu");
  // Splitting on a pattern with one group puts the matches at the odd indexes.
  return text.split(pattern).map((part, i) =>
    i % 2 ? (
      <mark key={i} className="rounded-sm bg-(--color-warning)/25 font-semibold text-(--color-foreground)">
        {part}
      </mark>
    ) : (
      part
    ),
  );
}

// RepoSearch searches the files of a branch, a tag or a commit for lines that
// contain a keyword, and lists them by file.
export function RepoSearch({ owner, name, data }: { owner: string; name: string; data: RepoSearchData }) {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const { data: repo } = useSuspenseQuery(repoHeaderQuery(owner, name));
  const [keyword, setKeyword] = useState(data.keyword);
  usePageTitle(`${data.keyword ? `${data.keyword} - ` : ""}${t("repo.code_search")} - ${repo.owner}/${repo.name}`);
  const params = { owner: repo.owner, repo: repo.name };
  const srcURL = (path: string, line?: number) =>
    subUrl(`/${repo.owner}/${repo.name}/src/${refPath(`${data.ref}/${path}`)}${line ? `#L${line}` : ""}`);

  const onSearch = (e: FormEvent) => {
    e.preventDefault();
    const q = keyword.trim();
    void navigate({
      to: "/$owner/$repo/search/$",
      params: { ...params, _splat: data.ref },
      search: { q: q || undefined },
    });
  };

  let message: string | null = null;
  if (!data.keyword) message = t("repo.code_search.hint", { ref: data.ref });
  else if (data.timedOut) message = t("repo.code_search.timed_out");
  else if (!data.files.length) message = t("repo.code_search.no_results");

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <RepoHeader repo={repo} activeTab="code" />
      <main className="mx-auto w-full max-w-7xl flex-1 px-4 py-6 sm:px-6">
        <h1 className="sr-only">{t("repo.code_search")}</h1>
        <div className="mb-5 flex flex-wrap items-center gap-3">
          <RefPicker
            current={data.ref}
            type={data.refType}
            branches={data.branches}
            tags={data.tags}
            renderLink={(ref, className, children) => (
              <Link
                to="/$owner/$repo/search/$"
                params={{ ...params, _splat: ref }}
                search={{ q: data.keyword || undefined }}
                className={className}
              >
                {children}
              </Link>
            )}
          />
          <form role="search" onSubmit={onSearch} className="flex w-full gap-2 sm:w-auto sm:flex-1">
            <div className="relative min-w-0 flex-1">
              <Search
                className="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-(--color-muted-foreground)"
                aria-hidden
              />
              <Input
                type="search"
                value={keyword}
                onChange={(e) => setKeyword(e.target.value)}
                placeholder={t("repo.code_search")}
                aria-label={t("repo.code_search")}
                autoFocus={!data.keyword}
                className="h-8 pl-9"
              />
            </div>
            <Button type="submit" variant="outline" size="sm">
              {t("explore.search")}
            </Button>
          </form>
        </div>

        {message ? (
          <p className="flex items-center justify-center gap-2 rounded-md border border-(--color-border) px-4 py-10 text-center text-sm text-(--color-muted-foreground)">
            {data.timedOut ? <TriangleAlert className="size-4 shrink-0 text-(--color-warning)" aria-hidden /> : null}
            <span className="wrap-anywhere">{message}</span>
          </p>
        ) : (
          <div className="flex flex-col gap-4">
            {data.files.map((file) => (
              <section
                key={file.path}
                aria-label={file.path}
                className="overflow-hidden rounded-md border border-(--color-border)"
              >
                <h2 className="flex items-center gap-2 border-b border-(--color-border) bg-(--color-surface) px-4 py-2 text-sm">
                  <FileCode className="size-4 shrink-0 text-(--color-muted-foreground)" aria-hidden />
                  <a
                    href={srcURL(file.path)}
                    className="font-mono font-semibold wrap-anywhere text-(--color-primary) hover:underline"
                  >
                    {file.path}
                  </a>
                </h2>
                <ol className="overflow-x-auto py-1 font-mono text-xs/5">
                  {file.matches.map((match) => (
                    <li key={match.line} className="flex min-w-max">
                      <a
                        href={srcURL(file.path, match.line)}
                        aria-label={t("repo.code_search.line", { line: match.line })}
                        className="sticky left-0 w-14 shrink-0 bg-(--color-background) pr-3 text-right text-(--color-muted-foreground) tabular-nums hover:text-(--color-primary) hover:underline"
                      >
                        {match.line}
                      </a>
                      <span className="pr-4 whitespace-pre">{highlight(match.text, data.keyword)}</span>
                    </li>
                  ))}
                </ol>
              </section>
            ))}
            {data.truncated ? (
              <p className="text-center text-sm text-(--color-muted-foreground)">
                {t("repo.code_search.truncated", { count: data.files.reduce((n, f) => n + f.matches.length, 0) })}
              </p>
            ) : null}
          </div>
        )}
      </main>
    </div>
  );
}
