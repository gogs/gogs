import { Link, useNavigate } from "@tanstack/react-router";
import { CircleCheck, CircleDot, GitPullRequest, GitPullRequestClosed, MessageSquare } from "lucide-react";
import type { ReactNode } from "react";
import { useTranslation } from "react-i18next";

import { ContextSwitcher, OrgNav } from "@/components/DashboardNav";
import { Pagination } from "@/components/Pagination";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { usePageTitle } from "@/lib/page-title";
import type { DashboardIssue, DashboardIssuesData } from "@/lib/queries/user";
import { formatAbsoluteTime, formatRelativeTime } from "@/lib/relative-time";
import { subUrl } from "@/lib/url";
import { cn } from "@/lib/utils";
import { type DashboardIssuesSearch, ISSUE_SORTS, type IssueSort } from "@/pages/user/DashboardIssues.search";

// DashboardIssues lists the issues or pull requests in the repositories of the
// signed-in user, or of one of their organizations.
export function DashboardIssues({
  data,
  search,
  kind,
}: {
  data: DashboardIssuesData;
  search: DashboardIssuesSearch;
  kind: "issues" | "pulls";
}) {
  const { t } = useTranslation();
  const title = kind === "pulls" ? t("pull_requests") : t("issues");
  usePageTitle(title);
  const isOrg = data.kind === "organization";

  return (
    <main className="mx-auto grid w-full max-w-6xl flex-1 gap-6 px-4 py-6 sm:px-6 md:grid-cols-[256px_minmax(0,1fr)] lg:grid-cols-[296px_minmax(0,1fr)]">
      <h1 className="sr-only">{title}</h1>
      <aside className="min-w-0">
        <ContextSwitcher data={data} page={kind} />
        <Filters data={data} search={search} />
      </aside>
      <section className="min-w-0" aria-label={title}>
        {isOrg ? <OrgNav name={data.name} page={kind} /> : null}
        <Toolbar data={data} search={search} className={isOrg ? "mt-4" : undefined} />
        {data.items.length > 0 ? (
          <ul>
            {data.items.map((issue) => (
              <IssueRow
                key={`${issue.repo}#${issue.index}`}
                issue={issue}
                kind={kind}
                showRepo={search.repo === undefined}
              />
            ))}
          </ul>
        ) : (
          <p className="py-8 text-center text-sm text-(--color-muted-foreground)">{t("repo.pulls.no_results")}</p>
        )}
        {data.totalPages > 1 ? (
          <div className="mt-4">
            <Pagination page={data.page} totalPages={data.totalPages} />
          </div>
        ) : null}
      </section>
    </main>
  );
}

const FILTER_CLASS = "flex items-center gap-2 rounded-md px-2 py-1.5 text-sm hover:bg-(--color-surface)";
const ACTIVE_FILTER_CLASS = "bg-(--color-surface) font-semibold text-(--color-foreground)";

// Filters picks whose issues to list and, for the issues in your repositories,
// the repository.
function Filters({ data, search }: { data: DashboardIssuesData; search: DashboardIssuesSearch }) {
  const { t } = useTranslation();
  const filters = [
    { type: undefined, label: t("home.issues.in_your_repos"), count: data.counts.yourRepos },
    ...(data.kind === "organization"
      ? []
      : [
          {
            type: "assigned" as const,
            label: t("repo.issues.filter_type.assigned_to_you"),
            count: data.counts.assigned,
          },
          {
            type: "created_by" as const,
            label: t("repo.issues.filter_type.created_by_you"),
            count: data.counts.created,
          },
        ]),
  ];
  return (
    <div className="mt-4">
      <ul>
        {filters.map(({ type, label, count }) => {
          const active = type === search.type;
          return (
            <li key={type ?? "your_repositories"}>
              <Link
                to="."
                search={(prev: DashboardIssuesSearch) => ({ ...prev, type, page: undefined })}
                aria-current={active ? "true" : undefined}
                className={cn(FILTER_CLASS, active && ACTIVE_FILTER_CLASS)}
              >
                <span className="min-w-0 flex-1 truncate">{label}</span>
                <Badge>{count}</Badge>
              </Link>
            </li>
          );
        })}
      </ul>
      {data.repos.length > 0 ? (
        <ul className="mt-2 border-t border-(--color-border) pt-2">
          {data.repos.map((repo) => {
            const active = repo.id === search.repo;
            return (
              <li key={repo.id}>
                <Link
                  to="."
                  // Choosing the selected repository again shows every repository.
                  search={(prev: DashboardIssuesSearch) => ({
                    ...prev,
                    repo: active ? undefined : repo.id,
                    page: undefined,
                  })}
                  aria-current={active ? "true" : undefined}
                  className={cn(FILTER_CLASS, active && ACTIVE_FILTER_CLASS)}
                >
                  <span className="min-w-0 flex-1 truncate">{repo.fullName}</span>
                  <Badge>{repo.count}</Badge>
                </Link>
              </li>
            );
          })}
        </ul>
      ) : null}
    </div>
  );
}

function Badge({ children }: { children: ReactNode }) {
  return (
    <span className="shrink-0 rounded-full bg-(--color-secondary) px-2 text-xs/5 font-medium tabular-nums text-(--color-secondary-foreground)">
      {children}
    </span>
  );
}

function Toolbar({
  data,
  search,
  className,
}: {
  data: DashboardIssuesData;
  search: DashboardIssuesSearch;
  className?: string;
}) {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const states = [
    { state: undefined, label: t("repo.issues.open_tab"), count: data.counts.open, icon: CircleDot },
    { state: "closed" as const, label: t("repo.issues.close_tab"), count: data.counts.closed, icon: CircleCheck },
  ];
  return (
    <div
      className={cn(
        "flex flex-wrap items-center justify-between gap-2 border-b border-(--color-border) pb-2",
        className,
      )}
    >
      <ul className="flex gap-1">
        {states.map(({ state, label, count, icon: Icon }) => {
          const active = state === search.state;
          return (
            <li key={state ?? "open"}>
              <Link
                to="."
                search={(prev: DashboardIssuesSearch) => ({ ...prev, state, page: undefined })}
                aria-current={active ? "true" : undefined}
                className={cn(
                  "inline-flex items-center gap-1.5 rounded-md px-2 py-1.5 text-sm tabular-nums hover:bg-(--color-surface)",
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
      <Select
        value={search.sort ?? "latest"}
        onValueChange={(value) =>
          void navigate({
            to: ".",
            search: (prev: DashboardIssuesSearch) => ({
              ...prev,
              sort: value === "latest" ? undefined : (value as DashboardIssuesSearch["sort"]),
              page: undefined,
            }),
          })
        }
      >
        <SelectTrigger aria-label={t("repo.issues.filter_sort")} className="h-9 w-auto gap-2">
          <span className="text-(--color-muted-foreground)">{t("repo.issues.filter_sort")}:</span>
          <SelectValue />
        </SelectTrigger>
        <SelectContent align="end">
          {ISSUE_SORTS.map((sort: IssueSort) => (
            <SelectItem key={sort} value={sort}>
              {t(`repo.issues.filter_sort.${sort}`)}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  );
}

function IssueRow({ issue, kind, showRepo }: { issue: DashboardIssue; kind: "issues" | "pulls"; showRepo: boolean }) {
  const { t } = useTranslation();
  const OpenIcon = kind === "pulls" ? GitPullRequest : CircleDot;
  const ClosedIcon = kind === "pulls" ? GitPullRequestClosed : CircleCheck;
  const Icon = issue.isClosed ? ClosedIcon : OpenIcon;
  const commentLabel = t("repo.issues.num_comments").replace(/%(?:\[1\])?d/, String(issue.commentCount));

  return (
    <li className="flex gap-3 border-b border-(--color-border) py-3">
      <Icon
        role="img"
        aria-label={issue.isClosed ? t("repo.issues.closed_title") : t("repo.issues.open_title")}
        className={cn(
          "mt-0.5 size-4 shrink-0",
          issue.isClosed ? "text-(--color-muted-foreground)" : "text-(--color-success)",
        )}
      />
      <div className="min-w-0 flex-1">
        <p className="text-sm break-words">
          {showRepo ? (
            <a
              href={subUrl(`/${issue.repo}`)}
              className="mr-1 text-(--color-muted-foreground) hover:text-(--color-primary) hover:underline"
            >
              {issue.repo}
            </a>
          ) : null}
          <a
            href={subUrl(`/${issue.repo}/${kind}/${issue.index}`)}
            className="font-semibold text-(--color-foreground) hover:text-(--color-primary)"
          >
            {issue.title}
          </a>
          {issue.labels.map((label) => (
            <span
              key={label.name}
              className="ml-1.5 inline-block rounded-full px-2 text-xs/5 font-medium align-text-bottom"
              style={{ backgroundColor: label.color, color: label.textColor }}
            >
              {label.name}
            </span>
          ))}
        </p>
        <p className="mt-1 text-xs text-(--color-muted-foreground)">
          #{issue.index} <OpenedBy issue={issue} />
        </p>
      </div>
      <div className="flex shrink-0 items-start gap-3">
        {issue.assignee ? (
          <Tooltip>
            <TooltipTrigger asChild>
              <Link to="/$username" params={{ username: issue.assignee.name }} aria-label={issue.assignee.name}>
                <img
                  src={issue.assignee.avatarURL}
                  alt=""
                  className="size-5 rounded-full border border-(--color-border) object-cover"
                />
              </Link>
            </TooltipTrigger>
            <TooltipContent>{issue.assignee.name}</TooltipContent>
          </Tooltip>
        ) : null}
        {issue.commentCount > 0 ? (
          <span className="inline-flex items-center gap-1 text-xs tabular-nums text-(--color-muted-foreground)">
            <MessageSquare className="size-3.5" aria-hidden />
            <span aria-hidden>{issue.commentCount}</span>
            <span className="sr-only">{commentLabel}</span>
          </span>
        ) : null}
      </div>
    </li>
  );
}

// OpenedBy renders the localized "opened <time> by <poster>" sentence. The
// translations place the time as `%[1]s` and the poster as a link around
// `%[3]s`.
function OpenedBy({ issue }: { issue: DashboardIssue }) {
  const { t } = useTranslation();
  const parts = t("repo.issues.opened_by").split(/(<a [^>]*>\s*%\[3\]s<\/a>|%\[3\]s|%\[1\]s?)/);
  return (
    <>
      {parts.map((part, i) => {
        if (part.startsWith("<a ") || part === "%[3]s") {
          return (
            <Link
              key={i}
              to="/$username"
              params={{ username: issue.poster.name }}
              className="hover:text-(--color-primary) hover:underline"
            >
              {issue.poster.name}
            </Link>
          );
        }
        if (part.startsWith("%[1]")) {
          return (
            <Tooltip key={i}>
              <TooltipTrigger asChild>
                <time dateTime={issue.createdAt}>{formatRelativeTime(t, issue.createdAt)}</time>
              </TooltipTrigger>
              <TooltipContent>{formatAbsoluteTime(issue.createdAt)}</TooltipContent>
            </Tooltip>
          );
        }
        return part;
      })}
    </>
  );
}
