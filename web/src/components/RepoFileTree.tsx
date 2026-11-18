import { FILE_TREE_DEFAULT_ITEM_HEIGHT, type FileTreeDirectoryHandle, type FileTreeIcons } from "@pierre/trees";
import { FileTree, useFileTree } from "@pierre/trees/react";
import { useQuery } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { useEffect, useRef, useState } from "react";

import { repoPathsQuery } from "@/lib/queries/repo";
import { TREE_THEME_STYLE } from "@/lib/tree-theme";

const ICON_OPTIONS: FileTreeIcons = { set: "complete", colored: true };

function ancestorsOf(path: string) {
  const parts = path.split("/");
  return parts.slice(1).map((_, i) => parts.slice(0, i + 1).join("/"));
}

// RepoFileTree shows every file of the commit as a tree beside the code
// browser, with the path the page shows open and selected. Picking a file
// opens it at the ref. It asks for the height of its rows, and shrinks to
// scroll when its parent has less room.
export function RepoFileTree(props: { owner: string; name: string; gitRef: string; commitID: string; path: string }) {
  const { data } = useQuery(repoPathsQuery(props.owner, props.name, props.commitID));
  if (!data) return <div className="min-h-0 flex-1" />;
  // The tree takes its paths once, so a new commit gets a new tree.
  return <Tree key={props.commitID} {...props} paths={data.paths} />;
}

function Tree({
  owner,
  name,
  gitRef: refName,
  path,
  paths,
}: {
  owner: string;
  name: string;
  gitRef: string;
  path: string;
  paths: string[];
}) {
  const navigate = useNavigate();
  const current = useRef(path);
  // The tree can report the initial selection while it is being created.
  const tree = useRef<{ getItem: (path: string) => { isDirectory(): boolean } | null } | null>(null);
  const open = useRef<(selected: string) => void>(() => {});
  useEffect(() => {
    open.current = (selected) => {
      if (selected === current.current) return;
      void navigate({ to: "/$owner/$repo/src/$", params: { owner, repo: name, _splat: `${refName}/${selected}` } });
    };
  }, [owner, name, refName, navigate]);

  const { model } = useFileTree({
    paths,
    icons: ICON_OPTIONS,
    initialExpansion: "closed",
    // A directory the page shows is open too. A file in the list is ignored.
    initialExpandedPaths: path ? [...ancestorsOf(path), path] : [],
    initialSelectedPaths: path ? [path] : [],
    flattenEmptyDirectories: true,
    stickyFolders: true,
    onSelectionChange: (selected) => {
      const target = selected[0];
      if (target && tree.current?.getItem(target)?.isDirectory() === false) open.current(target);
    },
  });

  useEffect(() => {
    tree.current = model;
  }, [model]);

  // Opening and closing folders changes how many rows there are.
  const [rows, setRows] = useState(() => model.getVisibleCount());
  useEffect(() => model.subscribe(() => setRows(model.getVisibleCount())), [model]);

  // Follow the page to another path, e.g., after the back button.
  useEffect(() => {
    current.current = path;
    for (const selected of model.getSelectedPaths()) {
      if (selected !== path) model.getItem(selected)?.deselect();
    }
    for (const dir of ancestorsOf(path)) {
      (model.getItem(dir) as FileTreeDirectoryHandle | null)?.expand();
    }
    const item = path ? model.getItem(path) : null;
    if (item?.isDirectory()) (item as FileTreeDirectoryHandle).expand();
    if (item && !item.isSelected()) item.select();
    if (item) model.scrollToPath(path, { offset: "nearest" });
  }, [model, path]);

  return (
    // The rows keep the inset of the header above them from the bottom edge.
    <div
      className="min-h-0 flex-auto pb-3"
      style={{ height: `calc(${rows * FILE_TREE_DEFAULT_ITEM_HEIGHT}px + 0.75rem)` }}
    >
      <FileTree model={model} style={{ ...TREE_THEME_STYLE, height: "100%" }} />
    </div>
  );
}
