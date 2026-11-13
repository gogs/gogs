import { useSuspenseQuery } from "@tanstack/react-query";
import { Link, useNavigate } from "@tanstack/react-router";
import { Check, Copy, FolderCode, Search } from "lucide-react";
import { type FormEvent, type ReactNode, useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";

import { RefPicker } from "@/components/RefPicker";
import { RepoHeader } from "@/components/RepoHeader";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { webContext } from "@/lib/context";
import { usePageTitle } from "@/lib/page-title";
import { type RepoCommitListItem, type RepoCommitsData, refPath, repoHeaderQuery } from "@/lib/queries/repo";
import { formatAbsoluteTime, formatRelativeTime } from "@/lib/relative-time";
import { subUrl } from "@/lib/url";
import { cn } from "@/lib/utils";

const DAY_FMT = new Intl.DateTimeFormat(webContext.lang, { dateStyle: "long" });

// splatOf is the route path of a ref, followed by a path when given.
function splatOf(ref: string, path?: string) {
  return path ? `${ref}/${path}` : ref;
}

// groupByDay groups the commits by the day they were authored, in the
// viewer's time zone, keeping their order.
function groupByDay(commits: RepoCommitListItem[]) {
  const groups: { day: string; commits: RepoCommitListItem[] }[] = [];
  for (const commit of commits) {
    const day = DAY_FMT.format(new Date(commit.author.when));
    const last = groups.at(-1);
    if (last?.day === day) last.commits.push(commit);
    else groups.push({ day, commits: [commit] });
  }
  return groups;
}

// RepoCommits lists the commits of a branch, a tag or a commit, newest first:
// all of them, those that changed a path, or those whose messages match a
// keyword.
export function RepoCommits({ owner, name, data }: { owner: string; name: string; data: RepoCommitsData }) {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const { data: repo } = useSuspenseQuery(repoHeaderQuery(owner, name));
  const [keyword, setKeyword] = useState(data.keyword);
  usePageTitle(
    `${data.path ? `${t("repo.commits.history_for")} ${data.path}` : t("repo.commits.commit_history")} - ${repo.owner}/${repo.name}`,
  );
  const params = { owner: repo.owner, repo: repo.name };
  const srcURL = (rev: string, path = "") => subUrl(`/${repo.owner}/${repo.name}/src/${refPath(splatOf(rev, path))}`);
  const pathParts = data.path ? data.path.split("/") : [];

  const onSearch = (e: FormEvent) => {
    e.preventDefault();
    const q = keyword.trim();
    void navigate({
      to: "/$owner/$repo/commits/$",
      params: { ...params, _splat: q ? `${data.ref}/search` : data.ref },
      search: q ? { q } : {},
    });
  };

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <RepoHeader repo={repo} activeTab="code" />
      <main className="mx-auto w-full max-w-7xl flex-1 px-4 py-6 sm:px-6">
        <h1 className="sr-only">{t("repo.commits.commit_history")}</h1>
        <div className="mb-5 flex flex-wrap items-center gap-3">
          <RefPicker
            current={data.ref}
            type={data.refType}
            branches={data.branches}
            tags={data.tags}
            renderLink={(ref, className, children) => (
              <Link
                to="/$owner/$repo/commits/$"
                params={{ ...params, _splat: splatOf(ref, data.search ? "search" : data.path) }}
                search={data.search ? { q: data.keyword || undefined } : {}}
                className={className}
              >
                {children}
              </Link>
            )}
          />
          {data.path ? (
            <p className="flex min-w-0 flex-wrap items-center gap-x-1 text-sm">
              <span className="text-(--color-muted-foreground)">{t("repo.commits.history_for")}</span>
              <a href={srcURL(data.ref)} className="font-semibold text-(--color-primary) hover:underline">
                {repo.name}
              </a>
              {pathParts.map((part, i) => (
                <span key={i} className="contents">
                  <span className="text-(--color-muted-foreground)">/</span>
                  {i === pathParts.length - 1 ? (
                    <span className="font-semibold wrap-anywhere">{part}</span>
                  ) : (
                    <a
                      href={srcURL(data.ref, pathParts.slice(0, i + 1).join("/"))}
                      className="text-(--color-primary) wrap-anywhere hover:underline"
                    >
                      {part}
                    </a>
                  )}
                </span>
              ))}
            </p>
          ) : null}
          <form role="search" onSubmit={onSearch} className="flex w-full gap-2 sm:ml-auto sm:w-auto">
            <div className="relative min-w-0 flex-1 sm:w-64">
              <Search
                className="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-(--color-muted-foreground)"
                aria-hidden
              />
              <Input
                type="search"
                value={keyword}
                onChange={(e) => setKeyword(e.target.value)}
                placeholder={t("repo.commits.search")}
                aria-label={t("repo.commits.search")}
                className="h-8 pl-9"
              />
            </div>
            <Button type="submit" variant="outline" size="sm">
              {t("repo.commits.find")}
            </Button>
          </form>
        </div>

        {data.commits.length ? (
          <div className="flex flex-col gap-6">
            {groupByDay(data.commits).map((group) => (
              <section key={group.day} aria-label={group.day}>
                <h2 className="mb-2 text-sm font-semibold text-(--color-muted-foreground)">{group.day}</h2>
                <ul className="divide-y divide-(--color-border) rounded-md border border-(--color-border)">
                  {group.commits.map((commit) => (
                    <CommitRow
                      key={commit.sha}
                      owner={repo.owner}
                      name={repo.name}
                      commit={commit}
                      browseURL={srcURL(commit.sha, data.path)}
                    />
                  ))}
                </ul>
              </section>
            ))}
          </div>
        ) : (
          <p className="rounded-md border border-(--color-border) py-10 text-center text-sm text-(--color-muted-foreground)">
            {t("repo.commits.no_commits")}
          </p>
        )}

        {!data.search && (data.page > 1 || data.hasNext) ? (
          <nav aria-label={t("page")} className="mt-6 flex justify-center gap-2">
            <PageButton page={data.page > 1 ? data.page - 1 : null}>{t("repo.commits.newer")}</PageButton>
            <PageButton page={data.hasNext ? data.page + 1 : null}>{t("repo.commits.older")}</PageButton>
          </nav>
        ) : null}
      </main>
    </div>
  );
}

// PageButton links another page of commits, or is disabled when there is
// none.
function PageButton({ page, children }: { page: number | null; children: ReactNode }) {
  if (page === null) {
    return (
      <Button variant="outline" size="sm" disabled>
        {children}
      </Button>
    );
  }
  return (
    <Button asChild variant="outline" size="sm">
      <Link to="." search={(prev: Record<string, unknown>) => ({ ...prev, page: page > 1 ? page : undefined })}>
        {children}
      </Link>
    </Button>
  );
}

function CommitRow({
  owner,
  name,
  commit,
  browseURL,
}: {
  owner: string;
  name: string;
  commit: RepoCommitListItem;
  browseURL: string;
}) {
  const { t } = useTranslation();
  const [copied, setCopied] = useState(false);
  const [before, after = ""] = t("repo.commits.committed").split("%s");
  const { author } = commit;
  const copySHA = () => {
    navigator.clipboard.writeText(commit.sha).then(
      () => {
        setCopied(true);
        setTimeout(() => setCopied(false), 1500);
      },
      () => toast.error(t("copy_failed")),
    );
  };

  return (
    <li className="flex flex-col gap-2 px-4 py-3 sm:flex-row sm:items-center sm:gap-4">
      <div className="min-w-0 flex-1">
        <p
          className={cn(
            "text-sm font-semibold wrap-anywhere [&_a]:text-(--color-primary) [&_a]:hover:underline",
            commit.isMerge && "font-normal text-(--color-muted-foreground)",
          )}
          dangerouslySetInnerHTML={{ __html: commit.subjectHTML }}
        />
        <p className="mt-1 flex flex-wrap items-center gap-x-1.5 text-xs text-(--color-muted-foreground)">
          <img src={author.avatarURL} alt="" className="size-5 shrink-0 rounded-full" />
          {author.profileURL ? (
            <a href={author.profileURL} className="font-semibold text-(--color-foreground) hover:underline">
              {author.name}
            </a>
          ) : (
            <span className="font-semibold text-(--color-foreground)">{author.name}</span>
          )}
          <span>
            {before}
            <Tooltip>
              <TooltipTrigger asChild>
                <time dateTime={author.when}>{formatRelativeTime(t, author.when)}</time>
              </TooltipTrigger>
              <TooltipContent>{formatAbsoluteTime(author.when)}</TooltipContent>
            </Tooltip>
            {after}
          </span>
        </p>
      </div>
      <div className="flex shrink-0 items-center gap-1">
        <Link
          to="/$owner/$repo/commit/$sha"
          params={{ owner, repo: name, sha: commit.sha }}
          className="rounded-md border border-(--color-border) px-2 py-1 font-mono text-xs hover:bg-(--color-surface)"
        >
          {commit.sha.slice(0, 10)}
        </Link>
        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              type="button"
              variant="ghost"
              size="icon"
              aria-label={t("repo.copy_full_sha")}
              className="size-8 text-(--color-muted-foreground)"
              onClick={copySHA}
            >
              {copied ? <Check className="size-4" aria-hidden /> : <Copy className="size-4" aria-hidden />}
            </Button>
          </TooltipTrigger>
          <TooltipContent>{t("repo.copy_full_sha")}</TooltipContent>
        </Tooltip>
        <Tooltip>
          <TooltipTrigger asChild>
            <Button asChild variant="ghost" size="icon" className="size-8 text-(--color-muted-foreground)">
              <a href={browseURL} aria-label={t("repo.browse_files")}>
                <FolderCode className="size-4" aria-hidden />
              </a>
            </Button>
          </TooltipTrigger>
          <TooltipContent>{t("repo.browse_files")}</TooltipContent>
        </Tooltip>
      </div>
    </li>
  );
}
