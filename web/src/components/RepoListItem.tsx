import { BookMarked, EyeOff, GitFork, Link as LinkIcon, Lock, Star } from "lucide-react";
import type { ComponentType } from "react";
import { useTranslation } from "react-i18next";

import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { formatAbsoluteTime, formatRelativeTime } from "@/lib/relative-time";
import { subUrl } from "@/lib/url";

export interface RepoListItemData {
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

// RepoListItem is one row of a repository list. Lists of a single owner's
// repositories leave out the owner name.
export function RepoListItem({ repo, showOwner = false }: { repo: RepoListItemData; showOwner?: boolean }) {
  const { t } = useTranslation();
  const repoLink = subUrl(`/${repo.owner}/${repo.name}`);
  return (
    <li className="flex gap-3 border-b border-(--color-border) py-4">
      {repo.avatarURL ? (
        <img
          src={repo.avatarURL}
          alt=""
          className="mt-1.25 size-10 shrink-0 rounded-md border border-(--color-border) object-cover"
        />
      ) : (
        <span className="mt-0.75 grid size-10 shrink-0 place-items-center text-(--color-muted-foreground)">
          <BookMarked className="size-10" strokeWidth={1.5} aria-hidden />
        </span>
      )}
      <div className="min-w-0 flex-1">
        <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
          <span className="flex min-w-0 items-center gap-0.5">
            <a href={repoLink} className="min-w-0 wrap-anywhere text-(--color-primary) hover:underline">
              {showOwner ? `${repo.owner} / ` : null}
              <span className="font-semibold">{repo.name}</span>
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
function RepoMarker({ repo }: { repo: RepoListItemData }) {
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
