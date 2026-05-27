import { type FileDiffMetadata, parseDiffFromFile, parsePatchFiles } from "@pierre/diffs";
import { CodeView, type CodeViewHandle, type CodeViewItem } from "@pierre/diffs/react";
import { useSuspenseQuery } from "@tanstack/react-query";
import { useLoaderData, useNavigate, useParams, useSearch } from "@tanstack/react-router";
import {
  Check,
  ChevronDown,
  ChevronRight,
  ChevronsDownUp,
  ChevronsUpDown,
  Copy,
  FileCode2,
  FolderTree,
  Loader2,
  Search,
  UnfoldVertical,
} from "lucide-react";
import { type CSSProperties, useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";

import { CommitFileTree, type CommitFileTreeHandle } from "@/components/CommitFileTree";
import { DiffSearch } from "@/components/DiffSearch";
import { DiffToolbar, type DiffToolbarSettings, type WhitespaceMode } from "@/components/DiffToolbar";
import { FileHeaderMenu } from "@/components/FileHeaderMenu";
import { RepoHeader } from "@/components/RepoHeader";
import { ResizableSidebar } from "@/components/ResizableSidebar";
import { Sheet, SheetClose, SheetContent, SheetTitle } from "@/components/ui/sheet";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { repoInfoQuery } from "@/lib/queries/repo";
import { formatAbsoluteTime, formatRelativeTime } from "@/lib/relative-time";
import { useTheme } from "@/lib/theme-context";
import { subUrl } from "@/lib/url";

export interface RepoCommitSignature {
  name: string;
  email: string;
  avatarURL: string;
  userPath?: string;
  when: string;
}

export interface RepoCommitPage {
  commit: {
    sha: string;
    shortSha: string;
    summary: string;
    message: string;
    author: RepoCommitSignature;
    committer: RepoCommitSignature;
    parents: string[];
  };
  patch: string;
  sourcePath: string;
  rawDiffURL: string;
}

// Pierre's default file header sits on the same buffer tint as the surrounding
// padding, so 200+ headers blur together. Give each header an opaque, slightly
// stronger tint plus a bottom hairline so they read as distinct cards. The
// background must be opaque or sticky headers bleed through scrolling content.
//
// The "N unmodified lines" metadata separator ships with `inset-inline: 100%
// auto` so it spans only as wide as a downstream column boundary forces it.
// Override to span the full row width, matching the file header above it.
// Page-local override that unconstrains the shared Navbar, Footer, and
// RepoHeader containers while this page is mounted (see the useEffect that
// sets `data-fullwidth`). We target the unique max-width utility classes
// those components use, which is brittle if those classes ever change but
// keeps the override localized to one file.
const FULLWIDTH_CSS = `
  html[data-fullwidth="commit-diff"] .max-w-6xl,
  html[data-fullwidth="commit-diff"] .max-w-7xl {
    max-width: none;
  }
  /* Pierre's CodeView inlines an 8px top/bottom margin on its virtual
     scroll container. We want the diff flush against the toolbar above,
     so override it. The selector targets the direct child of the
     overflow-scroller we render around CodeView. */
  html[data-fullwidth="commit-diff"] .gogs-diff-scroller > div {
    margin-top: 0 !important;
    margin-bottom: 0 !important;
  }
  /* Hide the global site footer on the commit diff page. The page locks the
     diff workspace to the viewport once the user scrolls past the commit
     metadata, and a footer flowing below the locked workspace breaks that
     model by occupying the bottom of the viewport. GitHub does the same. */
  html[data-fullwidth="commit-diff"] footer {
    display: none;
  }
`;

const DIFF_UNSAFE_CSS = `
  /* Pierre's <diffs-container> draws a 1px border on every side. The top
     and left ones sit right next to the toolbar's border-b and the
     sidebar's border-r, which makes those edges look 2px thick. Zero the
     adjacent sides so the surrounding chrome supplies the only line. */
  :host {
    border-top: 0 !important;
    border-left: 0 !important;
  }
  [data-diffs-header] {
    background: color-mix(in lab, var(--diffs-bg) 96%, var(--diffs-mixer));
    border-bottom: 1px solid color-mix(in lab, var(--diffs-bg) 85%, var(--diffs-mixer));
  }
  /* Pierre's default 8px gap leaves too much air between the chevron, the
     file-type icon, and the filename. Tighten to 4px for a denser, GitHub-
     like layout. */
  [data-header-content] {
    gap: 4px !important;
  }
  /* Reopen a slightly wider gap between the change-state icon and the
     filename so the filename doesn't crowd the dot. */
  [data-change-icon] {
    margin-right: 4px;
  }
  /* The +N / -N counts in Pierre's metadata row inherit a mono font from
     the diff body styles. They're UI chrome, not code — pin them to the
     surrounding sans stack to match the rest of the toolbar text. */
  [data-additions-count],
  [data-deletions-count] {
    font-family: inherit;
  }
  /* The filename is now a click target ("Copy file path"). Use the link
     cursor so the affordance is discoverable. */
  [data-title] {
    cursor: pointer;
  }
  /* Smooth crossfade between Pierre's change-state icon and the green
     check we inject via JS. When the filename is clicked, data-copied
     is set on the SVG for ~1.2s; the original use element fades out
     while the data-gogs-copied-check path fades in, then reverses.
     The transform on the check adds a tiny scale-up bounce. */
  [data-change-icon] {
    transition: transform 180ms ease-out;
  }
  [data-change-icon] > use {
    transition: opacity 180ms ease-out;
    opacity: 1;
  }
  [data-change-icon] > [data-gogs-copied-check] {
    transition: opacity 180ms ease-out, transform 180ms ease-out;
    opacity: 0;
    transform-origin: 8px 8px;
    transform: scale(0.7);
  }
  [data-change-icon][data-copied] > use {
    opacity: 0;
  }
  [data-change-icon][data-copied] > [data-gogs-copied-check] {
    opacity: 1;
    transform: scale(1);
  }
  /* File-to-file separator. The first header has no top border so it does
     not double up with the toolbar's bottom border above it. */
  * + [data-diffs-header] {
    border-top: 1px solid var(--color-border);
  }
  [data-separator=line-info],
  [data-separator=line-info-basic] {
    background-color: var(--diffs-bg-separator) !important;
  }
  /* GitHub-style yellow highlight for the in-page search match. Pierre
     reaches into these custom properties when computing the selected-line
     background and gutter tint; overriding the *-override hook keeps the
     blending logic intact while swapping the source color. */
  :host {
    --diffs-bg-selection-override: light-dark(#ffe066, #ffd633);
    --diffs-bg-selection-number-override: light-dark(#f5c518, #fff066);
  }
`;

function resolveTheme(theme: "light" | "dark" | "system"): "light" | "dark" {
  if (theme === "system") {
    return typeof window !== "undefined" && window.matchMedia("(prefers-color-scheme: dark)").matches
      ? "dark"
      : "light";
  }
  return theme;
}

// CSS variable bridge: pass our app tokens into the @pierre/trees shadow root
// so the tree adopts Gogs' light/dark palette without diverging from the
// surrounding chrome.
const TREE_THEME_STYLE: CSSProperties = {
  // @ts-expect-error -- CSS custom properties are valid in style objects.
  "--trees-fg-override": "var(--color-foreground)",
  "--trees-fg-muted-override": "var(--color-muted-foreground)",
  "--trees-bg-override": "var(--color-background)",
  "--trees-bg-muted-override": "var(--color-surface)",
  "--trees-accent-override": "var(--color-primary)",
  "--trees-border-color-override": "var(--color-border)",
  "--trees-selected-bg-override": "var(--color-surface)",
  "--trees-focus-ring-color-override": "var(--color-ring)",
  // The search input's defaults fall through `--trees-input-bg`, which uses
  // `light-dark()` keyed off the shadow host's own `color-scheme: light dark`
  // — that resolves to the OS preference, not the app's class-based theme,
  // so the box stays light when the page is dark. Pin it to our tokens.
  "--trees-search-bg-override": "var(--color-surface)",
  "--trees-search-fg-override": "var(--color-foreground)",
};

export function RepoCommit() {
  const data = useLoaderData({ from: "/$owner/$repo/commit/$sha" });
  const { owner, repo } = useParams({ from: "/$owner/$repo/commit/$sha" });
  const search = useSearch({ from: "/$owner/$repo/commit/$sha" });
  const navigate = useNavigate({ from: "/$owner/$repo/commit/$sha" });
  const { data: repoInfo } = useSuspenseQuery(repoInfoQuery(owner, repo));
  const { t } = useTranslation();
  const { theme } = useTheme();
  const resolvedTheme = resolveTheme(theme);
  const viewRef = useRef<CodeViewHandle<undefined> | null>(null);
  const treeRef = useRef<CommitFileTreeHandle | null>(null);
  const stickyWorkspaceRef = useRef<HTMLDivElement | null>(null);
  const [copied, setCopied] = useState(false);
  const [mobileTreeOpen, setMobileTreeOpen] = useState(false);
  const [treeSearchOpen, setTreeSearchOpen] = useState(false);
  // Auto-focus Pierre's search input when the user opens it. The input
  // lives in the tree's shadow root, so we focus through the imperative
  // handle the next tick (after the unsafeCSS toggle reveals the row).
  useEffect(() => {
    if (!treeSearchOpen) return;
    const id = window.setTimeout(() => treeRef.current?.focusSearch(), 0);
    return () => window.clearTimeout(id);
  }, [treeSearchOpen]);
  // Desktop tree starts open; the user can collapse it via the sidebar
  // header. The choice persists across navigations within the session.
  const [desktopTreeOpen, setDesktopTreeOpen] = useState<boolean>(() => {
    if (typeof window === "undefined") return true;
    return window.localStorage.getItem("gogs-commit-diff-sidebar-open") !== "false";
  });
  useEffect(() => {
    window.localStorage.setItem("gogs-commit-diff-sidebar-open", desktopTreeOpen ? "true" : "false");
  }, [desktopTreeOpen]);

  // Pierre's `<diffs-container>` swallows wheel events for its own virtual
  // scroller, which means scrolling down inside the diff before the page has
  // reached its locked state leaves the user stuck on the commit metadata.
  // Forward the wheel delta to the document scroller until the page reaches
  // its sticky-lock position, then again when scrolling back up from a
  // diff-top boundary so users can return to the commit metadata.
  useEffect(() => {
    const node = stickyWorkspaceRef.current;
    if (!node) return;
    const workspace = node;
    const desktopMatch = window.matchMedia("(min-width: 1024px)");

    function onWheel(event: WheelEvent) {
      // Desktop-only handler. The lock-and-forward dance was designed for the
      // two-pane workspace pinned to the viewport on `lg+`. On mobile the
      // workspace stacks into a single column and Pierre's container handles
      // wheel events directly — any redirection here breaks trackpad
      // scrolling inside the diff body.
      if (!desktopMatch.matches) return;
      const root = document.scrollingElement ?? document.documentElement;
      const pageMaxScroll = root.scrollHeight - root.clientHeight;
      const pageScroll = root.scrollTop;
      const atLockedState = pageScroll >= pageMaxScroll - 1;
      const dy = event.deltaY;
      if (dy === 0) return;

      // Scrolling down before the page has locked: take over so the page
      // scrolls into the locked state instead of being trapped by Pierre.
      if (dy > 0 && !atLockedState) {
        event.preventDefault();
        window.scrollBy({ top: dy, behavior: "auto" });
        return;
      }

      // Scrolling up while the page is locked and the diff scroller is
      // already at its top: forward the upward scroll to the page so the
      // user can reveal the commit metadata again.
      if (dy < 0 && atLockedState) {
        const diffScroller = workspace.querySelector<HTMLDivElement>(".gogs-diff-scroller");
        if (diffScroller && diffScroller.scrollTop <= 0) {
          event.preventDefault();
          window.scrollBy({ top: dy, behavior: "auto" });
        }
      }
    }

    // `passive: false` is required so `preventDefault()` actually blocks the
    // browser's default scroll handling on Pierre's container.
    node.addEventListener("wheel", onWheel, { passive: false });
    return () => {
      node.removeEventListener("wheel", onWheel);
    };
  }, []);

  // The commit diff page wants edge-to-edge chrome (navbar, repo header,
  // toolbar, footer) instead of the global max-width container. Flag the
  // document so a small CSS override (`FULLWIDTH_CSS`, below) unconstrains
  // the shared containers only while this page is mounted.
  useEffect(() => {
    document.documentElement.setAttribute("data-fullwidth", "commit-diff");
    return () => {
      document.documentElement.removeAttribute("data-fullwidth");
    };
  }, []);
  // Per-file collapse state lives in component state, not the URL: it's
  // keyed by item id (which contains the file's position in the patch),
  // so serializing it would bloat the URL and not be portably shareable.
  const [collapsedById, setCollapsedById] = useState<Record<string, boolean>>({});
  // Per-file expansion state. "loading" while the raw file contents are
  // being fetched, "done" once Pierre has been handed the full file (after
  // which the item is `isPartial: false` and native expansion controls
  // render). Missing key = not yet expanded.
  const [expandedById, setExpandedById] = useState<Record<string, "loading" | "done">>({});
  // Upgraded (non-partial) `FileDiffMetadata` per item id. When set, the
  // `items` useMemo swaps in the upgraded fileDiff so Pierre's controlled
  // CodeView re-renders the file with full file contents.
  const [upgradedById, setUpgradedById] = useState<Record<string, FileDiffMetadata>>({});
  // Per-file "copied filename" feedback. The flag is short-lived (1.2s) and
  // only used to drive a write-side state machine; we don't currently render
  // anything from it, but it gates further copies and keeps the indicator
  // intent explicit if the affordance needs to come back.
  const [, setCopiedPathById] = useState<Record<string, boolean>>({});

  // Derive the in-memory settings from the URL. Missing search fields fall
  // back to defaults, so the URL only carries non-default values.
  const whitespace: WhitespaceMode = search.whitespace ?? "show";
  const settings = useMemo<DiffToolbarSettings>(
    () => ({
      diffStyle: search.style === "split" ? "split" : "unified",
      wrapLines: search.wrap === true,
    }),
    [search.style, search.wrap],
  );

  const setSettings = useCallback(
    (next: DiffToolbarSettings) => {
      void navigate({
        search: (prev) => ({
          ...prev,
          style: next.diffStyle === "split" ? "split" : undefined,
          wrap: next.wrapLines ? true : undefined,
        }),
        resetScroll: false,
      });
    },
    [navigate],
  );

  const onWhitespaceChange = useCallback(
    (next: WhitespaceMode) => {
      // Whitespace is special: the loader re-fetches the patch from the
      // server because `-w` / `-b` happen at `git diff` time. The other
      // toggles are client-only, but all of them still ride the URL.
      void navigate({
        search: (prev) => ({
          ...prev,
          whitespace: next === "show" ? undefined : next,
        }),
        resetScroll: false,
      });
    },
    [navigate],
  );

  const allItems = useMemo<CodeViewItem[]>(
    () =>
      parsePatchFiles(data.patch).flatMap((parsed, patchIndex) =>
        parsed.files.map<CodeViewItem>((fileDiff, fileIndex) => ({
          id: `${patchIndex}:${fileIndex}:${fileDiff.name}`,
          type: "diff",
          fileDiff,
        })),
      ),
    [data.patch],
  );

  // Stamp each item with its current collapse state. Pierre's CodeView caches
  // item records by id and only re-reads their payload (including `collapsed`)
  // when `version` increases, so we encode the collapsed state into the
  // version too.
  const items = useMemo<CodeViewItem[]>(() => {
    return allItems.map((item) => {
      const collapsed = collapsedById[item.id] ?? false;
      const upgraded = item.type === "diff" ? upgradedById[item.id] : undefined;
      const next: CodeViewItem = upgraded != null && item.type === "diff" ? { ...item, fileDiff: upgraded } : item;
      // Bump version when collapsed state OR upgrade state changes so Pierre
      // re-reads the item payload.
      const version = (collapsed ? 1 : 0) + (upgraded != null ? 2 : 0);
      return { ...next, collapsed, version };
    });
  }, [allItems, collapsedById, upgradedById]);

  const stats = useMemo(() => {
    let additions = 0;
    let deletions = 0;
    for (const item of items) {
      if (item.type !== "diff") continue;
      for (const hunk of item.fileDiff.hunks) {
        additions += hunk.additionLines;
        deletions += hunk.deletionLines;
      }
    }
    return { fileCount: items.length, additions, deletions };
  }, [items]);

  const expandAllDiff = useCallback(() => {
    setCollapsedById({});
  }, []);

  const collapseAllDiff = useCallback(() => {
    setCollapsedById(() => {
      const next: Record<string, boolean> = {};
      for (const item of allItems) next[item.id] = true;
      return next;
    });
  }, [allItems]);

  // Pierre doesn't expose a file-header click event, so we delegate clicks
  // ourselves: when a user clicks anywhere on the file header that isn't an
  // interactive child (the kebab menu, etc.), toggle the matching item's
  // collapsed state. The header element has no item id, so we look up the
  // item by file path read from the header's `[data-title] bdi` element.
  const nameToItemIds = useMemo(() => {
    const map = new Map<string, string[]>();
    for (const item of allItems) {
      if (item.type !== "diff") continue;
      const list = map.get(item.fileDiff.name);
      if (list) {
        list.push(item.id);
      } else {
        map.set(item.fileDiff.name, [item.id]);
      }
    }
    return map;
  }, [allItems]);

  // Stable ref to `copyFilePath` so the click delegation effect (which lives
  // above the callback's declaration) always reaches the latest closure.
  const copyFilePathRef = useRef<(id: string, path: string) => void>(() => undefined);
  // Memoize the tooltip label so the effect dependency is stable per locale.
  const titleTooltipLabel = t("diff.copy_file_path");

  useEffect(() => {
    const scroller = document.querySelector<HTMLDivElement>(".gogs-diff-scroller");
    if (!scroller) return;

    function onClick(event: MouseEvent) {
      const target = event.target;
      if (!(target instanceof Element)) return;
      // Skip clicks on Pierre's metadata controls (the collapse chevron is
      // also inside the header but Pierre handles its own toggle behavior).
      // Walk composedPath through any shadow boundaries to find the header.
      const path = event.composedPath();
      const header = path.find(
        (node): node is Element => node instanceof Element && node.matches?.("[data-diffs-header]"),
      );
      if (!header) return;
      // Bail if the click landed on something interactive inside the header
      // (a button, link, or any element whose own listener already handled
      // the click, like our FileHeaderMenu trigger or Pierre's expand
      // chevron). Those nodes appear earlier in `path` than the header.
      let landedOnTitle = false;
      for (const node of path) {
        if (node === header) break;
        if (node instanceof Element) {
          if (node.matches("[data-title]")) {
            landedOnTitle = true;
          }
          if (node.matches("button, a, [role='button']") || node.hasAttribute("data-no-collapse-on-click")) {
            return;
          }
        }
      }
      const title = header.querySelector("[data-title] bdi")?.textContent?.trim();
      if (!title) return;
      const ids = nameToItemIds.get(title);
      if (!ids || ids.length === 0) return;
      // Filename click copies the path; header background click toggles the
      // file's collapsed state. Matches GitHub's behavior.
      if (landedOnTitle) {
        copyFilePathRef.current(ids[0], title);
        // Cross-fade Pierre's change-state icon to a green check for ~1.2s.
        // We append a sibling `<path>` inside the existing SVG and toggle
        // a `data-copied` flag on the SVG; the unsafeCSS below uses
        // `:has`/sibling selectors to crossfade between the two paths via
        // opacity transitions for a smooth swap.
        const icon = header.querySelector<SVGElement>("[data-change-icon]");
        if (icon) {
          icon.setAttribute("data-copied", "");
          window.setTimeout(() => {
            icon.removeAttribute("data-copied");
          }, 1200);
        }
        return;
      }
      setCollapsedById((prev) => {
        const next = { ...prev };
        for (const id of ids) {
          next[id] = !next[id];
        }
        return next;
      });
    }

    // Light-DOM tooltip for the filename. Pierre's [data-title] sits in the
    // shadow DOM and has overflow:hidden, so a Radix tooltip can't reach it
    // and a CSS pseudo-element would be clipped. We render one shared
    // tooltip element in document.body and reposition on hover.
    let tooltipEl = document.querySelector<HTMLDivElement>("[data-gogs-title-tooltip]");
    if (!tooltipEl) {
      tooltipEl = document.createElement("div");
      tooltipEl.setAttribute("data-gogs-title-tooltip", "");
      tooltipEl.setAttribute("role", "tooltip");
      // Match `@/components/ui/tooltip` (Radix): inverted foreground/background,
      // rounded-md, px-2 py-1, text-xs, subtle shadow-sm.
      Object.assign(tooltipEl.style, {
        position: "fixed",
        zIndex: "9999",
        padding: "4px 8px",
        borderRadius: "6px",
        background: "var(--color-foreground)",
        color: "var(--color-background)",
        fontSize: "12px",
        lineHeight: "16px",
        whiteSpace: "nowrap",
        opacity: "0",
        pointerEvents: "none",
        transform: "translate(-50%, 0)",
        transition: "opacity 80ms linear",
        boxShadow: "0 1px 2px 0 rgb(0 0 0 / 0.05)",
      });
      document.body.appendChild(tooltipEl);
    }
    const tooltip = tooltipEl;
    let tooltipShowTimer: number | null = null;
    let currentTitleEl: Element | null = null;
    function showTitleTooltip(titleEl: Element) {
      if (tooltipShowTimer != null) window.clearTimeout(tooltipShowTimer);
      currentTitleEl = titleEl;
      tooltipShowTimer = window.setTimeout(() => {
        if (currentTitleEl !== titleEl) return;
        const rect = titleEl.getBoundingClientRect();
        tooltip.textContent = titleTooltipLabel;
        tooltip.style.left = `${rect.left + Math.min(rect.width / 2, 180)}px`;
        // Render the tooltip ABOVE the filename to match every other Radix
        // tooltip on the page (Radix defaults to side="top"). We measure
        // after assigning text so offsetHeight reflects the final size.
        tooltip.style.opacity = "0";
        tooltip.style.top = `${rect.top - tooltip.offsetHeight - 6}px`;
        tooltip.style.opacity = "1";
      }, 80);
    }
    function hideTitleTooltip() {
      currentTitleEl = null;
      if (tooltipShowTimer != null) {
        window.clearTimeout(tooltipShowTimer);
        tooltipShowTimer = null;
      }
      tooltip.style.opacity = "0";
    }

    // Attach mouseenter/mouseleave directly on each `[data-title]` element.
    // Bubbling-based delegation (`mouseover` on the scroller) is unreliable
    // here because Pierre's `<diffs-container>` aggressively interposes
    // listeners; non-bubbling per-element listeners avoid the indirection.
    const titleListeners = new WeakMap<Element, { enter: () => void; leave: () => void }>();
    function attachTitleListeners(titleEl: Element) {
      if (titleListeners.has(titleEl)) return;
      const enter = () => showTitleTooltip(titleEl);
      const leave = () => {
        if (currentTitleEl === titleEl) hideTitleTooltip();
      };
      titleEl.addEventListener("mouseenter", enter);
      titleEl.addEventListener("mouseleave", leave);
      titleListeners.set(titleEl, { enter, leave });
    }
    // Inject the green-check `<path>` into each change-icon SVG once so the
    // copy-confirmation flash is a CSS opacity swap rather than an
    // innerHTML replace. Pairs with the [data-copied] selectors in
    // DIFF_UNSAFE_CSS below.
    function decorateChangeIcon(icon: SVGElement) {
      if (icon.querySelector("[data-gogs-copied-check]")) return;
      const ns = "http://www.w3.org/2000/svg";
      const check = document.createElementNS(ns, "path");
      check.setAttribute("data-gogs-copied-check", "");
      check.setAttribute("d", "M3.5 8.5 6.5 11.5 12.5 4.5");
      check.setAttribute("fill", "none");
      check.setAttribute("stroke", "var(--color-success)");
      check.setAttribute("stroke-width", "1.8");
      check.setAttribute("stroke-linecap", "round");
      check.setAttribute("stroke-linejoin", "round");
      icon.appendChild(check);
    }
    function scanTitles(root: ParentNode) {
      root.querySelectorAll("[data-title]").forEach((el) => attachTitleListeners(el));
      root.querySelectorAll<SVGElement>("[data-change-icon]").forEach(decorateChangeIcon);
    }
    const container = scroller.querySelector("diffs-container");
    const shadow = (container as (Element & { shadowRoot?: ShadowRoot }) | null)?.shadowRoot;
    if (shadow) {
      scanTitles(shadow);
      const observer = new MutationObserver((records) => {
        for (const record of records) {
          for (const node of record.addedNodes) {
            if (node instanceof Element) {
              if (node.matches("[data-title]")) attachTitleListeners(node);
              if (node.matches("[data-change-icon]")) decorateChangeIcon(node as unknown as SVGElement);
              scanTitles(node);
            }
          }
        }
      });
      observer.observe(shadow, { childList: true, subtree: true });
      scroller.addEventListener("click", onClick);
      return () => {
        scroller.removeEventListener("click", onClick);
        observer.disconnect();
        tooltip.remove();
      };
    }
    scroller.addEventListener("click", onClick);
    return () => {
      scroller.removeEventListener("click", onClick);
      tooltip.remove();
    };
  }, [nameToItemIds, titleTooltipLabel]);

  const { commit } = data;
  const authorLabel = commit.author.userPath ? (
    <a href={commit.author.userPath} className="font-semibold text-(--color-foreground) hover:underline">
      {commit.author.name}
    </a>
  ) : (
    <span className="font-semibold text-(--color-foreground)">{commit.author.name}</span>
  );

  const repoLink = subUrl(`/${owner}/${repo}`);
  const browseFilesHref = `${repoLink}/src/${commit.sha}`;

  // Snap the document scroller to the position where the sticky workspace
  // (toolbar + diff body) is locked to the viewport. We call this before
  // Pierre's scrollTo so the file header's sticky offset math has a stable
  // viewport to work against.
  const scrollPageToLock = useCallback(() => {
    const root = document.scrollingElement ?? document.documentElement;
    const maxScroll = root.scrollHeight - root.clientHeight;
    if (root.scrollTop < maxScroll) {
      root.scrollTo({ top: maxScroll, behavior: "auto" });
    }
  }, []);

  const toggleCollapsed = useCallback((id: string) => {
    setCollapsedById((prev) => ({ ...prev, [id]: !prev[id] }));
  }, []);

  const copyFilePath = useCallback((id: string, filePath: string) => {
    void (async () => {
      try {
        await navigator.clipboard.writeText(filePath);
        setCopiedPathById((prev) => ({ ...prev, [id]: true }));
        window.setTimeout(() => {
          setCopiedPathById((prev) => {
            const next = { ...prev };
            delete next[id];
            return next;
          });
        }, 1200);
      } catch {
        // Clipboard API can fail in insecure contexts. The file name is still
        // visible in the header so the user can copy manually.
      }
    })();
  }, []);

  // Sync the ref the click delegation effect reads.
  useEffect(() => {
    copyFilePathRef.current = copyFilePath;
  }, [copyFilePath]);

  const expandAllLinesFor = useCallback(
    async (item: CodeViewItem) => {
      if (item.type !== "diff") return;
      if (expandedById[item.id]) return;
      const fileDiff = item.fileDiff;
      const parent = commit.parents[0];
      // Added files have no pre-image; deleted files have no post-image.
      // Renames carry the pre-image at `prevName`.
      const prevPath = fileDiff.prevName ?? fileDiff.name;
      const fetchSide = async (sha: string | undefined, p: string) => {
        if (!sha) return "";
        const url = subUrl(`/${owner}/${repo}/raw/${sha}/${p}`);
        const res = await fetch(url, { credentials: "same-origin" });
        if (!res.ok) throw new Error(`raw fetch ${res.status}`);
        return res.text();
      };
      setExpandedById((prev) => ({ ...prev, [item.id]: "loading" }));
      try {
        const [oldContents, newContents] = await Promise.all([
          fileDiff.type === "new" ? Promise.resolve("") : fetchSide(parent, prevPath),
          fileDiff.type === "deleted" ? Promise.resolve("") : fetchSide(commit.sha, fileDiff.name),
        ]);
        const upgraded = parseDiffFromFile(
          { name: prevPath, contents: oldContents },
          { name: fileDiff.name, contents: newContents },
        );
        setUpgradedById((prev) => ({ ...prev, [item.id]: upgraded }));
        setExpandedById((prev) => ({ ...prev, [item.id]: "done" }));
      } catch (err) {
        console.error("expandAllLinesFor: failed", err);
        setExpandedById((prev) => {
          const next = { ...prev };
          delete next[item.id];
          return next;
        });
      }
    },
    [commit.parents, commit.sha, expandedById, owner, repo],
  );

  // Pierre renders our callback's output into a `<slot name="header-prefix">`
  // on the left of each file header (before its file-type icon and name).
  // We only put the collapse chevron here now — clicking the filename copies
  // the path (handled by the shadow-DOM click delegation below), and "Copy
  // file link" lives in the three-dot menu.
  const renderHeaderPrefix = useCallback(
    (item: CodeViewItem) => {
      if (item.type !== "diff") return null;
      const collapsed = collapsedById[item.id] ?? false;
      const Icon = collapsed ? ChevronRight : ChevronDown;
      const label = collapsed ? t("diff.expand_file") : t("diff.collapse_file");
      const buttonClass =
        "grid size-6 cursor-pointer place-items-center rounded text-(--color-muted-foreground) hover:bg-(--color-surface) hover:text-(--color-foreground)";
      return (
        <Tooltip>
          <TooltipTrigger asChild>
            <button
              type="button"
              aria-label={label}
              aria-expanded={!collapsed}
              onPointerDown={(e) => e.stopPropagation()}
              onMouseDown={(e) => e.stopPropagation()}
              onClick={(e) => {
                e.stopPropagation();
                toggleCollapsed(item.id);
              }}
              className={buttonClass}
            >
              <Icon className="size-3.5" aria-hidden />
            </button>
          </TooltipTrigger>
          <TooltipContent>{label}</TooltipContent>
        </Tooltip>
      );
    },
    [collapsedById, t, toggleCollapsed],
  );

  const renderHeaderMetadata = useCallback(
    (item: CodeViewItem) => {
      if (item.type !== "diff") return null;
      const path = item.fileDiff.name;
      const prev = item.fileDiff.prevName;
      const viewFileHref = `${repoLink}/src/${commit.sha}/${path}`;
      const rawFileHref = `${repoLink}/raw/${commit.sha}/${path}`;
      // Gogs' file-history view lives at `/commits/{ref}/{path}`. The ref can
      // be a SHA, so we point at this commit; gogs walks history back from
      // there.
      const historyHref = `${repoLink}/commits/${commit.sha}/${path}`;
      // Edit/Delete are omitted on the commit page: gogs' editor needs a
      // branch ref, and the commit SHA produces 404. The PR diff view (when
      // it lands here) is the right home for those.
      const expandState = expandedById[item.id];
      const supportsExpand = item.fileDiff.type !== "new" && item.fileDiff.type !== "deleted";
      const expandDone = expandState === "done";
      const expandLoading = expandState === "loading";
      const buttonClass =
        "grid size-6 cursor-pointer place-items-center rounded text-(--color-muted-foreground) hover:bg-(--color-surface) hover:text-(--color-foreground) disabled:cursor-not-allowed disabled:opacity-50 disabled:hover:bg-transparent disabled:hover:text-(--color-muted-foreground)";
      return (
        <span className="inline-flex items-center gap-0.5">
          {supportsExpand ? (
            <Tooltip>
              <TooltipTrigger asChild>
                <button
                  type="button"
                  aria-label={expandDone ? t("diff.all_lines_expanded") : t("diff.expand_all_lines")}
                  disabled={expandLoading || expandDone}
                  onPointerDown={(e) => e.stopPropagation()}
                  onMouseDown={(e) => e.stopPropagation()}
                  onClick={(e) => {
                    e.stopPropagation();
                    void expandAllLinesFor(item);
                  }}
                  className={`${buttonClass} hidden lg:grid`}
                  data-no-collapse-on-click
                >
                  {expandLoading ? (
                    <Loader2 className="size-3.5 animate-spin" aria-hidden />
                  ) : (
                    <UnfoldVertical className="size-3.5" aria-hidden />
                  )}
                </button>
              </TooltipTrigger>
              <TooltipContent>{expandDone ? t("diff.all_lines_expanded") : t("diff.expand_all_lines")}</TooltipContent>
            </Tooltip>
          ) : null}
          <FileHeaderMenu
            filePath={path}
            prevFilePath={prev}
            viewFileHref={viewFileHref}
            rawFileHref={rawFileHref}
            historyHref={historyHref}
            onExpandAllLines={supportsExpand ? () => void expandAllLinesFor(item) : undefined}
            expandAllLinesState={expandState}
          />
        </span>
      );
    },
    [commit.sha, expandAllLinesFor, expandedById, repoLink, t],
  );

  const copySha = useCallback(() => {
    void (async () => {
      try {
        await navigator.clipboard.writeText(commit.sha);
        setCopied(true);
        window.setTimeout(() => setCopied(false), 1500);
      } catch {
        // Clipboard API can fail in insecure contexts. The SHA is still
        // visible inline, so silently swallow.
      }
    })();
  }, [commit.sha]);

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <style>{FULLWIDTH_CSS}</style>
      <RepoHeader repo={repoInfo} activeTab="code" />

      <section className="mx-auto w-full max-w-7xl px-4 pt-6 pb-4 sm:px-6">
        <div className="flex flex-wrap items-start justify-between gap-x-4 gap-y-2">
          <h2 className="min-w-0 flex-1 text-xl font-semibold break-words text-(--color-foreground)">
            {commit.summary}
          </h2>
          <div className="basis-full sm:basis-auto">
            <a
              href={browseFilesHref}
              className="inline-flex h-7 shrink-0 items-center gap-1 rounded-md border border-(--color-border) px-2 text-sm hover:bg-(--color-surface)"
            >
              <FileCode2 className="size-3.5" aria-hidden />
              <span>{t("diff.browse_files")}</span>
            </a>
          </div>
        </div>
        {commit.message.trim() ? (
          <pre className="mt-2 max-w-3xl text-sm whitespace-pre-wrap text-(--color-muted-foreground)">
            {commit.message.trim()}
          </pre>
        ) : null}

        <div className="mt-4 flex flex-wrap items-center gap-x-3 gap-y-2 text-sm text-(--color-muted-foreground)">
          <span className="inline-flex items-center gap-1.5">
            <img src={commit.author.avatarURL} alt="" className="size-6 rounded-full" />
            {authorLabel}
            <span>{t("diff.authored")}</span>
            <time dateTime={commit.author.when} title={formatAbsoluteTime(commit.author.when)}>
              {formatRelativeTime(commit.author.when)}
            </time>
          </span>
          {/* TODO: render a "Verified" pill once the backend exposes commit
              signature verification. Hidden for now to avoid claiming
              verification we don't actually perform. */}

          <span aria-hidden className="hidden h-4 w-px bg-(--color-border) sm:inline-block" />

          <span className="inline-flex items-center gap-1 font-mono text-xs">
            <a href={subUrl(`/${owner}/${repo}/commit/${commit.sha}.patch`)} className="hover:underline">
              {t("diff.patch_short")}
            </a>
            <span aria-hidden>·</span>
            <a href={data.rawDiffURL} className="hover:underline">
              {t("diff.diff_short")}
            </a>
            {commit.parents.length > 0 ? (
              <>
                <span aria-hidden>·</span>
                <span>
                  {commit.parents.length > 1 ? `${commit.parents.length} ${t("diff.parents")}` : t("diff.parent")}
                </span>
                {commit.parents.map((p) => (
                  <a
                    key={p}
                    href={`${repoLink}/commit/${p}`}
                    className="rounded bg-(--color-surface) px-1.5 py-0.5 text-(--color-foreground) hover:underline"
                  >
                    {p.slice(0, 7)}
                  </a>
                ))}
              </>
            ) : null}
            <span aria-hidden>·</span>
            <span>{t("diff.commit")}</span>
            <code className="rounded bg-(--color-surface) px-1.5 py-0.5 text-(--color-foreground)">
              {commit.shortSha}
            </code>
            <Tooltip>
              <TooltipTrigger asChild>
                <button
                  type="button"
                  onClick={copySha}
                  aria-label={t("diff.copy_full_sha")}
                  className="grid size-6 cursor-pointer place-items-center rounded hover:bg-(--color-surface)"
                >
                  {copied ? (
                    <Check className="size-3.5 text-(--color-success)" aria-hidden />
                  ) : (
                    <Copy className="size-3.5" aria-hidden />
                  )}
                </button>
              </TooltipTrigger>
              <TooltipContent>{t("diff.copy_full_sha")}</TooltipContent>
            </Tooltip>
          </span>
        </div>
      </section>

      {/* Once the user scrolls past the commit metadata above, this wrapper
          pins to the bottom of the sticky navbar (3.5rem). It contains both
          the toolbar and the tree/diff row, so the entire two-pane workspace
          locks together at that point, same as GitHub's commit page. The
          inner row's height = viewport - navbar - toolbar so it fills the
          remaining space exactly when locked. Constrained to the same
          `max-w-7xl` + horizontal padding as the rest of the page chrome
          so the workspace doesn't span edge-to-edge. */}
      <div
        ref={stickyWorkspaceRef}
        className="sticky top-[3.5rem] z-10 flex h-[calc(100dvh-3.5rem)] min-w-0 flex-col px-4 sm:px-6"
      >
        <DiffToolbar
          stats={stats}
          settings={settings}
          onSettingsChange={setSettings}
          whitespace={whitespace}
          onWhitespaceChange={onWhitespaceChange}
          onExpandAll={expandAllDiff}
          onCollapseAll={collapseAllDiff}
          search={<DiffSearch items={items} viewRef={viewRef} />}
          onShowTreeMobile={() => setMobileTreeOpen(true)}
          onToggleTreeDesktop={() => setDesktopTreeOpen((open) => !open)}
          desktopTreeOpen={desktopTreeOpen}
        />

        <div className="flex min-h-0 min-w-0 flex-1 flex-col lg:flex-row">
          {desktopTreeOpen ? (
            <ResizableSidebar
              storageKey="gogs-commit-diff-sidebar-width"
              defaultWidth={320}
              minWidth={220}
              maxWidth={560}
              className="hidden border-b border-(--color-border) bg-(--color-background) lg:flex lg:border-r lg:border-b-0 lg:border-l"
              style={TREE_THEME_STYLE}
            >
              <div className="flex h-10 shrink-0 items-center justify-between gap-2 border-b border-(--color-border) pr-4 pl-1.5 text-xs">
                <FolderTree className="size-4 shrink-0 text-(--color-muted-foreground)" aria-hidden />
                <span className="inline-flex shrink-0 items-center gap-1">
                  <Tooltip>
                    <TooltipTrigger asChild>
                      <button
                        type="button"
                        onClick={() => setTreeSearchOpen((open) => !open)}
                        aria-label={treeSearchOpen ? t("diff.hide_search") : t("diff.search_files")}
                        aria-pressed={treeSearchOpen}
                        className="grid size-6 cursor-pointer place-items-center rounded text-(--color-muted-foreground) hover:bg-(--color-surface) hover:text-(--color-foreground)"
                      >
                        <Search className="size-3.5" aria-hidden />
                      </button>
                    </TooltipTrigger>
                    <TooltipContent>{treeSearchOpen ? t("diff.hide_search") : t("diff.search_files")}</TooltipContent>
                  </Tooltip>
                  <span className="inline-flex items-stretch overflow-hidden rounded-md border border-(--color-border)">
                    <Tooltip>
                      <TooltipTrigger asChild>
                        <button
                          type="button"
                          onClick={() => treeRef.current?.expandAll()}
                          aria-label={t("diff.expand_all_folders")}
                          className="grid size-6 cursor-pointer place-items-center text-(--color-muted-foreground) hover:bg-(--color-surface) hover:text-(--color-foreground)"
                        >
                          <ChevronsUpDown className="size-3.5" aria-hidden />
                        </button>
                      </TooltipTrigger>
                      <TooltipContent>{t("diff.expand_all_folders")}</TooltipContent>
                    </Tooltip>
                    <Tooltip>
                      <TooltipTrigger asChild>
                        <button
                          type="button"
                          onClick={() => treeRef.current?.collapseAll()}
                          aria-label={t("diff.collapse_all_folders")}
                          className="grid size-6 cursor-pointer place-items-center border-l border-(--color-border) text-(--color-muted-foreground) hover:bg-(--color-surface) hover:text-(--color-foreground)"
                        >
                          <ChevronsDownUp className="size-3.5" aria-hidden />
                        </button>
                      </TooltipTrigger>
                      <TooltipContent>{t("diff.collapse_all_folders")}</TooltipContent>
                    </Tooltip>
                  </span>
                </span>
              </div>
              <CommitFileTree
                ref={treeRef}
                items={items}
                searchOpen={treeSearchOpen}
                onSelectItem={(itemId) => {
                  // Make sure the page is in the locked state before scrolling
                  // inside Pierre, so the viewport layout matches what Pierre
                  // assumes when positioning the file's first line under its
                  // sticky header.
                  scrollPageToLock();
                  viewRef.current?.scrollTo({ type: "item", id: itemId, align: "start", behavior: "smooth" });
                }}
                className="lg:flex-1"
                style={{ height: "100%" }}
              />
            </ResizableSidebar>
          ) : null}

          {/* Mobile-only: a Sheet slide-over presents the same file tree.
              The trigger lives on the toolbar's "Showing N changed files"
              row (see DiffToolbar). The Sheet has no inline trigger of its
              own. */}
          <Sheet open={mobileTreeOpen} onOpenChange={setMobileTreeOpen}>
            <SheetContent
              side="left"
              className="flex w-[85vw] max-w-sm flex-col p-0"
              style={TREE_THEME_STYLE}
              onCloseAutoFocus={(event) => {
                // Don't yank focus back to the trigger. The trigger lives
                // in the diff pane and stealing focus from a just-selected
                // file would defeat the point of the mobile flow.
                event.preventDefault();
              }}
            >
              <SheetTitle className="border-b border-(--color-border) px-3 py-2 text-sm font-semibold">
                Files changed
                <span className="ml-2 rounded-full bg-(--color-surface) px-1.5 text-xs leading-5 tabular-nums text-(--color-muted-foreground)">
                  {stats.fileCount}
                </span>
              </SheetTitle>
              <CommitFileTree
                items={items}
                searchOpen={treeSearchOpen}
                onSelectItem={(itemId) => {
                  scrollPageToLock();
                  viewRef.current?.scrollTo({ type: "item", id: itemId, align: "start", behavior: "smooth" });
                  setMobileTreeOpen(false);
                }}
                className="flex-1"
                style={{ height: "100%" }}
              />
              <SheetClose asChild>
                <button
                  type="button"
                  className="cursor-pointer border-t border-(--color-border) px-3 py-2 text-left text-sm text-(--color-muted-foreground) hover:bg-(--color-surface)"
                >
                  Close
                </button>
              </SheetClose>
            </SheetContent>
          </Sheet>

          <div
            className={`relative min-h-0 min-w-0 flex-1 border-(--color-border) border-x lg:border-r ${
              desktopTreeOpen ? "lg:border-l-0" : "lg:border-l"
            }`}
          >
            <CodeView
              ref={viewRef}
              items={items}
              className="gogs-diff-scroller h-full overflow-auto"
              renderHeaderPrefix={renderHeaderPrefix}
              renderHeaderMetadata={renderHeaderMetadata}
              options={{
                theme: { light: "pierre-light", dark: "pierre-dark" },
                themeType: resolvedTheme,
                diffStyle: settings.diffStyle,
                overflow: settings.wrapLines ? "wrap" : "scroll",
                stickyHeaders: true,
                // No-op for partial files (the patch is all the data we have).
                // Once a file is upgraded via "Expand all lines", Pierre uses
                // this flag to render every context line from the full file.
                expandUnchanged: true,
                unsafeCSS: DIFF_UNSAFE_CSS,
              }}
            />
          </div>
        </div>
      </div>
    </div>
  );
}
