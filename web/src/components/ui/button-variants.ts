import { cva } from "class-variance-authority";

export const buttonVariants = cva(
  "inline-flex cursor-pointer items-center justify-center gap-2 rounded-md text-sm font-medium outline-none transition-colors focus-visible:ring-1 focus-visible:ring-(--color-ring) disabled:pointer-events-none disabled:opacity-60",
  {
    variants: {
      variant: {
        default: "bg-(--color-primary) text-(--color-primary-foreground) hover:opacity-90",
        outline:
          "border border-(--color-input) bg-(--color-background) text-(--color-foreground) hover:bg-(--color-surface)",
        ghost: "text-(--color-foreground) hover:bg-(--color-surface)",
        link: "text-(--color-foreground) underline-offset-4 hover:underline",
        destructive: "bg-(--color-destructive) text-(--color-destructive-foreground) hover:opacity-90",
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
