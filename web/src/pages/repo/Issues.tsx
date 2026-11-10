import { useSuspenseQuery } from "@tanstack/react-query";
import { Link, useNavigate } from "@tanstack/react-router";
import { CircleCheck, CircleDot, Milestone, Plus, Tag } from "lucide-react";
import { useTranslation } from "react-i18next";

import { IssueListItem } from "@/components/IssueListItem";
import { Pagination } from "@/components/Pagination";
import { RepoHeader } from "@/components/RepoHeader";
import { Button } from "@/components/ui/button";
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { usePageTitle } from "@/lib/page-title";
import { type RepoIssuesData, repoHeaderQuery } from "@/lib/queries/repo";
import { useUserInfo } from "@/lib/use-user-info";
import { cn } from "@/lib/utils";
import type { RepoIssuesSearch } from "@/pages/repo/Issues.search";
import { ISSUE_SORTS, type IssueSort } from "@/pages/user/DashboardIssues.search";

// NONE is the value of the filter choices that clear the filter, since a
// select item cannot have an empty value.
const NONE = "0";

// RepoIssues lists the issues or the pull requests of a repository.
export function RepoIssues({
  owner,
  name,
  kind,
  data,
  search,
}: {
  owner: string;
  name: string;
  kind: "issues" | "pulls";
  data: RepoIssuesData;
  search: RepoIssuesSearch;
}) {
  const { t } = useTranslation();
  const { data: repo } = useSuspenseQuery(repoHeaderQuery(owner, name));
  const title = kind === "pulls" ? t("pull_requests") : t("issues");
  usePageTitle(`${title} - ${repo.owner}/${repo.name}`);
  const params = { owner: repo.owner, repo: repo.name };
  const states = [
    { state: undefined, label: t("repo.issues.open_tab"), count: data.openCount, icon: CircleDot },
    { state: "closed" as const, label: t("repo.issues.close_tab"), count: data.closedCount, icon: CircleCheck },
  ];
  // Keep the other filters when following a label or a milestone in a row.
  const filterLink = (filter: Partial<RepoIssuesSearch>) => ({ ...search, ...filter, page: undefined });

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <RepoHeader repo={repo} activeTab={kind} />
      <main className="mx-auto w-full max-w-7xl flex-1 px-4 py-6 sm:px-6">
        <h1 className="sr-only">{title}</h1>
        <div className="mb-4 flex flex-wrap items-center justify-between gap-2">
          <nav aria-label={t("issues")} className="inline-flex rounded-md border border-(--color-input)">
            <Link
              to="/$owner/$repo/labels"
              params={params}
              className="inline-flex items-center gap-1.5 rounded-l-md px-3 py-1.5 text-sm text-(--color-muted-foreground) hover:bg-(--color-surface)"
            >
              <Tag className="size-4" aria-hidden />
              {t("repo.labels")}
            </Link>
            <Link
              to="/$owner/$repo/milestones"
              params={params}
              className="inline-flex items-center gap-1.5 rounded-r-md border-l border-(--color-input) px-3 py-1.5 text-sm text-(--color-muted-foreground) hover:bg-(--color-surface)"
            >
              <Milestone className="size-4" aria-hidden />
              {t("repo.milestones")}
            </Link>
          </nav>
          {kind === "issues" ? (
            <Button asChild variant="outline" size="sm">
              <Link to="/$owner/$repo/issues/new" params={params}>
                <Plus className="size-4" aria-hidden />
                {t("repo.issues.new")}
              </Link>
            </Button>
          ) : data.newPullRequestURL ? (
            // Comparing branches is still served by the server-rendered pages.
            <Button asChild variant="outline" size="sm">
              <a href={data.newPullRequestURL}>
                <Plus className="size-4" aria-hidden />
                {t("repo.pulls.new")}
              </a>
            </Button>
          ) : null}
        </div>

        <div className="rounded-md border border-(--color-border)">
          <div className="flex flex-wrap items-center justify-between gap-2 bg-(--color-surface) px-2 py-1.5">
            <ul className="flex gap-1">
              {states.map(({ state, label, count, icon: Icon }) => {
                const active = state === search.state;
                return (
                  <li key={state ?? "open"}>
                    <Link
                      to="."
                      search={filterLink({ state })}
                      aria-current={active ? "true" : undefined}
                      className={cn(
                        "inline-flex items-center gap-1.5 rounded-md px-2 py-1.5 text-sm tabular-nums hover:bg-(--color-background)",
                        active ? "font-semibold text-(--color-foreground)" : "text-(--color-muted-foreground)",
                      )}
                    >
                      <Icon className="size-4" aria-hidden />
                      {label.replace(/%(?:\[1\])?d/, String(count))}
                    </Link>
                  </li>
                );
              })}
            </ul>
            <Filters data={data} search={search} />
          </div>
          {data.items.length ? (
            <ul className="px-4 [&>li:last-child]:border-b-0">
              {data.items.map((issue) => (
                <IssueListItem
                  key={issue.index}
                  issue={issue}
                  kind={kind}
                  showRepo={false}
                  unread={issue.isUnread}
                  renderLabel={(label, chip) => (
                    <Link
                      to="."
                      search={filterLink({ labels: label.id })}
                      aria-label={`${t("repo.issues.filter_label")}: ${label.name}`}
                      className="hover:opacity-80"
                    >
                      {chip}
                    </Link>
                  )}
                  meta={
                    issue.milestone ? (
                      <Link
                        to="."
                        search={filterLink({ milestone: issue.milestone.id })}
                        className="inline-flex items-center gap-1 hover:text-(--color-primary) hover:underline"
                      >
                        <Milestone className="size-3.5" aria-hidden />
                        <span className="sr-only">{t("repo.issues.filter_milestone")}:</span>
                        {issue.milestone.name}
                      </Link>
                    ) : null
                  }
                />
              ))}
            </ul>
          ) : (
            <p className="border-t border-(--color-border) py-8 text-center text-sm text-(--color-muted-foreground)">
              {t("repo.pulls.no_results")}
            </p>
          )}
        </div>
        {data.totalPages > 1 ? (
          <div className="mt-4">
            <Pagination page={data.page} totalPages={data.totalPages} />
          </div>
        ) : null}
      </main>
    </div>
  );
}

const TRIGGER_CLASS = "h-9 w-auto gap-2 bg-(--color-background)";

// Filters narrows the list by label, milestone, assignee and whose issues they
// are, and sorts it.
function Filters({ data, search }: { data: RepoIssuesData; search: RepoIssuesSearch }) {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const user = useUserInfo();
  const setFilter = (filter: Partial<RepoIssuesSearch>) =>
    void navigate({ to: ".", search: (prev: RepoIssuesSearch) => ({ ...prev, ...filter, page: undefined }) });
  const id = (value: string) => (value === NONE ? undefined : Number(value));
  const openMilestones = data.milestones.filter((m) => !m.isClosed);
  const closedMilestones = data.milestones.filter((m) => m.isClosed);

  return (
    <div className="flex flex-wrap gap-2">
      <Select
        value={String(search.labels ?? NONE)}
        onValueChange={(v) => setFilter({ labels: id(v) })}
        disabled={data.labels.length === 0}
      >
        <SelectTrigger aria-label={t("repo.issues.filter_label")} className={TRIGGER_CLASS}>
          <span className="text-(--color-muted-foreground)">{t("repo.issues.filter_label")}:</span>
          <SelectValue />
        </SelectTrigger>
        <SelectContent align="end">
          <SelectItem value={NONE}>{t("repo.issues.filter_label_no_select")}</SelectItem>
          {data.labels.map((label) => (
            <SelectItem key={label.id} value={String(label.id)}>
              <span className="inline-flex items-center gap-2">
                <span className="size-3 shrink-0 rounded-full" style={{ backgroundColor: label.color }} aria-hidden />
                {label.name}
              </span>
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      <Select
        value={String(search.milestone ?? NONE)}
        onValueChange={(v) => setFilter({ milestone: id(v) })}
        disabled={data.milestones.length === 0}
      >
        <SelectTrigger aria-label={t("repo.issues.filter_milestone")} className={TRIGGER_CLASS}>
          <span className="text-(--color-muted-foreground)">{t("repo.issues.filter_milestone")}:</span>
          <SelectValue />
        </SelectTrigger>
        <SelectContent align="end">
          <SelectItem value={NONE}>{t("repo.issues.filter_milestone_no_select")}</SelectItem>
          {[
            { label: t("repo.issues.new.open_milestone"), items: openMilestones },
            { label: t("repo.issues.new.closed_milestone"), items: closedMilestones },
          ].map((group) =>
            group.items.length ? (
              <SelectGroup key={group.label}>
                <p className="px-2 pt-2 pb-1 text-xs font-semibold text-(--color-muted-foreground)">{group.label}</p>
                {group.items.map((m) => (
                  <SelectItem key={m.id} value={String(m.id)}>
                    {m.name}
                  </SelectItem>
                ))}
              </SelectGroup>
            ) : null,
          )}
        </SelectContent>
      </Select>
      <Select
        value={String(search.assignee ?? NONE)}
        onValueChange={(v) => setFilter({ assignee: id(v) })}
        disabled={data.assignees.length === 0}
      >
        <SelectTrigger aria-label={t("repo.issues.filter_assignee")} className={TRIGGER_CLASS}>
          <span className="text-(--color-muted-foreground)">{t("repo.issues.filter_assignee")}:</span>
          <SelectValue />
        </SelectTrigger>
        <SelectContent align="end">
          <SelectItem value={NONE}>{t("repo.issues.filter_assginee_no_select")}</SelectItem>
          {data.assignees.map((u) => (
            <SelectItem key={u.id} value={String(u.id)}>
              <span className="inline-flex items-center gap-2">
                <img src={u.avatarURL} alt="" className="size-4 rounded-full" />
                {u.name}
              </span>
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      {/* Lists about the viewer need a signed-in viewer. */}
      {user ? (
        <Select
          value={search.type ?? "all"}
          onValueChange={(v) => setFilter({ type: v === "all" ? undefined : (v as RepoIssuesSearch["type"]) })}
        >
          <SelectTrigger aria-label={t("repo.issues.filter_type")} className={TRIGGER_CLASS}>
            <span className="text-(--color-muted-foreground)">{t("repo.issues.filter_type")}:</span>
            <SelectValue />
          </SelectTrigger>
          <SelectContent align="end">
            <SelectItem value="all">{t("repo.issues.filter_type.all_issues")}</SelectItem>
            <SelectItem value="assigned">{t("repo.issues.filter_type.assigned_to_you")}</SelectItem>
            <SelectItem value="created_by">{t("repo.issues.filter_type.created_by_you")}</SelectItem>
            <SelectItem value="mentioned">{t("repo.issues.filter_type.mentioning_you")}</SelectItem>
          </SelectContent>
        </Select>
      ) : null}
      <Select
        value={search.sort ?? "latest"}
        onValueChange={(v) => setFilter({ sort: v === "latest" ? undefined : (v as Exclude<IssueSort, "latest">) })}
      >
        <SelectTrigger aria-label={t("repo.issues.filter_sort")} className={TRIGGER_CLASS}>
          <span className="text-(--color-muted-foreground)">{t("repo.issues.filter_sort")}:</span>
          <SelectValue />
        </SelectTrigger>
        <SelectContent align="end">
          {ISSUE_SORTS.map((sort) => (
            <SelectItem key={sort} value={sort}>
              {t(`repo.issues.filter_sort.${sort}`)}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  );
}
