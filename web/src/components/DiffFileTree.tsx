import type { ChangeTypes } from "@pierre/diffs";
import type { CodeViewItem } from "@pierre/diffs/react";
import type {
  FileTreeDirectoryHandle,
  FileTreeIcons,
  FileTreeItemHandle,
  GitStatus,
  GitStatusEntry,
} from "@pierre/trees";
import { FileTree, useFileTree, useFileTreeSearch } from "@pierre/trees/react";
import {
  type CSSProperties,
  type ReactNode,
  forwardRef,
  useCallback,
  useEffect,
  useImperativeHandle,
  useMemo,
  useRef,
} from "react";

interface Props {
  items: readonly CodeViewItem[];
  // Selecting a row in the tree fires this with the CodeViewItem id so the
  // page can drive the diff view's scroll. The callback abstraction lets us
  // avoid plumbing the diff view's `LAnnotation` generic through forwardRef.
  onSelectItem: (itemId: string) => void;
  // Show or hide Pierre's built-in search row. The model itself stays around,
  // so toggling does not lose tree state.
  searchOpen?: boolean;
  header?: ReactNode;
  className?: string;
  style?: CSSProperties;
}

// Pierre's FileTree renders into a shadow root. We can inject CSS via the
// `unsafeCSS` option, and gate the search row's visibility off a host
// attribute we set via the spread `data-*` prop on `<FileTree>`. That gives
// us a CSS-only toggle without remounting the tree.
const TREE_UNSAFE_CSS = `
  :host([data-search-open="false"]) [data-file-tree-search-input],
  :host([data-search-open="false"]) [data-file-tree-search-input]
    ~ *:not([data-file-tree-list]):not([role="tree"]) {
    display: none !important;
  }
  /* The search input lives inside a wrapper row; hide that whole row when
     closed so we don't leave an empty band of padding. */
  :host([data-search-open="false"]) [data-file-tree-search] {
    display: none !important;
  }
  [data-file-tree-search],
  [data-file-tree-search-input] {
    margin-top: 6px;
  }
`;

export interface DiffFileTreeHandle {
  expandAll(): void;
  collapseAll(): void;
  focusSearch(): void;
}

// Pierre's icon sets ship with file-type glyphs and chevrons. `standard`
// covers most common file types. `colored` adds the semantic per-type tint
// that makes 50+ files in a list scannable.
const ICON_OPTIONS: FileTreeIcons = { set: "standard", colored: true };

// Map `@pierre/diffs` change types onto the tree's git status vocabulary.
// `rename-pure` and `rename-changed` both surface as `renamed` (the tree only
// uses status for the colored row marker).
function diffTypeToStatus(t: ChangeTypes): GitStatus | null {
  switch (t) {
    case "new":
      return "added";
    case "deleted":
      return "deleted";
    case "change":
      return "modified";
    case "rename-pure":
    case "rename-changed":
      return "renamed";
    default:
      return null;
  }
}

// `isDirectory()` is declared `boolean` on the union base, so TS won't
// narrow `FileTreeItemHandle` to `FileTreeDirectoryHandle` from a call-site
// check alone. This guard does the discrimination explicitly.
function asDirectoryHandle(handle: FileTreeItemHandle | null | undefined): FileTreeDirectoryHandle | null {
  return handle && handle.isDirectory() ? (handle as FileTreeDirectoryHandle) : null;
}

// Walk path segments to collect every directory prefix (e.g. "a/b/c.ts" →
// ["a", "a/b"]). Used to drive expand-all/collapse-all from the toolbar
// because @pierre/trees doesn't ship a one-shot bulk-toggle API.
function collectDirectoryPaths(paths: readonly string[]): string[] {
  const dirs = new Set<string>();
  for (const p of paths) {
    const parts = p.split("/");
    for (let i = 1; i < parts.length; i++) {
      dirs.add(parts.slice(0, i).join("/"));
    }
  }
  return Array.from(dirs);
}

export const DiffFileTree = forwardRef<DiffFileTreeHandle, Props>(function DiffFileTreeImpl(
  { items, onSelectItem, searchOpen = true, header, className, style },
  ref,
) {
  const { paths, gitStatus, pathToItemId } = useMemo(() => {
    const collectedPaths: string[] = [];
    const status: GitStatusEntry[] = [];
    const map = new Map<string, string>();
    for (const item of items) {
      if (item.type !== "diff") continue;
      const path = item.fileDiff.name;
      collectedPaths.push(path);
      map.set(path, item.id);
      const s = diffTypeToStatus(item.fileDiff.type);
      if (s) status.push({ path, status: s });
    }
    return { paths: collectedPaths, gitStatus: status, pathToItemId: map };
  }, [items]);

  const directoryPaths = useMemo(() => collectDirectoryPaths(paths), [paths]);

  // Stable refs so the `onSelectionChange` closure passed to `useFileTree`
  // (which captures only on first render) always sees the latest path map
  // and select handler.
  const pathToItemIdRef = useRef(pathToItemId);
  const onSelectItemRef = useRef(onSelectItem);
  useEffect(() => {
    pathToItemIdRef.current = pathToItemId;
  }, [pathToItemId]);
  useEffect(() => {
    onSelectItemRef.current = onSelectItem;
  }, [onSelectItem]);

  // Set when a row is clicked, then consumed by the search-restore effect so
  // we only reopen search on selection (not on blur from clicks outside).
  const selectionJustFiredRef = useRef(false);
  const onSelectionChange = useCallback((selectedPaths: readonly string[]) => {
    const target = selectedPaths[0];
    if (!target) return;
    const id = pathToItemIdRef.current.get(target);
    if (!id) return;
    selectionJustFiredRef.current = true;
    onSelectItemRef.current(id);
  }, []);

  // Build a comparator that orders tree rows the same way the patch lists
  // them, so the file tree always agrees with what `CodeView` renders below.
  // Without this, Pierre's default alpha sort places root-level files
  // (e.g. go.mod, go.sum) in a different spot than the diff body shows them.
  // Each directory's index is the patch index of the earliest file under it,
  // so a directory sorts to the position of its first appearing child.
  const patchOrder = useMemo(() => {
    const fileIdx = new Map<string, number>();
    paths.forEach((p, i) => fileIdx.set(p, i));
    const dirIdx = new Map<string, number>();
    for (const [p, i] of fileIdx) {
      const parts = p.split("/");
      for (let n = 1; n < parts.length; n++) {
        const dir = parts.slice(0, n).join("/");
        const existing = dirIdx.get(dir);
        if (existing === undefined || i < existing) dirIdx.set(dir, i);
      }
    }
    return { fileIdx, dirIdx };
  }, [paths]);

  const sortComparator = useCallback(
    (a: { path: string; isDirectory: boolean }, b: { path: string; isDirectory: boolean }) => {
      const ai = a.isDirectory ? patchOrder.dirIdx.get(a.path) : patchOrder.fileIdx.get(a.path);
      const bi = b.isDirectory ? patchOrder.dirIdx.get(b.path) : patchOrder.fileIdx.get(b.path);
      return (ai ?? 0) - (bi ?? 0);
    },
    [patchOrder],
  );

  const { model } = useFileTree({
    paths,
    sort: sortComparator,
    icons: ICON_OPTIONS,
    initialExpansion: "open",
    flattenEmptyDirectories: true,
    search: true,
    stickyFolders: true,
    gitStatus,
    onSelectionChange,
    unsafeCSS: TREE_UNSAFE_CSS,
  });

  // Pierre closes search on row click. Reopen with the last typed value so
  // the user does not have to retype after navigating to a matched file.
  // Blur (clicking outside the tree) is intentionally NOT restored. Only
  // row-click closures are, gated by `selectionJustFiredRef`.
  const search = useFileTreeSearch(model);
  const searchValueRef = useRef(search.value);
  const searchOpenFnRef = useRef(search.open);
  searchOpenFnRef.current = search.open;
  useEffect(() => {
    if (search.value !== "") {
      searchValueRef.current = search.value;
    }
    if (!search.isOpen && selectionJustFiredRef.current && searchValueRef.current !== "") {
      searchOpenFnRef.current(searchValueRef.current);
    }
    selectionJustFiredRef.current = false;
  }, [search.value, search.isOpen]);

  // When the patch changes (e.g. navigating to a different commit without
  // unmounting), reset the tree contents and refresh git status without
  // recreating the model.
  const pathsRef = useRef(paths);
  const statusRef = useRef(gitStatus);
  useEffect(() => {
    if (pathsRef.current !== paths) {
      model.resetPaths(paths);
      pathsRef.current = paths;
    }
    if (statusRef.current !== gitStatus) {
      model.setGitStatus(gitStatus);
      statusRef.current = gitStatus;
    }
  }, [model, paths, gitStatus]);

  // Wrap the tree so we can reach its host element (and its shadow root) to
  // imperatively focus the search input on demand.
  const wrapperRef = useRef<HTMLDivElement | null>(null);

  useImperativeHandle(
    ref,
    () => ({
      expandAll: () => {
        for (const dir of directoryPaths) {
          const handle = asDirectoryHandle(model.getItem(dir));
          handle?.expand();
        }
      },
      collapseAll: () => {
        for (const dir of directoryPaths) {
          const handle = asDirectoryHandle(model.getItem(dir));
          handle?.collapse();
        }
      },
      focusSearch: () => {
        // The search input lives in Pierre's shadow root. Walk through any
        // descendant element with a shadowRoot to find it.
        const host = wrapperRef.current?.querySelector<HTMLElement>("[data-file-tree-search-input]");
        if (host instanceof HTMLInputElement) {
          host.focus();
          host.select();
          return;
        }
        // Fall back to descending into shadow roots if the input wasn't in
        // light DOM (it isn't, Pierre keeps it in the shadow root).
        const walker = wrapperRef.current?.querySelectorAll("*") ?? [];
        for (const el of Array.from(walker)) {
          const sr = (el as Element & { shadowRoot?: ShadowRoot }).shadowRoot;
          if (!sr) continue;
          const input = sr.querySelector<HTMLInputElement>("[data-file-tree-search-input]");
          if (input) {
            input.focus();
            input.select();
            return;
          }
        }
      },
    }),
    [directoryPaths, model],
  );

  return (
    <div ref={wrapperRef} className={className} style={style}>
      <FileTree
        model={model}
        header={header}
        data-search-open={searchOpen ? "true" : "false"}
        style={{ height: "100%" }}
      />
    </div>
  );
});
