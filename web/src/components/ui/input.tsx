import * as React from "react";

import { cn } from "@/lib/utils";

const Input = React.forwardRef<HTMLInputElement, React.InputHTMLAttributes<HTMLInputElement>>(
  ({ className, type, ...props }, ref) => (
    <input
      ref={ref}
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
  ),
);
Input.displayName = "Input";

export { Input };
