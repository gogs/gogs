import { useMutation, useQueryClient } from "@tanstack/react-query";
import type { TFunction } from "i18next";
import {
  Bell,
  CircleDot,
  Code,
  FileText,
  GitFork,
  GitPullRequest,
  Globe,
  Link as LinkIcon,
  Lock,
  Menu,
  Settings,
  Star,
} from "lucide-react";
import type { ComponentType, ReactNode } from "react";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";

import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import {
  type RepoHeaderData,
  type RepoStarResult,
  type RepoWatchResult,
  repoHeaderQuery,
  starRepo,
  unstarRepo,
  unwatchRepo,
  watchRepo,
} from "@/lib/queries/repo";
import { subUrl } from "@/lib/url";
import { useUserInfo } from "@/lib/use-user-info";
import { cn } from "@/lib/utils";

// Mobile collapses the tab strip after this many items into a hamburger
// overflow menu. The active tab is always pulled into the inline group so the
// user can see the active indicator without opening the menu.
const MOBILE_INLINE_LIMIT = 3;

export type RepoTab = "code" | "issues" | "pulls" | "commits" | "wiki" | "settings";

export interface RepoHeaderProps {
  repo: RepoHeaderData;
  activeTab: RepoTab;
}

function formatCount(n: number): string {
  if (n < 1000) return n.toLocaleString();
  if (n < 10_000) return (n / 1000).toFixed(1).replace(/\.0$/, "") + "k";
  if (n < 1_000_000) return Math.round(n / 1000) + "k";
  return (n / 1_000_000).toFixed(1).replace(/\.0$/, "") + "m";
}

export function RepoHeader({ repo, activeTab }: RepoHeaderProps) {
  const { t } = useTranslation();
  const repoLink = subUrl(`/${repo.owner}/${repo.name}`);
  const user = useUserInfo();
  const queryClient = useQueryClient();
  const signedIn = user !== null;
  const signInHref = subUrl(
    `/user/sign-in?redirect_to=${encodeURIComponent(window.location.pathname + window.location.search + window.location.hash)}`,
  );

  // Merge each mutation's response into the cached `repoInfo` so the button
  // labels and counts update without a refetch. The new `viewer*` flag is
  // derived from the action that was just dispatched, since the server's
  // outcome is unambiguous (POST always watches/stars, DELETE always undoes).
  const applyWatchResult = (result: RepoWatchResult, nextViewerIsWatching: boolean) => {
    queryClient.setQueryData(repoHeaderQuery(repo.owner, repo.name).queryKey, (prev) => {
      if (!prev) return prev;
      return {
        ...prev,
        viewerIsWatching: nextViewerIsWatching,
        watchCount: result.watchCount,
      };
    });
  };
  const applyStarResult = (result: RepoStarResult, nextViewerIsStarring: boolean) => {
    queryClient.setQueryData(repoHeaderQuery(repo.owner, repo.name).queryKey, (prev) => {
      if (!prev) return prev;
      return {
        ...prev,
        viewerIsStarring: nextViewerIsStarring,
        starCount: result.starCount,
      };
    });
  };

  const watchMutation = useMutation({
    mutationFn: async () => {
      const next = !repo.viewerIsWatching;
      const result = next ? await watchRepo(repo.owner, repo.name) : await unwatchRepo(repo.owner, repo.name);
      return { result, next };
    },
    onSuccess: ({ result, next }) => applyWatchResult(result, next),
    onError: (err) => toast.error(t("status.internal_server_error"), { description: err.message }),
  });
  const starMutation = useMutation({
    mutationFn: async () => {
      const next = !repo.viewerIsStarring;
      const result = next ? await starRepo(repo.owner, repo.name) : await unstarRepo(repo.owner, repo.name);
      return { result, next };
    },
    onSuccess: ({ result, next }) => applyStarResult(result, next),
    onError: (err) => toast.error(t("status.internal_server_error"), { description: err.message }),
  });

  return (
    <div className="border-b border-(--color-border) bg-(--color-background)">
      <div className="mx-auto max-w-7xl px-4 pt-4 sm:px-6">
        <div className="flex flex-wrap items-start justify-between gap-3 pb-3">
          <h1 className="flex min-w-0 flex-wrap items-center gap-2 text-base">
            <img
              src={repo.avatarURL}
              alt=""
              className="relative size-5 shrink-0 rounded border border-(--color-border) bg-(--color-surface) object-cover"
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
                <span className="shrink-0">{t("repo.mirror_of")}</span>
                <a href={repo.mirrorOf} className="truncate hover:underline" rel="noopener noreferrer" target="_blank">
                  {repo.mirrorOf}
                </a>
              </span>
            ) : null}
          </h1>

          <div className="flex shrink-0 flex-wrap items-center gap-2">
            <SplitActionButton
              countHref={`${repoLink}/watchers`}
              onAction={signedIn ? () => watchMutation.mutate() : undefined}
              signInHref={signInHref}
              disabled={watchMutation.isPending}
              signedIn={signedIn}
              signInTooltip={t("repo.sign_in_to_watch")}
              icon={Bell}
              label={repo.viewerIsWatching ? t("repo.unwatch") : t("repo.watch")}
              count={repo.watchCount}
              ariaLabel={repo.viewerIsWatching ? t("repo.unwatch_this_repository") : t("repo.watch_this_repository")}
              countAriaLabel={t("repo.view_watchers")}
              active={repo.viewerIsWatching}
            />
            <SplitActionButton
              countHref={`${repoLink}/stars`}
              onAction={signedIn ? () => starMutation.mutate() : undefined}
              signInHref={signInHref}
              disabled={starMutation.isPending}
              signedIn={signedIn}
              signInTooltip={t("repo.sign_in_to_star")}
              icon={Star}
              label={repo.viewerIsStarring ? t("repo.starred") : t("repo.star")}
              count={repo.starCount}
              ariaLabel={repo.viewerIsStarring ? t("repo.unstar_this_repository") : t("repo.star_this_repository")}
              countAriaLabel={t("repo.view_stargazers")}
              active={repo.viewerIsStarring}
            />
            <SplitActionButton
              countHref={`${repoLink}/forks`}
              // Fork still goes through the legacy "choose where to fork to"
              // page (not yet migrated). Treat it as a navigation link, not a
              // one-click action.
              actionHref={signedIn ? subUrl(`/repo/fork/${repo.id}`) : undefined}
              signInHref={signInHref}
              signedIn={signedIn}
              signInTooltip={t("repo.sign_in_to_fork")}
              icon={GitFork}
              label={t("repo.fork")}
              count={repo.forkCount}
              ariaLabel={t("repo.fork_this_repository")}
              countAriaLabel={t("repo.view_forks")}
            />
          </div>
        </div>

        <RepoTabs repo={repo} activeTab={activeTab} repoLink={repoLink} t={t} />
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

function buildTabs(repo: RepoHeaderData, repoLink: string, t: TFunction): TabDescriptor[] {
  const tabs: TabDescriptor[] = [{ key: "code", href: repoLink, icon: Code, label: t("repo.files") }];
  if (repo.issuesEnabled !== false) {
    tabs.push({
      key: "issues",
      href: `${repoLink}/issues`,
      icon: CircleDot,
      label: t("issues"),
      badge: repo.openIssueCount,
    });
  }
  if (repo.pullRequestsEnabled !== false) {
    tabs.push({
      key: "pulls",
      href: `${repoLink}/pulls`,
      icon: GitPullRequest,
      label: t("pull_requests"),
      badge: repo.openPullRequestCount,
    });
  }
  if (repo.wikiEnabled !== false) {
    tabs.push({ key: "wiki", href: `${repoLink}/wiki`, icon: FileText, label: t("repo.wiki") });
  }
  if (repo.viewerCanAdminister) {
    tabs.push({ key: "settings", href: `${repoLink}/settings`, icon: Settings, label: t("repo.settings") });
  }
  return tabs;
}

function RepoTabs({
  repo,
  activeTab,
  repoLink,
  t,
}: {
  repo: RepoHeaderData;
  activeTab: RepoTab;
  repoLink: string;
  t: TFunction;
}) {
  const tabs = buildTabs(repo, repoLink, t);

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
      <nav className="-mb-px flex items-end gap-1 sm:hidden" aria-label={t("repository")}>
        {mobileInline.map((tab) => (
          <TabLink key={tab.key} href={tab.href} icon={tab.icon} active={activeTab === tab.key} badge={tab.badge}>
            {tab.label}
          </TabLink>
        ))}
        {mobileOverflow.length > 0 ? <OverflowMenu tabs={mobileOverflow} activeTab={activeTab} t={t} /> : null}
      </nav>

      {/* sm and up: full strip, scrolls horizontally if it ever overflows. */}
      <nav className="-mb-px hidden gap-1 overflow-x-auto sm:flex" aria-label={t("repository")}>
        {tabs.map((tab) => (
          <TabLink key={tab.key} href={tab.href} icon={tab.icon} active={activeTab === tab.key} badge={tab.badge}>
            {tab.label}
          </TabLink>
        ))}
      </nav>
    </>
  );
}

function OverflowMenu({ tabs, activeTab, t }: { tabs: TabDescriptor[]; activeTab: RepoTab; t: TFunction }) {
  const [open, setOpen] = useState(false);
  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <button
          type="button"
          aria-label={t("more_tabs")}
          className="flex items-center gap-2 border-b-2 border-transparent px-3 py-2 text-sm whitespace-nowrap text-(--color-muted-foreground) hover:border-(--color-border) hover:text-(--color-foreground)"
        >
          <Menu className="size-4" aria-hidden />
          <span>{t("more")}</span>
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

function VisibilityBadge({ visibility }: { visibility: RepoHeaderData["visibility"] }) {
  const { t } = useTranslation();
  const isPrivate = visibility === "private";
  const Icon = isPrivate ? Lock : Globe;
  const tooltip = isPrivate ? t("repo.visibility_private") : t("repo.visibility_public");
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span
          aria-label={tooltip}
          className="ml-1 grid size-5 place-items-center rounded text-(--color-muted-foreground)"
        >
          <Icon className="size-3.5" aria-hidden />
        </span>
      </TooltipTrigger>
      <TooltipContent>{tooltip}</TooltipContent>
    </Tooltip>
  );
}

interface SplitActionButtonProps {
  // URL for the count half (always renders as `<a>`).
  countHref: string;
  // When set, the action half fires this callback on click (used for
  // signed-in users with a real mutation in flight).
  onAction?: () => void;
  // Fallback href for the action half when `onAction` is not set (legacy
  // page-navigation actions like Fork). Only used when the viewer is signed
  // in; `signInHref` takes over when signed out.
  actionHref?: string;
  // Where to send signed-out viewers when they click the action half. The
  // half renders disabled-looking but stays clickable so the affordance
  // works without forcing the viewer to dig for a sign-in button.
  signInHref?: string;
  // Whether the viewer is signed in. Drives both the click target and the
  // disabled-looking styling + sign-in tooltip below.
  signedIn?: boolean;
  // Tooltip text shown when signed out. Should explain the gated action,
  // e.g. "Sign in to watch this repository".
  signInTooltip?: string;
  icon: ComponentType<{ className?: string; "aria-hidden"?: boolean; fill?: string }>;
  label: string;
  count: number;
  ariaLabel: string;
  // Accessible name for the count half (e.g. "View watchers"). The visible
  // text is just the number, so this label tells assistive tech what the
  // link goes to.
  countAriaLabel: string;
  active?: boolean;
  disabled?: boolean;
}

function SplitActionButton({
  countHref,
  onAction,
  actionHref,
  signInHref,
  signedIn = true,
  signInTooltip,
  icon: Icon,
  label,
  count,
  ariaLabel,
  countAriaLabel,
  active,
  disabled,
}: SplitActionButtonProps) {
  const actionClassName = cn(
    "flex items-center gap-1.5 px-2 hover:bg-(--color-surface)",
    active && "text-(--color-primary)",
    disabled && "cursor-not-allowed opacity-60 hover:bg-transparent",
  );
  const actionContent = (
    <>
      <Icon className="size-3.5" aria-hidden fill={active ? "currentColor" : "none"} />
      <span>{label}</span>
    </>
  );

  let action: ReactNode;
  if (!signedIn) {
    const href = signInHref ?? countHref;
    action = (
      <Tooltip>
        <TooltipTrigger asChild>
          <a href={href} aria-label={ariaLabel} className={actionClassName}>
            {actionContent}
          </a>
        </TooltipTrigger>
        {signInTooltip ? <TooltipContent>{signInTooltip}</TooltipContent> : null}
      </Tooltip>
    );
  } else if (onAction) {
    action = (
      <button
        type="button"
        onClick={onAction}
        disabled={disabled}
        aria-label={ariaLabel}
        className={cn(actionClassName, "cursor-pointer")}
      >
        {actionContent}
      </button>
    );
  } else {
    const href = actionHref ?? countHref;
    action = (
      <a href={href} aria-label={ariaLabel} className={actionClassName}>
        {actionContent}
      </a>
    );
  }

  return (
    <span className="inline-flex h-7 items-stretch overflow-hidden rounded-md border border-(--color-border) text-xs">
      {action}
      <a
        href={countHref}
        aria-label={countAriaLabel}
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
