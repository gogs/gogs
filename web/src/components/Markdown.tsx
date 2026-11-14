import { useMemo } from "react";
import { useTranslation } from "react-i18next";

import { CodeBlock } from "@/components/CodeBlock";
import { cn } from "@/lib/utils";

type Segment = { html: string } | { code: string; lang: string };

// splitCodeBlocks splits rendered Markdown at its top-level fenced code blocks
// that name a language, so they can be highlighted.
function splitCodeBlocks(html: string, anchorLabel: string): Segment[] {
  const body = new DOMParser().parseFromString(html, "text/html").body;
  // Headings have an id like "user-content-setup", as on GitHub. Their anchor
  // gets "setup", so links to "#setup" work.
  for (const heading of Array.from(body.querySelectorAll<HTMLElement>("h1, h2, h3, h4, h5, h6"))) {
    if (!heading.id.startsWith("user-content-")) continue;
    const slug = heading.id.slice("user-content-".length);
    const anchor = document.createElement("a");
    anchor.id = slug;
    anchor.href = `#${encodeURIComponent(slug)}`;
    anchor.className = "heading-anchor";
    anchor.setAttribute("aria-label", anchorLabel);
    anchor.textContent = "#";
    heading.append(anchor);
  }
  const segments: Segment[] = [];
  let pending = "";
  for (const node of Array.from(body.childNodes)) {
    const code = node instanceof HTMLPreElement ? node.querySelector(":scope > code[class^='language-']") : null;
    if (!code) {
      // Serializing through an element keeps text escaped.
      const box = document.createElement("div");
      box.append(node.cloneNode(true));
      pending += box.innerHTML;
      continue;
    }
    if (pending.trim()) segments.push({ html: pending });
    pending = "";
    segments.push({
      code: (code.textContent ?? "").replace(/\n$/, ""),
      lang: code.className.slice("language-".length),
    });
  }
  if (pending.trim()) segments.push({ html: pending });
  return segments;
}

// Markdown shows Markdown that the server rendered and sanitized, e.g., the
// content of an issue, highlighting its code blocks like CodeBlock.
export function Markdown({ html, className }: { html: string; className?: string }) {
  const { t } = useTranslation();
  const anchorLabel = t("link_to_section");
  const segments = useMemo(() => splitCodeBlocks(html, anchorLabel), [html, anchorLabel]);
  return (
    <div className={cn("markdown", className)}>
      {segments.map((segment, i) =>
        "code" in segment ? (
          <CodeBlock key={i} code={segment.code} lang={segment.lang} />
        ) : (
          <div key={i} dangerouslySetInnerHTML={{ __html: segment.html }} />
        ),
      )}
    </div>
  );
}
