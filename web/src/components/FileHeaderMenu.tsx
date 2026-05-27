import { Binary, FileCode2, History, Loader2, MoreHorizontal, Pencil, Trash2, UnfoldVertical } from "lucide-react";
import { type ButtonHTMLAttributes, forwardRef, useState } from "react";
import { useTranslation } from "react-i18next";

import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";

export interface FileHeaderMenuProps {
  filePath: string;
  prevFilePath?: string;
  viewFileHref: string;
  rawFileHref: string;
  historyHref: string;
  // Edit/Delete only make sense when the diff is anchored to a branch (e.g.
  // PR diffs). Omit on commit pages — gogs' editor needs a branch ref, and
  // routing through a SHA returns 404.
  editFileHref?: string;
  deleteFileHref?: string;
  // Mobile-only "Expand all lines" surfaced inside the menu (only visible
  // below `lg`). The desktop chrome renders the button inline in the right-
  // side metadata, so it's hidden on desktop here to avoid double-listing.
  onExpandAllLines?: () => void;
  expandAllLinesState?: "loading" | "done";
}

// Per-file overflow menu rendered into Pierre's `renderHeaderMetadata` slot.
// Sits on the right of each file header (next to the +/- stats and collapse
// chevron) and matches GitHub's three-dot pattern.
export function FileHeaderMenu({
  prevFilePath,
  viewFileHref,
  rawFileHref,
  historyHref,
  editFileHref,
  deleteFileHref,
  onExpandAllLines,
  expandAllLinesState,
}: FileHeaderMenuProps) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const expandLoading = expandAllLinesState === "loading";
  const expandDone = expandAllLinesState === "done";

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <Tooltip>
        <TooltipTrigger asChild>
          <PopoverTrigger asChild>
            <MenuTrigger aria-label={t("diff.more_actions")} />
          </PopoverTrigger>
        </TooltipTrigger>
        {!open ? <TooltipContent>{t("diff.more_actions")}</TooltipContent> : null}
      </Tooltip>
      <PopoverContent align="end" sideOffset={4} className="w-48 p-1 text-sm">
        <ul role="menu" className="flex flex-col">
          {onExpandAllLines ? (
            <li className="lg:hidden">
              <button
                type="button"
                role="menuitem"
                disabled={expandLoading || expandDone}
                onClick={() => {
                  onExpandAllLines();
                  setOpen(false);
                }}
                className="flex w-full cursor-pointer items-center gap-2 rounded px-2 py-1.5 text-left hover:bg-(--color-surface) disabled:cursor-not-allowed disabled:opacity-50 disabled:hover:bg-transparent"
              >
                {expandLoading ? (
                  <Loader2 className="size-3.5 shrink-0 animate-spin" aria-hidden />
                ) : (
                  <UnfoldVertical className="size-3.5 shrink-0" aria-hidden />
                )}
                <span>{expandDone ? t("diff.all_lines_expanded") : t("diff.expand_all_lines")}</span>
              </button>
            </li>
          ) : null}
          {onExpandAllLines ? <li role="separator" className="my-1 h-px bg-(--color-border) lg:hidden" /> : null}
          <li>
            <a
              href={viewFileHref}
              role="menuitem"
              className="flex items-center gap-2 rounded px-2 py-1.5 hover:bg-(--color-surface)"
            >
              <FileCode2 className="size-3.5 shrink-0" aria-hidden />
              <span>{t("diff.view_file")}</span>
            </a>
          </li>
          <li>
            <a
              href={rawFileHref}
              role="menuitem"
              className="flex items-center gap-2 rounded px-2 py-1.5 hover:bg-(--color-surface)"
            >
              <Binary className="size-3.5 shrink-0" aria-hidden />
              <span>{t("diff.view_raw")}</span>
            </a>
          </li>
          <li>
            <a
              href={historyHref}
              role="menuitem"
              className="flex items-center gap-2 rounded px-2 py-1.5 hover:bg-(--color-surface)"
            >
              <History className="size-3.5 shrink-0" aria-hidden />
              <span>{t("diff.view_history")}</span>
            </a>
          </li>
          {editFileHref || deleteFileHref ? <li role="separator" className="my-1 h-px bg-(--color-border)" /> : null}
          {editFileHref ? (
            <li>
              <a
                href={editFileHref}
                role="menuitem"
                className="flex items-center gap-2 rounded px-2 py-1.5 hover:bg-(--color-surface)"
              >
                <Pencil className="size-3.5 shrink-0" aria-hidden />
                <span>{t("editor.edit_file")}</span>
              </a>
            </li>
          ) : null}
          {deleteFileHref ? (
            <li>
              <a
                href={deleteFileHref}
                role="menuitem"
                className="flex items-center gap-2 rounded px-2 py-1.5 text-(--color-destructive) hover:bg-(--color-surface)"
              >
                <Trash2 className="size-3.5 shrink-0" aria-hidden />
                <span>{t("editor.delete_this_file")}</span>
              </a>
            </li>
          ) : null}
          {prevFilePath ? (
            <>
              <li role="separator" className="my-1 h-px bg-(--color-border)" />
              <li role="menuitem" aria-disabled className="px-2 py-1.5 text-xs text-(--color-muted-foreground)">
                {t("diff.renamed_from")} <span className="font-mono">{prevFilePath}</span>
              </li>
            </>
          ) : null}
        </ul>
      </PopoverContent>
    </Popover>
  );
}

// Pierre's `renderHeaderMetadata` callback returns the node into a slot inside
// its rendered header. Radix's `asChild` requires a forwardRef so the popover
// can attach its trigger refs and ARIA wiring.
// Pierre's `<diffs-container>` attaches gesture handlers higher up the tree
// that interpret clicks anywhere inside the file header. Without stopping
// propagation, clicking the kebab triggers Pierre's header click path,
// which steals focus and bubbles through to the next focusable navbar link.
// Stop the events at the source so the menu trigger behaves like a normal
// button in isolation.
const MenuTrigger = forwardRef<HTMLButtonElement, ButtonHTMLAttributes<HTMLButtonElement>>(function MenuTrigger(
  { onPointerDown, onClick, onMouseDown, ...rest },
  ref,
) {
  return (
    <button
      ref={ref}
      type="button"
      className="grid size-6 cursor-pointer place-items-center rounded text-(--color-muted-foreground) hover:bg-(--color-surface) hover:text-(--color-foreground)"
      onPointerDown={(e) => {
        e.stopPropagation();
        onPointerDown?.(e);
      }}
      onMouseDown={(e) => {
        e.stopPropagation();
        onMouseDown?.(e);
      }}
      onClick={(e) => {
        e.stopPropagation();
        onClick?.(e);
      }}
      {...rest}
    >
      <MoreHorizontal className="size-4" aria-hidden />
    </button>
  );
});
