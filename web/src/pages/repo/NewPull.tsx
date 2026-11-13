import { useSuspenseQuery } from "@tanstack/react-query";
import { Link, useNavigate } from "@tanstack/react-router";
import { ArrowLeft, GitCompareArrows } from "lucide-react";
import { useTranslation } from "react-i18next";

import { DiffWorkspace } from "@/components/DiffWorkspace";
import { RefPicker } from "@/components/RefPicker";
import { RepoHeader } from "@/components/RepoHeader";
import { RichText } from "@/components/RichText";
import { usePageTitle } from "@/lib/page-title";
import { type RepoNewPullData, createRepoPull, repoHeaderQuery } from "@/lib/queries/repo";

import type { RepoCommitSearch } from "./Commit.search";
import { CommitList } from "./Commits";
import { NewIssueForm } from "./NewIssue";

// RepoNewPull compares a branch with a base branch of the repository, and
// opens a pull request to merge it.
export function RepoNewPull({
  owner,
  name,
  data,
  patch,
  search,
}: {
  owner: string;
  name: string;
  data: RepoNewPullData;
  patch: string | null;
  search: RepoCommitSearch;
}) {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const { data: repo } = useSuspenseQuery(repoHeaderQuery(owner, name));
  usePageTitle(`${t("repo.pulls.compare_changes")} - ${repo.owner}/${repo.name}`);
  const params = { owner: repo.owner, repo: repo.name };
  const isFork = data.head !== data.headBranch;
  // Links to the comparison of other branches keep the head in its fork.
  const compareLink = (base: string, head: string) => ({
    to: "/$owner/$repo/compare/$" as const,
    params: { ...params, _splat: `${base}...${isFork ? `${data.headOwner}:` : ""}${head}` },
  });
  const message = {
    unrelated: t("repo.pulls.nothing_merge_base"),
    even: t("repo.pulls.nothing_to_compare"),
    exists: t("repo.pulls.has_pull_request")
      .replace("%[2]s", `${repo.owner}/${repo.name}`)
      .replaceAll("%[3]d", String(data.pullIndex)),
    ready: "",
  }[data.state];

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <RepoHeader repo={repo} activeTab="pulls" />

      <main className="mx-auto w-full max-w-7xl px-4 pt-6 pb-4 sm:px-6">
        <h1 className="text-2xl font-semibold">{t("repo.pulls.compare_changes")}</h1>
        <p className="mt-1 text-sm text-(--color-muted-foreground)">{t("repo.pulls.compare_changes_desc")}</p>

        <div className="mt-4 flex flex-wrap items-center gap-2 rounded-md border border-(--color-border) bg-(--color-surface) p-2">
          <GitCompareArrows className="mx-1 size-4 shrink-0 text-(--color-muted-foreground)" aria-hidden />
          <RefPicker
            current={data.baseBranch}
            type="branch"
            label={t("repo.pulls.compare_base")}
            branches={data.branches}
            renderLink={(ref, className, children) => (
              <Link {...compareLink(ref, data.headBranch)} search={search} className={className}>
                {children}
              </Link>
            )}
          />
          <ArrowLeft className="size-4 shrink-0 text-(--color-muted-foreground)" aria-hidden />
          {isFork ? (
            <span className="min-w-0 font-mono text-sm break-all text-(--color-muted-foreground)">
              {data.headOwner}/{data.headName}
            </span>
          ) : null}
          <RefPicker
            current={data.headBranch}
            type="branch"
            label={t("repo.pulls.compare_compare")}
            branches={data.headBranches}
            renderLink={(ref, className, children) => (
              <Link {...compareLink(data.baseBranch, ref)} search={search} className={className}>
                {children}
              </Link>
            )}
          />
        </div>

        {message ? (
          <p className="mt-4 rounded-md border border-(--color-border) px-4 py-6 text-center text-sm text-(--color-muted-foreground)">
            <RichText text={message} links={[`/${repo.owner}/${repo.name}/pulls/${data.pullIndex}`]} />
          </p>
        ) : (
          <div className="mt-6">
            <NewIssueForm
              owner={repo.owner}
              name={repo.name}
              options={data}
              label={t("repo.pulls.new")}
              submitLabel={t("repo.pulls.create")}
              title={data.title}
              content={data.template}
              create={(form) =>
                createRepoPull(repo.owner, repo.name, { ...form, base: data.baseBranch, head: data.head })
              }
              onCreated={(index) =>
                navigate({ to: "/$owner/$repo/pulls/$index", params: { ...params, index: String(index) } })
              }
            />
          </div>
        )}

        {data.commits.length ? (
          <section className="mt-8">
            <h2 className="mb-3 text-lg font-semibold">
              {t(data.commits.length === 1 ? "repo.commits.n_commit" : "repo.commits.n_commits", {
                count: data.commits.length,
              })}
            </h2>
            <CommitList owner={data.headOwner} name={data.headName} commits={data.commits} />
          </section>
        ) : null}
      </main>

      {data.diff && patch ? (
        <DiffWorkspace
          owner={data.diff.owner}
          repo={data.diff.name}
          patch={patch}
          oldRef={data.diff.base}
          newRef={data.diff.head}
          search={search}
        />
      ) : null}
    </div>
  );
}
