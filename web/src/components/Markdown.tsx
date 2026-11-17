import { useLayoutEffect, useMemo, useRef, useState } from "react";
import { createPortal } from "react-dom";

import { CodeBlock } from "@/components/CodeBlock";
import { cn } from "@/lib/utils";

interface CodeBlockContent {
  code: string;
  lang: string;
}

// extractCodeBlocks replaces the code blocks, wherever they are nested, with
// placeholders that CodeBlock renders into.
function extractCodeBlocks(html: string): { html: string; blocks: CodeBlockContent[] } {
  const body = new DOMParser().parseFromString(html, "text/html").body;
  const blocks: CodeBlockContent[] = [];
  for (const code of Array.from(body.querySelectorAll("pre > code"))) {
    // The server sanitizer drops data attributes, so content cannot forge a
    // placeholder.
    const placeholder = document.createElement("div");
    placeholder.dataset.codeBlock = String(blocks.length);
    blocks.push({
      code: (code.textContent ?? "").replace(/\n$/, ""),
      lang: code.className.startsWith("language-") ? code.className.slice("language-".length) : "text",
    });
    code.parentElement!.replaceWith(placeholder);
  }
  return { html: body.innerHTML, blocks };
}

// Markdown shows Markdown that the server rendered and sanitized, e.g., the
// content of an issue, highlighting its code blocks like CodeBlock.
export function Markdown({ html, className }: { html: string; className?: string }) {
  const content = useMemo(() => extractCodeBlocks(html), [html]);
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

  return (
    <div className={cn("markdown", className)}>
      <div ref={container} dangerouslySetInnerHTML={innerHTML} />
      {placeholders?.content === content
        ? placeholders.elements.map((element) => {
            const index = Number(element.dataset.codeBlock);
            const block = content.blocks[index];
            return createPortal(<CodeBlock code={block.code} lang={block.lang} />, element, index);
          })
        : null}
    </div>
  );
}
