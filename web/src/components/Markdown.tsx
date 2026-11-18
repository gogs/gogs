import { File } from "@pierre/diffs/react";
import { useEffect, useLayoutEffect, useMemo, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { useTranslation } from "react-i18next";

import { CodeBlock } from "@/components/CodeBlock";
import { MermaidDiagram } from "@/components/MermaidDiagram";
import { resolveTheme, useTheme } from "@/lib/theme-context";
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

// prepareHTML replaces the code blocks, wherever they are nested, with
// placeholders that CodeBlock renders into, and, given permalinkLabel, gives
// each heading with an id a link to itself. The lines of a permalink, which
// the server embeds below it, render with where they come from.
function prepareHTML(
  html: string,
  permalinkLabel?: (heading: string) => string,
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
export function Markdown({
  html,
  className,
  headingAnchors,
}: {
  html: string;
  className?: string;
  headingAnchors?: boolean;
}) {
  const { t } = useTranslation();
  const content = useMemo(
    () => prepareHTML(html, headingAnchors ? (heading) => t("heading_permalink", { heading }) : undefined),
    [html, headingAnchors, t],
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
      <div ref={container} dangerouslySetInnerHTML={innerHTML} />
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
