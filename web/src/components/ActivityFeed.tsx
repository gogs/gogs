import {
  BookMarked,
  CircleCheck,
  CircleDot,
  GitBranch,
  GitCommitHorizontal,
  GitFork,
  GitMerge,
  GitPullRequest,
  Link as LinkIcon,
  MessageSquare,
  RotateCcw,
  Tag,
  TriangleAlert,
} from "lucide-react";
import { type ComponentType, useEffect, useRef } from "react";
import { useTranslation } from "react-i18next";

import { Button } from "@/components/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import type { ActivityItem } from "@/lib/queries/user";
import { formatAbsoluteTime, formatRelativeTime } from "@/lib/relative-time";
import { subUrl } from "@/lib/url";

// Icons by `ActionType`, after the octicons of the server-rendered feeds.
const ICONS: Record<number, ComponentType<{ className?: string }>> = {
  1: BookMarked,
  2: BookMarked,
  5: GitCommitHorizontal,
  6: CircleDot,
  7: GitPullRequest,
  8: BookMarked,
  9: Tag,
  10: MessageSquare,
  11: GitMerge,
  12: CircleCheck,
  13: RotateCcw,
  14: CircleCheck,
  15: RotateCcw,
  16: GitBranch,
  17: TriangleAlert,
  18: TriangleAlert,
  19: GitFork,
  20: LinkIcon,
  21: LinkIcon,
  22: LinkIcon,
};

// ActivityFeed lists actions newest first, with a button that loads older
// ones while there are more.
export function ActivityFeed({
  items,
  hasMore,
  loadingMore,
  onLoadMore,
}: {
  items: ActivityItem[];
  hasMore: boolean;
  loadingMore: boolean;
  onLoadMore: () => void;
}) {
  const { t } = useTranslation();
  const listRef = useRef<HTMLUListElement>(null);
  // The number of items before the last "show more", whose first new item
  // takes focus once loaded so keyboard users keep their place even when the
  // button goes away.
  const focusFromRef = useRef<number | null>(null);
  useEffect(() => {
    const from = focusFromRef.current;
    if (from === null || items.length <= from) return;
    focusFromRef.current = null;
    listRef.current?.children[from]?.querySelector("a")?.focus();
  }, [items.length]);

  return (
    <div>
      <ul ref={listRef}>
        {items.map((item) => (
          <ActivityRow key={item.id} item={item} />
        ))}
      </ul>
      {hasMore ? (
        <div className="mt-4 flex justify-center">
          <Button
            variant="outline"
            onClick={() => {
              focusFromRef.current = items.length;
              onLoadMore();
            }}
            disabled={loadingMore}
          >
            {t("show_more")}
          </Button>
        </div>
      ) : null}
    </div>
  );
}

function ActivityRow({ item }: { item: ActivityItem }) {
  const { t } = useTranslation();
  const Icon = ICONS[item.opType];
  return (
    <li className="flex gap-3 border-b border-(--color-border) py-4">
      <img
        src={item.actorAvatarURL}
        alt=""
        className="size-8 shrink-0 rounded-full border border-(--color-border) object-cover"
      />
      <div className="min-w-0 flex-1 text-sm">
        <p className="break-words [&_a]:text-(--color-primary) [&_a:hover]:underline">
          <a href={subUrl(`/${item.actorName}`)} className="font-semibold">
            {item.actorName}
          </a>{" "}
          <span dangerouslySetInnerHTML={{ __html: item.headlineHTML }} />
        </p>
        {item.commits?.length ? (
          <ul className="mt-2 grid gap-1">
            {item.commits.map((commit) => (
              <li key={commit.sha} className="flex min-w-0 items-center gap-2">
                <img src={commit.avatarURL} alt="" className="size-4 shrink-0 rounded-full object-cover" />
                <a
                  href={`${item.repoURL}/commit/${commit.sha}`}
                  className="shrink-0 font-mono text-xs text-(--color-primary) hover:underline"
                >
                  {commit.sha.slice(0, 10)}
                </a>
                <span className="truncate text-(--color-muted-foreground)">{commit.message}</span>
              </li>
            ))}
            {item.compareURL ? (
              <li>
                <a href={item.compareURL} className="text-(--color-primary) hover:underline">
                  {t("action.compare_commits").replace(/%(?:\[1\])?d/, String(item.commitCount))}
                </a>
              </li>
            ) : null}
          </ul>
        ) : null}
        {item.title ? <p className="mt-1 break-words">{item.title}</p> : null}
        {item.detail ? <p className="mt-1 break-words text-(--color-muted-foreground)">{item.detail}</p> : null}
        <p className="mt-1 text-xs text-(--color-muted-foreground)">
          <Tooltip>
            <TooltipTrigger asChild>
              <time dateTime={item.createdAt}>{formatRelativeTime(t, item.createdAt)}</time>
            </TooltipTrigger>
            <TooltipContent>{formatAbsoluteTime(item.createdAt)}</TooltipContent>
          </Tooltip>
        </p>
      </div>
      {Icon ? <Icon className="mt-0.5 size-4 shrink-0 text-(--color-muted-foreground)" aria-hidden /> : null}
    </li>
  );
}
