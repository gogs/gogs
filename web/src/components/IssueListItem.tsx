import { Link } from "@tanstack/react-router";
import { CircleCheck, CircleDot, GitPullRequest, GitPullRequestClosed, MessageSquare } from "lucide-react";
import { Fragment, type ReactNode } from "react";
import { useTranslation } from "react-i18next";

import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import type { DashboardIssue } from "@/lib/queries/user";
import { formatAbsoluteTime, formatRelativeTime } from "@/lib/relative-time";
import { subUrl } from "@/lib/url";
import { cn } from "@/lib/utils";

// IssueListItem is one row of a list of issues or pull requests.
export function IssueListItem({
  issue,
  kind,
  showRepo,
  unread = false,
  renderLabel,
  meta,
}: {
  issue: DashboardIssue;
  kind: "issues" | "pulls";
  showRepo: boolean;
  // unread marks an issue the signed-in viewer has not read since it changed.
  unread?: boolean;
  // renderLabel wraps the chip of a label, e.g., in a link that filters by it.
  renderLabel?: (label: DashboardIssue["labels"][number], chip: ReactNode) => ReactNode;
  // meta follows the line that says who opened the issue and when.
  meta?: ReactNode;
}) {
  const { t } = useTranslation();
  const OpenIcon = kind === "pulls" ? GitPullRequest : CircleDot;
  const ClosedIcon = kind === "pulls" ? GitPullRequestClosed : CircleCheck;
  const Icon = issue.isClosed ? ClosedIcon : OpenIcon;
  const [owner, repo] = issue.repo.split("/");
  const titleClass = "font-semibold text-(--color-foreground) hover:text-(--color-primary)";
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
          {unread ? (
            <Tooltip>
              <TooltipTrigger asChild>
                <span
                  role="img"
                  aria-label={t("repo.issues.unread")}
                  className="mr-1.5 inline-block size-2 rounded-full bg-(--color-primary) align-middle"
                />
              </TooltipTrigger>
              <TooltipContent>{t("repo.issues.unread")}</TooltipContent>
            </Tooltip>
          ) : null}
          {kind === "issues" ? (
            <Link
              to="/$owner/$repo/issues/$index"
              params={{ owner, repo, index: String(issue.index) }}
              className={titleClass}
            >
              {issue.title}
            </Link>
          ) : (
            // The pull request page is still served by the server-rendered pages.
            <a href={subUrl(`/${issue.repo}/pulls/${issue.index}`)} className={titleClass}>
              {issue.title}
            </a>
          )}
          {issue.labels.map((label) => {
            const chip = (
              <span
                className="inline-block max-w-full truncate rounded-full px-2 align-text-bottom text-xs/5 font-medium"
                style={{ backgroundColor: label.color, color: label.textColor }}
              >
                {label.name}
              </span>
            );
            return <Fragment key={label.id}> {renderLabel ? renderLabel(label, chip) : chip}</Fragment>;
          })}
        </p>
        <p className="mt-1 flex flex-wrap gap-x-3 gap-y-1 text-xs text-(--color-muted-foreground)">
          <span>
            #{issue.index} <OpenedBy issue={issue} />
          </span>
          {meta}
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
export function OpenedBy({ issue }: { issue: Pick<DashboardIssue, "poster" | "createdAt"> }) {
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
