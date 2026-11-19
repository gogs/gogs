import { cva } from "class-variance-authority";

// A ring around a filled control merges with its fill, so it sits a gap away.
const SOLID_FOCUS = "focus-visible:ring-offset-2 focus-visible:ring-offset-(--color-background)";

export const buttonVariants = cva(
  "inline-flex cursor-pointer items-center justify-center gap-2 rounded-md text-sm font-medium outline-none transition-colors focus-visible:ring-1 focus-visible:ring-(--color-ring) disabled:pointer-events-none disabled:opacity-60",
  {
    variants: {
      variant: {
        default: `glass-solid bg-(--color-primary) text-(--color-primary-foreground) [--glass-solid:var(--color-primary)] hover:opacity-90 ${SOLID_FOCUS}`,
        outline:
          "glass-control border border-(--color-input) bg-(--color-field) text-(--color-foreground) hover:bg-(--color-surface)",
        ghost: "text-(--color-foreground) hover:bg-(--color-surface)",
        link: "text-(--color-foreground) underline-offset-4 hover:underline",
        destructive: `glass-solid bg-(--color-destructive) text-(--color-destructive-foreground) [--glass-solid:var(--color-destructive)] hover:opacity-90 ${SOLID_FOCUS}`,
      },
      size: {
        default: "h-10 px-4 py-2",
        sm: "h-8 px-3",
        icon: "size-9",
        inline: "h-auto p-0",
      },
    },
    defaultVariants: {
      variant: "default",
      size: "default",
    },
  },
);
