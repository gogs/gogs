import { type ChangeTypes, parsePatchFiles } from "@pierre/diffs";
import { CodeView, type CodeViewHandle, type CodeViewItem } from "@pierre/diffs/react";
import { useLoaderData, useNavigate, useParams, useSearch } from "@tanstack/react-router";
import {
  Check,
  ChevronDown,
  ChevronRight,
  ChevronsDownUp,
  ChevronsUpDown,
  Copy,
  ExternalLink,
  FileCode2,
  PanelLeftOpen,
  ShieldCheck,
} from "lucide-react";
import { type CSSProperties, useCallback, useEffect, useMemo, useRef, useState } from "react";

import { CommitFileTree, type CommitFileTreeHandle } from "@/components/CommitFileTree";
import { DiffSearch } from "@/components/DiffSearch";
import { DiffToolbar, type DiffToolbarSettings, type WhitespaceMode } from "@/components/DiffToolbar";
import { FileHeaderMenu } from "@/components/FileHeaderMenu";
import { RepoHeader, type RepoHeaderRepo } from "@/components/RepoHeader";
import { ResizableSidebar } from "@/components/ResizableSidebar";
import { Sheet, SheetClose, SheetContent, SheetTitle, SheetTrigger } from "@/components/ui/sheet";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { formatAbsoluteTime, formatRelativeTime } from "@/lib/relative-time";
import { useTheme } from "@/lib/theme-context";
import { subUrl } from "@/lib/url";
import { type DiffFileStatus, parseStatusFilter, serializeStatusFilter } from "@/pages/CommitDiff.search";

export interface CommitDiffSignature {
  name: string;
  email: string;
  avatarUrl: string;
  userPath?: string;
  when: string;
}

export interface CommitDiffPage {
  commit: {
    sha: string;
    shortSha: string;
    summary: string;
    message: string;
    author: CommitDiffSignature;
    committer: CommitDiffSignature;
    parents: string[];
  };
  patch: string;
  sourcePath: string;
  rawDiffUrl: string;
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

function diffTypeToFilterKey(t: ChangeTypes): DiffFileStatus {
  switch (t) {
    case "new":
      return "added";
    case "deleted":
      return "deleted";
    case "rename-pure":
    case "rename-changed":
      return "renamed";
    default:
      return "modified";
  }
}

export function CommitDiff() {
  const data = useLoaderData({ from: "/$owner/$repo/_diff/$sha" });
  const { owner, repo } = useParams({ from: "/$owner/$repo/_diff/$sha" });
  const search = useSearch({ from: "/$owner/$repo/_diff/$sha" });
  const navigate = useNavigate({ from: "/$owner/$repo/_diff/$sha" });
  const { theme } = useTheme();
  const resolvedTheme = resolveTheme(theme);
  const viewRef = useRef<CodeViewHandle<undefined> | null>(null);
  const treeRef = useRef<CommitFileTreeHandle | null>(null);
  const stickyWorkspaceRef = useRef<HTMLDivElement | null>(null);
  const [copied, setCopied] = useState(false);
  const [mobileTreeOpen, setMobileTreeOpen] = useState(false);

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

    function onWheel(event: WheelEvent) {
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

  // Derive the in-memory settings from the URL. Missing search fields fall
  // back to defaults, so the URL only carries non-default values.
  const whitespace: WhitespaceMode = search.whitespace ?? "show";
  const settings = useMemo<DiffToolbarSettings>(
    () => ({
      diffStyle: search.style === "split" ? "split" : "unified",
      wrapLines: search.wrap === true,
      statusFilter: parseStatusFilter(search.status),
    }),
    [search.status, search.style, search.wrap],
  );

  const setSettings = useCallback(
    (next: DiffToolbarSettings) => {
      void navigate({
        search: (prev) => ({
          ...prev,
          style: next.diffStyle === "split" ? "split" : undefined,
          wrap: next.wrapLines ? true : undefined,
          status: serializeStatusFilter(next.statusFilter),
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

  // Apply the status filter and stamp each item with its current collapse
  // state. Pierre's CodeView caches item records by id and only re-reads
  // their payload (including `collapsed`) when `version` increases, so we
  // encode the collapsed state into the version too.
  const items = useMemo<CodeViewItem[]>(() => {
    return allItems
      .filter((item) => {
        if (item.type !== "diff") return true;
        return settings.statusFilter[diffTypeToFilterKey(item.fileDiff.type)];
      })
      .map((item) => {
        const collapsed = collapsedById[item.id] ?? false;
        return { ...item, collapsed, version: collapsed ? 1 : 0 };
      });
  }, [allItems, settings.statusFilter, collapsedById]);

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
      for (const node of path) {
        if (node === header) break;
        if (
          node instanceof Element &&
          (node.matches("button, a, [role='button']") || node.hasAttribute("data-no-collapse-on-click"))
        ) {
          return;
        }
      }
      const title = header.querySelector("[data-title] bdi")?.textContent?.trim();
      if (!title) return;
      const ids = nameToItemIds.get(title);
      if (!ids || ids.length === 0) return;
      setCollapsedById((prev) => {
        const next = { ...prev };
        for (const id of ids) {
          next[id] = !next[id];
        }
        return next;
      });
    }

    scroller.addEventListener("click", onClick);
    return () => {
      scroller.removeEventListener("click", onClick);
    };
  }, [nameToItemIds]);

  // Repo header data is mocked until the diff JSON payload includes the repo
  // metadata. Shape matches what the backend will eventually return so the
  // swap is a single field-mapping change.
  const mockRepo: RepoHeaderRepo = {
    owner,
    name: repo,
    avatarUrl: subUrl("/img/favicon.png"),
    visibility: "public",
    isAdmin: true,
    enableIssues: true,
    allowsPulls: true,
    enableWiki: true,
    counts: {
      watchers: 42,
      stars: 1284,
      forks: 96,
      openIssues: 17,
      openPulls: 4,
    },
    mirrorOf: "https://github.com/gogs/gogs",
  };

  const { commit } = data;
  const authorLabel = commit.author.userPath ? (
    <a href={commit.author.userPath} className="font-semibold text-(--color-foreground) hover:underline">
      {commit.author.name}
    </a>
  ) : (
    <span className="font-semibold text-(--color-foreground)">{commit.author.name}</span>
  );

  const committerDiffers = commit.committer.email !== commit.author.email;
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

  // Pierre renders our callback's output into a `<slot name="header-prefix">`
  // on the left of each file header. We use it for a GitHub-style chevron
  // that mirrors the file's collapsed state, separate from the click-anywhere
  // header behavior wired by the scroller click delegation.
  const renderHeaderPrefix = useCallback(
    (item: CodeViewItem) => {
      if (item.type !== "diff") return null;
      const collapsed = collapsedById[item.id] ?? false;
      const Icon = collapsed ? ChevronRight : ChevronDown;
      const label = collapsed ? "Expand file" : "Collapse file";
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
              className="mr-1 grid size-6 cursor-pointer place-items-center rounded border border-(--color-border) bg-(--color-background) text-(--color-muted-foreground) hover:bg-(--color-surface) hover:text-(--color-foreground)"
            >
              <Icon className="size-3.5" aria-hidden />
            </button>
          </TooltipTrigger>
          <TooltipContent>{label}</TooltipContent>
        </Tooltip>
      );
    },
    [collapsedById, toggleCollapsed],
  );

  const renderHeaderMetadata = useCallback(
    (item: CodeViewItem) => {
      if (item.type !== "diff") return null;
      const path = item.fileDiff.name;
      const prev = item.fileDiff.prevName;
      const viewFileHref = `${repoLink}/src/${commit.sha}/${path}`;
      const blameHref = `${repoLink}/blame/${commit.sha}/${path}`;
      const permalinkHref = `${window.location.pathname}#${item.id}`;
      return (
        <FileHeaderMenu
          filePath={path}
          prevFilePath={prev}
          viewFileHref={viewFileHref}
          blameHref={blameHref}
          permalinkHref={permalinkHref}
        />
      );
    },
    [repoLink, commit.sha],
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
      <RepoHeader repo={mockRepo} activeTab="code" />

      <section className="mx-auto w-full max-w-7xl px-4 pt-6 pb-4 sm:px-6">
        <h2 className="text-xl font-semibold break-words text-(--color-foreground)">{commit.summary}</h2>
        {commit.message.trim() ? (
          <pre className="mt-2 max-w-3xl text-sm whitespace-pre-wrap text-(--color-muted-foreground)">
            {commit.message.trim()}
          </pre>
        ) : null}

        <div className="mt-4 flex flex-wrap items-center gap-x-3 gap-y-2 text-sm text-(--color-muted-foreground)">
          <img src={commit.author.avatarUrl} alt="" className="size-6 rounded-full" />
          {authorLabel}
          <span>authored</span>
          <time dateTime={commit.author.when} title={formatAbsoluteTime(commit.author.when)}>
            {formatRelativeTime(commit.author.when)}
          </time>
          {committerDiffers ? (
            <>
              <span aria-hidden>·</span>
              <img src={commit.committer.avatarUrl} alt="" className="size-5 rounded-full" />
              <span>
                committed by{" "}
                {commit.committer.userPath ? (
                  <a href={commit.committer.userPath} className="font-medium text-(--color-foreground) hover:underline">
                    {commit.committer.name}
                  </a>
                ) : (
                  <span className="font-medium text-(--color-foreground)">{commit.committer.name}</span>
                )}
              </span>
            </>
          ) : null}
          <span
            aria-label="Signature verified"
            className="ml-1 inline-flex items-center gap-1 rounded-full border border-(--color-success)/40 px-2 py-0 text-xs leading-5 text-(--color-success)"
          >
            <ShieldCheck className="size-3" aria-hidden />
            Verified
          </span>
        </div>

        <div className="mt-3 flex flex-wrap items-center gap-2 text-xs text-(--color-muted-foreground)">
          {commit.parents.length > 0 ? (
            <span className="inline-flex items-center gap-1">
              <span>{commit.parents.length > 1 ? `${commit.parents.length} parents` : "parent"}</span>
              {commit.parents.map((p, i) => (
                <a
                  key={p}
                  href={`${repoLink}/commit/${p}`}
                  className="rounded bg-(--color-surface) px-1.5 py-0.5 font-mono text-[0.7rem] text-(--color-foreground) hover:underline"
                >
                  {p.slice(0, 7)}
                  {i < commit.parents.length - 1 ? "," : ""}
                </a>
              ))}
            </span>
          ) : null}

          <span aria-hidden className="opacity-50">
            ·
          </span>

          <span className="inline-flex items-center gap-1">
            <span>commit</span>
            <code className="rounded bg-(--color-surface) px-1.5 py-0.5 font-mono text-[0.7rem] text-(--color-foreground)">
              {commit.shortSha}
            </code>
            <button
              type="button"
              onClick={copySha}
              aria-label="Copy full SHA"
              className="grid size-6 cursor-pointer place-items-center rounded hover:bg-(--color-surface)"
            >
              {copied ? (
                <Check className="size-3.5 text-(--color-success)" aria-hidden />
              ) : (
                <Copy className="size-3.5" aria-hidden />
              )}
            </button>
          </span>

          <span className="ml-auto inline-flex items-center gap-2">
            <a
              href={data.rawDiffUrl}
              className="inline-flex h-7 items-center gap-1 rounded-md border border-(--color-border) px-2 hover:bg-(--color-surface)"
            >
              <ExternalLink className="size-3.5" aria-hidden />
              <span>View patch</span>
            </a>
            <a
              href={browseFilesHref}
              className="inline-flex h-7 items-center gap-1 rounded-md border border-(--color-border) px-2 hover:bg-(--color-surface)"
            >
              <FileCode2 className="size-3.5" aria-hidden />
              <span>Browse files</span>
            </a>
          </span>
        </div>
      </section>

      {/* Once the user scrolls past the commit metadata above, this wrapper
          pins to the bottom of the sticky navbar (3.5rem). It contains both
          the toolbar and the tree/diff row, so the entire two-pane workspace
          locks together at that point, same as GitHub's commit page. The
          inner row's height = viewport - navbar - toolbar so it fills the
          remaining space exactly when locked. */}
      <div ref={stickyWorkspaceRef} className="sticky top-[3.5rem] z-10 flex h-[calc(100dvh-3.5rem)] flex-col">
        <DiffToolbar
          stats={stats}
          settings={settings}
          onSettingsChange={setSettings}
          whitespace={whitespace}
          onWhitespaceChange={onWhitespaceChange}
          onExpandAll={expandAllDiff}
          onCollapseAll={collapseAllDiff}
        />

        <div className="flex min-h-0 flex-1 flex-col lg:flex-row">
          <ResizableSidebar
            storageKey="gogs-commit-diff-sidebar-width"
            defaultWidth={320}
            minWidth={220}
            maxWidth={560}
            className="hidden border-b border-(--color-border) bg-(--color-background) lg:flex lg:border-r lg:border-b-0"
            style={TREE_THEME_STYLE}
          >
            <div className="flex h-10 shrink-0 items-center justify-between gap-2 border-b border-(--color-border) px-3 text-xs">
              <span className="inline-flex min-w-0 items-center gap-1.5 font-semibold text-(--color-foreground)">
                <ChevronRight className="size-3.5 shrink-0" aria-hidden />
                <span className="truncate">Files changed</span>
                <span className="rounded-full bg-(--color-surface) px-1.5 leading-5 tabular-nums text-(--color-muted-foreground)">
                  {stats.fileCount}
                </span>
              </span>
              <span className="inline-flex shrink-0 items-stretch overflow-hidden rounded-md border border-(--color-border)">
                <button
                  type="button"
                  onClick={() => treeRef.current?.expandAll()}
                  aria-label="Expand all folders"
                  className="grid size-6 cursor-pointer place-items-center text-(--color-muted-foreground) hover:bg-(--color-surface) hover:text-(--color-foreground)"
                >
                  <ChevronsUpDown className="size-3.5" aria-hidden />
                </button>
                <button
                  type="button"
                  onClick={() => treeRef.current?.collapseAll()}
                  aria-label="Collapse all folders"
                  className="grid size-6 cursor-pointer place-items-center border-l border-(--color-border) text-(--color-muted-foreground) hover:bg-(--color-surface) hover:text-(--color-foreground)"
                >
                  <ChevronsDownUp className="size-3.5" aria-hidden />
                </button>
              </span>
            </div>
            <CommitFileTree
              ref={treeRef}
              items={items}
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

          {/* Mobile-only: a Sheet slide-over presents the same file tree.
              The trigger button is rendered inside the diff pane below `lg`
              because the desktop sidebar is hidden at that breakpoint. */}
          <Sheet open={mobileTreeOpen} onOpenChange={setMobileTreeOpen}>
            <SheetTrigger asChild>
              <button
                type="button"
                className="inline-flex h-9 shrink-0 cursor-pointer items-center gap-2 border-b border-(--color-border) bg-(--color-background) px-4 text-sm font-medium text-(--color-foreground) hover:bg-(--color-surface) lg:hidden"
              >
                <PanelLeftOpen className="size-4" aria-hidden />
                <span>Files changed</span>
                <span className="rounded-full bg-(--color-surface) px-1.5 leading-5 tabular-nums text-(--color-muted-foreground)">
                  {stats.fileCount}
                </span>
              </button>
            </SheetTrigger>
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

          <div className="relative min-h-0 flex-1">
            <DiffSearch items={items} viewRef={viewRef} />
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
                unsafeCSS: DIFF_UNSAFE_CSS,
              }}
            />
          </div>
        </div>
      </div>
    </div>
  );
}
