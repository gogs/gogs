import type { ReactNode } from "react";

// InlineCode is code within text, e.g., a configuration setting. Its children
// are optional for a translation to fill them in through Trans.
export function InlineCode({ children }: { children?: ReactNode }) {
  return (
    // Cloned decoration keeps the chip's padding and corners on every line a
    // long value wraps to.
    <code className="rounded border border-(--color-border) box-decoration-clone bg-(--color-surface) px-1.5 font-mono text-[0.8125rem] text-(--color-foreground)">
      {children}
    </code>
  );
}
