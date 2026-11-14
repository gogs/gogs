import { useSuspenseQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";

import { DiffWorkspace } from "@/components/DiffWorkspace";
import { RepoHeader } from "@/components/RepoHeader";
import { usePageTitle } from "@/lib/page-title";
import { type RepoCompareData, repoHeaderQuery } from "@/lib/queries/repo";
import { subUrl } from "@/lib/url";

import type { RepoCommitSearch } from "./Commit.search";
import { CommitList } from "./Commits";

// RepoCompare shows the commits and the changes from one commit to another,
// e.g., of a push in the activity feed.
export function RepoCompare({
  owner,
  name,
  data,
  search,
}: {
  owner: string;
  name: string;
  data: RepoCompareData & { patch: string };
  search: RepoCommitSearch;
}) {
  const { t } = useTranslation();
  const { data: repo } = useSuspenseQuery(repoHeaderQuery(owner, name));
  const { before, after, base, baseIsEmptyTree, commits, patch } = data;
  const repoLink = subUrl(`/${owner}/${name}`);
  usePageTitle(`${base.slice(0, 7)}...${after.slice(0, 7)} - ${owner}/${name}`);

  const shaClass = "rounded bg-(--color-surface) px-1.5 py-0.5 font-mono text-sm text-(--color-foreground)";
  const shaLink = (sha: string) => (
    <a href={`${repoLink}/commit/${sha}`} className={`${shaClass} hover:underline`}>
      {sha.slice(0, 7)}
    </a>
  );

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <RepoHeader repo={repo} activeTab="code" />

      <section className="mx-auto w-full max-w-7xl px-4 pt-6 pb-4 sm:px-6">
        <div className="mb-4 flex flex-wrap items-center justify-between gap-x-4 gap-y-2">
          <h2 className="flex flex-wrap items-center gap-x-2 gap-y-1 text-xl font-semibold text-(--color-foreground)">
            <span>
              {t(commits.length === 1 ? "repo.commits.n_commit" : "repo.commits.n_commits", { count: commits.length })}
            </span>
            <span className="inline-flex items-center gap-1 font-normal text-(--color-muted-foreground)">
              {/* The empty tree is not a commit, so it has no page to link to. */}
              {baseIsEmptyTree ? <span className={shaClass}>{base.slice(0, 7)}</span> : shaLink(base)}
              <span aria-hidden>...</span>
              {shaLink(after)}
            </span>
          </h2>
          <a
            href={`${repoLink}/compare/${before}...${after}.diff`}
            className="inline-flex h-7 shrink-0 items-center rounded-md border border-(--color-border) px-2 font-mono text-xs hover:bg-(--color-surface)"
          >
            {t("repo.diff_label")}
          </a>
        </div>
        {commits.length ? (
          <CommitList owner={owner} name={name} commits={commits} />
        ) : (
          <p className="rounded-md border border-(--color-border) px-4 py-6 text-center text-sm text-(--color-muted-foreground)">
            {t("repo.commits.no_commits")}
          </p>
        )}
      </section>

      {patch ? (
        <DiffWorkspace
          owner={owner}
          repo={name}
          patch={patch}
          oldRef={baseIsEmptyTree ? undefined : base}
          newRef={after}
          search={search}
        />
      ) : null}
    </div>
  );
}
