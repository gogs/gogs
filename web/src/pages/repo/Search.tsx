import { useSuspenseQuery } from "@tanstack/react-query";
import { Link, useNavigate } from "@tanstack/react-router";
import { TriangleAlert } from "lucide-react";
import { useTranslation } from "react-i18next";

import { CodeSearchForm, CodeSearchHelp, CodeSearchResults } from "@/components/CodeSearchResults";
import { RefPicker } from "@/components/RefPicker";
import { RepoHeader } from "@/components/RepoHeader";
import { usePageTitle } from "@/lib/page-title";
import { type RepoSearchData, refPath, repoHeaderQuery } from "@/lib/queries/repo";
import { subUrl } from "@/lib/url";

// RepoSearch searches the files of a branch, a tag or a commit for lines that
// contain a keyword, and lists them by file.
export function RepoSearch({ owner, name, data }: { owner: string; name: string; data: RepoSearchData }) {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const { data: repo } = useSuspenseQuery(repoHeaderQuery(owner, name));
  usePageTitle(`${data.query ? `${data.query} - ` : ""}${t("repo.code_search")} - ${repo.owner}/${repo.name}`);
  const params = { owner: repo.owner, repo: repo.name };
  const fullName = `${repo.owner}/${repo.name}`;

  let message: string | null = null;
  if (data.keyword && data.timedOut) message = t("repo.code_search.timed_out");
  else if (data.keyword && !data.files.length) message = t("repo.code_search.no_results");

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
                search={{ q: data.query || undefined }}
                className={className}
              >
                {children}
              </Link>
            )}
          />
          <CodeSearchForm
            query={data.query}
            autoFocus={!data.query}
            onSearch={(q) =>
              void navigate({
                to: "/$owner/$repo/search/$",
                params: { ...params, _splat: data.ref },
                search: { q: q || undefined },
              })
            }
          />
        </div>

        {!data.keyword ? (
          <CodeSearchHelp hint={t("repo.code_search.hint", { ref: data.ref })} />
        ) : message ? (
          <p className="flex items-center justify-center gap-2 rounded-md border border-(--color-border) px-4 py-10 text-center text-sm text-(--color-muted-foreground)">
            {data.timedOut ? <TriangleAlert className="size-4 shrink-0 text-(--color-warning)" aria-hidden /> : null}
            <span className="wrap-anywhere">{message}</span>
          </p>
        ) : (
          <>
            <CodeSearchResults
              files={data.files}
              keyword={data.keyword}
              urlOf={(file, line) =>
                subUrl(`/${fullName}/src/${refPath(`${data.ref}/${file.path}`)}${line ? `#L${line}` : ""}`)
              }
            />
            {data.truncated ? (
              <p className="mt-4 text-center text-sm text-(--color-muted-foreground)">
                {t("repo.code_search.truncated", {
                  count: data.files.reduce((n, f) => n + f.lines.filter((l) => l.isMatch).length, 0),
                })}
              </p>
            ) : null}
          </>
        )}

        {data.keyword ? (
          <p className="mt-6 text-center text-sm">
            <Link to="/search" search={{ q: data.query }} className="text-(--color-primary) hover:underline">
              {t("repo.code_search.all_repos")}
            </Link>
          </p>
        ) : null}
      </main>
    </div>
  );
}
