import { Link, getRouteApi, useNavigate } from "@tanstack/react-router";
import {
  BookMarked,
  Building2,
  Clock,
  EyeOff,
  GitFork,
  Link as LinkIcon,
  Lock,
  Mail,
  MapPin,
  Search,
  Star,
  User,
} from "lucide-react";
import type { ComponentType, FormEvent, ReactNode } from "react";
import { useTranslation } from "react-i18next";

import { Pagination } from "@/components/Pagination";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { webContext } from "@/lib/context";
import { usePageTitle } from "@/lib/page-title";
import { formatAbsoluteTime, formatRelativeTime } from "@/lib/relative-time";
import { subUrl } from "@/lib/url";
import { cn } from "@/lib/utils";

import type { ExploreKind, ExploreSearch } from "./Explore.search";

export interface ExploreRepo {
  owner: string;
  name: string;
  // Sanitized by the server, safe to render as HTML.
  descriptionHTML: string;
  avatarURL?: string;
  isPrivate: boolean;
  isUnlisted: boolean;
  isFork: boolean;
  isMirror: boolean;
  starCount: number;
  forkCount: number;
  updatedAt: string;
}

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
  | ({ kind: "repos" } & ExploreList<ExploreRepo>)
  | ({ kind: "users" | "organizations" } & ExploreList<ExploreUser>);

const route = getRouteApi("/explore/$kind");

const JOINED_DATE_FMT = new Intl.DateTimeFormat(webContext.lang, { year: "numeric", month: "short", day: "2-digit" });

export function Explore() {
  const { t } = useTranslation();
  const data = route.useLoaderData();
  const search: ExploreSearch = route.useSearch();
  const navigate = useNavigate({ from: "/explore/$kind" });
  usePageTitle(t("explore"));

  const tabs: { kind: ExploreKind; label: string; icon: ComponentType<{ className?: string }> }[] = [
    { kind: "repos", label: t("explore.repos"), icon: BookMarked },
    { kind: "users", label: t("explore.users"), icon: User },
    { kind: "organizations", label: t("explore.organizations"), icon: Building2 },
  ];

  function onSearch(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const q = (new FormData(event.currentTarget).get("q") as string | null)?.trim() ?? "";
    void navigate({ search: { q: q || undefined } });
  }

  return (
    <main className="mx-auto w-full max-w-6xl flex-1 px-4 py-6 sm:px-6">
      <h1 className="mb-3 text-xl font-semibold">{t("explore")}</h1>
      <nav aria-label={t("explore")} className="mb-4 flex overflow-x-auto border-b border-(--color-border)">
        {tabs.map(({ kind, label, icon: Icon }) => {
          const active = kind === data.kind;
          return (
            <Link
              key={kind}
              to="/explore/$kind"
              params={{ kind }}
              aria-current={active ? "page" : undefined}
              className={cn(
                "-mb-px flex items-center gap-2 border-b-2 px-3 py-2 text-sm whitespace-nowrap",
                active
                  ? "border-(--color-primary) font-semibold text-(--color-foreground)"
                  : "border-transparent text-(--color-muted-foreground) hover:border-(--color-border) hover:text-(--color-foreground)",
              )}
            >
              <Icon className="size-4" aria-hidden />
              {label}
            </Link>
          );
        })}
      </nav>

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
          ? data.items.map((repo) => <RepoItem key={`${repo.owner}/${repo.name}`} repo={repo} />)
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

function RepoItem({ repo }: { repo: ExploreRepo }) {
  const { t } = useTranslation();
  const repoLink = subUrl(`/${repo.owner}/${repo.name}`);
  return (
    <li className="flex gap-3 border-b border-(--color-border) py-4">
      {repo.avatarURL ? (
        <img
          src={repo.avatarURL}
          alt=""
          className="mt-1 size-10 shrink-0 rounded-md border border-(--color-border) object-cover"
        />
      ) : (
        <span className="mt-0.5 grid size-10 shrink-0 place-items-center text-(--color-muted-foreground)">
          <BookMarked className="size-10" strokeWidth={1.5} aria-hidden />
        </span>
      )}
      <div className="min-w-0 flex-1">
        <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
          <span className="flex min-w-0 items-center gap-0.5">
            <a href={repoLink} className="min-w-0 break-all text-(--color-primary) hover:underline">
              {repo.owner} / <span className="font-semibold">{repo.name}</span>
            </a>
            <RepoMarker repo={repo} />
          </span>
          <span className="ml-auto flex items-center gap-3 text-xs tabular-nums text-(--color-muted-foreground)">
            <span className="inline-flex items-center gap-1">
              <Star className="size-3.5" aria-hidden />
              <span className="sr-only">{t("repo.stargazers")}</span>
              {repo.starCount}
            </span>
            <span className="inline-flex items-center gap-1">
              <GitFork className="size-3.5" aria-hidden />
              <span className="sr-only">{t("repo.forks")}</span>
              {repo.forkCount}
            </span>
          </span>
        </div>
        {repo.descriptionHTML ? (
          <p
            className="mt-1 text-sm break-words text-(--color-foreground)"
            dangerouslySetInnerHTML={{ __html: repo.descriptionHTML }}
          />
        ) : null}
        <p className="mt-1 text-xs text-(--color-muted-foreground)">
          {t("org.repo_updated")}{" "}
          <Tooltip>
            <TooltipTrigger asChild>
              <time dateTime={repo.updatedAt}>{formatRelativeTime(t, repo.updatedAt)}</time>
            </TooltipTrigger>
            <TooltipContent>{formatAbsoluteTime(repo.updatedAt)}</TooltipContent>
          </Tooltip>
        </p>
      </div>
    </li>
  );
}

// RepoMarker shows the first applicable of private, unlisted, fork and mirror,
// in the same precedence as the legacy list.
function RepoMarker({ repo }: { repo: ExploreRepo }) {
  const { t } = useTranslation();
  let marker: { icon: ComponentType<{ className?: string }>; label: string } | null = null;
  if (repo.isPrivate) marker = { icon: Lock, label: t("repo.visibility_private") };
  else if (repo.isUnlisted) marker = { icon: EyeOff, label: t("repo.unlisted") };
  else if (repo.isFork) marker = { icon: GitFork, label: t("repo.fork") };
  else if (repo.isMirror) marker = { icon: LinkIcon, label: t("mirror") };
  if (!marker) return null;

  const Icon = marker.icon;
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span
          role="img"
          aria-label={marker.label}
          className="grid size-6 place-items-center rounded text-(--color-muted-foreground)"
        >
          <Icon className="size-3.5" />
        </span>
      </TooltipTrigger>
      <TooltipContent>{marker.label}</TooltipContent>
    </Tooltip>
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
          "mt-1 size-10 shrink-0 border border-(--color-border) object-cover",
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
