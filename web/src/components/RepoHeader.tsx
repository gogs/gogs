import {
  Bell,
  CircleDot,
  Code,
  FileText,
  GitFork,
  GitPullRequest,
  Link as LinkIcon,
  Menu,
  Settings,
  Star,
} from "lucide-react";
import type { ComponentType, ReactNode } from "react";
import { useState } from "react";

import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import { subUrl } from "@/lib/url";
import { cn } from "@/lib/utils";

// Mobile collapses the tab strip after this many items into a hamburger
// overflow menu. The active tab is always pulled into the inline group so the
// user can see the active indicator without opening the menu.
const MOBILE_INLINE_LIMIT = 3;

export type RepoVisibility = "public" | "private";

export type RepoTab = "code" | "issues" | "pulls" | "commits" | "wiki" | "settings";

export interface RepoHeaderRepo {
  owner: string;
  name: string;
  // Per-repo avatar URL. Gogs lets repos set their own avatar and falls back
  // to the owner's avatar, then to a default badge. Callers should pass the
  // already-resolved URL; this component does no fallback chain itself.
  avatarUrl: string;
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
            <img
              src={repo.avatarUrl}
              alt=""
              className="relative top-0.5 size-5 shrink-0 rounded border border-(--color-border) bg-(--color-surface) object-cover"
            />
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

        <RepoTabs repo={repo} activeTab={activeTab} repoLink={repoLink} />
      </div>
    </div>
  );
}

interface TabDescriptor {
  key: RepoTab;
  href: string;
  icon: ComponentType<{ className?: string; "aria-hidden"?: boolean }>;
  label: string;
  badge?: number;
}

function buildTabs(repo: RepoHeaderRepo, repoLink: string): TabDescriptor[] {
  const tabs: TabDescriptor[] = [{ key: "code", href: repoLink, icon: Code, label: "Code" }];
  if (repo.enableIssues !== false) {
    tabs.push({
      key: "issues",
      href: `${repoLink}/issues`,
      icon: CircleDot,
      label: "Issues",
      badge: repo.counts.openIssues,
    });
  }
  if (repo.allowsPulls !== false) {
    tabs.push({
      key: "pulls",
      href: `${repoLink}/pulls`,
      icon: GitPullRequest,
      label: "Pull requests",
      badge: repo.counts.openPulls,
    });
  }
  if (repo.enableWiki !== false) {
    tabs.push({ key: "wiki", href: `${repoLink}/wiki`, icon: FileText, label: "Wiki" });
  }
  if (repo.isAdmin) {
    tabs.push({ key: "settings", href: `${repoLink}/settings`, icon: Settings, label: "Settings" });
  }
  return tabs;
}

function RepoTabs({ repo, activeTab, repoLink }: { repo: RepoHeaderRepo; activeTab: RepoTab; repoLink: string }) {
  const tabs = buildTabs(repo, repoLink);

  // On mobile, only `MOBILE_INLINE_LIMIT` tabs are shown inline; the rest
  // fold into a hamburger overflow. If the active tab is past the cutoff,
  // swap it into the last inline slot so the indicator stays visible without
  // opening the menu.
  const activeIndex = tabs.findIndex((t) => t.key === activeTab);
  let mobileInline = tabs.slice(0, MOBILE_INLINE_LIMIT);
  let mobileOverflow = tabs.slice(MOBILE_INLINE_LIMIT);
  if (activeIndex >= MOBILE_INLINE_LIMIT) {
    const swappedOut = mobileInline[MOBILE_INLINE_LIMIT - 1];
    mobileInline = [...mobileInline.slice(0, MOBILE_INLINE_LIMIT - 1), tabs[activeIndex]];
    mobileOverflow = mobileOverflow.map((t) => (t.key === tabs[activeIndex].key ? swappedOut : t));
  }

  return (
    <>
      {/* Mobile: first 3 inline + hamburger overflow for the rest. */}
      <nav className="-mb-px flex items-end gap-1 sm:hidden" aria-label="Repository">
        {mobileInline.map((tab) => (
          <TabLink key={tab.key} href={tab.href} icon={tab.icon} active={activeTab === tab.key} badge={tab.badge}>
            {tab.label}
          </TabLink>
        ))}
        {mobileOverflow.length > 0 ? <OverflowMenu tabs={mobileOverflow} activeTab={activeTab} /> : null}
      </nav>

      {/* sm and up: full strip, scrolls horizontally if it ever overflows. */}
      <nav className="-mb-px hidden gap-1 overflow-x-auto sm:flex" aria-label="Repository">
        {tabs.map((tab) => (
          <TabLink key={tab.key} href={tab.href} icon={tab.icon} active={activeTab === tab.key} badge={tab.badge}>
            {tab.label}
          </TabLink>
        ))}
      </nav>
    </>
  );
}

function OverflowMenu({ tabs, activeTab }: { tabs: TabDescriptor[]; activeTab: RepoTab }) {
  const [open, setOpen] = useState(false);
  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <button
          type="button"
          aria-label="More tabs"
          className="flex items-center gap-2 border-b-2 border-transparent px-3 py-2 text-sm whitespace-nowrap text-(--color-muted-foreground) hover:border-(--color-border) hover:text-(--color-foreground)"
        >
          <Menu className="size-4" aria-hidden />
          <span>More</span>
        </button>
      </PopoverTrigger>
      <PopoverContent align="end" className="w-56 p-1">
        {tabs.map((tab) => {
          const Icon = tab.icon;
          const active = tab.key === activeTab;
          return (
            <a
              key={tab.key}
              href={tab.href}
              onClick={() => setOpen(false)}
              aria-current={active ? "page" : undefined}
              className={cn(
                "flex items-center gap-2 rounded px-2 py-1.5 text-sm",
                active
                  ? "bg-(--color-surface) font-semibold text-(--color-foreground)"
                  : "text-(--color-foreground) hover:bg-(--color-surface)",
              )}
            >
              <Icon className="size-4" aria-hidden />
              <span className="flex-1">{tab.label}</span>
              {tab.badge && tab.badge > 0 ? (
                <span className="rounded-full bg-(--color-background) px-1.5 text-xs leading-5 tabular-nums text-(--color-muted-foreground)">
                  {formatCount(tab.badge)}
                </span>
              ) : null}
            </a>
          );
        })}
      </PopoverContent>
    </Popover>
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
