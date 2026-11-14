import { Link, getRouteApi, useNavigate } from "@tanstack/react-router";
import { BookMarked, Building2, Clock, Link as LinkIcon, Mail, MapPin, Search, User } from "lucide-react";
import type { ComponentType, FormEvent, ReactNode } from "react";
import { useTranslation } from "react-i18next";

import { type PageTab, PageTabs } from "@/components/PageTabs";
import { Pagination } from "@/components/Pagination";
import { RepoListItem, type RepoListItemData } from "@/components/RepoListItem";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { webContext } from "@/lib/context";
import { usePageTitle } from "@/lib/page-title";
import { subUrl } from "@/lib/url";
import { cn } from "@/lib/utils";

import type { ExploreKind, ExploreSearch } from "./Explore.search";

export interface ExploreUser {
  name: string;
  fullName?: string;
  avatarURL: string;
  location?: string;
  email?: string;
  website?: string;
  description?: string;
  createdAt?: string;
}

interface ExploreList<T> {
  items: T[];
  page: number;
  totalPages: number;
}

export type ExplorePage =
  | ({ kind: "repos" } & ExploreList<RepoListItemData>)
  | ({ kind: "users" | "organizations" } & ExploreList<ExploreUser>);

const route = getRouteApi("/explore/$kind");

const JOINED_DATE_FMT = new Intl.DateTimeFormat(webContext.lang, { year: "numeric", month: "short", day: "2-digit" });

export function Explore() {
  const { t } = useTranslation();
  const data = route.useLoaderData();
  const search: ExploreSearch = route.useSearch();
  const navigate = useNavigate({ from: "/explore/$kind" });
  usePageTitle(t("explore"));

  const tabs: PageTab<ExploreKind>[] = [
    { key: "repos", label: t("explore.repos"), icon: BookMarked },
    { key: "users", label: t("explore.users"), icon: User },
    { key: "organizations", label: t("explore.organizations"), icon: Building2 },
  ];

  function onSearch(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const q = (new FormData(event.currentTarget).get("q") as string | null)?.trim() ?? "";
    void navigate({ search: { q: q || undefined } });
  }

  return (
    <main className="mx-auto w-full max-w-6xl flex-1 px-4 py-6 sm:px-6">
      <h1 className="mb-3 text-xl font-semibold">{t("explore")}</h1>
      <PageTabs
        label={t("explore")}
        tabs={tabs}
        active={data.kind}
        // Only the active tab fits next to the menu at 320px.
        inline={1}
        renderLink={(tab, props) => <Link to="/explore/$kind" params={{ kind: tab.key }} {...props} />}
        className="mb-4 border-b border-(--color-border)"
      />

      <form role="search" onSubmit={onSearch} className="flex gap-2">
        <div className="relative min-w-0 flex-1">
          <Search
            className="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-(--color-muted-foreground)"
            aria-hidden
          />
          {/* Remount on navigation so the field always shows the keyword of the current URL. */}
          <Input
            key={`${data.kind}:${search.q ?? ""}`}
            name="q"
            type="search"
            defaultValue={search.q}
            placeholder={`${t("explore.search")}...`}
            aria-label={t("explore.search")}
            className="h-10 pl-9"
          />
        </div>
        <Button type="submit">{t("explore.search")}</Button>
      </form>

      <ul className="mt-4 border-t border-(--color-border)">
        {data.kind === "repos"
          ? data.items.map((repo) => <RepoListItem key={`${repo.owner}/${repo.name}`} repo={repo} showOwner />)
          : data.items.map((user) => (
              <UserItem key={user.name} user={user} organization={data.kind === "organizations"} />
            ))}
      </ul>

      {data.totalPages > 1 ? (
        <div className="mt-4">
          <Pagination page={data.page} totalPages={data.totalPages} />
        </div>
      ) : null}
    </main>
  );
}

function UserItem({ user, organization }: { user: ExploreUser; organization: boolean }) {
  const { t } = useTranslation();
  const isWebsiteLink = user.website ? /^https?:\/\//i.test(user.website) : false;
  return (
    <li className="flex gap-3 border-b border-(--color-border) py-4">
      <img
        src={user.avatarURL}
        alt=""
        className={cn(
          "mt-1.25 size-10 shrink-0 border border-(--color-border) object-cover",
          organization ? "rounded-md" : "rounded-full",
        )}
      />
      <div className="min-w-0 flex-1">
        <p className="flex flex-wrap items-baseline gap-x-2">
          <a href={subUrl(`/${user.name}`)} className="font-semibold text-(--color-primary) hover:underline">
            {user.name}
          </a>
          {user.fullName ? <span className="text-sm text-(--color-muted-foreground)">{user.fullName}</span> : null}
        </p>
        {organization ? (
          user.description ? (
            <p className="mt-1 text-sm break-words text-(--color-foreground)">{user.description}</p>
          ) : (
            <p className="mt-1 text-sm text-(--color-muted-foreground) italic">{t("repo.no_desc")}</p>
          )
        ) : null}
        <p className="mt-1 flex flex-wrap gap-x-4 gap-y-1 text-xs text-(--color-muted-foreground)">
          {user.location ? (
            <Meta icon={MapPin} label={t("settings.location")}>
              {user.location}
            </Meta>
          ) : null}
          {user.email ? (
            <Meta icon={Mail} label={t("email")}>
              <a
                href={`mailto:${user.email}`}
                rel="nofollow"
                className="hover:text-(--color-foreground) hover:underline"
              >
                {user.email}
              </a>
            </Meta>
          ) : null}
          {user.website ? (
            <Meta icon={LinkIcon} label={t("website")}>
              {isWebsiteLink ? (
                <a href={user.website} rel="nofollow" className="hover:text-(--color-foreground) hover:underline">
                  {user.website}
                </a>
              ) : (
                user.website
              )}
            </Meta>
          ) : null}
          {user.createdAt ? (
            <Meta icon={Clock}>
              {t("user.join_on")}{" "}
              <time dateTime={user.createdAt}>{JOINED_DATE_FMT.format(new Date(user.createdAt))}</time>
            </Meta>
          ) : null}
        </p>
      </div>
    </li>
  );
}

function Meta({
  icon: Icon,
  label,
  children,
}: {
  icon: ComponentType<{ className?: string }>;
  label?: string;
  children: ReactNode;
}) {
  return (
    <span className="inline-flex min-w-0 items-center gap-1 break-all">
      <Icon className="size-3.5 shrink-0" aria-hidden />
      {label ? <span className="sr-only">{label}</span> : null}
      {children}
    </span>
  );
}
