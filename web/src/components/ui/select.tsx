import * as SelectPrimitive from "@radix-ui/react-select";
import { Check, ChevronDown } from "lucide-react";
import * as React from "react";

import { cn } from "@/lib/utils";

type SelectAria = Pick<React.AriaAttributes, "aria-invalid" | "aria-describedby">;

const SelectAriaContext = React.createContext<SelectAria>({});

// Select hands the error state and description that FormField gives it to its
// trigger, since the root renders no element of its own.
function Select({
  "aria-invalid": ariaInvalid,
  "aria-describedby": ariaDescribedBy,
  ...props
}: React.ComponentProps<typeof SelectPrimitive.Root> & SelectAria) {
  const aria = React.useMemo(
    () => ({ "aria-invalid": ariaInvalid, "aria-describedby": ariaDescribedBy }),
    [ariaInvalid, ariaDescribedBy],
  );
  return (
    <SelectAriaContext.Provider value={aria}>
      <SelectPrimitive.Root {...props} />
    </SelectAriaContext.Provider>
  );
}

const SelectGroup = SelectPrimitive.Group;
const SelectValue = SelectPrimitive.Value;

function SelectTrigger({ className, children, ...props }: React.ComponentProps<typeof SelectPrimitive.Trigger>) {
  const aria = React.use(SelectAriaContext);
  return (
    <SelectPrimitive.Trigger
      {...aria}
      className={cn(
        "flex h-10 w-full items-center justify-between rounded-md border border-(--color-input) bg-(--color-field) px-3 py-2 text-sm text-(--color-foreground) outline-none transition-colors",
        "focus-visible:border-(--color-ring) focus-visible:ring-1 focus-visible:ring-(--color-ring)",
        "aria-invalid:border-(--color-destructive) aria-invalid:focus-visible:border-(--color-destructive) aria-invalid:focus-visible:ring-(--color-destructive)",
        "disabled:cursor-not-allowed disabled:opacity-60",
        "data-[placeholder]:text-(--color-muted-foreground)",
        className,
      )}
      {...props}
    >
      {children}
      <SelectPrimitive.Icon asChild>
        <ChevronDown className="size-4 text-(--color-muted-foreground)" aria-hidden />
      </SelectPrimitive.Icon>
    </SelectPrimitive.Trigger>
  );
}

function SelectContent({
  className,
  children,
  position = "popper",
  ...props
}: React.ComponentProps<typeof SelectPrimitive.Content>) {
  return (
    <SelectPrimitive.Portal>
      <SelectPrimitive.Content
        position={position}
        className={cn(
          "relative z-50 max-h-(--radix-select-content-available-height) min-w-(--radix-select-trigger-width) overflow-hidden rounded-md border border-(--color-border) bg-(--color-popover) text-(--color-popover-foreground) shadow-md backdrop-blur-xl outline-none",
          "data-[state=open]:animate-in data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0 data-[state=closed]:zoom-out-95 data-[state=open]:zoom-in-95",
          position === "popper" && "data-[side=bottom]:translate-y-1 data-[side=top]:-translate-y-1",
          className,
        )}
        {...props}
      >
        <SelectPrimitive.Viewport className="p-1">{children}</SelectPrimitive.Viewport>
      </SelectPrimitive.Content>
    </SelectPrimitive.Portal>
  );
}

function SelectItem({ className, children, ...props }: React.ComponentProps<typeof SelectPrimitive.Item>) {
  return (
    <SelectPrimitive.Item
      className={cn(
        "relative flex w-full cursor-pointer select-none items-center rounded-sm py-1.5 pl-8 pr-2 text-sm outline-none",
        "focus:bg-(--color-surface) data-[disabled]:pointer-events-none data-[disabled]:opacity-60",
        className,
      )}
      {...props}
    >
      <span className="absolute left-2 flex size-3.5 items-center justify-center">
        <SelectPrimitive.ItemIndicator>
          <Check className="size-4" aria-hidden />
        </SelectPrimitive.ItemIndicator>
      </span>
      <SelectPrimitive.ItemText>{children}</SelectPrimitive.ItemText>
    </SelectPrimitive.Item>
  );
}

export { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue };
