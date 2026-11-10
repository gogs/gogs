import { useMemo } from "react";

import { CodeBlock } from "@/components/CodeBlock";
import { cn } from "@/lib/utils";

type Segment = { html: string } | { code: string; lang: string };

// splitCodeBlocks splits rendered Markdown at its top-level fenced code blocks
// that name a language, so they can be highlighted.
function splitCodeBlocks(html: string): Segment[] {
  const body = new DOMParser().parseFromString(html, "text/html").body;
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
  const segments = useMemo(() => splitCodeBlocks(html), [html]);
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
