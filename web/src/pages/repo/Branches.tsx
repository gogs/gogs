import { useSuspenseQuery } from "@tanstack/react-query";
import { Link, useNavigate } from "@tanstack/react-router";
import {
  ChevronLeft,
  ChevronRight,
  Clock,
  GitBranch,
  GitPullRequest,
  List,
  Search,
  ShieldCheck,
  User,
  Zap,
} from "lucide-react";
import { type ReactNode, useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";

import { RelativeTime } from "@/components/RelativeTime";
import { RepoHeader } from "@/components/RepoHeader";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { usePageTitle } from "@/lib/page-title";
import {
  type RepoBranch,
  type RepoBranchSection,
  type RepoBranchesData,
  refPath,
  repoHeaderQuery,
} from "@/lib/queries/repo";
import { subUrl } from "@/lib/url";
import { useUserInfo } from "@/lib/use-user-info";
import { cn } from "@/lib/utils";

// How long typing in the search field pauses before the URL follows.
const SEARCH_DEBOUNCE_MS = 300;

// Searching from the overview moves to all branches, which mounts the page
// again, so the new page takes over the focus of the search field.
let focusSearchOnMount = false;

// RepoBranches lists the branches of a repository in the view the server
// picked them for: the overview groups the default branch, the viewer's
// branches, and the active and stale ones, and the other views are paged
// lists. The `query` search param narrows any view by branch name.
export function RepoBranches({
  owner,
  name,
  query,
  data,
}: {
  owner: string;
  name: string;
  query?: string;
  data: RepoBranchesData;
}) {
  const { t } = useTranslation();
  const { data: repo } = useSuspenseQuery(repoHeaderQuery(owner, name));
  usePageTitle(`${t("repo.git_branches")} - ${repo.owner}/${repo.name}`);
  const signedIn = useUserInfo() !== null;
  const view = data.view;
  const params = { owner: repo.owner, repo: repo.name };
  const tabs = [
    { key: "overview", to: "/$owner/$repo/branches", label: t("repo.branches.overview"), icon: GitBranch },
    ...(signedIn
      ? [{ key: "yours", to: "/$owner/$repo/branches/yours", label: t("repo.branches.yours"), icon: User } as const]
      : []),
    { key: "active", to: "/$owner/$repo/branches/active", label: t("repo.branches.active"), icon: Zap },
    { key: "stale", to: "/$owner/$repo/branches/stale", label: t("repo.branches.stale"), icon: Clock },
    { key: "all", to: "/$owner/$repo/branches/all", label: t("repo.branches.all_tab"), icon: List },
  ] as const;
  const sectionTitles: Record<RepoBranchSection["kind"], string> = {
    default: t("repo.settings.default_branch"),
    yours: t("repo.branches.your_branches"),
    active: t("repo.branches.active_branches"),
    stale: t("repo.branches.stale_branches"),
  };

  const row = (branch: RepoBranch) => (
    <BranchRow key={branch.name} owner={owner} name={name} branch={branch} data={data} />
  );
  const list = (branches: RepoBranch[]) => (
    <ul className="divide-y divide-(--color-border) rounded-md border border-(--color-border)">{branches.map(row)}</ul>
  );
  const tabsRef = useRef<HTMLElement>(null);
  useEffect(() => {
    tabsRef.current?.querySelector('[aria-current="page"]')?.scrollIntoView({ block: "nearest", inline: "nearest" });
  }, [view]);
  const isEmpty = view === "overview" ? data.sections.length === 0 : data.branches.length === 0;

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <RepoHeader repo={repo} activeTab="code" />
      <main className="mx-auto w-full max-w-7xl flex-1 px-4 py-6 sm:px-6">
        <h1 className="sr-only">{t("repo.git_branches")}</h1>
        <div className="mb-6 flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
          {/* The tabs scroll sideways on their own when they do not fit. */}
          <div className="-mx-4 overflow-x-auto px-4 sm:mx-0 sm:px-0">
            <nav
              ref={tabsRef}
              aria-label={t("repo.git_branches")}
              className="inline-flex rounded-md border border-(--color-input)"
            >
              {tabs.map(({ key, to, label, icon: Icon }) => (
                <Link
                  key={key}
                  to={to}
                  params={params}
                  search={key === "overview" || !query ? {} : { query }}
                  activeOptions={{ exact: true, includeSearch: false }}
                  aria-current={key === view ? "page" : undefined}
                  className={cn(
                    "inline-flex shrink-0 items-center gap-1.5 px-3 py-1.5 text-sm whitespace-nowrap first:rounded-l-md last:rounded-r-md not-first:border-l not-first:border-(--color-input) hover:bg-(--color-surface)",
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
          </div>
          <BranchSearch owner={repo.owner} name={repo.name} view={view} query={query} />
        </div>
        {isEmpty ? (
          <p className="rounded-md border border-(--color-border) px-4 py-8 text-center text-sm text-(--color-muted-foreground)">
            {query ? t("repo.branches.no_match").replace("%s", query) : t("repo.branches.none")}
          </p>
        ) : view === "overview" ? (
          <div className="flex flex-col gap-8">
            {data.sections.map(({ kind, branches, hasMore }) => (
              <section key={kind}>
                <h2 className="mb-2 text-base font-semibold">{sectionTitles[kind]}</h2>
                {list(branches)}
                {hasMore && kind !== "default" ? (
                  <Link
                    to={`/$owner/$repo/branches/${kind}`}
                    params={params}
                    search={query ? { query } : {}}
                    aria-label={`${t("repo.branches.view_more")}: ${sectionTitles[kind]}`}
                    className="mt-2 inline-block text-sm text-(--color-primary) hover:underline"
                  >
                    {t("repo.branches.view_more")}
                  </Link>
                ) : null}
              </section>
            ))}
          </div>
        ) : (
          <div className="flex flex-col gap-6">
            {list(data.branches)}
            <BranchPager page={data.page} hasNext={data.hasNext} />
          </div>
        )}
      </main>
    </div>
  );
}

// BranchSearch narrows the branches by name. The URL follows the field once
// typing pauses, and the field follows the URL, e.g., after going back.
function BranchSearch({
  owner,
  name,
  view,
  query,
}: {
  owner: string;
  name: string;
  view: RepoBranchesData["view"];
  query?: string;
}) {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const inputRef = useRef<HTMLInputElement>(null);
  const [value, setValue] = useState(query ?? "");
  // The keyword the URL was last asked to show. A URL that shows another one
  // was changed elsewhere, so the field takes it over.
  const [requested, setRequested] = useState(query ?? "");
  if ((query ?? "") !== requested) {
    setRequested(query ?? "");
    setValue(query ?? "");
  }

  useEffect(() => {
    if (focusSearchOnMount) {
      focusSearchOnMount = false;
      const input = inputRef.current;
      input?.focus();
      input?.setSelectionRange(input.value.length, input.value.length);
    }
  }, []);

  useEffect(() => {
    const keyword = value.trim();
    if (keyword === requested) return;
    const timer = window.setTimeout(() => {
      setRequested(keyword);
      // Refining a search replaces it in the history, so going back leaves
      // the search instead of stepping through every keystroke.
      const replace = Boolean(query);
      const search = { query: keyword || undefined };
      if (view === "overview") {
        focusSearchOnMount = true;
        void navigate({ to: "/$owner/$repo/branches/all", params: { owner, repo: name }, search, replace });
      } else {
        void navigate({ to: ".", search, replace });
      }
    }, SEARCH_DEBOUNCE_MS);
    return () => window.clearTimeout(timer);
  }, [value, requested, query, view, owner, name, navigate]);

  return (
    <div role="search" className="relative sm:w-72">
      <Search
        className="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-(--color-muted-foreground)"
        aria-hidden
      />
      <Input
        ref={inputRef}
        type="search"
        value={value}
        onChange={(e) => setValue(e.target.value)}
        placeholder={t("repo.branches.search")}
        aria-label={t("repo.branches.search_label")}
        className="h-9 pl-9"
      />
    </div>
  );
}

// BranchPager links the previous and the next page of a paged view, whose
// total the server does not count.
function BranchPager({ page, hasNext }: { page: number; hasNext: boolean }) {
  const { t } = useTranslation();
  if (page <= 1 && !hasNext) return null;
  const itemClassName = "inline-flex h-9 items-center gap-1 rounded-md px-2.5 text-sm";
  const pageLink = (to: number, children: ReactNode) => (
    <Link
      to="."
      search={(prev: Record<string, unknown>) => ({ ...prev, page: to > 1 ? to : undefined })}
      className={cn(itemClassName, "text-(--color-foreground) hover:bg-(--color-surface)")}
    >
      {children}
    </Link>
  );
  const disabled = (children: ReactNode) => (
    <span aria-disabled="true" className={cn(itemClassName, "text-(--color-muted-foreground)")}>
      {children}
    </span>
  );
  const previous = (
    <>
      <ChevronLeft className="size-4" aria-hidden />
      {t("repo.issues.previous")}
    </>
  );
  const next = (
    <>
      {t("repo.issues.next")}
      <ChevronRight className="size-4" aria-hidden />
    </>
  );
  return (
    <nav aria-label={t("page")} className="flex items-center justify-center gap-1">
      {page > 1 ? pageLink(page - 1, previous) : disabled(previous)}
      {hasNext ? pageLink(page + 1, next) : disabled(next)}
    </nav>
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
          return <RelativeTime key={i} time={branch.committedAt} />;
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
