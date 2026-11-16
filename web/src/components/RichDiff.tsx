import { useQuery } from "@tanstack/react-query";
import { useMemo } from "react";
import { useTranslation } from "react-i18next";

import { Markdown } from "@/components/Markdown";
import { LoaderResponseError } from "@/lib/loader-error";
import { fetchRepoMarkdownFile } from "@/lib/queries/repo";

// maxDiffCells caps the block comparison, which takes the product of the block
// counts of both versions. Larger files show every block as changed.
const maxDiffCells = 1_000_000;

function blocksOf(html: string): Element[] {
  const body = new DOMParser().parseFromString(html, "text/html").body;
  const blocks: Element[] = [];
  for (const node of Array.from(body.childNodes)) {
    if (node instanceof Element) {
      blocks.push(node);
    } else if (node.textContent?.trim()) {
      const p = document.createElement("p");
      p.textContent = node.textContent;
      blocks.push(p);
    }
  }
  return blocks;
}

type Op =
  | { type: "same"; before: Element; after: Element }
  | { type: "removed"; element: Element }
  | { type: "added"; element: Element };

// diffElements pairs up the elements of both lists with equal keys by their
// longest common subsequence, and marks the rest as removed or added.
function diffElements(a: Element[], b: Element[], key: (e: Element) => string): Op[] {
  const x = a.map(key);
  const y = b.map(key);
  if (x.length * y.length > maxDiffCells) {
    return [
      ...a.map((element) => ({ type: "removed" as const, element })),
      ...b.map((element) => ({ type: "added" as const, element })),
    ];
  }
  // lcs[i][j] is the length of the longest common subsequence of x[i:] and y[j:].
  const lcs = Array.from({ length: x.length + 1 }, () => new Uint32Array(y.length + 1));
  for (let i = x.length - 1; i >= 0; i--) {
    for (let j = y.length - 1; j >= 0; j--) {
      lcs[i][j] = x[i] === y[j] ? lcs[i + 1][j + 1] + 1 : Math.max(lcs[i + 1][j], lcs[i][j + 1]);
    }
  }
  const ops: Op[] = [];
  let i = 0;
  let j = 0;
  while (i < x.length || j < y.length) {
    if (i < x.length && j < y.length && x[i] === y[j]) {
      ops.push({ type: "same", before: a[i], after: b[j] });
      i++;
      j++;
    } else if (j >= y.length || (i < x.length && lcs[i + 1][j] >= lcs[i][j + 1])) {
      ops.push({ type: "removed", element: a[i] });
      i++;
    } else {
      ops.push({ type: "added", element: b[j] });
      j++;
    }
  }
  return ops;
}

const outerHTMLOf = (e: Element) => e.outerHTML;
const tagName = (e: Element) => e.tagName;

function marked(type: "removed" | "added", element: Element) {
  const tag = type === "removed" ? "del" : "ins";
  return `<${tag} class="rich-diff-${type}">${element.outerHTML}</${tag}>`;
}

// changedPair shows a block that changed in place: a list with its items
// marked, or else the old block followed by the new one.
function changedPair(before: Element, after: Element) {
  if (before.tagName !== "UL" && before.tagName !== "OL") {
    return marked("removed", before) + marked("added", after);
  }
  const list = after.cloneNode(false) as Element;
  for (const op of diffElements(Array.from(before.children), Array.from(after.children), outerHTMLOf)) {
    const li = (op.type === "same" ? op.after : op.element).cloneNode(true) as Element;
    if (op.type !== "same") li.classList.add(`rich-diff-${op.type}`);
    list.append(li);
  }
  return list.outerHTML;
}

// diffBlocks compares the rendered blocks of both versions, e.g., paragraphs
// and lists, and marks the blocks only one of them has. Within a run of
// changes, blocks of the same kind pair up, so each shows next to the block
// it replaced.
function diffBlocks(oldHTML: string, newHTML: string): string {
  const ops = diffElements(blocksOf(oldHTML), blocksOf(newHTML), outerHTMLOf);
  const out: string[] = [];
  let removed: Element[] = [];
  let added: Element[] = [];
  const flush = () => {
    for (const op of diffElements(removed, added, tagName)) {
      out.push(op.type === "same" ? changedPair(op.before, op.after) : marked(op.type, op.element));
    }
    removed = [];
    added = [];
  };
  for (const op of ops) {
    if (op.type === "removed") {
      removed.push(op.element);
    } else if (op.type === "added") {
      added.push(op.element);
    } else {
      flush();
      out.push(op.after.outerHTML);
    }
  }
  flush();
  return out.join("");
}

// RichDiff shows how a Markdown file renders before and after a change, with
// the removed and the added blocks marked. Without oldRef or newRef, that
// side is empty, e.g., for an added or a deleted file.
export function RichDiff({
  owner,
  repo,
  oldRef,
  oldPath,
  newRef,
  newPath,
}: {
  owner: string;
  repo: string;
  oldRef: string | undefined;
  oldPath: string;
  newRef: string | undefined;
  newPath: string;
}) {
  const { t } = useTranslation();
  const render = (ref: string | undefined, path: string) => ({
    queryKey: ["markdown-file", owner, repo, ref, path],
    queryFn: () => (ref ? fetchRepoMarkdownFile(owner, repo, ref, path) : ""),
    staleTime: Infinity,
  });
  const before = useQuery(render(oldRef, oldPath));
  const after = useQuery(render(newRef, newPath));
  const html = useMemo(
    () => (before.data !== undefined && after.data !== undefined ? diffBlocks(before.data, after.data) : undefined),
    [before.data, after.data],
  );

  const error = before.error ?? after.error;
  return (
    <div className="px-3 py-4 font-sans sm:px-6 text-(--color-foreground)" data-no-collapse-on-click>
      {html !== undefined ? (
        <Markdown html={html} className="rich-diff" />
      ) : error ? (
        <p className="text-sm text-(--color-destructive)">
          {error instanceof LoaderResponseError && error.status === 413
            ? t("repo.diff.rich_diff_too_large")
            : error.message}
        </p>
      ) : (
        <p className="text-sm text-(--color-muted-foreground)">{t("repo.release.loading")}</p>
      )}
    </div>
  );
}
