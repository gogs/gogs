import { Link } from "@tanstack/react-router";
import {
  CircleAlert,
  CircleCheck,
  Clock,
  GitMerge,
  GitPullRequestClosed,
  LoaderCircle,
  type LucideIcon,
  Maximize2,
  MessageSquare,
  Minus,
  Plus,
  RefreshCw,
  Scan,
} from "lucide-react";
import { useEffect, useId, useLayoutEffect, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";

import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import type { PullStack as PullStackData, PullStackNode, PullStackStatus } from "@/lib/queries/repo";
import { formatRelativeTime } from "@/lib/relative-time";
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
// NEARBY is how many pull requests the sidebar shows on each side of the
// current one.
const NEARBY = 3;
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
  cardSide = "left",
  className,
}: {
  owner: string;
  repo: string;
  stack: PullStackData;
  current: number;
  // cardSide is where the card of a hovered pull request opens, beside the
  // list in the sidebar and below it where the list spans the page.
  cardSide?: "left" | "bottom";
  className?: string;
}) {
  const { t } = useTranslation();
  const titleId = useId();
  const [active, setActive] = useState<number | null>(null);
  const [graphOpen, setGraphOpen] = useState(false);
  // A graph taller than a laptop screen keeps the dialog at full height, so
  // zooming moves the graph and not the edges of the dialog.
  const tallGraph = useMemo(() => layoutLayers(stack.nodes).height + GRAPH_PAD * 2 > 560, [stack]);

  const byIndex = useMemo(() => new Map(stack.nodes.map((n) => [n.index, n])), [stack]);
  const parentsOf = (i: number) => byIndex.get(i)?.parents ?? [];
  const childrenOf = (i: number) => stack.nodes.filter((n) => n.parents.includes(i)).map((n) => n.index);
  const lineage = new Set([current, ...reach(current, parentsOf), ...reach(current, childrenOf)]);
  // The rows run children first, so the ones above the current pull request
  // depend on it and the ones below it are what it depends on. Only the
  // nearest few of each stay in the sidebar, the dialog shows the rest.
  const related = stack.nodes.filter((n) => lineage.has(n.index));
  const at = related.findIndex((n) => n.index === current);
  const nodes = related.slice(Math.max(0, at - NEARBY), at + NEARBY + 1);
  const hidden = stack.nodes.length - nodes.length;
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
    <Dialog open={graphOpen} onOpenChange={setGraphOpen}>
      <section aria-labelledby={titleId} className={className} onMouseLeave={() => setActive(null)}>
        <div className="mb-2 flex items-center gap-2">
          <h2 id={titleId} className="text-xs font-semibold text-(--color-muted-foreground)">
            {t("repo.pulls.stack")}
          </h2>
          <p title={summary} className="min-w-0 flex-1 truncate text-right text-xs text-(--color-muted-foreground)">
            {summary}
          </p>
          <Tooltip>
            <TooltipTrigger asChild>
              <DialogTrigger asChild>
                <button
                  type="button"
                  aria-label={t("repo.pulls.stack_expand")}
                  className="-my-1 grid size-7 shrink-0 cursor-pointer place-items-center rounded-md text-(--color-muted-foreground) hover:bg-(--color-surface) hover:text-(--color-foreground) focus-visible:outline-2 focus-visible:outline-(--color-ring)"
                >
                  <Maximize2 className="size-3.5" aria-hidden />
                </button>
              </DialogTrigger>
            </TooltipTrigger>
            <TooltipContent>{t("repo.pulls.stack_expand")}</TooltipContent>
          </Tooltip>
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
                  strokeDasharray={
                    // Dotted when the parents are cut from the sidebar, so the
                    // edge reaches the base branch through them.
                    parent === TRUNK && parentsOf(child).length
                      ? "1 3"
                      : byIndex.get(parent)?.status === "merged"
                        ? "3 3"
                        : undefined
                  }
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
                  <Tooltip delayDuration={200}>
                    <TooltipTrigger asChild>
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
                        <Icon
                          className={cn(
                            "size-4 shrink-0",
                            text,
                            node.status === "checking" && "motion-safe:animate-spin",
                          )}
                          aria-hidden
                        />
                        <span className="sr-only">
                          , {status}
                          {isCurrent ? `, ${t("repo.pulls.stack_this")}` : ""}
                        </span>
                      </Link>
                    </TooltipTrigger>
                    <TooltipContent
                      side={cardSide}
                      align="start"
                      sideOffset={cardSide === "left" ? railWidth + 12 : 4}
                      className={CARD}
                    >
                      <StackCard node={node} stack={stack} />
                    </TooltipContent>
                  </Tooltip>
                </li>
              );
            })}
          </ol>
        </div>
        {hidden > 0 ? (
          <button
            type="button"
            onClick={() => setGraphOpen(true)}
            className="mt-1 min-h-6 cursor-pointer rounded-md text-xs text-(--color-muted-foreground) hover:text-(--color-primary) focus-visible:outline-2 focus-visible:outline-(--color-ring)"
          >
            {t("repo.pulls.stack_show_more").replace("%d", String(hidden))}
          </button>
        ) : null}
        <DialogContent className={cn("sm:max-w-[min(76rem,calc(100vw-2rem))]", tallGraph && "h-[90dvh] sm:h-[85dvh]")}>
          <DialogHeader>
            <DialogTitle>{t("repo.pulls.stack")}</DialogTitle>
            <DialogDescription>{summary}</DialogDescription>
          </DialogHeader>
          <StackGraph
            owner={owner}
            repo={repo}
            stack={stack}
            current={current}
            onNavigate={() => setGraphOpen(false)}
          />
          <div className="hidden flex-wrap items-center gap-x-4 gap-y-1 border-t border-(--color-border) px-4 py-3 text-xs text-(--color-muted-foreground) sm:flex">
            <p className="flex-1">{t("repo.pulls.stack_graph_hint")}</p>
            {LEGEND.map((status) => {
              const { icon: Icon, text } = STATUS[status];
              return (
                <span key={status} className="inline-flex items-center gap-1">
                  <Icon className={cn("size-3.5", text)} aria-hidden />
                  {t(`repo.pulls.stack_status_${status}`)}
                </span>
              );
            })}
          </div>
        </DialogContent>
      </section>
    </Dialog>
  );
}

const LEGEND: PullStackStatus[] = ["mergeable", "behind", "blocked", "conflict", "merged", "closed"];

const CARD =
  "w-72 max-w-[calc(100vw-2rem)] rounded-lg border border-(--color-border) bg-(--color-popover) p-3 text-(--color-popover-foreground) shadow-md";

function indexes(list: number[]) {
  return list.map((i) => `#${i}`).join(", ");
}

// StackCard tells what a pull request of the stack is about and where it
// stands, so the viewer need not open it to decide.
function StackCard({ node, stack }: { node: PullStackNode; stack: PullStackData }) {
  const { t } = useTranslation();
  const { icon: Icon, text } = STATUS[node.status];
  const children = stack.nodes.filter((n) => n.parents.includes(node.index)).map((n) => n.index);
  return (
    <div className="flex flex-col gap-2 text-sm">
      <p className={cn("flex items-center gap-1.5 text-xs font-medium", text)}>
        <Icon className="size-3.5" aria-hidden />
        {t(`repo.pulls.stack_status_${node.status}`)}
      </p>
      <p className="leading-snug font-semibold break-words">
        {node.title} <span className="font-normal text-(--color-muted-foreground)">#{node.index}</span>
      </p>
      <p className="flex min-w-0 items-center gap-1.5 text-xs text-(--color-muted-foreground)">
        <img src={node.poster.avatarURL} alt="" className="size-4 shrink-0 rounded-full" />
        <span className="truncate font-medium text-(--color-foreground)">{node.poster.name}</span>
        <span aria-hidden>·</span>
        <span className="shrink-0">
          {t("repo.pulls.stack_card_updated").replace("%s", formatRelativeTime(t, node.updatedAt))}
        </span>
      </p>
      <dl className="grid grid-cols-[auto_1fr] gap-x-3 gap-y-1 border-t border-(--color-border) pt-2 text-xs">
        <dt className="text-(--color-muted-foreground)">{t("repo.pulls.stack_card_branch")}</dt>
        <dd className="min-w-0 truncate font-mono">{node.headBranch}</dd>
        <dt className="text-(--color-muted-foreground)">{t("repo.pulls.depends_on")}</dt>
        <dd className="min-w-0 truncate">
          {node.parents.length ? indexes(node.parents) : <span className="font-mono">{stack.baseBranch}</span>}
        </dd>
        {children.length ? (
          <>
            <dt className="text-(--color-muted-foreground)">{t("repo.pulls.stack_card_needed_by")}</dt>
            <dd className="min-w-0 truncate">{indexes(children)}</dd>
          </>
        ) : null}
        <dt className="text-(--color-muted-foreground)">{t("repo.pulls.stack_card_comments")}</dt>
        <dd className="tabular-nums">{node.comments}</dd>
      </dl>
    </div>
  );
}

const NODE_W = 240;
const NODE_H = 80;
const COL_GAP = 64;
const ROW_GAP = 16;
const TRUNK_W = 112;
const TRUNK_H = 32;
const GRAPH_PAD = 24;
const MIN_ZOOM = 0.25;
const MAX_ZOOM = 1.5;

// layoutLayers puts each pull request one column right of the deepest one it
// depends on, with the base branch in the first column, and orders each column
// by where its parents sit so that edges cross less.
function layoutLayers(nodes: PullStackNode[]) {
  const shown = new Set(nodes.map((n) => n.index));
  const parentsOf = (n: PullStackNode) => n.parents.filter((p) => shown.has(p));
  const depth = new Map<number, number>();
  // The nodes come parents first.
  for (const n of nodes) depth.set(n.index, 1 + Math.max(0, ...parentsOf(n).map((p) => depth.get(p) ?? 0)));
  const columns: PullStackNode[][] = [];
  for (const n of nodes) (columns[(depth.get(n.index) as number) - 1] ??= []).push(n);

  const row = new Map<number, number>();
  columns.forEach((column, c) => {
    const weight = (n: PullStackNode) => {
      const rows = parentsOf(n).map((p) => row.get(p) ?? 0);
      return c === 0 || !rows.length ? n.index : rows.reduce((a, b) => a + b, 0) / rows.length;
    };
    column.sort((a, b) => weight(a) - weight(b) || a.index - b.index);
    column.forEach((n, r) => row.set(n.index, r));
  });

  const tallest = Math.max(1, ...columns.map((c) => c.length));
  const height = tallest * (NODE_H + ROW_GAP) - ROW_GAP;
  const pos = new Map<number, { x: number; y: number }>();
  columns.forEach((column, c) => {
    const top = (height - (column.length * (NODE_H + ROW_GAP) - ROW_GAP)) / 2;
    column.forEach((n, r) => {
      pos.set(n.index, { x: TRUNK_W + COL_GAP + c * (NODE_W + COL_GAP), y: top + r * (NODE_H + ROW_GAP) });
    });
  });
  const width = TRUNK_W + columns.length * (NODE_W + COL_GAP);
  return { pos, width, height, parentsOf };
}

// StackGraph draws the whole stack as columns of cards, from the base branch
// on the left to the pull requests that depend on the most on the right.
function StackGraph({
  owner,
  repo,
  stack,
  current,
  onNavigate,
}: {
  owner: string;
  repo: string;
  stack: PullStackData;
  current: number;
  onNavigate: () => void;
}) {
  const { t } = useTranslation();
  const [active, setActive] = useState<number | null>(null);
  const layout = useMemo(() => layoutLayers(stack.nodes), [stack]);
  const byIndex = useMemo(() => new Map(stack.nodes.map((n) => [n.index, n])), [stack]);

  const up = (i: number) => {
    const n = byIndex.get(i);
    if (!n) return [];
    const parents = layout.parentsOf(n);
    return parents.length ? parents : [TRUNK];
  };
  const down = (i: number) => stack.nodes.filter((n) => layout.parentsOf(n).includes(i)).map((n) => n.index);
  // Until the viewer points at another one, the edges of the current pull
  // request stay lit, so a large stack shows where the viewer is.
  const focus = active ?? current;
  const focusUp = reach(focus, up);
  const focusDown = reach(focus, down);
  const related = (i: number) => i === focus || focusUp.has(i) || focusDown.has(i);
  const lit = (i: number) => active === null || related(i);

  const viewportRef = useRef<HTMLDivElement>(null);
  const [zoom, setZoom] = useState(1);
  // anchorRef keeps the graph point under the pointer, or the center, in
  // place while the zoom changes.
  const anchorRef = useRef<{ gx: number; gy: number; vx: number; vy: number } | null>(null);
  const dragRef = useRef<{ x: number; y: number; left: number; top: number; moved: boolean } | null>(null);
  const suppressClickRef = useRef(false);
  const fullWidth = layout.width + GRAPH_PAD * 2;
  const fullHeight = layout.height + GRAPH_PAD * 2;

  const zoomTo = (next: number, at?: { x: number; y: number }) => {
    const el = viewportRef.current;
    if (!el) return;
    const fit = Math.min(el.clientWidth / fullWidth, el.clientHeight / fullHeight, 1);
    const clamped = Math.min(MAX_ZOOM, Math.max(Math.min(MIN_ZOOM, fit), next));
    const vx = at?.x ?? el.clientWidth / 2;
    const vy = at?.y ?? el.clientHeight / 2;
    anchorRef.current = { gx: (el.scrollLeft + vx) / zoom, gy: (el.scrollTop + vy) / zoom, vx, vy };
    setZoom(clamped);
  };
  const fitZoom = () => {
    const el = viewportRef.current;
    if (el) setZoom(Math.min(el.clientWidth / fullWidth, el.clientHeight / fullHeight, 1));
  };

  useLayoutEffect(() => {
    const el = viewportRef.current;
    const anchor = anchorRef.current;
    if (!el || !anchor) return;
    el.scrollLeft = anchor.gx * zoom - anchor.vx;
    el.scrollTop = anchor.gy * zoom - anchor.vy;
    anchorRef.current = null;
  }, [zoom]);

  // The dialog opens on the current pull request, wherever it sits.
  useLayoutEffect(() => {
    const el = viewportRef.current;
    const pos = layout.pos.get(current);
    if (!el || !pos) return;
    el.scrollLeft = GRAPH_PAD + pos.x + NODE_W / 2 - el.clientWidth / 2;
    el.scrollTop = GRAPH_PAD + pos.y + NODE_H / 2 - el.clientHeight / 2;
  }, [layout, current]);

  // Holding Ctrl or Cmd while scrolling zooms, which is also what a trackpad
  // pinch sends. React listens to wheel events passively, so it cannot
  // keep the page from zooming instead.
  const zoomRef = useRef(zoomTo);
  const zoomLevelRef = useRef(zoom);
  useLayoutEffect(() => {
    zoomRef.current = zoomTo;
    zoomLevelRef.current = zoom;
  });
  useEffect(() => {
    const el = viewportRef.current;
    if (!el) return;
    const onWheel = (event: WheelEvent) => {
      if (!event.ctrlKey && !event.metaKey) return;
      event.preventDefault();
      const rect = el.getBoundingClientRect();
      zoomRef.current(zoomLevelRef.current * Math.exp(-event.deltaY / 300), {
        x: event.clientX - rect.left,
        y: event.clientY - rect.top,
      });
    };
    el.addEventListener("wheel", onWheel, { passive: false });
    return () => el.removeEventListener("wheel", onWheel);
  }, []);

  const trunkY = layout.height / 2;
  const edges = stack.nodes.flatMap((n) => {
    const parents = layout.parentsOf(n);
    return parents.length ? parents.map((p) => ({ child: n.index, parent: p })) : [{ child: n.index, parent: TRUNK }];
  });

  return (
    <>
      {/* Columns of cards need more room than a phone has, so it gets the
        same cards in a list instead, ordered like the rail. */}
      <ol className="min-h-0 flex-1 overflow-y-auto border-t border-(--color-border) bg-(--color-surface)/40 p-4 sm:hidden">
        {[...stack.nodes].reverse().map((node) => (
          <li key={node.index} className="mb-2">
            <StackNodeCard
              owner={owner}
              repo={repo}
              node={node}
              current={current}
              onNavigate={onNavigate}
              showParents
              baseBranch={stack.baseBranch}
            />
          </li>
        ))}
        <li>
          <code className="rounded-md border border-(--color-border) bg-(--color-background) px-2 py-1 font-mono text-xs">
            {stack.baseBranch}
          </code>
        </li>
      </ol>
      <div className="relative hidden min-h-0 flex-1 border-t border-(--color-border) sm:flex">
        <div
          ref={viewportRef}
          className="min-h-0 flex-1 cursor-grab overflow-auto bg-(--color-surface)/40 select-none data-[dragging]:cursor-grabbing"
          onMouseLeave={() => setActive(null)}
          onPointerDown={(event) => {
            if (event.button !== 0 || event.pointerType !== "mouse") return;
            const el = event.currentTarget;
            dragRef.current = {
              x: event.clientX,
              y: event.clientY,
              left: el.scrollLeft,
              top: el.scrollTop,
              moved: false,
            };
          }}
          onPointerMove={(event) => {
            const drag = dragRef.current;
            if (!drag) return;
            const [dx, dy] = [event.clientX - drag.x, event.clientY - drag.y];
            const el = event.currentTarget;
            if (!drag.moved && Math.hypot(dx, dy) > 4) {
              drag.moved = true;
              el.setPointerCapture(event.pointerId);
              el.dataset.dragging = "";
            }
            if (drag.moved) {
              el.scrollLeft = drag.left - dx;
              el.scrollTop = drag.top - dy;
            }
          }}
          onPointerUp={(event) => {
            suppressClickRef.current = !!dragRef.current?.moved;
            dragRef.current = null;
            delete event.currentTarget.dataset.dragging;
          }}
          onPointerCancel={(event) => {
            dragRef.current = null;
            delete event.currentTarget.dataset.dragging;
          }}
          // Cards are links with avatars, which the browser would otherwise
          // start dragging out of the page, cancelling the pan.
          onDragStart={(event) => event.preventDefault()}
          onClickCapture={(event) => {
            // A drag that ends on a card moves the graph, it does not open it.
            if (!suppressClickRef.current) return;
            suppressClickRef.current = false;
            event.preventDefault();
            event.stopPropagation();
          }}
        >
          <div className="flex min-h-full min-w-max">
            <div className="relative m-auto shrink-0" style={{ width: fullWidth * zoom, height: fullHeight * zoom }}>
              <div
                className="absolute top-0 left-0 origin-top-left"
                style={{ width: fullWidth, height: fullHeight, padding: GRAPH_PAD, transform: `scale(${zoom})` }}
              >
                <svg
                  className="absolute"
                  style={{ left: GRAPH_PAD, top: GRAPH_PAD }}
                  width={layout.width}
                  height={layout.height}
                  aria-hidden
                >
                  {edges.map(({ child, parent }) => {
                    const c = layout.pos.get(child) as { x: number; y: number };
                    const [px, py] =
                      parent === TRUNK
                        ? [TRUNK_W, trunkY]
                        : [
                            (layout.pos.get(parent) as { x: number }).x + NODE_W,
                            (layout.pos.get(parent) as { y: number }).y + NODE_H / 2,
                          ];
                    const [cx, cy] = [c.x, c.y + NODE_H / 2];
                    const bend = Math.min(COL_GAP, (cx - px) / 2);
                    const on =
                      (child === focus || parent === focus || focusUp.has(parent) || focusDown.has(child)) &&
                      related(child) &&
                      (parent === TRUNK || related(parent));
                    return (
                      <path
                        key={`${child}-${parent}`}
                        d={`M ${px} ${py} C ${px + bend} ${py}, ${cx - bend} ${cy}, ${cx} ${cy}`}
                        fill="none"
                        strokeLinecap="round"
                        strokeDasharray={byIndex.get(parent)?.status === "merged" ? "4 4" : undefined}
                        strokeWidth={on ? 2.5 : 1.5}
                        className={cn(
                          "motion-safe:transition-opacity motion-safe:duration-150",
                          on ? "stroke-(--color-primary)" : "stroke-(--color-muted-foreground)",
                          active !== null && !on && "opacity-20",
                        )}
                      />
                    );
                  })}
                </svg>
                <div
                  className="absolute flex items-center justify-center"
                  style={{ left: GRAPH_PAD, top: GRAPH_PAD + trunkY - TRUNK_H / 2, width: TRUNK_W, height: TRUNK_H }}
                >
                  <code className="max-w-full truncate rounded-md border border-(--color-border) bg-(--color-background) px-2 py-1 font-mono text-xs">
                    {stack.baseBranch}
                  </code>
                </div>
                <ol>
                  {stack.nodes.map((node) => {
                    const { x, y } = layout.pos.get(node.index) as { x: number; y: number };
                    return (
                      <li
                        key={node.index}
                        className={cn("absolute motion-safe:transition-opacity", !lit(node.index) && "opacity-40")}
                        style={{ left: GRAPH_PAD + x, top: GRAPH_PAD + y, width: NODE_W, height: NODE_H }}
                      >
                        <StackNodeCard
                          owner={owner}
                          repo={repo}
                          node={node}
                          current={current}
                          onNavigate={onNavigate}
                          onActive={setActive}
                          baseBranch={stack.baseBranch}
                        />
                      </li>
                    );
                  })}
                </ol>
              </div>
            </div>
          </div>
        </div>
        <div className="absolute right-3 bottom-3 flex items-center rounded-md border border-(--color-border) bg-(--color-background) shadow-sm">
          <ZoomButton label={t("repo.pulls.stack_zoom_out")} onClick={() => zoomTo(zoom / 1.25)}>
            <Minus className="size-4" aria-hidden />
          </ZoomButton>
          <span
            className="min-w-12 text-center text-xs tabular-nums text-(--color-muted-foreground)"
            aria-live="polite"
          >
            {Math.round(zoom * 100)}%
          </span>
          <ZoomButton label={t("repo.pulls.stack_zoom_in")} onClick={() => zoomTo(zoom * 1.25)}>
            <Plus className="size-4" aria-hidden />
          </ZoomButton>
          <span className="h-5 w-px bg-(--color-border)" aria-hidden />
          <ZoomButton label={t("repo.pulls.stack_zoom_fit")} onClick={fitZoom}>
            <Scan className="size-4" aria-hidden />
          </ZoomButton>
        </div>
      </div>
    </>
  );
}

function ZoomButton({ label, onClick, children }: { label: string; onClick: () => void; children: React.ReactNode }) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <button
          type="button"
          aria-label={label}
          onClick={onClick}
          className="grid size-8 cursor-pointer place-items-center rounded-md text-(--color-muted-foreground) hover:bg-(--color-surface) hover:text-(--color-foreground) focus-visible:outline-2 focus-visible:outline-(--color-ring)"
        >
          {children}
        </button>
      </TooltipTrigger>
      <TooltipContent>{label}</TooltipContent>
    </Tooltip>
  );
}

// StackNodeCard is one pull request of the expanded stack: where it stands,
// what it is called, and who touched it last.
function StackNodeCard({
  owner,
  repo,
  node,
  current,
  onNavigate,
  onActive,
  showParents,
  baseBranch,
}: {
  owner: string;
  repo: string;
  node: PullStackNode;
  current: number;
  onNavigate: () => void;
  onActive?: (index: number | null) => void;
  // showParents names what it depends on, for layouts that draw no edges.
  showParents?: boolean;
  baseBranch?: string;
}) {
  const { t } = useTranslation();
  const { icon: Icon, text } = STATUS[node.status];
  const status = t(`repo.pulls.stack_status_${node.status}`);
  const isCurrent = node.index === current;
  const done = node.status === "merged" || node.status === "closed";
  return (
    <Link
      to="/$owner/$repo/pulls/$index"
      params={{ owner, repo, index: String(node.index) }}
      aria-current={isCurrent ? "page" : undefined}
      onClick={onNavigate}
      onMouseEnter={() => onActive?.(node.index)}
      onFocus={() => onActive?.(node.index)}
      onBlur={() => onActive?.(null)}
      className={cn(
        "flex h-full flex-col justify-between gap-1 rounded-lg border bg-(--color-background) px-3 py-2 shadow-xs outline-none hover:border-(--color-primary) focus-visible:ring-2 focus-visible:ring-(--color-ring)",
        isCurrent ? "border-(--color-primary) ring-1 ring-(--color-primary)" : "border-(--color-border)",
        done && "border-dashed",
      )}
    >
      <span className="flex min-w-0 items-center gap-1.5 text-xs text-(--color-muted-foreground)">
        <span className="tabular-nums">#{node.index}</span>
        <span className="min-w-0 flex-1 truncate font-mono">{node.headBranch}</span>
        <span className={cn("inline-flex shrink-0 items-center gap-1 font-medium", text)}>
          <Icon className={cn("size-3.5", node.status === "checking" && "motion-safe:animate-spin")} aria-hidden />
          {status}
        </span>
      </span>
      <span className={cn("truncate text-sm font-semibold", done && "text-(--color-muted-foreground)")}>
        {node.title}
      </span>
      <span className="flex min-w-0 items-center gap-1.5 text-xs text-(--color-muted-foreground)">
        <img src={node.poster.avatarURL} alt="" className="size-4 shrink-0 rounded-full" />
        <span className="truncate">{node.poster.name}</span>
        <span aria-hidden>·</span>
        <span className="shrink-0">{formatRelativeTime(t, node.updatedAt)}</span>
        {node.comments > 0 ? (
          <span className="ml-auto inline-flex shrink-0 items-center gap-0.5 tabular-nums">
            <MessageSquare className="size-3" aria-hidden />
            {node.comments}
            <span className="sr-only">{t("repo.pulls.stack_card_comments")}</span>
          </span>
        ) : null}
      </span>
      {showParents ? (
        <span className="text-xs text-(--color-muted-foreground)">
          {t("repo.pulls.depends_on")}{" "}
          <span className="text-(--color-foreground)">
            {node.parents.length ? indexes(node.parents) : <span className="font-mono">{baseBranch}</span>}
          </span>
        </span>
      ) : (
        <span className="sr-only">
          , {t("repo.pulls.depends_on")} {node.parents.length ? indexes(node.parents) : baseBranch}
        </span>
      )}
      {isCurrent ? <span className="sr-only">, {t("repo.pulls.stack_this")}</span> : null}
    </Link>
  );
}
