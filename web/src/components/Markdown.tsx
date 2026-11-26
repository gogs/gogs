import { File } from "@pierre/diffs/react";
import { useEffect, useLayoutEffect, useMemo, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";

import { CodeBlock } from "@/components/CodeBlock";
import { MermaidDiagram } from "@/components/MermaidDiagram";
import { resolveTheme, useTheme } from "@/lib/theme-context";
import { useAppLinkClick } from "@/lib/use-app-link-click";
import { cn } from "@/lib/utils";

// Permalink is where the lines of an embedded permalink come from.
interface Permalink {
  href: string;
  path: string;
  commit: string;
  start: number;
  end: number;
}

interface CodeBlockContent {
  code: string;
  lang: string;
  permalink?: Permalink;
}

// HEADING_ID_PREFIX keeps the ids of headings from clobbering ids of the page.
// Links to a heading leave it out, as on GitHub, e.g., `#installation`.
const HEADING_ID_PREFIX = "user-content-";

// LINK_ICON is the "link" icon of Lucide.
const LINK_ICON =
  '<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M10 13a5 5 0 0 0 7.54.54l3-3a5 5 0 0 0-7.07-7.07l-1.72 1.71"/><path d="M14 11a5 5 0 0 0-7.54-.54l-3 3a5 5 0 0 0 7.07 7.07l1.71-1.71"/></svg>';

// GRIP_ICON is the "grip-vertical" icon of Lucide.
const GRIP_ICON =
  '<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><circle cx="9" cy="12" r="1"/><circle cx="9" cy="5" r="1"/><circle cx="9" cy="19" r="1"/><circle cx="15" cy="12" r="1"/><circle cx="15" cy="5" r="1"/><circle cx="15" cy="19" r="1"/></svg>';

// TASK_CHECKBOX finds the checkbox of the task list item it is called on, which
// the server renders before the text of the item, in its first paragraph when
// the list is loose, see dariomark.
const TASK_CHECKBOX = ':scope > input[type="checkbox"], :scope > p:first-of-type > input[type="checkbox"]';

// taskLines returns the first and the last line of the source of the task list
// item, as the server marks them.
function taskLines(item: HTMLElement): [number, number] {
  const [first, last] = (item.dataset.sourceLines ?? "").split("-").map(Number);
  return [first, last];
}

// splitLines splits Markdown source into the lines the server numbers: without
// a byte order mark, at "\r\n", "\n" or "\r".
function splitLines(source: string): string[] {
  return source.replace(/^\uFEFF/, "").split(/\r\n|\r|\n/);
}

function joinLines(lines: string[], source: string): string {
  return lines.join(source.includes("\r\n") ? "\r\n" : "\n");
}

const LIST_MARKER = String.raw`(?:[ \t]*>)*[ \t]*(?:[-+*]|\d{1,9}[.)])`;
const TASK_MARKER_AFTER_LIST_MARKER = new RegExp(String.raw`^(${LIST_MARKER}[ \t]+)\[[ xX\t]\]`);
const EMPTY_LIST_ITEM = new RegExp(String.raw`^${LIST_MARKER}[ \t]*$`);
const TASK_MARKER = /^((?:[ \t]*>)*[ \t]*)\[[ xX\t]\]/;

// taskMarker returns the index in lines of the line with the task list item
// marker of the item that starts at the 1-based line, and the pattern that
// finds the marker on it. The marker follows the list marker, e.g., "- [ ] a",
// or follows an empty first line of the item on the next line, e.g.,
// "-\n  [ ] a". It returns undefined when the item starts on the line of
// another list item, e.g., the second item of "- - [ ] a", whose line the
// changes to the item cannot tell from that of the other one.
function taskMarker(lines: string[], line: number): { index: number; pattern: RegExp } | undefined {
  const first = lines[line - 1] ?? "";
  if (TASK_MARKER_AFTER_LIST_MARKER.test(first)) return { index: line - 1, pattern: TASK_MARKER_AFTER_LIST_MARKER };
  if (EMPTY_LIST_ITEM.test(first) && TASK_MARKER.test(lines[line] ?? "")) return { index: line, pattern: TASK_MARKER };
  return undefined;
}

const LIST_ITEM_START = /^((?:[ \t]*>)*)([ \t]*)(?:[-+*]|\d{1,9}[.)])[ \t]/;
// ENDS_PARAGRAPH matches the start of a block that ends a paragraph before it,
// rather than continuing it: a list item, a heading, a code fence or a block
// quote.
const ENDS_PARAGRAPH = /^(?:(?:[-+*]|\d{1,9}[.)])[ \t]+\S|#{1,6}(?:[ \t]|$)|```|~~~|>)/;

// movableList reports whether the task list items of a list, which are
// editable, can be reordered by moving their lines in source without changing
// what the lines around them are part of. That needs every item to:
//   - have its marker on its first line, since an empty first line moved first
//     would underline the text before the list as a heading, e.g., "-\n  [ ] a";
//   - start at the same indentation, since an item moved above one indented
//     more would take it in as a nested item.
// And the line after the list must be blank, or start a block that ends a
// paragraph at most as indented as the items, since text after a code block
// that ends the last item would continue an item that ends in a paragraph.
function movableList(items: HTMLElement[], lines: string[]): boolean {
  let start: RegExpExecArray | null = null;
  for (const item of items) {
    const first = taskLines(item)[0];
    if (taskMarker(lines, first)!.index !== first - 1) return false;
    const m = LIST_ITEM_START.exec(lines[first - 1]);
    if (!m || (start && m[1] + m[2] !== start[1] + start[2])) return false;
    start = m;
  }
  const next = lines[Math.max(...items.map((item) => taskLines(item)[1]))];
  if (next === undefined || next.trim() === "") return true;
  const quotes = new RegExp(String.raw`^(?:[ \t]*>){${start![1].split(">").length - 1}}`).exec(next)?.[0];
  if (quotes === undefined) return false;
  const rest = next.slice(quotes.length);
  if (rest.trim() === "") return true;
  const indent = /^[ \t]*/.exec(rest)![0];
  return (
    columnAfter(columnAfter(0, quotes), indent) <= columnAfter(columnAfter(0, start![1]), start![2]) &&
    ENDS_PARAGRAPH.test(rest.slice(indent.length))
  );
}

// enableTasks turns on the checkboxes of the task list items in body whose
// marker is found in source, labeled with the text of their item, and gives
// the items of lists that only hold such items a handle to move them, when the
// list can be reordered, see movableList.
function enableTasks(body: HTMLElement, source: string, moveLabel: (task: string) => string) {
  const lines = splitLines(source);
  const editable = (li: Element) =>
    li.matches("li.task-list-item[data-source-lines]") &&
    li.querySelector(TASK_CHECKBOX) !== null &&
    taskMarker(lines, taskLines(li as HTMLElement)[0]) !== undefined;
  const movable = new Map<Element, boolean>();
  for (const item of Array.from(body.querySelectorAll<HTMLElement>("li.task-list-item[data-source-lines]"))) {
    if (!editable(item)) continue;
    const checkbox = item.querySelector<HTMLInputElement>(TASK_CHECKBOX)!;
    const text = item.cloneNode(true) as HTMLElement;
    text.querySelectorAll("ul, ol").forEach((list) => list.remove());
    const label = (text.textContent ?? "").replace(/\s+/g, " ").trim();
    checkbox.disabled = false;
    checkbox.setAttribute("aria-label", label);

    const list = item.parentElement!;
    if (!movable.has(list)) {
      const siblings = Array.from(list.children) as HTMLElement[];
      movable.set(list, siblings.length > 1 && siblings.every(editable) && movableList(siblings, lines));
    }
    if (!movable.get(list)) continue;
    const handle = document.createElement("button");
    handle.type = "button";
    handle.className = "task-drag-handle";
    handle.setAttribute("aria-label", moveLabel(label));
    handle.innerHTML = GRIP_ICON;
    item.prepend(handle);
  }
}

// toggleTask returns source with the task list item marker of the item that
// starts at the 1-based line checked or unchecked, or null when the item has
// no marker there, see taskMarker.
function toggleTask(source: string, line: number, checked: boolean): string | null {
  const lines = splitLines(source);
  const marker = taskMarker(lines, line);
  if (!marker) return null;
  lines[marker.index] = lines[marker.index].replace(marker.pattern, `$1[${checked ? "x" : " "}]`);
  return joinLines(lines, source);
}

const ORDERED_LIST_NUMBER = /^((?:[ \t]*>)*[ \t]*)(\d{1,9})(?=[.)])/;
const ORDERED_LIST_MARKER = /^(?:[ \t]*>)*[ \t]*\d{1,9}[.)][ \t]*/;

// columnAfter returns the column after text that starts at column, with tabs
// stopping at multiples of 4.
function columnAfter(column: number, text: string): number {
  for (const c of text) column = c === "\t" ? column + 4 - (column % 4) : column + 1;
  return column;
}

// contentColumn returns the column the content of the ordered list item that
// starts on line starts at.
function contentColumn(line: string): number {
  return columnAfter(0, ORDERED_LIST_MARKER.exec(line)?.[0] ?? "");
}

// indent returns line with the indentation after the quotes block quote
// markers of the list it is in, if any, changed by delta spaces and written
// with spaces only, since a tab would expand to the same tab stop. Blank lines
// stay as they are.
function indent(line: string, quotes: number, delta: number): string {
  const quotesPrefix = new RegExp(String.raw`^(?:[ \t]*> ?){0,${quotes}}`).exec(line)![0];
  const rest = line.slice(quotesPrefix.length);
  if (rest.trim() === "") return line;
  const whitespace = /^[ \t]*/.exec(rest)![0];
  const start = columnAfter(0, quotesPrefix);
  const width = columnAfter(start, whitespace) - start;
  return quotesPrefix + " ".repeat(Math.max(0, width + delta)) + rest.slice(whitespace.length);
}

// moveTasks returns source with the items of a list, whose source spans the
// 1-based lines ranges in order, put in the order of the indexes of ranges in
// order, and the first line of each item in that order. What is between items,
// e.g., blank lines of a loose list, stays in place, and so do the numbers of
// an ordered list. An item that takes a number of another width has the rest
// of its lines indented to match, to keep them in the item.
function moveTasks(
  source: string,
  ranges: [number, number][],
  order: number[],
  ordered: boolean,
): { source: string; firstLines: number[] } {
  const lines = splitLines(source);
  const items = ranges.map(([first, last]) => lines.slice(first - 1, last));
  const out = lines.slice(0, ranges[0][0] - 1);
  const firstLines: number[] = [];
  order.forEach((from, to) => {
    let item = items[from];
    const own = ordered ? ORDERED_LIST_NUMBER.exec(item[0]) : null;
    const taken = ordered ? ORDERED_LIST_NUMBER.exec(items[to][0]) : null;
    if (own && taken) {
      const first = item[0].replace(ORDERED_LIST_NUMBER, `$1${taken[2]}`);
      const delta = contentColumn(first) - contentColumn(item[0]);
      const quotes = own[1].split(">").length - 1;
      item = [first, ...item.slice(1).map((l) => indent(l, quotes, delta))];
    }
    firstLines.push(out.length + 1);
    out.push(...item);
    if (to + 1 < ranges.length) out.push(...lines.slice(ranges[to][1], ranges[to + 1][0] - 1));
  });
  out.push(...lines.slice(ranges[ranges.length - 1][1]));
  return { source: joinLines(out, source), firstLines };
}

// prepareHTML replaces the code blocks, wherever they are nested, with
// placeholders that CodeBlock renders into, and, given permalinkLabel, gives
// each heading with an id a link to itself, and, given tasks, makes task lists
// interactive, see enableTasks. The lines of a permalink, which the server
// embeds below it, render with where they come from.
function prepareHTML(
  html: string,
  permalinkLabel?: (heading: string) => string,
  tasks?: { source: string; moveLabel: (task: string) => string },
): { html: string; blocks: CodeBlockContent[] } {
  const body = new DOMParser().parseFromString(html, "text/html").body;
  const blocks: CodeBlockContent[] = [];
  for (const code of Array.from(body.querySelectorAll("pre > code"))) {
    // The server sanitizer drops data attributes, so content cannot forge a
    // placeholder or an embed.
    const placeholder = document.createElement("div");
    placeholder.dataset.codeBlock = String(blocks.length);
    const embed = code.parentElement!.parentElement;
    const data = embed?.dataset;
    if (embed && data?.permalink !== undefined) {
      blocks.push({
        code: code.textContent ?? "",
        lang: "text",
        permalink: {
          href: data.permalink,
          path: data.path ?? "",
          commit: data.commit ?? "",
          start: Number(data.start),
          end: Number(data.end),
        },
      });
      embed.replaceWith(placeholder);
      continue;
    }
    blocks.push({
      code: (code.textContent ?? "").replace(/\n$/, ""),
      lang: code.className.startsWith("language-") ? code.className.slice("language-".length) : "text",
    });
    code.parentElement!.replaceWith(placeholder);
  }
  for (const heading of Array.from(body.querySelectorAll<HTMLElement>("h1, h2, h3, h4, h5, h6"))) {
    if (!permalinkLabel || !heading.id.startsWith(HEADING_ID_PREFIX)) continue;
    const anchor = document.createElement("a");
    anchor.className = "heading-anchor";
    anchor.setAttribute("href", `#${heading.id.slice(HEADING_ID_PREFIX.length)}`);
    anchor.setAttribute("aria-label", permalinkLabel(heading.textContent?.trim() ?? ""));
    anchor.innerHTML = LINK_ICON;
    heading.append(anchor);
  }
  if (tasks) enableTasks(body, tasks.source, tasks.moveLabel);
  return { html: body.innerHTML, blocks };
}

// scrollToHeading scrolls to the heading the location hash names without its
// id prefix. The browser cannot find it by itself, and looks before the
// Markdown renders anyway.
function scrollToHeading() {
  let slug: string;
  try {
    slug = decodeURIComponent(window.location.hash.slice(1));
  } catch {
    return;
  }
  if (slug) document.getElementById(HEADING_ID_PREFIX + slug)?.scrollIntoView();
}

// PermalinkSnippet shows the lines of a permalink highlighted, numbered as in
// their file, below the path, the lines and the commit they link to.
function PermalinkSnippet({ code, permalink }: { code: string; permalink: Permalink }) {
  const { t } = useTranslation();
  const { theme } = useTheme();
  const { href, path, commit, start, end } = permalink;
  const lines =
    start === end ? t("repo.permalink_line", { line: start, path }) : t("repo.permalink_lines", { start, end, path });
  // Pierre numbers lines from 1, so the numbers are drawn from a counter that
  // starts at the first line of the permalink.
  const numbersCSS = `[data-gutter] { counter-reset: permalink-line ${start - 1}; }
[data-column-number] { counter-increment: permalink-line; }
[data-line-number-content] { font-size: 0; }
[data-line-number-content]::before { content: counter(permalink-line); font-size: var(--diffs-font-size, 13px); }`;
  return (
    <div className="overflow-hidden rounded-md border border-(--color-border) text-xs">
      <div className="flex flex-wrap items-baseline gap-x-2 border-b border-(--color-border) bg-(--color-surface) px-3 py-1.5">
        <a href={href} className="min-w-0 font-mono font-medium wrap-anywhere text-(--color-primary) hover:underline">
          {path}
        </a>
        <span className="text-(--color-muted-foreground)">
          <span className="sr-only">{lines}, </span>
          <span aria-hidden>{start === end ? `L${start}` : `L${start}-L${end}`}</span>{" "}
          <code className="font-mono">{commit.slice(0, 7)}</code>
        </span>
      </div>
      <File
        file={{ name: path.split("/").pop() ?? path, contents: code }}
        style={{ border: "none" }}
        options={{
          theme: { light: "pierre-light", dark: "pierre-dark" },
          themeType: resolveTheme(theme),
          disableFileHeader: true,
          overflow: "scroll",
          unsafeCSS: numbersCSS,
        }}
      />
    </div>
  );
}

// Markdown shows Markdown that the server rendered and sanitized, e.g., the
// content of an issue, highlighting its code blocks like CodeBlock and drawing
// Mermaid code blocks as diagrams.
//
// headingAnchors links headings to themselves and scrolls to the heading the
// location hash names. Only the one document of a page may set it: documents
// rendered separately share heading ids, e.g., the "Steps" of two comments.
//
// tasks lets the viewer check and uncheck task list items, and move them by
// dragging their handle or with the arrow keys on it, by saving the Markdown
// source that html was rendered from with the change. html is expected to
// change once save resolves.
export function Markdown({
  html,
  className,
  headingAnchors,
  tasks,
}: {
  html: string;
  className?: string;
  headingAnchors?: boolean;
  tasks?: { source: string; save: (source: string) => Promise<unknown> };
}) {
  const { t } = useTranslation();
  const taskSource = tasks?.source;
  const editableTasks = taskSource !== undefined;
  const onLinkClick = useAppLinkClick({ markedOnly: true });
  const content = useMemo(
    () =>
      prepareHTML(
        html,
        headingAnchors ? (heading) => t("heading_permalink", { heading }) : undefined,
        taskSource !== undefined ? { source: taskSource, moveLabel: (task) => t("move_task", { task }) } : undefined,
      ),
    [html, headingAnchors, taskSource, t],
  );
  // React sets the HTML again whenever this object changes, which would drop
  // the code blocks rendered into the placeholders.
  const innerHTML = useMemo(() => ({ __html: content.html }), [content]);
  const container = useRef<HTMLDivElement>(null);
  // The placeholders exist only once React sets the HTML, so they are collected
  // after each commit along with the content they belong to.
  const [placeholders, setPlaceholders] = useState<{ content: typeof content; elements: HTMLElement[] }>();
  useLayoutEffect(() => {
    setPlaceholders({
      content,
      elements: Array.from(container.current!.querySelectorAll<HTMLElement>("[data-code-block]")),
    });
  }, [content]);

  const tasksRef = useRef(tasks);
  // focusLine is the first line of the item moved with the keyboard, whose
  // handle gets the focus back once the content renders again, unless it is
  // other content than was saved, e.g., when saving failed.
  const focusLine = useRef<{ line: number; source: string }>(undefined);
  useLayoutEffect(() => {
    tasksRef.current = tasks;
  });
  useLayoutEffect(() => {
    const target = focusLine.current;
    focusLine.current = undefined;
    if (target === undefined || tasksRef.current?.source !== target.source) return;
    const line = target.line;
    for (const item of Array.from(container.current!.querySelectorAll<HTMLElement>("li[data-source-lines]"))) {
      if (taskLines(item)[0] === line) item.querySelector<HTMLElement>(":scope > .task-drag-handle")?.focus();
    }
  }, [content]);

  useEffect(() => {
    if (!editableTasks) return;
    const root = container.current!;
    let busy = false;
    // save saves the source with a change already shown, and undoes the change
    // with revert when that fails.
    const save = async (source: string, revert: () => void) => {
      busy = true;
      root.setAttribute("aria-busy", "true");
      try {
        await tasksRef.current!.save(source);
      } catch (err) {
        focusLine.current = undefined;
        revert();
        toast.error(err instanceof Error ? err.message : String(err));
      } finally {
        busy = false;
        root.removeAttribute("aria-busy");
      }
    };
    // saveOrder saves the order of the items of the list, which were in the
    // order of before.
    const saveOrder = (list: HTMLElement, before: HTMLElement[], moved: HTMLElement, focus: boolean) => {
      // The content may have rendered again during a drag, which leaves the
      // list out of the document with the lines of the content before.
      if (!root.contains(list)) return;
      if (busy) {
        list.append(...before);
        return;
      }
      const after = Array.from(list.children) as HTMLElement[];
      const inSource = [...before].sort((a, b) => taskLines(a)[0] - taskLines(b)[0]);
      const order = after.map((item) => inSource.indexOf(item));
      if (order.every((from, to) => from === to)) return;
      // Moving the focused handle in the document takes the focus away.
      const revert = () => {
        list.append(...before);
        if (focus) moved.querySelector<HTMLElement>(":scope > .task-drag-handle")?.focus();
      };
      const result = moveTasks(tasksRef.current!.source, inSource.map(taskLines), order, list.tagName === "OL");
      // Swapping items with the same source changes nothing to save.
      if (result.source === tasksRef.current!.source) {
        revert();
        return;
      }
      if (focus) focusLine.current = { line: result.firstLines[after.indexOf(moved)], source: result.source };
      void save(result.source, revert);
    };

    const onClick = (e: MouseEvent) => {
      const checkbox = e.target instanceof HTMLInputElement ? e.target : null;
      const item = checkbox?.closest<HTMLElement>("li.task-list-item[data-source-lines]");
      if (!checkbox || !item || item.querySelector(TASK_CHECKBOX) !== checkbox) return;
      const source = busy ? null : toggleTask(tasksRef.current!.source, taskLines(item)[0], checkbox.checked);
      if (source === null) {
        e.preventDefault();
        return;
      }
      void save(source, () => (checkbox.checked = !checkbox.checked));
    };

    const onKeyDown = (e: KeyboardEvent) => {
      const handle = e.target instanceof Element ? e.target.closest<HTMLElement>(".task-drag-handle") : null;
      if (!handle || (e.key !== "ArrowUp" && e.key !== "ArrowDown")) return;
      e.preventDefault();
      const item = handle.parentElement!;
      const list = item.parentElement!;
      const sibling = e.key === "ArrowUp" ? item.previousElementSibling : item.nextElementSibling?.nextElementSibling;
      if (busy || (e.key === "ArrowDown" && !item.nextElementSibling) || (e.key === "ArrowUp" && !sibling)) return;
      const before = Array.from(list.children) as HTMLElement[];
      list.insertBefore(item, sibling ?? null);
      handle.focus();
      saveOrder(list, before, item, true);
    };

    const onPointerDown = (e: PointerEvent) => {
      const handle = e.target instanceof Element ? e.target.closest<HTMLElement>(".task-drag-handle") : null;
      if (!handle || e.button !== 0 || busy) return;
      e.preventDefault();
      const item = handle.parentElement!;
      const list = item.parentElement!;
      const before = Array.from(list.children) as HTMLElement[];
      item.classList.add("task-dragging");
      // The item goes before the first other item whose middle is below the
      // pointer, or last.
      const onMove = (e: PointerEvent) => {
        const next =
          before.find((other) => {
            if (other === item) return false;
            const rect = other.getBoundingClientRect();
            return e.clientY < rect.top + rect.height / 2;
          }) ?? null;
        if (item.nextElementSibling !== next) list.insertBefore(item, next);
      };
      const end = (e: PointerEvent) => {
        window.removeEventListener("pointermove", onMove);
        window.removeEventListener("pointerup", end);
        window.removeEventListener("pointercancel", end);
        item.classList.remove("task-dragging");
        if (e.type === "pointercancel") list.append(...before);
        else saveOrder(list, before, item, false);
      };
      // Moving the item in the document releases a pointer capture of the
      // handle, so the window follows the pointer instead.
      window.addEventListener("pointermove", onMove);
      window.addEventListener("pointerup", end);
      window.addEventListener("pointercancel", end);
    };

    root.addEventListener("click", onClick);
    root.addEventListener("keydown", onKeyDown);
    root.addEventListener("pointerdown", onPointerDown);
    return () => {
      root.removeEventListener("click", onClick);
      root.removeEventListener("keydown", onKeyDown);
      root.removeEventListener("pointerdown", onPointerDown);
    };
  }, [editableTasks]);

  useEffect(() => {
    if (!headingAnchors) return;
    scrollToHeading();
    // Following a link to the heading the hash already names does not change
    // the hash, so clicks on such links scroll as well.
    const onClick = (e: MouseEvent) => {
      const link = e.target instanceof Element ? e.target.closest("a") : null;
      if (link?.hash && link.href === window.location.href) scrollToHeading();
    };
    window.addEventListener("hashchange", scrollToHeading);
    document.addEventListener("click", onClick);
    return () => {
      window.removeEventListener("hashchange", scrollToHeading);
      document.removeEventListener("click", onClick);
    };
  }, [content, headingAnchors]);

  return (
    <div className={cn("markdown", className)}>
      <div ref={container} onClick={onLinkClick} dangerouslySetInnerHTML={innerHTML} />
      {placeholders?.content === content
        ? placeholders.elements.map((element) => {
            const index = Number(element.dataset.codeBlock);
            const block = content.blocks[index];
            const node = block.permalink ? (
              <PermalinkSnippet code={block.code} permalink={block.permalink} />
            ) : block.lang === "mermaid" ? (
              <MermaidDiagram code={block.code} />
            ) : (
              <CodeBlock code={block.code} lang={block.lang} />
            );
            return createPortal(node, element, index);
          })
        : null}
    </div>
  );
}
