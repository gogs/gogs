import { Link } from "@tanstack/react-router";
import type { ReactNode } from "react";

const MARKUP = /(<a\b[^>]*>.*?<\/a>|<code>.*?<\/code>|<b>.*?<\/b>|<strong>.*?<\/strong>|<span\b[^>]*>.*?<\/span>)/;
const LINK_CLASS = "text-(--color-primary) underline underline-offset-2";

// RichText renders a translated sentence that carries the markup the
// server-rendered pages used: links, inline code, and bold or highlighted text. Translations
// keep Go placeholders in that markup, so links take their targets from
// `links`, in order, and code takes its content from `code`. App paths
// navigate within the app.
export function RichText({ text, links = [], code }: { text: string; links?: string[]; code?: string }) {
  let nextLink = 0;
  const parts = text.split(MARKUP).map((part, i): ReactNode => {
    const inner = part.replace(/^<[^>]+>|<\/[a-z]+>$/g, "");
    if (part.startsWith("<a")) {
      const href = links[nextLink++] ?? "";
      return href.startsWith("/") ? (
        <Link key={i} to={href} className={LINK_CLASS}>
          {inner}
        </Link>
      ) : (
        <a key={i} href={href} target="_blank" rel="noopener noreferrer" className={LINK_CLASS}>
          {inner}
        </a>
      );
    }
    if (part.startsWith("<code>")) {
      return (
        <code key={i} className="rounded bg-(--color-surface) px-1 font-mono text-[0.9em] wrap-anywhere">
          {code ?? inner}
        </code>
      );
    }
    // Highlighted words, e.g., "Private" in a colored span, show in bold.
    if (part.startsWith("<b>") || part.startsWith("<strong>") || part.startsWith("<span")) {
      return <strong key={i}>{inner}</strong>;
    }
    return part;
  });
  return <>{parts}</>;
}
