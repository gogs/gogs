import * as React from "react";

import { cn } from "@/lib/utils";

function Input({ className, type, ...props }: React.ComponentProps<"input">) {
  return (
    <input
      type={type}
      className={cn(
        "block w-full rounded-md border border-(--color-input) bg-(--color-background) px-3 py-2 text-sm text-(--color-foreground) placeholder:text-(--color-muted-foreground) outline-none transition-colors",
        "focus-visible:border-(--color-ring) focus-visible:ring-1 focus-visible:ring-(--color-ring)",
        "disabled:cursor-not-allowed disabled:opacity-60",
        "aria-invalid:border-(--color-destructive) aria-invalid:focus-visible:border-(--color-destructive) aria-invalid:focus-visible:ring-(--color-destructive)",
        className,
      )}
      {...props}
    />
  );
}

export { Input };
