import {
  type CodeViewOptions,
  type FileDiffContentsLoader,
  type FileDiffMetadata,
  parsePatchFiles,
} from "@pierre/diffs";
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
  Search,
  UnfoldVertical,
  X,
} from "lucide-react";
import { type CSSProperties, useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";

import { DiffFileTree, type DiffFileTreeHandle } from "@/components/DiffFileTree";
import { DiffSearch } from "@/components/DiffSearch";
import { DiffToolbar, type DiffToolbarSettings, type WhitespaceMode } from "@/components/DiffToolbar";
import { FileHeaderMenu } from "@/components/FileHeaderMenu";
import { RepoHeader } from "@/components/RepoHeader";
import { ResizableSidebar } from "@/components/ResizableSidebar";
import { Sheet, SheetClose, SheetContent, SheetTitle } from "@/components/ui/sheet";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { repoHeaderQuery } from "@/lib/queries/repo";
import { formatAbsoluteTime, formatRelativeTime } from "@/lib/relative-time";
import { useTheme } from "@/lib/theme-context";
import { subUrl } from "@/lib/url";

import type { RepoCommitSearch } from "./Commit.search";

export interface RepoCommitSignature {
  name: string;
  email: string;
  avatarURL: string;
  profileURL?: string;
  when: string;
}

export interface RepoCommitPage {
  sha: string;
  subject: string;
  body: string;
  author: RepoCommitSignature;
  parents: string[];
  patch: string;
}

// Page-local override of the unique max-width utilities on Navbar, Footer,
// and RepoHeader. Brittle if those classes change, but keeps the override
// in one file.
const FULLWIDTH_CSS = `
  html[data-fullwidth="commit-diff"] .max-w-6xl,
  html[data-fullwidth="commit-diff"] .max-w-7xl {
    max-width: none;
  }
  /* Pierre inlines 8px top/bottom margin on the virtual scroll container. */
  html[data-fullwidth="commit-diff"] .gogs-diff-scroller > div {
    margin-top: 0 !important;
    margin-bottom: 0 !important;
  }
  /* A footer below the locked workspace occupies the bottom of the viewport. */
  html[data-fullwidth="commit-diff"] footer {
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
  /* Skip the first header so it does not double the toolbar's bottom border. */
  * + [data-diffs-header] {
    border-top: 1px solid var(--color-border);
  }
  [data-separator=line-info],
  [data-separator=line-info-basic] {
    background-color: var(--diffs-bg-separator) !important;
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

// Bridge app tokens into the @pierre/trees shadow root.
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
  // `--trees-input-bg` uses `light-dark()` off the shadow host's
  // `color-scheme`, which follows the OS, not the app's class-based theme.
  "--trees-search-bg-override": "var(--color-surface)",
  "--trees-search-fg-override": "var(--color-foreground)",
};

const BODY_CLAMP_LINES = 3;

function CommitBody({ body }: { body: string }) {
  const { t } = useTranslation();
  const lines = body.split("\n");
  const needsClamp = lines.length > BODY_CLAMP_LINES;
  const [expanded, setExpanded] = useState(!needsClamp);
  const visible = expanded ? body : lines.slice(0, BODY_CLAMP_LINES).join("\n");

  return (
    <div className="mt-2 max-w-3xl">
      <pre className="text-sm whitespace-pre-wrap text-(--color-muted-foreground)">{visible}</pre>
      {needsClamp ? (
        <button
          type="button"
          onClick={() => setExpanded((e) => !e)}
          className="mt-1.5 cursor-pointer rounded border border-(--color-border) bg-(--color-surface) px-2 py-0.5 text-xs text-(--color-muted-foreground) hover:bg-(--color-surface)/80 hover:text-(--color-foreground)"
        >
          {expanded ? t("show_less") : t("show_more")}
        </button>
      ) : null}
    </div>
  );
}

export function RepoCommit() {
  const data = useLoaderData({ from: "/$owner/$repo/commit/$sha" });
  const { sha, subject, body, author, parents, patch } = data;
  const { owner, repo } = useParams({ from: "/$owner/$repo/commit/$sha" });
  const search: RepoCommitSearch = useSearch({ from: "/$owner/$repo/commit/$sha" });
  const navigate = useNavigate({ from: "/$owner/$repo/commit/$sha" });
  const { data: repoHeader } = useSuspenseQuery(repoHeaderQuery(owner, repo));
  const { t } = useTranslation();
  const { theme } = useTheme();
  const resolvedTheme = resolveTheme(theme);
  const viewRef = useRef<CodeViewHandle<undefined, undefined> | null>(null);
  const treeRef = useRef<DiffFileTreeHandle | null>(null);
  const mobileTreeRef = useRef<DiffFileTreeHandle | null>(null);
  const stickyWorkspaceRef = useRef<HTMLDivElement | null>(null);
  const [copied, setCopied] = useState(false);
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

  // Pierre's container swallows wheel events for its virtual scroller, which
  // traps the page on the commit metadata until we forward them.
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
    document.documentElement.setAttribute("data-fullwidth", "commit-diff");
    return () => {
      document.documentElement.removeAttribute("data-fullwidth");
    };
  }, []);
  // Collapse is keyed by item id (path plus patch position), so putting it
  // in the URL would bloat it and would not be portably shareable.
  const [collapsedById, setCollapsedById] = useState<Record<string, boolean>>({});
  const [copiedPathById, setCopiedPathById] = useState<Record<string, boolean>>({});
  // Key by metadata identity so a replacement patch starts with expansion
  // actions available, even when it reuses the same file IDs.
  const [fullyExpandedDiffs, setFullyExpandedDiffs] = useState<Set<FileDiffMetadata>>(() => new Set());
  const expansionCheckLine = useRef(new WeakMap<FileDiffMetadata, number>());
  const onDiffPostRender = useCallback<NonNullable<CodeViewOptions<undefined, undefined>["onPostRender"]>>(
    (_node, _instance, _phase, context) => {
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
        search: (prev: RepoCommitSearch) => ({
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
        search: (prev: RepoCommitSearch) => ({
          ...prev,
          whitespace: next === "show" ? undefined : next,
        }),
        resetScroll: false,
      });
    },
    [navigate],
  );

  const allItems = useMemo<CodeViewItem<undefined>[]>(
    () =>
      parsePatchFiles(patch).flatMap((parsed, patchIndex) =>
        parsed.files.map<CodeViewItem<undefined>>((fileDiff, fileIndex) => ({
          id: `${patchIndex}:${fileIndex}:${fileDiff.name}`,
          type: "diff",
          fileDiff,
        })),
      ),
    [patch],
  );

  // Pierre caches items by id and only re-reads `collapsed` when `version`
  // increases. Line expansion is Pierre-owned after `loadDiffFiles` hydrates.
  const items = useMemo<CodeViewItem<undefined>[]>(() => {
    return allItems.map((item) => {
      const collapsed = collapsedById[item.id] ?? false;
      const version = collapsed ? 1 : 0;
      return { ...item, collapsed, version };
    });
  }, [allItems, collapsedById]);

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

  // Pierre hardcodes separator labels in shadow DOM with no render slot.
  // Re-run on mutations because expansion rebuilds separators.
  useEffect(() => {
    const found = document.querySelector<HTMLDivElement>(".gogs-diff-scroller");
    if (!found) return;
    const root: ParentNode = found;

    // Match Pierre's English source strings, then replace with a localized value.
    const unmodifiedLinesRe = /^(\d+) unmodified lines?$/;
    const moreContextText = "More unchanged context may be available";
    const noNewlineText = "No newline at end of file";

    function setText(el: HTMLElement, text: string) {
      if (el.textContent !== text) el.textContent = text;
    }

    function localizeIn(root: ParentNode) {
      for (const span of root.querySelectorAll<HTMLElement>("[data-unmodified-lines]")) {
        const text = span.textContent ?? "";
        const match = unmodifiedLinesRe.exec(text);
        if (match) {
          const count = Number(match[1]);
          setText(span, t(count === 1 ? "repo.diff.unmodified_line" : "repo.diff.unmodified_lines", { count }));
        } else if (text === moreContextText) {
          setText(span, "");
        }
      }
      for (const span of root.querySelectorAll<HTMLElement>("[data-no-newline] span")) {
        if ((span.textContent ?? "") === noNewlineText) setText(span, t("repo.diff.no_newline_at_eof"));
      }
    }

    // MutationObserver does not cross shadow boundaries. Pierre nests each
    // file's body in a shadow root (sometimes deeper) and rebuilds separators
    // on expand, so we observe each root as we find it.
    const observed = new WeakSet<ShadowRoot>();
    const observer = new MutationObserver(schedule);

    function localizeDeep(node: ParentNode) {
      localizeIn(node);
      for (const el of node.querySelectorAll<HTMLElement>("*")) {
        const shadow = el.shadowRoot;
        if (!shadow) continue;
        if (!observed.has(shadow)) {
          observed.add(shadow);
          observer.observe(shadow, { childList: true, subtree: true, characterData: true });
        }
        localizeDeep(shadow);
      }
    }

    let frame = 0;
    function schedule() {
      if (frame !== 0) return;
      frame = window.requestAnimationFrame(() => {
        frame = 0;
        localizeDeep(root);
      });
    }

    observer.observe(root, { childList: true, subtree: true, characterData: true });
    schedule();
    return () => {
      observer.disconnect();
      if (frame !== 0) window.cancelAnimationFrame(frame);
    };
  }, [t]);

  const authorLabel = author.profileURL ? (
    <a href={author.profileURL} className="font-semibold text-(--color-foreground) hover:underline">
      {author.name}
    </a>
  ) : (
    <span className="font-semibold text-(--color-foreground)">{author.name}</span>
  );

  const repoLink = subUrl(`/${owner}/${repo}`);
  const browseFilesHref = `${repoLink}/src/${sha}`;

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
      const parent = parents[0];
      const prevPath = fileDiff.prevName ?? fileDiff.name;
      const [oldFile, newFile] = await Promise.all([
        fileDiff.type === "rename-pure" ? Promise.resolve(null) : fetchRawFile(parent, prevPath),
        fetchRawFile(sha, fileDiff.name),
      ]);
      return { oldFile, newFile };
    },
    [parents, sha, fetchRawFile],
  );

  // Pierre slot: `header-prefix`, left of the file-type icon.
  const renderHeaderPrefix = useCallback(
    (item: CodeViewItem<undefined>) => {
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
  const expandAllLinesFor = useCallback((item: CodeViewItem<undefined>) => {
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

  // Pierre slot: `header-filename-suffix`, immediately after the filename.
  const renderHeaderFilenameSuffix = useCallback(
    (item: CodeViewItem<undefined>) => {
      if (item.type !== "diff") return null;
      const path = item.fileDiff.name;
      const justCopied = copiedPathById[item.id] === true;
      // New, deleted, and pure-rename files have no collapsed context.
      const canExpand =
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
    [copiedPathById, copyFilePath, expandAllLinesFor, fullyExpandedDiffs, t],
  );

  const renderHeaderMetadata = useCallback(
    (item: CodeViewItem<undefined>) => {
      if (item.type !== "diff") return null;
      const path = item.fileDiff.name;
      const prev = item.fileDiff.prevName;
      const viewFileHref = `${repoLink}/src/${sha}/${path}`;
      const rawFileHref = `${repoLink}/raw/${sha}/${path}`;
      // File history is `/commits/{ref}/{path}`. A commit SHA is a valid ref.
      const historyHref = `${repoLink}/commits/${sha}/${path}`;
      // Edit/Delete need a branch ref. A commit SHA 404s in the editor.
      return (
        <FileHeaderMenu
          filePath={path}
          prevFilePath={prev}
          viewFileHref={viewFileHref}
          rawFileHref={rawFileHref}
          historyHref={historyHref}
        />
      );
    },
    [repoLink, sha],
  );

  const copySha = useCallback(() => {
    void (async () => {
      try {
        await navigator.clipboard.writeText(sha);
        setCopied(true);
        window.setTimeout(() => setCopied(false), 1500);
      } catch {
        toast.error(t("copy_failed"));
      }
    })();
  }, [sha, t]);

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <style>{FULLWIDTH_CSS}</style>
      <RepoHeader repo={repoHeader} activeTab="code" />

      <section className="mx-auto w-full max-w-7xl px-4 pt-6 pb-4 sm:px-6">
        <div className="flex flex-wrap items-start justify-between gap-x-4 gap-y-2">
          <h2 className="min-w-0 flex-1 text-xl font-semibold break-words text-(--color-foreground)">{subject}</h2>
          <div className="basis-full sm:basis-auto">
            <a
              href={browseFilesHref}
              className="inline-flex h-7 shrink-0 items-center gap-1 rounded-md border border-(--color-border) px-2 text-sm hover:bg-(--color-surface)"
            >
              <FileCode2 className="size-3.5" aria-hidden />
              <span>{t("repo.browse_files")}</span>
            </a>
          </div>
        </div>
        {(body ?? "").trim() ? <CommitBody body={(body ?? "").trim()} /> : null}

        <div className="mt-4 flex flex-wrap items-center gap-x-3 gap-y-2 text-sm text-(--color-muted-foreground)">
          <span className="inline-flex items-center gap-1.5">
            <img src={author.avatarURL} alt="" className="size-6 rounded-full" />
            {authorLabel}
            <span>{t("repo.authored")}</span>
            <Tooltip>
              <TooltipTrigger asChild>
                <time dateTime={author.when}>{formatRelativeTime(t, author.when)}</time>
              </TooltipTrigger>
              <TooltipContent>{formatAbsoluteTime(author.when)}</TooltipContent>
            </Tooltip>
          </span>
          {/* TODO: Verified pill once the backend exposes signature verification. */}

          <span aria-hidden className="hidden h-4 w-px bg-(--color-border) sm:inline-block" />

          <span className="inline-flex items-center gap-1 font-mono text-xs">
            <a href={subUrl(`/${owner}/${repo}/commit/${sha}.patch`)} className="hover:underline">
              {t("repo.patch_label")}
            </a>
            <span aria-hidden>·</span>
            <a href={subUrl(`/${owner}/${repo}/commit/${sha}.diff`)} className="hover:underline">
              {t("repo.diff_label")}
            </a>
            {parents.length > 0 ? (
              <>
                <span aria-hidden>·</span>
                <span>{parents.length > 1 ? `${parents.length} ${t("repo.parents")}` : t("repo.commit_parent")}</span>
                {parents.map((p) => (
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
            <span>{t("repo.commit_label")}</span>
            <code className="rounded bg-(--color-surface) px-1.5 py-0.5 text-(--color-foreground)">
              {sha.slice(0, 10)}
            </code>
            <Tooltip>
              <TooltipTrigger asChild>
                <button
                  type="button"
                  onClick={copySha}
                  aria-label={t("repo.copy_full_sha")}
                  className="grid size-6 cursor-pointer place-items-center rounded hover:bg-(--color-surface)"
                >
                  {copied ? (
                    <Check className="size-3.5 text-(--color-success)" aria-hidden />
                  ) : (
                    <Copy className="size-3.5" aria-hidden />
                  )}
                </button>
              </TooltipTrigger>
              <TooltipContent>{t("repo.copy_full_sha")}</TooltipContent>
            </Tooltip>
          </span>
        </div>
      </section>

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
        />

        <div className="flex min-h-0 min-w-0 flex-1 flex-col lg:flex-row">
          {desktopTreeOpen ? (
            <ResizableSidebar
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
                        aria-label={treeSearchOpen ? t("repo.search_hide") : t("repo.search_files")}
                        aria-pressed={treeSearchOpen}
                        className="grid size-6 cursor-pointer place-items-center rounded text-(--color-muted-foreground) hover:bg-(--color-surface) hover:text-(--color-foreground)"
                      >
                        <Search className="size-3.5" aria-hidden />
                      </button>
                    </TooltipTrigger>
                    <TooltipContent>{treeSearchOpen ? t("repo.search_hide") : t("repo.search_files")}</TooltipContent>
                  </Tooltip>
                  <span className="inline-flex items-stretch overflow-hidden rounded-md border border-(--color-border)">
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
                  <span className="inline-flex items-stretch overflow-hidden rounded-md border border-(--color-border)">
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
            className={`relative min-h-0 min-w-0 flex-1 border-(--color-border) border-x lg:border-r ${
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
              }}
            />
          </div>
        </div>
      </div>
    </div>
  );
}
