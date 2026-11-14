import { useSuspenseInfiniteQuery, useSuspenseQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { BookMarked, GitFork, Link as LinkIcon, Lock, Plus, RefreshCw, Star } from "lucide-react";
import { type KeyboardEvent, useId, useRef, useState } from "react";
import { useTranslation } from "react-i18next";

import { ActivityFeed } from "@/components/ActivityFeed";
import { ContextSwitcher, OrgNav } from "@/components/DashboardNav";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { usePageTitle } from "@/lib/page-title";
import { type DashboardData, type DashboardRepo, dashboardActivityQuery, dashboardQuery } from "@/lib/queries/user";
import { subUrl } from "@/lib/url";
import { useUserInfo } from "@/lib/use-user-info";
import { cn } from "@/lib/utils";

// Dashboard is the home page of signed-in users, for themselves or for one of
// their organizations.
export function Dashboard({ org }: { org?: string }) {
  const { t } = useTranslation();
  const { data } = useSuspenseQuery(dashboardQuery(org));
  usePageTitle(`${data.name} - ${t("dashboard")}`);

  return (
    <main className="mx-auto grid w-full max-w-6xl flex-1 content-start gap-6 px-4 py-6 sm:px-6 md:grid-cols-[256px_minmax(0,1fr)] lg:grid-cols-[296px_minmax(0,1fr)]">
      <h1 className="sr-only">{t("dashboard")}</h1>
      <aside className="min-w-0">
        <ContextSwitcher data={data} page="dashboard" />
        <DashboardLists data={data} />
      </aside>
      <section className="min-w-0" aria-label={t("activities")}>
        {data.kind === "organization" ? (
          <OrgNav name={data.name} page="dashboard" />
        ) : (
          <h2 className="border-b border-(--color-border) pb-2 text-sm font-semibold">{t("activities")}</h2>
        )}
        <DashboardActivity org={org} />
      </section>
    </main>
  );
}

function DashboardActivity({ org }: { org?: string }) {
  const { t } = useTranslation();
  const { data, hasNextPage, isFetchingNextPage, fetchNextPage } = useSuspenseInfiniteQuery(
    dashboardActivityQuery(org),
  );
  const items = data.pages.flatMap((page) => page.items);
  if (items.length === 0) {
    return <p className="py-8 text-center text-sm text-(--color-muted-foreground)">{t("settings.no_activity")}</p>;
  }
  return (
    <ActivityFeed
      items={items}
      hasMore={hasNextPage}
      loadingMore={isFetchingNextPage}
      onLoadMore={() => void fetchNextPage()}
    />
  );
}

type ListKey = "repos" | "orgs" | "mirrors";

// DashboardLists shows the repositories, organizations and mirrors of the
// dashboard owner in tabs, one list at a time.
function DashboardLists({ data }: { data: DashboardData }) {
  const { t } = useTranslation();
  const user = useUserInfo();
  const id = useId();
  const [active, setActive] = useState<ListKey>("repos");
  const tabRefs = useRef<Partial<Record<ListKey, HTMLButtonElement | null>>>({});
  const isOrg = data.kind === "organization";
  const tabs: { key: ListKey; label: string }[] = [
    { key: "repos", label: t("repository") },
    ...(isOrg ? [] : [{ key: "orgs" as const, label: t("organization") }]),
    { key: "mirrors", label: t("mirror") },
  ];
  const orgQuery = isOrg ? `org=${data.id}` : "";

  // Arrow keys, Home and End move between tabs, as in the WAI-ARIA tabs pattern.
  const onKeyDown = (e: KeyboardEvent) => {
    const index = tabs.findIndex((tab) => tab.key === active);
    const next = {
      ArrowLeft: (index - 1 + tabs.length) % tabs.length,
      ArrowRight: (index + 1) % tabs.length,
      Home: 0,
      End: tabs.length - 1,
    }[e.key];
    if (next === undefined) return;
    e.preventDefault();
    setActive(tabs[next].key);
    tabRefs.current[tabs[next].key]?.focus();
  };

  return (
    <div className="mt-4">
      <div role="tablist" className="flex border-b border-(--color-border)" onKeyDown={onKeyDown}>
        {tabs.map(({ key, label }) => {
          const selected = key === active;
          return (
            <button
              key={key}
              ref={(el) => {
                tabRefs.current[key] = el;
              }}
              type="button"
              role="tab"
              id={`${id}-${key}-tab`}
              aria-selected={selected}
              aria-controls={`${id}-${key}`}
              tabIndex={selected ? 0 : -1}
              onClick={() => setActive(key)}
              className={cn(
                "-mb-px flex-1 cursor-pointer border-b-2 px-2 py-2 text-sm",
                selected
                  ? "border-(--color-primary) font-semibold text-(--color-foreground)"
                  : "border-transparent text-(--color-muted-foreground) hover:border-(--color-border) hover:text-(--color-foreground)",
              )}
            >
              {label}
            </button>
          );
        })}
      </div>

      <div role="tabpanel" id={`${id}-repos`} aria-labelledby={`${id}-repos-tab`} hidden={active !== "repos"}>
        <ListHeader
          title={t("home.my_repos")}
          count={data.repoCount}
          addLabel={t("new_repo")}
          addHref={`/repo/create${orgQuery ? `?${orgQuery}` : ""}`}
        />
        <ul>
          {data.repos.map((repo) => (
            <RepoRow key={repo.name} repo={repo} />
          ))}
          {data.hasMoreRepos ? (
            <li>
              <Link
                to="/$username"
                params={{ username: data.name }}
                className="block rounded-md px-2 py-1.5 text-sm text-(--color-primary) hover:bg-(--color-surface)"
              >
                {t("home.show_more_repos")}
              </Link>
            </li>
          ) : null}
        </ul>
        {data.collaborativeRepos ? (
          <>
            <ListHeader title={t("home.collaborative_repos")} />
            <ul>
              {data.collaborativeRepos.map((repo) => (
                <RepoRow key={`${repo.owner}/${repo.name}`} repo={repo} showOwner />
              ))}
            </ul>
          </>
        ) : null}
      </div>

      {isOrg ? null : (
        <div role="tabpanel" id={`${id}-orgs`} aria-labelledby={`${id}-orgs-tab`} hidden={active !== "orgs"}>
          <ListHeader
            title={t("home.my_orgs")}
            count={data.organizations.length}
            addLabel={user?.canCreateOrganization ? t("new_org") : undefined}
            addHref="/org/create"
          />
          <ul>
            {data.organizations.map((org) => (
              <li key={org.name}>
                <Link to="/$username" params={{ username: org.name }} className={ROW_CLASS}>
                  <img src={org.avatarURL} alt="" className="size-5 shrink-0 rounded-sm object-cover" />
                  <span className="min-w-0 flex-1 truncate font-semibold">{org.name}</span>
                  <Count value={org.repoCount} icon={BookMarked} label={t("org.lower_repositories")} />
                </Link>
              </li>
            ))}
          </ul>
        </div>
      )}

      <div role="tabpanel" id={`${id}-mirrors`} aria-labelledby={`${id}-mirrors-tab`} hidden={active !== "mirrors"}>
        <ListHeader
          title={t("home.my_mirrors")}
          count={data.mirrors.length}
          addLabel={t("new_mirror")}
          addHref={`/repo/migrate?mirror=1${orgQuery ? `&${orgQuery}` : ""}`}
        />
        <ul>
          {data.mirrors.map((mirror) => (
            <li key={mirror.name}>
              <a href={subUrl(`/${data.name}/${mirror.name}`)} className={ROW_CLASS}>
                <RepoIcon repo={{ ...mirror, isFork: false, isMirror: true }} />
                <span className="min-w-0 flex-1 truncate font-semibold">{mirror.name}</span>
                <Count value={`${mirror.intervalHours}H`} icon={RefreshCw} />
              </a>
            </li>
          ))}
        </ul>
      </div>
    </div>
  );
}

const ROW_CLASS = "flex items-center gap-2 rounded-md px-2 py-1.5 text-sm hover:bg-(--color-surface)";

function ListHeader({
  title,
  count,
  addLabel,
  addHref,
}: {
  title: string;
  count?: number;
  addLabel?: string;
  addHref?: string;
}) {
  return (
    <div className="mt-3 mb-1 flex min-h-8 items-center gap-2 px-2">
      <h2 className="text-sm font-semibold">{title}</h2>
      {count !== undefined ? (
        <span className="rounded-full bg-(--color-secondary) px-2 text-xs/5 font-medium tabular-nums text-(--color-secondary-foreground)">
          {count}
        </span>
      ) : null}
      {addLabel && addHref ? (
        <Tooltip>
          <TooltipTrigger asChild>
            <a
              href={subUrl(addHref)}
              aria-label={addLabel}
              className="ml-auto grid size-8 place-items-center rounded-md text-(--color-muted-foreground) hover:bg-(--color-surface) hover:text-(--color-foreground)"
            >
              <Plus className="size-4" aria-hidden />
            </a>
          </TooltipTrigger>
          <TooltipContent>{addLabel}</TooltipContent>
        </Tooltip>
      ) : null}
    </div>
  );
}

function RepoRow({ repo, showOwner = false }: { repo: DashboardRepo; showOwner?: boolean }) {
  const { t } = useTranslation();
  return (
    <li>
      <a href={subUrl(`/${repo.owner}/${repo.name}`)} className={ROW_CLASS}>
        <RepoIcon repo={repo} />
        <span className="min-w-0 flex-1 truncate">
          {showOwner ? <span className="text-(--color-muted-foreground)">{repo.owner} / </span> : null}
          <span className="font-semibold">{repo.name}</span>
        </span>
        <Count value={repo.starCount} icon={Star} label={t("repo.stargazers")} />
      </a>
    </li>
  );
}

// RepoIcon marks private repositories, forks and mirrors, in that precedence.
function RepoIcon({ repo }: { repo: Pick<DashboardRepo, "isPrivate" | "isFork" | "isMirror"> }) {
  const { t } = useTranslation();
  const className = "size-4 shrink-0 text-(--color-muted-foreground)";
  if (repo.isPrivate) return <Lock className={className} role="img" aria-label={t("repo.visibility_private")} />;
  if (repo.isFork) return <GitFork className={className} role="img" aria-label={t("repo.fork")} />;
  if (repo.isMirror) return <LinkIcon className={className} role="img" aria-label={t("mirror")} />;
  return <BookMarked className={className} aria-hidden />;
}

function Count({ value, icon: Icon, label }: { value: number | string; icon: typeof Star; label?: string }) {
  return (
    <span className="inline-flex shrink-0 items-center gap-1 text-xs tabular-nums text-(--color-muted-foreground)">
      {value}
      <Icon className="size-3.5" aria-hidden />
      {label ? <span className="sr-only">{label}</span> : null}
    </span>
  );
}
