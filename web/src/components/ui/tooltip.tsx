import * as TooltipPrimitive from "@radix-ui/react-tooltip";
import * as React from "react";

import { cn } from "@/lib/utils";

const TooltipProvider = TooltipPrimitive.Provider;

// Radix keeps a tooltip open while the pointer travels toward it, so the
// pointer can move onto it, and opens no other tooltip meanwhile. Next to a
// wide tooltip, that blocks the tooltips of neighboring controls. Instead,
// entering another trigger closes the open tooltip and opens that one.
let closeOpenTooltip: (() => void) | undefined;
const SetTooltipOpenContext = React.createContext<((open: boolean) => void) | undefined>(undefined);

function Tooltip({
  open: openProp,
  defaultOpen = false,
  onOpenChange,
  ...props
}: React.ComponentProps<typeof TooltipPrimitive.Root>) {
  const [uncontrolledOpen, setUncontrolledOpen] = React.useState(defaultOpen);
  const open = openProp ?? uncontrolledOpen;
  const setOpen = React.useCallback(
    (next: boolean) => {
      setUncontrolledOpen(next);
      onOpenChange?.(next);
    },
    [onOpenChange],
  );

  React.useEffect(() => {
    if (!open) return;
    const close = () => setOpen(false);
    closeOpenTooltip?.();
    closeOpenTooltip = close;
    return () => {
      if (closeOpenTooltip === close) closeOpenTooltip = undefined;
    };
  }, [open, setOpen]);

  return (
    <SetTooltipOpenContext.Provider value={setOpen}>
      <TooltipPrimitive.Root open={open} onOpenChange={setOpen} {...props} />
    </SetTooltipOpenContext.Provider>
  );
}

function TooltipTrigger({ onPointerEnter, ...props }: React.ComponentProps<typeof TooltipPrimitive.Trigger>) {
  const setOpen = React.useContext(SetTooltipOpenContext);
  return (
    <TooltipPrimitive.Trigger
      onPointerEnter={(event) => {
        onPointerEnter?.(event);
        if (event.pointerType !== "touch" && closeOpenTooltip) setOpen?.(true);
      }}
      {...props}
    />
  );
}

function TooltipContent({
  className,
  sideOffset = 4,
  ...props
}: React.ComponentProps<typeof TooltipPrimitive.Content>) {
  return (
    <TooltipPrimitive.Portal>
      <TooltipPrimitive.Content
        sideOffset={sideOffset}
        className={cn(
          "z-50 overflow-hidden rounded-md bg-(--color-foreground) px-2 py-1 text-xs text-(--color-background) shadow-sm",
          "data-[state=delayed-open]:animate-in data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=delayed-open]:fade-in-0 data-[state=closed]:zoom-out-95 data-[state=delayed-open]:zoom-in-95",
          className,
        )}
        {...props}
      />
    </TooltipPrimitive.Portal>
  );
}

export { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger };
