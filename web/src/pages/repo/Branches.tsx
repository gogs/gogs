import { useSuspenseQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { GitBranch, GitPullRequest, List, ShieldCheck } from "lucide-react";
import { type ReactNode, useState } from "react";
import { useTranslation } from "react-i18next";

import { RepoHeader } from "@/components/RepoHeader";
import { Button } from "@/components/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { usePageTitle } from "@/lib/page-title";
import { type RepoBranch, type RepoBranchesData, refPath, repoHeaderQuery } from "@/lib/queries/repo";
import { formatAbsoluteTime, formatRelativeTime } from "@/lib/relative-time";
import { subUrl } from "@/lib/url";
import { cn } from "@/lib/utils";

const DAY_MS = 24 * 60 * 60 * 1000;

type BranchesView = "overview" | "all";

// RepoBranches lists the branches of a repository. The overview groups them
// like the server-rendered page did: the default branch, the branches updated
// in the last 30 days, and the ones not updated for 90 days.
export function RepoBranches({
  owner,
  name,
  view,
  data,
}: {
  owner: string;
  name: string;
  view: BranchesView;
  data: RepoBranchesData;
}) {
  const { t } = useTranslation();
  const { data: repo } = useSuspenseQuery(repoHeaderQuery(owner, name));
  usePageTitle(`${t("repo.git_branches")} - ${repo.owner}/${repo.name}`);
  // The branches are grouped by their age when the page opens.
  const [now] = useState(() => Date.now());
  const params = { owner: repo.owner, repo: repo.name };
  const tabs = [
    { key: "overview", to: "/$owner/$repo/branches", label: t("repo.branches.overview"), icon: GitBranch },
    { key: "all", to: "/$owner/$repo/branches/all", label: t("repo.branches.all"), icon: List },
  ] as const;

  const row = (branch: RepoBranch) => (
    <BranchRow key={branch.name} owner={owner} name={name} branch={branch} data={data} />
  );
  let sections: { title: string; branches: RepoBranch[] }[];
  if (view === "all") {
    sections = [{ title: t("repo.branches.all"), branches: data.branches }];
  } else {
    const others = data.branches.filter((b) => b.name !== data.defaultBranch);
    const age = (b: RepoBranch) => now - Date.parse(b.committedAt);
    sections = [
      {
        title: t("repo.settings.default_branch"),
        branches: data.branches.filter((b) => b.name === data.defaultBranch),
      },
      { title: t("repo.branches.active_branches"), branches: others.filter((b) => age(b) < 30 * DAY_MS) },
      { title: t("repo.branches.stale_branches"), branches: others.filter((b) => age(b) > 90 * DAY_MS) },
    ];
  }

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <RepoHeader repo={repo} activeTab="code" />
      <main className="mx-auto w-full max-w-7xl flex-1 px-4 py-6 sm:px-6">
        <h1 className="sr-only">{t("repo.git_branches")}</h1>
        <nav aria-label={t("repo.git_branches")} className="mb-6 inline-flex rounded-md border border-(--color-input)">
          {tabs.map(({ key, to, label, icon: Icon }) => (
            <Link
              key={key}
              to={to}
              params={params}
              aria-current={key === view ? "page" : undefined}
              className={cn(
                "inline-flex items-center gap-1.5 px-3 py-1.5 text-sm first:rounded-l-md last:rounded-r-md not-first:border-l not-first:border-(--color-input) hover:bg-(--color-surface)",
                key === view
                  ? "bg-(--color-surface) font-semibold text-(--color-foreground)"
                  : "text-(--color-muted-foreground)",
              )}
            >
              <Icon className="size-4" aria-hidden />
              {label}
            </Link>
          ))}
        </nav>
        <div className="flex flex-col gap-8">
          {sections.map(({ title, branches }) =>
            branches.length ? (
              <section key={title}>
                <h2 className="mb-2 text-base font-semibold">{title}</h2>
                <ul className="divide-y divide-(--color-border) rounded-md border border-(--color-border)">
                  {branches.map(row)}
                </ul>
              </section>
            ) : null,
          )}
        </div>
      </main>
    </div>
  );
}

function BranchRow({
  owner,
  name,
  branch,
  data,
}: {
  owner: string;
  name: string;
  branch: RepoBranch;
  data: RepoBranchesData;
}) {
  const { t } = useTranslation();
  const repoLink = `/${owner}/${name}`;
  const isDefault = branch.name === data.defaultBranch;
  let action: ReactNode = null;
  if (isDefault) {
    if (data.viewerCanChangeDefault) {
      action = (
        <Button asChild variant="outline" size="sm">
          <Link to="/$owner/$repo/settings/branches" params={{ owner, repo: name }}>
            {t("repo.branches.change_default_branch")}
          </Link>
        </Button>
      );
    }
  } else if (data.viewerCanWrite && data.allowsPulls) {
    action = (
      <Button asChild variant="outline" size="sm">
        <a
          href={subUrl(`${repoLink}/compare/${refPath(data.defaultBranch)}...${refPath(branch.name)}`)}
          aria-label={`${t("repo.pulls.new")}: ${branch.name}`}
        >
          <GitPullRequest className="size-4" aria-hidden />
          {t("repo.pulls.new")}
        </a>
      </Button>
    );
  }

  return (
    <li className="flex flex-wrap items-center gap-x-4 gap-y-2 px-4 py-3">
      <div className="flex min-w-0 flex-1 basis-64 flex-col gap-1">
        <div className="flex flex-wrap items-center gap-2">
          <a
            href={subUrl(`${repoLink}/src/${refPath(branch.name)}`)}
            className="rounded bg-(--color-surface) px-1.5 py-0.5 font-mono text-sm wrap-anywhere text-(--color-primary) hover:underline"
          >
            {branch.name}
          </a>
          {branch.isProtected ? (
            <span className="inline-flex items-center gap-1 rounded-full border border-(--color-border) px-2 text-xs leading-5 font-medium text-(--color-muted-foreground)">
              <ShieldCheck className="size-3.5" aria-hidden />
              {t("repo.branches.protected")}
            </span>
          ) : null}
        </div>
        <p className="text-sm text-(--color-muted-foreground)">
          <UpdatedBy branch={branch} />
        </p>
      </div>
      {action}
    </li>
  );
}

// UpdatedBy renders the localized "Updated <time> by <committer>" sentence,
// whose translations place the time as `%[1]s` and the committer as `%[2]s`.
function UpdatedBy({ branch }: { branch: RepoBranch }) {
  const { t } = useTranslation();
  const parts = t("repo.branches.updated_by").split(/(%\[[12]\]s)/);
  return (
    <>
      {parts.map((part, i) => {
        if (part === "%[1]s") {
          return (
            <Tooltip key={i}>
              <TooltipTrigger asChild>
                <time dateTime={branch.committedAt}>{formatRelativeTime(t, branch.committedAt)}</time>
              </TooltipTrigger>
              <TooltipContent>{formatAbsoluteTime(branch.committedAt)}</TooltipContent>
            </Tooltip>
          );
        }
        if (part === "%[2]s") {
          return (
            <span key={i} className="font-medium text-(--color-foreground)">
              {branch.committerName}
            </span>
          );
        }
        return part;
      })}
    </>
  );
}
