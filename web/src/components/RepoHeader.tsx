import {
  Bell,
  BookOpen,
  Clock,
  Code,
  FileText,
  GitFork,
  GitPullRequest,
  Link as LinkIcon,
  Lock,
  Settings,
  Star,
} from "lucide-react";
import type { ComponentType, ReactNode } from "react";

import { subUrl } from "@/lib/url";
import { cn } from "@/lib/utils";

export type RepoVisibility = "public" | "private";

export type RepoTab = "code" | "issues" | "pulls" | "commits" | "wiki" | "settings";

export interface RepoHeaderRepo {
  owner: string;
  name: string;
  visibility: RepoVisibility;
  isAdmin?: boolean;
  enableIssues?: boolean;
  allowsPulls?: boolean;
  enableWiki?: boolean;
  counts: {
    watchers: number;
    stars: number;
    forks: number;
    openIssues?: number;
    openPulls?: number;
  };
  viewerWatching?: boolean;
  viewerStarred?: boolean;
  // When set, the repo is a mirror of this upstream URL. Rendered next to
  // the owner/name breadcrumb to match the existing Gogs repo page.
  mirrorOf?: string;
}

export interface RepoHeaderProps {
  repo: RepoHeaderRepo;
  activeTab: RepoTab;
}

function formatCount(n: number): string {
  if (n < 1000) return n.toLocaleString();
  if (n < 10_000) return (n / 1000).toFixed(1).replace(/\.0$/, "") + "k";
  if (n < 1_000_000) return Math.round(n / 1000) + "k";
  return (n / 1_000_000).toFixed(1).replace(/\.0$/, "") + "m";
}

export function RepoHeader({ repo, activeTab }: RepoHeaderProps) {
  const repoLink = subUrl(`/${repo.owner}/${repo.name}`);

  return (
    <div className="border-b border-(--color-border) bg-(--color-background)">
      <div className="mx-auto max-w-7xl px-4 pt-4 sm:px-6">
        <div className="flex flex-wrap items-start justify-between gap-3 pb-3">
          <h1 className="flex min-w-0 flex-wrap items-center gap-2 text-base">
            {repo.visibility === "private" ? (
              <Lock className="size-4 shrink-0 text-(--color-muted-foreground)" aria-hidden />
            ) : (
              <BookOpen className="size-4 shrink-0 text-(--color-muted-foreground)" aria-hidden />
            )}
            <a href={subUrl(`/${repo.owner}`)} className="text-(--color-primary) hover:underline">
              {repo.owner}
            </a>
            <span className="text-(--color-muted-foreground)" aria-hidden>
              /
            </span>
            <a href={repoLink} className="font-semibold text-(--color-primary) hover:underline">
              {repo.name}
            </a>
            <VisibilityBadge visibility={repo.visibility} />
            {repo.mirrorOf ? (
              <span className="inline-flex min-w-0 items-center gap-1 text-xs text-(--color-muted-foreground)">
                <LinkIcon className="size-3 shrink-0" aria-hidden />
                <span className="shrink-0">mirror of</span>
                <a href={repo.mirrorOf} className="truncate hover:underline" rel="noopener noreferrer" target="_blank">
                  {repo.mirrorOf}
                </a>
              </span>
            ) : null}
          </h1>

          <div className="flex shrink-0 flex-wrap items-center gap-2">
            <SplitActionButton
              href={`${repoLink}/watchers`}
              actionHref={`${repoLink}/action/${repo.viewerWatching ? "un" : ""}watch`}
              icon={Bell}
              label={repo.viewerWatching ? "Unwatch" : "Watch"}
              count={repo.counts.watchers}
              ariaLabel="Watch this repository"
            />
            <SplitActionButton
              href={`${repoLink}/stars`}
              actionHref={`${repoLink}/action/${repo.viewerStarred ? "un" : ""}star`}
              icon={Star}
              label={repo.viewerStarred ? "Starred" : "Star"}
              count={repo.counts.stars}
              ariaLabel="Star this repository"
              active={repo.viewerStarred}
            />
            <SplitActionButton
              href={`${repoLink}/forks`}
              actionHref={subUrl(`/repo/fork/${repo.owner}/${repo.name}`)}
              icon={GitFork}
              label="Fork"
              count={repo.counts.forks}
              ariaLabel="Fork this repository"
            />
          </div>
        </div>

        <nav className="-mb-px flex gap-1 overflow-x-auto" aria-label="Repository">
          <TabLink href={repoLink} icon={Code} active={activeTab === "code"}>
            Code
          </TabLink>
          {repo.enableIssues !== false ? (
            <TabLink
              href={`${repoLink}/issues`}
              icon={Clock}
              active={activeTab === "issues"}
              badge={repo.counts.openIssues}
            >
              Issues
            </TabLink>
          ) : null}
          {repo.allowsPulls !== false ? (
            <TabLink
              href={`${repoLink}/pulls`}
              icon={GitPullRequest}
              active={activeTab === "pulls"}
              badge={repo.counts.openPulls}
            >
              Pull requests
            </TabLink>
          ) : null}
          {repo.enableWiki !== false ? (
            <TabLink href={`${repoLink}/wiki`} icon={FileText} active={activeTab === "wiki"}>
              Wiki
            </TabLink>
          ) : null}
          {repo.isAdmin ? (
            <TabLink href={`${repoLink}/settings`} icon={Settings} active={activeTab === "settings"}>
              Settings
            </TabLink>
          ) : null}
        </nav>
      </div>
    </div>
  );
}

function VisibilityBadge({ visibility }: { visibility: RepoVisibility }) {
  const isPrivate = visibility === "private";
  return (
    <span
      className={cn(
        "ml-1 rounded-full border px-2 py-0 text-xs leading-5",
        isPrivate
          ? "border-(--color-border) bg-(--color-surface) text-(--color-muted-foreground)"
          : "border-(--color-border) text-(--color-muted-foreground)",
      )}
    >
      {isPrivate ? "Private" : "Public"}
    </span>
  );
}

interface SplitActionButtonProps {
  href: string;
  actionHref: string;
  icon: ComponentType<{ className?: string; "aria-hidden"?: boolean }>;
  label: string;
  count: number;
  ariaLabel: string;
  active?: boolean;
}

function SplitActionButton({ href, actionHref, icon: Icon, label, count, ariaLabel, active }: SplitActionButtonProps) {
  return (
    <span className="inline-flex h-7 items-stretch overflow-hidden rounded-md border border-(--color-border) text-xs">
      <a
        href={actionHref}
        aria-label={ariaLabel}
        className={cn("flex items-center gap-1.5 px-2 hover:bg-(--color-surface)", active && "text-(--color-primary)")}
      >
        <Icon className="size-3.5" aria-hidden />
        <span>{label}</span>
      </a>
      <a
        href={href}
        aria-label={`${label} count`}
        className="flex items-center border-l border-(--color-border) bg-(--color-surface)/60 px-2 tabular-nums hover:bg-(--color-surface)"
      >
        {formatCount(count)}
      </a>
    </span>
  );
}

interface TabLinkProps {
  href: string;
  icon: ComponentType<{ className?: string; "aria-hidden"?: boolean }>;
  active: boolean;
  badge?: number;
  children: ReactNode;
}

function TabLink({ href, icon: Icon, active, badge, children }: TabLinkProps) {
  return (
    <a
      href={href}
      className={cn(
        "flex items-center gap-2 border-b-2 px-3 py-2 text-sm whitespace-nowrap",
        active
          ? "border-(--color-primary) font-semibold text-(--color-foreground)"
          : "border-transparent text-(--color-muted-foreground) hover:border-(--color-border) hover:text-(--color-foreground)",
      )}
      aria-current={active ? "page" : undefined}
    >
      <Icon className="size-4" aria-hidden />
      <span>{children}</span>
      {badge && badge > 0 ? (
        <span className="rounded-full bg-(--color-surface) px-1.5 text-xs leading-5 tabular-nums text-(--color-muted-foreground)">
          {formatCount(badge)}
        </span>
      ) : null}
    </a>
  );
}
