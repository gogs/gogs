import { Link, useNavigate } from "@tanstack/react-router";
import { CircleCheck, CircleDot } from "lucide-react";
import type { ReactNode } from "react";
import { useTranslation } from "react-i18next";

import { ContextSwitcher, OrgNav } from "@/components/DashboardNav";
import { IssueListItem } from "@/components/IssueListItem";
import { Pagination } from "@/components/Pagination";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { usePageTitle } from "@/lib/page-title";
import type { DashboardIssuesData } from "@/lib/queries/user";
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
    <main className="mx-auto grid w-full max-w-6xl flex-1 content-start gap-6 px-4 py-6 sm:px-6 md:grid-cols-[256px_minmax(0,1fr)] lg:grid-cols-[296px_minmax(0,1fr)]">
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
              <IssueListItem
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
