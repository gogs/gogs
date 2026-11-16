import { useEffect, useId, useState } from "react";
import { useTranslation } from "react-i18next";

import { CodeBlock } from "@/components/CodeBlock";
import { resolveTheme, useTheme } from "@/lib/theme-context";

declare global {
  interface Window {
    // Site admins can set this in custom/templates/inject/head.tmpl to change
    // how diagrams render, as with the server-rendered pages.
    MERMAID_INITIALIZE_OPTIONS?: Record<string, unknown>;
  }
}

// Mermaid is large, so it is only loaded on pages that have a diagram.
const loadMermaid = () => import("mermaid").then((m) => m.default);

// Mermaid keeps one configuration for the page, so concurrent renders with
// different themes would interfere. They run one after another instead.
let queue: Promise<unknown> = Promise.resolve();

function renderDiagram(id: string, code: string, theme: "light" | "dark"): Promise<string> {
  const next = queue.then(async () => {
    const mermaid = await loadMermaid();
    mermaid.initialize({
      startOnLoad: false,
      // Labels must not run scripts or link anywhere, because anyone who can
      // write Markdown writes them.
      securityLevel: "strict",
      // A diagram that fails to draw is shown as code instead of Mermaid's
      // error graphic, which it would add to the end of the page.
      suppressErrorRendering: true,
      theme: theme === "dark" ? "dark" : "default",
      ...window.MERMAID_INITIALIZE_OPTIONS,
    });
    const { svg } = await mermaid.render(id, code);
    return svg;
  });
  queue = next.catch(() => {});
  return next;
}

// MermaidDiagram draws a Mermaid code block as a diagram, or shows its code
// when the diagram cannot be drawn.
export function MermaidDiagram({ code }: { code: string }) {
  const { t } = useTranslation();
  const { theme } = useTheme();
  const resolved = resolveTheme(theme);
  // Mermaid uses the id for elements it adds to the document while drawing,
  // which must be valid in a CSS selector.
  const id = `mermaid-${useId().replace(/[^\w-]/g, "")}`;
  const [result, setResult] = useState<{ svg?: string; failed?: boolean }>({});

  useEffect(() => {
    let current = true;
    renderDiagram(id, code, resolved).then(
      (svg) => current && setResult({ svg }),
      () => current && setResult({ failed: true }),
    );
    return () => {
      current = false;
    };
  }, [id, code, resolved]);

  if (result.failed) {
    return <CodeBlock code={code} lang="text" />;
  }
  return (
    <div
      role="figure"
      aria-label={t("mermaid_diagram")}
      aria-busy={!result.svg}
      className="mermaid-diagram overflow-x-auto"
      // Mermaid sanitizes the SVG it draws in strict mode.
      dangerouslySetInnerHTML={{ __html: result.svg ?? "" }}
    />
  );
}
