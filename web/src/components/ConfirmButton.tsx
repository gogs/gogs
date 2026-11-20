import { useMutation } from "@tanstack/react-query";
import type { LucideIcon } from "lucide-react";
import { useState } from "react";
import { useTranslation } from "react-i18next";

import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { toastError } from "@/lib/toast-error";
import { cn } from "@/lib/utils";

// ConfirmButton asks in a dialog before an action that cannot be taken back,
// such as deleting a team or leaving a repository.
export function ConfirmButton({
  label,
  ariaLabel,
  title,
  description,
  size,
  icon: Icon,
  disabled,
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
  // icon shows the button as the icon alone, with the label in a tooltip,
  // e.g., among the actions in the header of a comment.
  icon?: LucideIcon;
  disabled?: boolean;
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
    onError: (err) => toastError(err),
  });
  return (
    <Dialog open={open} onOpenChange={setOpen}>
      {Icon ? (
        <Tooltip>
          <TooltipTrigger asChild>
            <DialogTrigger asChild>
              <Button
                type="button"
                variant="ghost"
                size="icon"
                aria-label={ariaLabel ?? label}
                disabled={disabled}
                className="size-8 text-(--color-muted-foreground) hover:text-(--color-destructive)"
              >
                <Icon className="size-4" aria-hidden />
              </Button>
            </DialogTrigger>
          </TooltipTrigger>
          <TooltipContent>{label}</TooltipContent>
        </Tooltip>
      ) : (
        <DialogTrigger asChild>
          <Button
            type="button"
            variant="outline"
            size={size}
            aria-label={ariaLabel}
            disabled={disabled}
            className={cn("text-(--color-destructive)", size === "sm" && "shrink-0")}
          >
            {label}
          </Button>
        </DialogTrigger>
      )}
      <DialogContent>
        <DialogHeader className="pb-3">
          <DialogTitle className="wrap-anywhere">{title}</DialogTitle>
          <DialogDescription>{description}</DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <Button type="button" variant="outline" onClick={() => setOpen(false)}>
            {t("cancel")}
          </Button>
          <Button
            type="button"
            variant="destructive"
            // A long title wraps, also inside a name without spaces, so the button grows with it.
            className="h-auto min-h-10 min-w-0 wrap-anywhere"
            disabled={mutation.isPending}
            onClick={() => mutation.mutate()}
          >
            {title}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
