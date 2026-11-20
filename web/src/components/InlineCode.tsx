import type { ReactNode } from "react";

import { cn } from "@/lib/utils";

// InlineCode is code within text, e.g., a configuration setting. Its children
// are optional for a translation to fill them in through Trans. Inside a link
// it takes the color of the link through `className`, since it sets its own.
export function InlineCode({ children, className }: { children?: ReactNode; className?: string }) {
  return (
    // Cloned decoration keeps the chip's padding and corners on every line a
    // long value wraps to.
    <code
      className={cn(
        "rounded border border-(--color-border) box-decoration-clone bg-(--color-surface) px-1.5 font-mono text-[0.8125rem] text-(--color-foreground)",
        className,
      )}
    >
      {children}
    </code>
  );
}
