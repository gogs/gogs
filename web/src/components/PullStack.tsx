import { Link } from "@tanstack/react-router";
import {
  CircleAlert,
  CircleCheck,
  Clock,
  GitMerge,
  GitPullRequestClosed,
  LoaderCircle,
  type LucideIcon,
  RefreshCw,
} from "lucide-react";
import { useId, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";

import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import type { PullStack as PullStackData, PullStackNode, PullStackStatus } from "@/lib/queries/repo";
import { cn } from "@/lib/utils";

const STATUS: Record<PullStackStatus, { icon: LucideIcon; text: string; dot: string }> = {
  merged: { icon: GitMerge, text: "text-(--color-merged)", dot: "fill-(--color-merged)" },
  closed: {
    icon: GitPullRequestClosed,
    text: "text-(--color-muted-foreground)",
    dot: "fill-(--color-muted-foreground)",
  },
  behind: { icon: RefreshCw, text: "text-(--color-primary)", dot: "fill-(--color-primary)" },
  blocked: { icon: Clock, text: "text-(--color-muted-foreground)", dot: "fill-(--color-muted-foreground)" },
  checking: { icon: LoaderCircle, text: "text-(--color-muted-foreground)", dot: "fill-(--color-muted-foreground)" },
  mergeable: { icon: CircleCheck, text: "text-(--color-success)", dot: "fill-(--color-success)" },
  conflict: { icon: CircleAlert, text: "text-(--color-destructive)", dot: "fill-(--color-destructive)" },
};

const ROW = 32;
const LANE = 16;
const PAD = 9;
// TRUNK stands for the base branch, which every pull request without shown
// parents builds on. Pull request numbers start at 1.
const TRUNK = 0;

// reach returns the pull requests reachable from the given one through next.
function reach(start: number, next: (index: number) => number[]) {
  const seen = new Set<number>();
  const pending = [...next(start)];
  while (pending.length) {
    const index = pending.pop() as number;
    if (seen.has(index)) continue;
    seen.add(index);
    pending.push(...next(index));
  }
  return seen;
}

// layoutRails places the pull requests in rows, children above their parents
// and the base branch last, and gives each a lane the way "git log --graph"
// does: a lane carries an edge down to the parent it waits for, and edges to a
// parent that already has a lane join it.
function layoutRails(nodes: PullStackNode[]) {
  const shown = new Set(nodes.map((n) => n.index));
  const parentsOf = (index: number) => {
    if (index === TRUNK) return [];
    const parents = nodes.find((n) => n.index === index)?.parents.filter((p) => shown.has(p)) ?? [];
    return parents.length ? parents : [TRUNK];
  };
  // The nodes come parents first, so reversed they come children first.
  const order = [...nodes.map((n) => n.index).reverse(), TRUNK];
  const lanes: (number | null)[] = [];
  const pos = new Map<number, { row: number; col: number }>();
  const edges: { child: number; parent: number; lane: number }[] = [];
  let width = 1;
  const freeLane = () => {
    const free = lanes.indexOf(null);
    if (free >= 0) return free;
    lanes.push(null);
    return lanes.length - 1;
  };
  order.forEach((index, row) => {
    let col = lanes.indexOf(index);
    if (col < 0) col = freeLane();
    for (let i = 0; i < lanes.length; i++) if (lanes[i] === index) lanes[i] = null;
    pos.set(index, { row, col });
    parentsOf(index).forEach((parent, k) => {
      let lane = lanes.indexOf(parent);
      if (lane < 0) {
        lane = k === 0 && lanes[col] === null ? col : freeLane();
        lanes[lane] = parent;
      }
      edges.push({ child: index, parent, lane });
    });
    width = Math.max(width, lanes.length);
  });
  return { order, pos, edges, width, parentsOf };
}

// PullStack lists the pull requests connected to one by dependencies, one row
// each, with a rail that draws how they depend on one another, like
// "git log --graph". It shows what the current pull request depends on and
// what depends on it, and folds the rest of the stack away.
export function PullStack({
  owner,
  repo,
  stack,
  current,
  className,
}: {
  owner: string;
  repo: string;
  stack: PullStackData;
  current: number;
  className?: string;
}) {
  const { t } = useTranslation();
  const titleId = useId();
  const [expanded, setExpanded] = useState(false);
  const [active, setActive] = useState<number | null>(null);

  const byIndex = useMemo(() => new Map(stack.nodes.map((n) => [n.index, n])), [stack]);
  const parentsOf = (i: number) => byIndex.get(i)?.parents ?? [];
  const childrenOf = (i: number) => stack.nodes.filter((n) => n.parents.includes(i)).map((n) => n.index);
  const lineage = new Set([current, ...reach(current, parentsOf), ...reach(current, childrenOf)]);
  const hidden = stack.nodes.length - lineage.size;
  const nodes = expanded ? stack.nodes : stack.nodes.filter((n) => lineage.has(n.index));
  const rails = useMemo(() => layoutRails(nodes), [nodes]);

  const activeUp = active === null ? null : reach(active, rails.parentsOf);
  const activeDown =
    active === null
      ? null
      : reach(active, (i) => nodes.filter((n) => rails.parentsOf(n.index).includes(i)).map((n) => n.index));
  const lit = (index: number) =>
    active === null || index === active || !!activeUp?.has(index) || !!activeDown?.has(index);

  const x = (col: number) => PAD + col * LANE;
  const y = (row: number) => row * ROW + ROW / 2;
  const railWidth = PAD * 2 + (rails.width - 1) * LANE;
  const height = rails.order.length * ROW;

  const merged = stack.nodes.filter((n) => n.status === "merged").length;
  const ready = stack.nodes.filter((n) => n.status === "mergeable").length;
  const summary = [
    t("repo.pulls.stack_count").replace("%d", String(stack.nodes.length)),
    merged ? t("repo.pulls.stack_merged_count").replace("%d", String(merged)) : null,
    ready ? t("repo.pulls.stack_ready_count").replace("%d", String(ready)) : null,
  ]
    .filter(Boolean)
    .join(" · ");

  return (
    <section aria-labelledby={titleId} className={className} onMouseLeave={() => setActive(null)}>
      <div className="mb-2 flex items-baseline justify-between gap-2">
        <h2 id={titleId} className="text-xs font-semibold text-(--color-muted-foreground)">
          {t("repo.pulls.stack")}
        </h2>
        <p className="truncate text-xs text-(--color-muted-foreground)">{summary}</p>
      </div>
      <div className="relative" style={{ height }}>
        <svg className="absolute top-0 left-0" width={railWidth} height={height} aria-hidden>
          {rails.edges.map(({ child, parent, lane }) => {
            const c = rails.pos.get(child) as { row: number; col: number };
            const p = rails.pos.get(parent) as { row: number; col: number };
            const [cx, cy, px, py, lx] = [x(c.col), y(c.row), x(p.col), y(p.row), x(lane)];
            const d =
              p.row - c.row === 1
                ? `M ${cx} ${cy} C ${cx} ${cy + ROW / 2}, ${px} ${py - ROW / 2}, ${px} ${py}`
                : `M ${cx} ${cy} C ${cx} ${cy + ROW / 2}, ${lx} ${cy + ROW / 2}, ${lx} ${cy + ROW} L ${lx} ${py - ROW} C ${lx} ${py - ROW / 2}, ${px} ${py - ROW / 2}, ${px} ${py}`;
            const onLineage =
              active !== null &&
              lit(child) &&
              lit(parent) &&
              (child === active || parent === active || activeUp?.has(parent) || activeDown?.has(child));
            return (
              <path
                key={`${child}-${parent}`}
                d={d}
                fill="none"
                strokeLinecap="round"
                strokeDasharray={byIndex.get(parent)?.status === "merged" ? "3 3" : undefined}
                strokeWidth={onLineage ? 2 : 1.5}
                className={cn(
                  "motion-safe:transition-opacity motion-safe:duration-150",
                  onLineage ? "stroke-(--color-primary)" : "stroke-(--color-muted-foreground)",
                  active !== null && !onLineage && "opacity-25",
                )}
              />
            );
          })}
          {rails.order.map((index) => {
            const { row, col } = rails.pos.get(index) as { row: number; col: number };
            if (index === TRUNK) {
              return (
                <rect
                  key={index}
                  x={x(col) - 4}
                  y={y(row) - 4}
                  width={8}
                  height={8}
                  rx={1.5}
                  className="fill-(--color-background) stroke-(--color-muted-foreground)"
                  strokeWidth={1.5}
                />
              );
            }
            const node = byIndex.get(index) as PullStackNode;
            return (
              <g key={index} className={cn("motion-safe:transition-opacity", !lit(index) && "opacity-30")}>
                {index === current ? (
                  <circle
                    cx={x(col)}
                    cy={y(row)}
                    r={7}
                    className="fill-none stroke-(--color-primary)"
                    strokeWidth={2}
                  />
                ) : null}
                <circle
                  cx={x(col)}
                  cy={y(row)}
                  r={4}
                  className={cn(STATUS[node.status].dot, "stroke-(--color-background)")}
                  strokeWidth={1.5}
                />
              </g>
            );
          })}
        </svg>
        <ol className="absolute inset-0" style={{ paddingLeft: railWidth + 4 }}>
          {rails.order.map((index) => {
            if (index === TRUNK) {
              return (
                <li key={index} className="flex items-center" style={{ height: ROW }}>
                  <code className="truncate rounded border border-(--color-border) bg-(--color-surface) px-1.5 font-mono text-xs">
                    {stack.baseBranch}
                  </code>
                </li>
              );
            }
            const node = byIndex.get(index) as PullStackNode;
            const { icon: Icon, text } = STATUS[node.status];
            const isCurrent = index === current;
            const status = t(`repo.pulls.stack_status_${node.status}`);
            return (
              <li
                key={index}
                style={{ height: ROW }}
                className={cn("motion-safe:transition-opacity", !lit(index) && "opacity-40")}
              >
                <Link
                  to="/$owner/$repo/pulls/$index"
                  params={{ owner, repo, index: String(index) }}
                  aria-current={isCurrent ? "page" : undefined}
                  onMouseEnter={() => setActive(index)}
                  onFocus={() => setActive(index)}
                  onBlur={() => setActive(null)}
                  className={cn(
                    "-mx-1 flex h-full min-w-0 items-center gap-1.5 rounded-md px-1 text-sm outline-none hover:bg-(--color-surface) focus-visible:ring-2 focus-visible:ring-(--color-ring)",
                    isCurrent && "bg-(--color-surface) font-semibold",
                  )}
                >
                  <span className="shrink-0 text-(--color-muted-foreground) tabular-nums">#{index}</span>
                  <span className="min-w-0 flex-1 truncate">{node.title}</span>
                  <Tooltip>
                    <TooltipTrigger asChild>
                      <span className={cn("shrink-0", text)}>
                        <Icon
                          className={cn("size-4", node.status === "checking" && "motion-safe:animate-spin")}
                          aria-hidden
                        />
                      </span>
                    </TooltipTrigger>
                    <TooltipContent>{status}</TooltipContent>
                  </Tooltip>
                  <span className="sr-only">
                    , {status}
                    {isCurrent ? `, ${t("repo.pulls.stack_this")}` : ""}
                  </span>
                </Link>
              </li>
            );
          })}
        </ol>
      </div>
      {hidden > 0 ? (
        <button
          type="button"
          aria-expanded={expanded}
          onClick={() => setExpanded(!expanded)}
          className="mt-1 min-h-6 cursor-pointer rounded-md text-xs text-(--color-muted-foreground) hover:text-(--color-primary) focus-visible:outline-2 focus-visible:outline-(--color-ring)"
        >
          {expanded ? t("repo.pulls.stack_show_less") : t("repo.pulls.stack_show_more").replace("%d", String(hidden))}
        </button>
      ) : null}
    </section>
  );
}
