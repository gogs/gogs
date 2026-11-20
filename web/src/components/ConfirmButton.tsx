import { useMutation } from "@tanstack/react-query";
import { LoaderCircle, type LucideIcon } from "lucide-react";
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
import { isSessionExpired } from "@/lib/queries/request";
import { toastError } from "@/lib/toast-error";
import { cn } from "@/lib/utils";

// Not `disabled`, which would take the focus off the button the dialog returns it to. The text
// dims, not the button, which would dim its focus ring too.
const BUSY = "pointer-events-none text-(--color-destructive)/60";
const BUSY_ICON = "pointer-events-none text-(--color-muted-foreground)/60";

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
  busy,
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
  // busy shows the button as working and keeps the dialog shut, for an action
  // that only starts something, which goes on after the dialog closed.
  busy?: boolean;
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
    onError: (err) => {
      // The toast of an expired session stays up, and an open dialog keeps the pointer and the
      // keyboard from it.
      if (isSessionExpired(err)) setOpen(false);
      toastError(err);
    },
  });
  return (
    <Dialog open={open} onOpenChange={(next) => setOpen(next && !busy)}>
      {Icon ? (
        <Tooltip>
          <TooltipTrigger asChild>
            <DialogTrigger asChild>
              <Button
                type="button"
                variant="ghost"
                size="icon"
                aria-label={ariaLabel ?? label}
                aria-disabled={busy || undefined}
                disabled={disabled}
                className={cn(
                  "size-8 text-(--color-muted-foreground) hover:text-(--color-destructive)",
                  busy && BUSY_ICON,
                )}
              >
                {busy ? (
                  <LoaderCircle className="size-4 animate-spin" aria-hidden />
                ) : (
                  <Icon className="size-4" aria-hidden />
                )}
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
            aria-disabled={busy || undefined}
            disabled={disabled}
            className={cn("text-(--color-destructive)", size === "sm" && "shrink-0", busy && BUSY)}
          >
            {busy ? <LoaderCircle className="size-3.5 animate-spin" aria-hidden /> : null}
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
