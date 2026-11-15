import { useMutation } from "@tanstack/react-query";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import { cn } from "@/lib/utils";

// ConfirmButton asks in a popover before an action that cannot be taken back,
// such as deleting a team or leaving a repository.
export function ConfirmButton({
  label,
  ariaLabel,
  title,
  description,
  size,
  align = "end",
  action,
  onDone,
}: {
  label: string;
  // ariaLabel names the button when its label alone is ambiguous, e.g., in a
  // list.
  ariaLabel?: string;
  title: string;
  description: string;
  size?: "sm";
  // align lines the popover up with the start or the end of the button.
  align?: "start" | "end";
  action: () => Promise<void>;
  onDone: () => Promise<void> | void;
}) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const mutation = useMutation({
    mutationFn: action,
    onSuccess: async () => {
      setOpen(false);
      await onDone();
    },
    onError: (err) => toast.error(err.message),
  });
  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          type="button"
          variant="outline"
          size={size}
          aria-label={ariaLabel}
          className={cn("text-(--color-destructive)", size === "sm" && "shrink-0")}
        >
          {label}
        </Button>
      </PopoverTrigger>
      <PopoverContent align={align} collisionPadding={16} className="w-80 max-w-[calc(100vw-2rem)] p-4">
        <p className="font-semibold">{title}</p>
        <p className="mt-1 text-sm text-(--color-muted-foreground)">{description}</p>
        <div className="mt-4 flex justify-end gap-2">
          <Button type="button" variant="outline" size="sm" onClick={() => setOpen(false)}>
            {t("cancel")}
          </Button>
          <Button
            type="button"
            variant="destructive"
            size="sm"
            // A long title wraps in narrow popovers, so the button grows with it.
            className="h-auto min-h-8 py-1.5"
            disabled={mutation.isPending}
            onClick={() => mutation.mutate()}
          >
            {title}
          </Button>
        </div>
      </PopoverContent>
    </Popover>
  );
}
