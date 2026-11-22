import {
  type CodeViewOptions,
  type DiffLineAnnotation,
  type FileDiffContentsLoader,
  type FileDiffMetadata,
  type SelectedLineRange,
  parsePatchFiles,
} from "@pierre/diffs";
import { CodeView, type CodeViewAnnotationArgs, type CodeViewHandle, type CodeViewItem } from "@pierre/diffs/react";
import { useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import {
  Check,
  ChevronDown,
  ChevronRight,
  ChevronsDownUp,
  ChevronsUpDown,
  Copy,
  FileCode2,
  FileText,
  FolderTree,
  LoaderCircle,
  Search,
  UnfoldVertical,
  X,
} from "lucide-react";
import { type ReactNode, useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";

import { DiffFileTree, type DiffFileTreeHandle } from "@/components/DiffFileTree";
import { DiffImage } from "@/components/DiffImage";
import { DiffSearch } from "@/components/DiffSearch";
import { DiffToolbar, type DiffToolbarSettings, type WhitespaceMode } from "@/components/DiffToolbar";
import { FileHeaderMenu } from "@/components/FileHeaderMenu";
import { ResizableSidebar } from "@/components/ResizableSidebar";
import { RichDiff } from "@/components/RichDiff";
import { Sheet, SheetClose, SheetContent, SheetTitle } from "@/components/ui/sheet";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { refPath, repoMarkdownFileQuery } from "@/lib/queries/repo";
import { resolveTheme, useTheme } from "@/lib/theme-context";
import { TREE_THEME_STYLE } from "@/lib/tree-theme";
import { subUrl } from "@/lib/url";
import type { RepoCommitSearch } from "@/pages/repo/Commit.search";

// The data-fullwidth attribute widens the page chrome, see index.css. These
// rules only apply to the diff workspace.
const FULLWIDTH_CSS = `
  /* Pierre inlines 8px top/bottom margin on the virtual scroll container. */
  html[data-fullwidth="diff"] .gogs-diff-scroller > div {
    margin-top: 0 !important;
    margin-bottom: 0 !important;
  }
  /* A footer below the locked workspace occupies the bottom of the viewport. */
  html[data-fullwidth="diff"] footer {
    display: none;
  }
`;

const DIFF_UNSAFE_CSS = `
  /* Pierre draws a 1px border on every side. Top and left sit against the
     toolbar and sidebar borders, so those edges look 2px thick. */
  :host {
    border-top: 0 !important;
    border-left: 0 !important;
  }
  [data-diffs-header] {
    background: color-mix(in lab, var(--diffs-bg) 96%, var(--diffs-mixer));
    border-bottom: 1px solid color-mix(in lab, var(--diffs-bg) 85%, var(--diffs-mixer));
  }
  /* Pierre defaults this gap to 8px. */
  [data-header-content] {
    gap: 4px !important;
  }
  [data-change-icon] {
    margin-right: 4px;
  }
  /* Pierre inherits a mono font from the diff body. These counts are chrome. */
  [data-additions-count],
  [data-deletions-count] {
    font-family: inherit;
  }
  /* Image diffs have no lines to count. Only they and rich diffs, which hide
     their counts anyway, render a line 0 annotation. */
  [data-diffs-header]:has(~ pre [data-line-annotation="-1,-1"]) :is([data-additions-count], [data-deletions-count]) {
    display: none;
  }
  /* Skip the first header so it does not double the toolbar's bottom border. */
  * + [data-diffs-header] {
    border-top: 1px solid var(--color-border);
  }
  [data-separator=line-info],
  [data-separator=line-info-basic] {
    background-color: var(--diffs-bg-separator) !important;
  }
  /* A rich diff has no lines, so Pierre counts none. The header metadata slot
     shows the counts of its source diff instead. */
  :host([data-rich-diff]) [data-additions-count],
  :host([data-rich-diff]) [data-deletions-count] {
    display: none;
  }
  /* In split view, Pierre lays out a changed file in two columns. The rich
     diff is on the additions side, so it takes the full width instead. */
  :host([data-rich-diff]) [data-diff-type=split] {
    grid-template-columns: 1fr;
  }
  :host([data-rich-diff]) [data-diff-type=split][data-overflow=wrap] {
    grid-template-columns: var(--diffs-code-grid);
  }
  :host([data-rich-diff]) [data-diff-type=split] [data-deletions] {
    display: none;
  }
  :host([data-rich-diff]) [data-diff-type=split][data-overflow=wrap] [data-additions] {
    & [data-gutter] {
      grid-column: 1;
    }
    & [data-content] {
      grid-column: 2;
    }
  }
  /* Pierre handles context expansion on this span, so an empty label must
     still fill the separator's clickable area. */
  [data-unmodified-lines]:empty {
    flex: 1;
    align-self: stretch;
  }
  /* Override Pierre's selected-line hook so blending stays intact. */
  :host {
    --diffs-bg-selection-override: light-dark(#ffe066, #ffd633);
    --diffs-bg-selection-number-override: light-dark(#f5c518, #fff066);
    /* Pierre's blue number on the yellow selection is below 4.5:1 in dark mode. */
    --diffs-selection-number-fg: var(--diffs-fg);
  }
`;

// richFileDiffs are the line-less copies of file diffs that show a rich diff
// in their place. A WeakSet keeps a copy recognizable wherever Pierre hands
// it back, without holding on to it.
const richFileDiffs = new WeakSet<FileDiffMetadata>();

// richFileDiffOf returns the line-less copy of the file diff, made once, which
// renders only the file-level annotation that holds the rich diff.
const richCopies = new WeakMap<FileDiffMetadata, FileDiffMetadata>();
function richFileDiffOf(fileDiff: FileDiffMetadata): FileDiffMetadata {
  let copy = richCopies.get(fileDiff);
  if (!copy) {
    copy = { ...fileDiff, hunks: [], splitLineCount: 0, unifiedLineCount: 0 };
    richCopies.set(fileDiff, copy);
    richFileDiffs.add(copy);
  }
  return copy;
}

// Extensions whose content type the raw file endpoint detects, so browsers
// render them despite `X-Content-Type-Options: nosniff`.
const IMAGE_EXTENSIONS = new Set(["png", "jpg", "jpeg", "gif", "webp", "bmp", "ico"]);

function isImagePath(path: string): boolean {
  const dot = path.lastIndexOf(".");
  return dot >= 0 && IMAGE_EXTENSIONS.has(path.slice(dot + 1).toLowerCase());
}

// A binary image diff has no hunks. Pierre renders an annotation on line 0
// above a file's lines, in the column of its side, and measures its height, so
// one annotation per existing side shows the old and new images.
function imageAnnotations(fileDiff: FileDiffMetadata): DiffLineAnnotation<DiffAnnotation>[] | undefined {
  if (fileDiff.hunks.length > 0 || !isImagePath(fileDiff.name)) return undefined;
  const annotations: DiffLineAnnotation<DiffAnnotation>[] = [];
  if (fileDiff.type !== "new") annotations.push({ side: "deletions", lineNumber: 0, metadata: { kind: "image" } });
  if (fileDiff.type !== "deleted") annotations.push({ side: "additions", lineNumber: 0, metadata: { kind: "image" } });
  return annotations;
}

// DiffAnnotation is what shows below a line of a file: the rich diff of the
// whole file, one side of an image diff, a thread of comments, or the form to
// start one.
type DiffAnnotation = { kind: "rich" } | { kind: "image" } | { kind: "thread"; id: number } | { kind: "composer" };

// DiffLineTarget is the lines of a file that a new thread is on. Sides are
// "old" or "new", and line is the last line of the range.
export interface DiffLineTarget {
  path: string;
  // oldPath is the path at the base of a renamed file.
  oldPath?: string;
  side: "old" | "new";
  startSide: "old" | "new";
  startLine: number;
  line: number;
}

// DiffReview shows threads of comments below the lines they are on, and lets
// the viewer start new ones from the gutter.
export interface DiffReview {
  threads: { id: number; path: string; side: "old" | "new"; line: number }[];
  renderThread: (id: number) => ReactNode;
  // renderComposer is unset when the viewer cannot comment.
  renderComposer?: (target: DiffLineTarget, onClose: () => void) => ReactNode;
  // actions show in the toolbar, e.g., to finish a review.
  actions?: ReactNode;
}

const toAnnotationSide = (side: "old" | "new") => (side === "old" ? "deletions" : "additions");

// hashString returns a number that changes with the string, small enough to
// leave room for flags below it.
function hashString(s: string) {
  let h = 0;
  for (let i = 0; i < s.length; i++) h = (h * 31 + s.charCodeAt(i)) % 0x7ffffff;
  return h;
}

// DiffWorkspace shows the files a patch changes, between the old and the new
// revision, with a tree of the files beside them. It locks to the viewport
// below the content above it, and widens the page to the full width. The
// view settings live in the search params of the page.
export function DiffWorkspace({
  owner,
  repo,
  patch,
  oldRef,
  newRef,
  markdownExtensions,
  search,
  review,
}: {
  owner: string;
  repo: string;
  patch: string;
  // oldRef is undefined for a root commit, which has nothing before it.
  oldRef: string | undefined;
  newRef: string;
  // markdownExtensions are the extensions of files that can show a rich diff.
  markdownExtensions: string[];
  search: RepoCommitSearch;
  review?: DiffReview;
}) {
  const navigate = useNavigate();
  const [composer, setComposer] = useState<{ itemId: string; target: DiffLineTarget } | null>(null);
  const { t } = useTranslation();
  const { theme } = useTheme();
  const resolvedTheme = resolveTheme(theme);
  const viewRef = useRef<CodeViewHandle<DiffAnnotation, undefined> | null>(null);
  const treeRef = useRef<DiffFileTreeHandle | null>(null);
  const mobileTreeRef = useRef<DiffFileTreeHandle | null>(null);
  const stickyWorkspaceRef = useRef<HTMLDivElement | null>(null);
  const [mobileTreeOpen, setMobileTreeOpen] = useState(false);
  const [treeSearchOpen, setTreeSearchOpen] = useState(false);
  // The search input lives in the tree's shadow root. Focus it the next
  // tick, after the unsafeCSS toggle reveals the row.
  useEffect(() => {
    if (!treeSearchOpen) return;
    const id = window.setTimeout(() => treeRef.current?.focusSearch(), 0);
    return () => window.clearTimeout(id);
  }, [treeSearchOpen]);
  // Persist only on toggle so a future default still applies to users who
  // never changed it.
  const [desktopTreeOpen, setDesktopTreeOpen] = useState<boolean>(() => {
    if (typeof window === "undefined") return true;
    return window.localStorage.getItem("gogs-file-tree-open") !== "false";
  });
  const toggleDesktopTree = useCallback(() => {
    setDesktopTreeOpen((prev) => {
      const next = !prev;
      window.localStorage.setItem("gogs-file-tree-open", next ? "true" : "false");
      return next;
    });
  }, []);

  // Whether threads show below their lines. Like the file tree, it is kept
  // only once changed.
  const [commentsShown, setCommentsShown] = useState<boolean>(() => {
    if (typeof window === "undefined") return true;
    return window.localStorage.getItem("gogs-diff-comments-shown") !== "false";
  });
  const changeCommentsShown = useCallback((next: boolean) => {
    window.localStorage.setItem("gogs-diff-comments-shown", next ? "true" : "false");
    setCommentsShown(next);
  }, []);

  // Pierre's container swallows wheel events for its virtual scroller, which
  // traps the page on the content above the workspace until we forward them.
  useEffect(() => {
    const node = stickyWorkspaceRef.current;
    if (!node) return;
    const workspace = node;
    const desktopMatch = window.matchMedia("(min-width: 1024px)");

    function onWheel(event: WheelEvent) {
      // On mobile the workspace is a single column and forwarding breaks
      // trackpad scrolling inside the diff body.
      if (!desktopMatch.matches) return;
      const root = document.scrollingElement ?? document.documentElement;
      const pageMaxScroll = root.scrollHeight - root.clientHeight;
      const pageScroll = root.scrollTop;
      const atLockedState = pageScroll >= pageMaxScroll - 1;
      const dy = event.deltaY;
      if (dy === 0) return;

      if (dy > 0 && !atLockedState) {
        event.preventDefault();
        window.scrollBy({ top: dy, behavior: "auto" });
        return;
      }

      if (dy < 0 && atLockedState) {
        const diffScroller = workspace.querySelector<HTMLDivElement>(".gogs-diff-scroller");
        if (diffScroller && diffScroller.scrollTop <= 0) {
          event.preventDefault();
          window.scrollBy({ top: dy, behavior: "auto" });
        }
      }
    }

    // `passive: false` is required for `preventDefault()` to stop Pierre's
    // default scroll handling.
    node.addEventListener("wheel", onWheel, { passive: false });
    return () => {
      node.removeEventListener("wheel", onWheel);
    };
  }, []);

  useEffect(() => {
    document.documentElement.setAttribute("data-fullwidth", "diff");
    return () => {
      document.documentElement.removeAttribute("data-fullwidth");
    };
  }, []);
  // Collapse is keyed by item id (path plus patch position), so putting it
  // in the URL would bloat it and would not be portably shareable.
  const [collapsedById, setCollapsedById] = useState<Record<string, boolean>>({});
  const [richById, setRichById] = useState<Record<string, boolean>>({});
  // loadingRichId is the item whose rich diff is being rendered before it
  // shows, so the source diff stays in place meanwhile.
  const [loadingRichId, setLoadingRichId] = useState<string>();
  const queryClient = useQueryClient();
  const [copiedPathById, setCopiedPathById] = useState<Record<string, boolean>>({});
  // Key by metadata identity so a replacement patch starts with expansion
  // actions available, even when it reuses the same file IDs.
  const [fullyExpandedDiffs, setFullyExpandedDiffs] = useState<Set<FileDiffMetadata>>(() => new Set());
  const expansionCheckLine = useRef(new WeakMap<FileDiffMetadata, number>());
  const onDiffPostRender = useCallback<NonNullable<CodeViewOptions<DiffAnnotation, undefined>["onPostRender"]>>(
    (node, _instance, _phase, context) => {
      node.toggleAttribute("data-rich-diff", context.type === "diff" && richFileDiffs.has(context.item.fileDiff));
      if (context.type !== "diff" || context.item.collapsed) return;
      const { fileDiff } = context.item;
      if (fileDiff.isPartial) return;
      const checkedLine = expansionCheckLine.current.get(fileDiff);
      if (checkedLine != null && checkedLine > fileDiff.additionLines.length) return;
      let line = checkedLine ?? 1;
      // Context only expands within a patch. Resume at the first hidden line
      // rather than rescanning already verified lines on every virtual render.
      while (line <= fileDiff.additionLines.length && context.instance.isLineRenderable(line)) line++;
      expansionCheckLine.current.set(fileDiff, line);
      if (line > fileDiff.additionLines.length) {
        setFullyExpandedDiffs((prev) => new Set(prev).add(fileDiff));
      }
    },
    [],
  );

  // The URL stores only non-default values.
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
        to: ".",
        search: (prev: Record<string, unknown>) => ({
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
      // Whitespace changes refetch the patch because `-w` / `-b` are applied
      // at `git diff` time. The other toggles are client-only.
      void navigate({
        to: ".",
        search: (prev: Record<string, unknown>) => ({
          ...prev,
          whitespace: next === "show" ? undefined : next,
        }),
        resetScroll: false,
      });
    },
    [navigate],
  );

  const allItems = useMemo<CodeViewItem<DiffAnnotation>[]>(
    () =>
      parsePatchFiles(patch).flatMap((parsed, patchIndex) =>
        parsed.files.map<CodeViewItem<DiffAnnotation>>((fileDiff, fileIndex) => ({
          id: `${patchIndex}:${fileIndex}:${fileDiff.name}`,
          type: "diff",
          fileDiff,
          annotations: imageAnnotations(fileDiff),
        })),
      ),
    [patch],
  );

  const isMarkdown = useCallback(
    (item: CodeViewItem<DiffAnnotation>) =>
      item.type === "diff" &&
      markdownExtensions.some((ext) => item.fileDiff.name.toLowerCase().endsWith(ext.toLowerCase())),
    [markdownExtensions],
  );

  // Pierre caches items by id and only re-reads `collapsed` when `version`
  // increases. Line expansion is Pierre-owned after `loadDiffFiles` hydrates.
  // A rich diff replaces the lines with a file-level annotation, on the side
  // that has the file.
  const items = useMemo<CodeViewItem<DiffAnnotation>[]>(() => {
    return allItems.map((item) => {
      const collapsed = collapsedById[item.id] ?? false;
      const rich = item.type === "diff" && richById[item.id] === true;
      if (item.type !== "diff") return item;
      const annotations: DiffLineAnnotation<DiffAnnotation>[] = [];
      if (rich) {
        annotations.push({
          side: item.fileDiff.type === "deleted" ? "deletions" : "additions",
          lineNumber: 0,
          metadata: { kind: "rich" },
        });
      } else {
        annotations.push(...(item.annotations ?? []));
        for (const thread of commentsShown ? (review?.threads ?? []) : []) {
          if (thread.path !== item.fileDiff.name) continue;
          annotations.push({
            side: toAnnotationSide(thread.side),
            lineNumber: thread.line,
            metadata: { kind: "thread", id: thread.id },
          });
        }
        if (composer?.itemId === item.id) {
          annotations.push({
            side: toAnnotationSide(composer.target.side),
            lineNumber: composer.target.line,
            metadata: { kind: "composer" },
          });
        }
      }
      // Pierre re-reads the annotations of an item when its version changes.
      const version = (collapsed ? 1 : 0) + (rich ? 2 : 0) + 4 * hashString(JSON.stringify(annotations));
      return {
        ...item,
        collapsed,
        version,
        ...(rich ? { fileDiff: richFileDiffOf(item.fileDiff) } : {}),
        annotations,
      };
    });
  }, [allItems, collapsedById, richById, review?.threads, commentsShown, composer]);

  // The gutter button starts a thread on the line it is on, or on the
  // selected lines.
  const onGutterUtilityClick = useCallback<
    NonNullable<CodeViewOptions<DiffAnnotation, undefined>["onGutterUtilityClick"]>
  >((range: SelectedLineRange, context?: { type: string; item: CodeViewItem<DiffAnnotation> }) => {
    if (context?.item.type !== "diff") return;
    const toSide = (side?: "deletions" | "additions"): "old" | "new" => (side === "deletions" ? "old" : "new");
    let start = { side: toSide(range.side), line: range.start };
    let end = { side: toSide(range.endSide ?? range.side), line: range.end };
    // The old side shows first, and lines go down within a side.
    if ((start.side === end.side && start.line > end.line) || (start.side === "new" && end.side === "old")) {
      [start, end] = [end, start];
    }
    setComposer({
      itemId: context.item.id,
      target: {
        path: context.item.fileDiff.name,
        oldPath: context.item.fileDiff.prevName,
        side: end.side,
        startSide: start.side,
        startLine: start.line,
        line: end.line,
      },
    });
  }, []);

  // Counted on the source diffs, so showing a rich diff leaves them unchanged.
  const lineCounts = useMemo(() => {
    const counts = new Map<string, { additions: number; deletions: number }>();
    for (const item of allItems) {
      if (item.type !== "diff") continue;
      let additions = 0;
      let deletions = 0;
      for (const hunk of item.fileDiff.hunks) {
        additions += hunk.additionLines;
        deletions += hunk.deletionLines;
      }
      counts.set(item.id, { additions, deletions });
    }
    return counts;
  }, [allItems]);

  const stats = useMemo(() => {
    let additions = 0;
    let deletions = 0;
    for (const count of lineCounts.values()) {
      additions += count.additions;
      deletions += count.deletions;
    }
    return { fileCount: allItems.length, additions, deletions };
  }, [allItems, lineCounts]);

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

  // Pierre has no file-header click event and no item id on the header.
  // Look up the file by `[data-title] bdi` and toggle it ourselves.
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
      // `composedPath` crosses shadow boundaries.
      const path = event.composedPath();
      const header = path.find(
        (node): node is Element => node instanceof Element && node.matches?.("[data-diffs-header]"),
      );
      if (!header) return;
      // Interactive children appear earlier in `path` than the header.
      for (const node of path) {
        if (node === header) break;
        if (node instanceof Element) {
          if (node.matches("button, a, [role='button']") || node.hasAttribute("data-no-collapse-on-click")) {
            return;
          }
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

  // Lock the page first so Pierre's sticky header offset math has a stable
  // viewport.
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

  const copyFilePath = useCallback(
    (id: string, filePath: string) => {
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
          toast.error(t("copy_failed"));
        }
      })();
    },
    [t],
  );

  const fetchRawFile = useCallback(
    async (ref: string | undefined, filePath: string) => {
      if (!ref) throw new Error("raw fetch: missing ref");
      // Encode per segment so `#` / `?` in a name do not truncate the URL,
      // while `/` stays a path separator.
      const encodedPath = filePath.split("/").map(encodeURIComponent).join("/");
      const url = subUrl(`/${owner}/${repo}/raw/${ref}/${encodedPath}`);
      const res = await fetch(url, { credentials: "same-origin" });
      if (!res.ok) throw new Error(`raw fetch ${res.status}`);
      const contents = await res.text();
      return { name: filePath, contents };
    },
    [owner, repo],
  );

  // Pierre calls this once per file on first context expand, then reveals
  // further lines from the hydrated in-memory contents.
  const loadDiffFiles = useCallback<FileDiffContentsLoader>(
    async (fileDiff) => {
      const parent = oldRef;
      const prevPath = fileDiff.prevName ?? fileDiff.name;
      const [oldFile, newFile] = await Promise.all([
        fileDiff.type === "rename-pure" ? Promise.resolve(null) : fetchRawFile(parent, prevPath),
        fetchRawFile(newRef, fileDiff.name),
      ]);
      return { oldFile, newFile };
    },
    [oldRef, newRef, fetchRawFile],
  );

  // Pierre slot: `header-prefix`, left of the file-type icon.
  const renderHeaderPrefix = useCallback(
    (item: CodeViewItem<DiffAnnotation>) => {
      if (item.type !== "diff") return null;
      const collapsed = collapsedById[item.id] ?? false;
      const Icon = collapsed ? ChevronRight : ChevronDown;
      const label = collapsed ? t("repo.diff.expand_file") : t("repo.diff.collapse_file");
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

  // Each collapsed gap is the `fromStart` side of an expansion region: the
  // gap before hunk `i` is index `i`, and the gap after the last hunk is
  // `hunks.length`. Those regions read the `up` direction.
  // `Number.POSITIVE_INFINITY` is Pierre's "expand all" value. Re-expanding
  // an open gap is a no-op.
  const expandAllLinesFor = useCallback((item: CodeViewItem<DiffAnnotation>) => {
    if (item.type !== "diff") return;
    const rendered = viewRef.current
      ?.getInstance()
      ?.getRenderedItems()
      .find((r) => r.id === item.id);
    if (rendered?.type !== "diff") return;
    for (let i = 0; i <= item.fileDiff.hunks.length; i++) {
      rendered.instance.expandHunk(i, "up", Number.POSITIVE_INFINITY);
    }
  }, []);

  // The versions a rich diff compares. An added file has no old version, and
  // a deleted file no new one.
  const richQueriesOf = useCallback(
    (fileDiff: FileDiffMetadata) => [
      repoMarkdownFileQuery(
        owner,
        repo,
        fileDiff.type === "new" ? undefined : oldRef,
        fileDiff.prevName ?? fileDiff.name,
      ),
      repoMarkdownFileQuery(owner, repo, fileDiff.type === "deleted" ? undefined : newRef, fileDiff.name),
    ],
    [owner, repo, oldRef, newRef],
  );

  const prefetchRich = useCallback(
    (fileDiff: FileDiffMetadata) =>
      Promise.all(richQueriesOf(fileDiff).map((query) => queryClient.prefetchQuery(query))),
    [queryClient, richQueriesOf],
  );

  // Switching to a rich diff waits for it to render, so the file never shows
  // a short loading state between two tall views.
  const toggleRich = useCallback(
    (item: CodeViewItem<DiffAnnotation>) => {
      if (item.type !== "diff") return;
      if (richById[item.id]) {
        setRichById((prev) => ({ ...prev, [item.id]: false }));
        return;
      }
      setLoadingRichId(item.id);
      void prefetchRich(item.fileDiff).then(() => {
        setLoadingRichId((id) => (id === item.id ? undefined : id));
        setRichById((prev) => ({ ...prev, [item.id]: true }));
      });
    },
    [prefetchRich, richById],
  );

  // Pierre slot: `header-filename-suffix`, immediately after the filename.
  const renderHeaderFilenameSuffix = useCallback(
    (item: CodeViewItem<DiffAnnotation>) => {
      if (item.type !== "diff") return null;
      const path = item.fileDiff.name;
      const justCopied = copiedPathById[item.id] === true;
      const rich = richById[item.id] === true;
      const richLabel = rich ? t("repo.diff.show_source_diff") : t("repo.diff.show_rich_diff");
      const richLoading = loadingRichId === item.id;
      const RichIcon = richLoading ? LoaderCircle : rich ? FileCode2 : FileText;
      // New, deleted, and pure-rename files have no collapsed context.
      const canExpand =
        !rich &&
        item.fileDiff.type !== "new" &&
        item.fileDiff.type !== "deleted" &&
        item.fileDiff.type !== "rename-pure" &&
        !fullyExpandedDiffs.has(item.fileDiff);
      const buttonClass =
        "grid size-6 cursor-pointer place-items-center rounded text-(--color-muted-foreground) hover:bg-(--color-surface) hover:text-(--color-foreground)";
      return (
        <span className="inline-flex items-center gap-0.5">
          <Tooltip>
            <TooltipTrigger asChild>
              <button
                type="button"
                aria-label={t("repo.copy_file_path")}
                onPointerDown={(e) => e.stopPropagation()}
                onMouseDown={(e) => e.stopPropagation()}
                onClick={(e) => {
                  e.stopPropagation();
                  copyFilePath(item.id, path);
                }}
                className={buttonClass}
                data-no-collapse-on-click
              >
                {justCopied ? (
                  <Check className="size-3.5 text-(--color-success)" aria-hidden />
                ) : (
                  <Copy className="size-3.5" aria-hidden />
                )}
              </button>
            </TooltipTrigger>
            <TooltipContent>{t("repo.copy_file_path")}</TooltipContent>
          </Tooltip>
          {isMarkdown(item) ? (
            <Tooltip>
              <TooltipTrigger asChild>
                <button
                  type="button"
                  aria-label={richLabel}
                  aria-pressed={rich}
                  aria-busy={richLoading || undefined}
                  onPointerEnter={() => void prefetchRich(item.fileDiff)}
                  onFocus={() => void prefetchRich(item.fileDiff)}
                  onPointerDown={(e) => e.stopPropagation()}
                  onMouseDown={(e) => e.stopPropagation()}
                  onClick={(e) => {
                    e.stopPropagation();
                    toggleRich(item);
                  }}
                  className={buttonClass}
                  data-no-collapse-on-click
                >
                  <RichIcon className={richLoading ? "size-3.5 motion-safe:animate-spin" : "size-3.5"} aria-hidden />
                </button>
              </TooltipTrigger>
              <TooltipContent>{richLabel}</TooltipContent>
            </Tooltip>
          ) : null}
          {canExpand ? (
            <Tooltip>
              <TooltipTrigger asChild>
                <button
                  type="button"
                  aria-label={t("repo.diff.expand_all_lines")}
                  onPointerDown={(e) => e.stopPropagation()}
                  onMouseDown={(e) => e.stopPropagation()}
                  onClick={(e) => {
                    e.stopPropagation();
                    expandAllLinesFor(item);
                  }}
                  className={buttonClass}
                  data-no-collapse-on-click
                >
                  <UnfoldVertical className="size-3.5" aria-hidden />
                </button>
              </TooltipTrigger>
              <TooltipContent>{t("repo.diff.expand_all_lines")}</TooltipContent>
            </Tooltip>
          ) : null}
        </span>
      );
    },
    [
      copiedPathById,
      copyFilePath,
      expandAllLinesFor,
      fullyExpandedDiffs,
      isMarkdown,
      loadingRichId,
      prefetchRich,
      richById,
      t,
      toggleRich,
    ],
  );

  const renderHeaderMetadata = useCallback(
    (item: CodeViewItem<DiffAnnotation>) => {
      if (item.type !== "diff") return null;
      const path = item.fileDiff.name;
      const prev = item.fileDiff.prevName;
      const viewFileHref = `/${owner}/${repo}/src/${refPath(`${newRef}/${path}`)}`;
      const rawFileHref = subUrl(`/${owner}/${repo}/raw/${newRef}/${path}`);
      // File history is `/commits/{ref}/{path}`. A commit SHA is a valid ref.
      const historyHref = `/${owner}/${repo}/commits/${refPath(`${newRef}/${path}`)}`;
      // The menu leaves out Edit/Delete, which need a branch ref. A commit SHA
      // 404s in the editor.
      const counts = richById[item.id] ? lineCounts.get(item.id) : undefined;
      return (
        <span className="inline-flex items-center gap-[1ch]">
          {counts ? (
            <>
              {counts.deletions > 0 || counts.additions === 0 ? (
                <span className="text-(--color-diff-removed)">-{counts.deletions}</span>
              ) : null}
              {counts.additions > 0 || counts.deletions === 0 ? (
                <span className="text-(--color-diff-added)">+{counts.additions}</span>
              ) : null}
            </>
          ) : null}
          <FileHeaderMenu
            filePath={path}
            prevFilePath={prev}
            viewFileHref={viewFileHref}
            rawFileHref={rawFileHref}
            historyHref={historyHref}
          />
        </span>
      );
    },
    [owner, repo, newRef, richById, lineCounts],
  );

  // A file-level annotation holds either the rich diff of an item that shows
  // it, or one side of an image diff. The threads and the form to start one
  // show below their lines.
  const renderAnnotation = useCallback(
    (...[annotation, fileDiff]: CodeViewAnnotationArgs<DiffAnnotation>) => {
      const metadata = annotation.metadata;
      if (metadata?.kind === "thread")
        return <div className="font-sans leading-normal whitespace-normal">{review?.renderThread(metadata.id)}</div>;
      if (metadata?.kind === "composer") {
        if (!composer || !review?.renderComposer) return null;
        return (
          <div className="font-sans leading-normal whitespace-normal">
            {review.renderComposer(composer.target, () => setComposer(null))}
          </div>
        );
      }
      if (!("hunks" in fileDiff)) return null;
      if (richFileDiffs.has(fileDiff)) {
        const [before, after] = richQueriesOf(fileDiff);
        return <RichDiff queries={[before, after]} />;
      }
      if (!("side" in annotation)) return null;
      const isOld = annotation.side === "deletions";
      const ref = isOld ? oldRef : newRef;
      const path = isOld ? (fileDiff.prevName ?? fileDiff.name) : fileDiff.name;
      if (!ref) return null;
      const encodedPath = path.split("/").map(encodeURIComponent).join("/");
      return <DiffImage src={subUrl(`/${owner}/${repo}/raw/${ref}/${encodedPath}`)} side={annotation.side} />;
    },
    [owner, repo, oldRef, newRef, richQueriesOf, review, composer],
  );

  return (
    <>
      <style>{FULLWIDTH_CSS}</style>
      {/* 3.5rem is the sticky navbar height. Toolbar and tree/diff lock as one. */}
      <div
        ref={stickyWorkspaceRef}
        className="sticky top-[calc(3.5rem+1px)] z-10 flex h-[calc(100dvh-3.5rem-1px)] min-w-0 flex-col px-4 sm:px-6"
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
          onToggleTreeDesktop={toggleDesktopTree}
          desktopTreeOpen={desktopTreeOpen}
          actions={review?.actions}
          comments={review ? { shown: commentsShown, onChange: changeCommentsShown } : undefined}
        />

        <div className="glass flex min-h-0 min-w-0 flex-1 flex-col lg:flex-row glass-ui:mb-4 glass-ui:overflow-hidden glass-ui:border">
          {desktopTreeOpen ? (
            <ResizableSidebar
              defaultWidth={320}
              minWidth={220}
              maxWidth={560}
              className="hidden border-b border-(--color-border) bg-(--color-background) lg:flex lg:border-r lg:border-b-0 lg:border-l glass-ui:bg-transparent glass-ui:lg:border-l-0"
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
                        aria-label={treeSearchOpen ? t("repo.search_hide") : t("repo.search_files")}
                        aria-pressed={treeSearchOpen}
                        className="grid size-6 cursor-pointer place-items-center rounded text-(--color-muted-foreground) hover:bg-(--color-surface) hover:text-(--color-foreground)"
                      >
                        <Search className="size-3.5" aria-hidden />
                      </button>
                    </TooltipTrigger>
                    <TooltipContent>{treeSearchOpen ? t("repo.search_hide") : t("repo.search_files")}</TooltipContent>
                  </Tooltip>
                  <span className="inline-flex items-stretch overflow-hidden rounded-md glass-control border border-(--color-border)">
                    <Tooltip>
                      <TooltipTrigger asChild>
                        <button
                          type="button"
                          onClick={() => treeRef.current?.expandAll()}
                          aria-label={t("repo.expand_all_directories")}
                          className="grid size-6 cursor-pointer place-items-center text-(--color-muted-foreground) hover:bg-(--color-surface) hover:text-(--color-foreground)"
                        >
                          <ChevronsUpDown className="size-3.5" aria-hidden />
                        </button>
                      </TooltipTrigger>
                      <TooltipContent>{t("repo.expand_all_directories")}</TooltipContent>
                    </Tooltip>
                    <Tooltip>
                      <TooltipTrigger asChild>
                        <button
                          type="button"
                          onClick={() => treeRef.current?.collapseAll()}
                          aria-label={t("repo.collapse_all_directories")}
                          className="grid size-6 cursor-pointer place-items-center border-l border-(--color-border) text-(--color-muted-foreground) hover:bg-(--color-surface) hover:text-(--color-foreground)"
                        >
                          <ChevronsDownUp className="size-3.5" aria-hidden />
                        </button>
                      </TooltipTrigger>
                      <TooltipContent>{t("repo.collapse_all_directories")}</TooltipContent>
                    </Tooltip>
                  </span>
                </span>
              </div>
              <DiffFileTree
                ref={treeRef}
                items={items}
                searchOpen={treeSearchOpen}
                onSelectItem={(itemId) => {
                  scrollPageToLock();
                  // `align: "start"` clips the header border and first line by a few pixels.
                  viewRef.current?.scrollTo({
                    type: "item",
                    id: itemId,
                    align: "start",
                    offset: 8,
                    behavior: "smooth",
                  });
                }}
                className="lg:flex-1"
                style={{ height: "100%" }}
              />
            </ResizableSidebar>
          ) : null}

          {/* Trigger lives on the toolbar. This Sheet has no inline trigger. */}
          <Sheet open={mobileTreeOpen} onOpenChange={setMobileTreeOpen}>
            <SheetContent
              side="left"
              className="flex w-[85vw] max-w-sm flex-col p-0"
              hideCloseButton
              style={TREE_THEME_STYLE}
              onOpenAutoFocus={(event) => {
                // Radix would focus the first button and open its tooltip.
                event.preventDefault();
              }}
              onCloseAutoFocus={(event) => {
                // The trigger is in the diff pane. Restoring focus would
                // steal it from the selected file.
                event.preventDefault();
              }}
            >
              <SheetTitle className="flex items-center justify-between border-b border-(--color-border) py-2 pl-4 pr-3 text-sm font-semibold">
                <span className="inline-flex items-center text-(--color-muted-foreground)">
                  <FolderTree className="size-4 shrink-0" aria-hidden />
                  <span className="sr-only">{t("repo.show_file_tree")}</span>
                </span>
                <span className="inline-flex items-center gap-1">
                  <span className="inline-flex items-stretch overflow-hidden rounded-md glass-control border border-(--color-border)">
                    <Tooltip>
                      <TooltipTrigger asChild>
                        <button
                          type="button"
                          onClick={() => mobileTreeRef.current?.expandAll()}
                          aria-label={t("repo.expand_all_directories")}
                          className="grid size-7 cursor-pointer place-items-center text-(--color-muted-foreground) hover:bg-(--color-surface) hover:text-(--color-foreground)"
                        >
                          <ChevronsUpDown className="size-3.5" aria-hidden />
                        </button>
                      </TooltipTrigger>
                      <TooltipContent>{t("repo.expand_all_directories")}</TooltipContent>
                    </Tooltip>
                    <Tooltip>
                      <TooltipTrigger asChild>
                        <button
                          type="button"
                          onClick={() => mobileTreeRef.current?.collapseAll()}
                          aria-label={t("repo.collapse_all_directories")}
                          className="grid size-7 cursor-pointer place-items-center border-l border-(--color-border) text-(--color-muted-foreground) hover:bg-(--color-surface) hover:text-(--color-foreground)"
                        >
                          <ChevronsDownUp className="size-3.5" aria-hidden />
                        </button>
                      </TooltipTrigger>
                      <TooltipContent>{t("repo.collapse_all_directories")}</TooltipContent>
                    </Tooltip>
                  </span>
                  <SheetClose asChild>
                    <button
                      type="button"
                      aria-label={t("close")}
                      className="grid size-7 cursor-pointer place-items-center rounded-md text-(--color-muted-foreground) hover:bg-(--color-surface) hover:text-(--color-foreground)"
                    >
                      <X className="size-4" aria-hidden />
                    </button>
                  </SheetClose>
                </span>
              </SheetTitle>
              <DiffFileTree
                ref={mobileTreeRef}
                items={items}
                searchOpen
                onSelectItem={(itemId) => {
                  scrollPageToLock();
                  // `align: "start"` clips the header border and first line by a few pixels.
                  viewRef.current?.scrollTo({
                    type: "item",
                    id: itemId,
                    align: "start",
                    offset: 8,
                    behavior: "smooth",
                  });
                  setMobileTreeOpen(false);
                }}
                className="flex-1"
                style={{ height: "100%" }}
              />
            </SheetContent>
          </Sheet>

          <div
            className={`relative min-h-0 min-w-0 flex-1 border-(--color-border) border-x lg:border-r glass-ui:border-x-0! ${
              desktopTreeOpen ? "lg:border-l-0" : "lg:border-l"
            }`}
          >
            <CodeView
              ref={viewRef}
              items={items}
              className="gogs-diff-scroller h-full overflow-auto"
              renderHeaderPrefix={renderHeaderPrefix}
              renderHeaderFilenameSuffix={renderHeaderFilenameSuffix}
              renderHeaderMetadata={renderHeaderMetadata}
              renderAnnotation={renderAnnotation}
              options={{
                theme: { light: "pierre-light", dark: "pierre-dark" },
                themeType: resolvedTheme,
                diffStyle: settings.diffStyle,
                overflow: settings.wrapLines ? "wrap" : "scroll",
                stickyHeaders: true,
                hunkSeparators: "line-info",
                // Reveal at most 100 lines per click. Larger gaps get up/down plus expand-all.
                expansionLineCount: 100,
                loadDiffFiles,
                onPostRender: onDiffPostRender,
                unsafeCSS: DIFF_UNSAFE_CSS,
                enableGutterUtility: !!review?.renderComposer,
                enableLineSelection: !!review?.renderComposer,
                onGutterUtilityClick: review?.renderComposer ? onGutterUtilityClick : undefined,
              }}
            />
          </div>
        </div>
      </div>
    </>
  );
}
