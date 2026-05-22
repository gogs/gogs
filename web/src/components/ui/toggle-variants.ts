import { cva } from "class-variance-authority";

export const toggleVariants = cva(
  "inline-flex cursor-pointer items-center justify-center gap-1 rounded-md text-sm font-medium text-(--color-muted-foreground) outline-none transition-colors hover:bg-(--color-surface) hover:text-(--color-foreground) focus-visible:ring-1 focus-visible:ring-(--color-ring) disabled:pointer-events-none disabled:opacity-60 data-[state=on]:bg-(--color-surface) data-[state=on]:text-(--color-foreground)",
  {
    variants: {
      variant: {
        default: "bg-transparent",
        outline:
          "border border-(--color-input) bg-transparent hover:bg-(--color-surface) data-[state=on]:border-(--color-ring)",
      },
      size: {
        default: "h-10 px-3",
        sm: "h-8 px-2",
        tile: "h-auto flex-col px-2 py-2 text-xs",
      },
    },
    defaultVariants: {
      variant: "default",
      size: "default",
    },
  },
);
