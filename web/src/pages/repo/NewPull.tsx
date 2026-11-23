import { useSuspenseQuery } from "@tanstack/react-query";
import { Link, useNavigate } from "@tanstack/react-router";
import { ArrowLeft, BookMarked, Check, ChevronDown, GitCompareArrows } from "lucide-react";
import { useState } from "react";
import { useTranslation } from "react-i18next";

import { DiffWorkspace } from "@/components/DiffWorkspace";
import { RefPicker } from "@/components/RefPicker";
import { RepoHeader } from "@/components/RepoHeader";
import { RichText } from "@/components/RichText";
import { Button } from "@/components/ui/button";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import { tCount } from "@/lib/i18n";
import { usePageTitle } from "@/lib/page-title";
import { type RepoNewPullData, createRepoPull, repoHeaderQuery } from "@/lib/queries/repo";
import { cn } from "@/lib/utils";

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
    exists: t("repo.pulls.has_pull_request", { repo: `${repo.owner}/${repo.name}`, index: data.pullIndex }),
    ready: "",
  }[data.state];

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <RepoHeader repo={repo} activeTab="pulls" />

      <main className="mx-auto w-full max-w-7xl px-4 pt-6 pb-4 sm:px-6">
        <h1 className="text-2xl font-semibold">{t("repo.pulls.compare_changes")}</h1>
        <p className="mt-1 text-sm text-(--color-muted-foreground)">{t("repo.pulls.compare_changes_desc")}</p>

        <div className="glass mt-4 flex flex-wrap items-center gap-2 rounded-md border border-(--color-border) bg-(--color-surface) p-2">
          <GitCompareArrows className="mx-1 size-4 shrink-0 text-(--color-muted-foreground)" aria-hidden />
          {data.baseRepos.length > 1 ? (
            <BaseRepoPicker owner={repo.owner} name={repo.name} data={data} search={search} />
          ) : null}
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
          <p className="mt-4 glass rounded-md border border-(--color-border) px-4 py-6 text-center text-sm text-(--color-muted-foreground)">
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
          <section className="glass-section mt-8">
            <h2 className="mb-3 text-lg font-semibold">
              {tCount("repo.commits.n_commits", data.commits.length, ["repo.commits.n_commit"])}
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
          markdownExtensions={data.diff.markdownExtensions}
          search={search}
        />
      ) : null}
    </div>
  );
}

// BaseRepoPicker switches the repository to merge into, between the
// repository of the head branch and its base repository. The comparison
// starts from the default branch of the chosen repository.
function BaseRepoPicker({
  owner,
  name,
  data,
  search,
}: {
  owner: string;
  name: string;
  data: RepoNewPullData;
  search: RepoCommitSearch;
}) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button variant="outline" size="sm" className="max-w-full min-w-0">
          <BookMarked className="size-4 shrink-0" aria-hidden />
          <span className="text-(--color-muted-foreground)">{t("repo.pulls.compare_base_repo")}:</span>
          <span className="min-w-0 truncate font-semibold">
            {owner}/{name}
          </span>
          <ChevronDown className="size-4 shrink-0" aria-hidden />
        </Button>
      </PopoverTrigger>
      <PopoverContent align="start" collisionPadding={16} className="w-72 max-w-[calc(100vw-2rem)] p-0">
        <p className="border-b border-(--color-border) px-3 py-2 text-sm font-semibold">
          {t("repo.pulls.choose_base_repo")}
        </p>
        <ul className="p-1">
          {data.baseRepos.map((base) => {
            const isHead = base.owner === data.headOwner && base.name === data.headName;
            const current = base.owner === owner && base.name === name;
            return (
              <li key={`${base.owner}/${base.name}`} onClick={() => setOpen(false)}>
                <Link
                  to="/$owner/$repo/compare/$"
                  params={{
                    owner: base.owner,
                    repo: base.name,
                    _splat: `${base.defaultBranch}...${isHead ? "" : `${data.headOwner}:`}${data.headBranch}`,
                  }}
                  search={search}
                  aria-current={current ? "page" : undefined}
                  className="flex w-full items-center gap-2 rounded-sm px-2 py-2 text-left text-sm hover:bg-(--color-surface) focus-visible:bg-(--color-surface) focus-visible:outline-none"
                >
                  <Check className={cn("size-4 shrink-0", !current && "invisible")} aria-hidden />
                  <img
                    src={base.ownerAvatarURL}
                    alt=""
                    className={cn(
                      "size-5 shrink-0 object-cover",
                      base.ownerIsOrganization ? "rounded-sm" : "rounded-full",
                    )}
                  />
                  <span className="min-w-0 flex-1 truncate">
                    {base.owner}/<span className="font-semibold">{base.name}</span>
                  </span>
                  <span className="shrink-0 rounded-full border border-(--color-border) px-2 py-0.5 text-xs text-(--color-muted-foreground)">
                    {t(isHead ? "repo.pulls.base_repo_fork" : "repo.pulls.base_repo_upstream")}
                  </span>
                </Link>
              </li>
            );
          })}
        </ul>
      </PopoverContent>
    </Popover>
  );
}
