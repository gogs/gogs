import * as CheckboxPrimitive from "@radix-ui/react-checkbox";
import { Check, Minus } from "lucide-react";
import * as React from "react";

import { cn } from "@/lib/utils";

const Checkbox = React.forwardRef<
  React.ElementRef<typeof CheckboxPrimitive.Root>,
  React.ComponentPropsWithoutRef<typeof CheckboxPrimitive.Root>
>(({ className, ...props }, ref) => (
  <CheckboxPrimitive.Root
    ref={ref}
    className={cn(
      "peer flex size-4 shrink-0 items-center justify-center rounded-sm border border-(--color-input) bg-(--color-field) outline-none transition-colors",
      // The gap keeps the ring apart from the fill of a checked box.
      "focus-visible:border-(--color-ring) focus-visible:ring-1 focus-visible:ring-(--color-ring) focus-visible:ring-offset-2 focus-visible:ring-offset-(--color-background)",
      "disabled:cursor-not-allowed disabled:opacity-60",
      "data-[state=checked]:border-(--color-primary) data-[state=checked]:bg-(--color-primary) data-[state=checked]:text-(--color-primary-foreground)",
      "data-[state=indeterminate]:border-(--color-primary) data-[state=indeterminate]:bg-(--color-primary) data-[state=indeterminate]:text-(--color-primary-foreground)",
      className,
    )}
    {...props}
  >
    <CheckboxPrimitive.Indicator className="flex items-center justify-center">
      {props.checked === "indeterminate" ? (
        <Minus className="size-3" strokeWidth={3} aria-hidden />
      ) : (
        <Check className="size-3" strokeWidth={3} aria-hidden />
      )}
    </CheckboxPrimitive.Indicator>
  </CheckboxPrimitive.Root>
));
Checkbox.displayName = CheckboxPrimitive.Root.displayName;

export { Checkbox };
