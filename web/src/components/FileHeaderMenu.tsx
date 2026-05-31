import { Binary, FileCode2, History, Loader2, MoreHorizontal, Pencil, Trash2, UnfoldVertical } from "lucide-react";
import { type ButtonHTMLAttributes, type Ref, useState } from "react";
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
  // PR diffs). Omit on commit pages, since gogs' editor needs a branch ref
  // and routing through a SHA returns 404.
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
            <MenuTrigger aria-label={t("more_actions")} />
          </PopoverTrigger>
        </TooltipTrigger>
        {!open ? <TooltipContent>{t("more_actions")}</TooltipContent> : null}
      </Tooltip>
      <PopoverContent align="end" sideOffset={4} className="w-48 p-1 text-sm">
        <ul className="flex flex-col">
          {onExpandAllLines ? (
            <li className="lg:hidden">
              <button
                type="button"
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
                <span>{expandDone ? t("repo.diff.all_lines_expanded") : t("repo.diff.expand_all_lines")}</span>
              </button>
            </li>
          ) : null}
          {onExpandAllLines ? (
            <li role="presentation" className="lg:hidden">
              <hr className="my-1 border-t border-(--color-border)" />
            </li>
          ) : null}
          <li>
            <a href={viewFileHref} className="flex items-center gap-2 rounded px-2 py-1.5 hover:bg-(--color-surface)">
              <FileCode2 className="size-3.5 shrink-0" aria-hidden />
              <span>{t("repo.view_file")}</span>
            </a>
          </li>
          <li>
            <a href={rawFileHref} className="flex items-center gap-2 rounded px-2 py-1.5 hover:bg-(--color-surface)">
              <Binary className="size-3.5 shrink-0" aria-hidden />
              <span>{t("repo.view_raw")}</span>
            </a>
          </li>
          <li>
            <a href={historyHref} className="flex items-center gap-2 rounded px-2 py-1.5 hover:bg-(--color-surface)">
              <History className="size-3.5 shrink-0" aria-hidden />
              <span>{t("repo.view_history")}</span>
            </a>
          </li>
          {editFileHref || deleteFileHref ? (
            <li role="presentation">
              <hr className="my-1 border-t border-(--color-border)" />
            </li>
          ) : null}
          {editFileHref ? (
            <li>
              <a href={editFileHref} className="flex items-center gap-2 rounded px-2 py-1.5 hover:bg-(--color-surface)">
                <Pencil className="size-3.5 shrink-0" aria-hidden />
                <span>{t("repo.editor.edit_file")}</span>
              </a>
            </li>
          ) : null}
          {deleteFileHref ? (
            <li>
              <a
                href={deleteFileHref}
                className="flex items-center gap-2 rounded px-2 py-1.5 text-(--color-destructive) hover:bg-(--color-surface)"
              >
                <Trash2 className="size-3.5 shrink-0" aria-hidden />
                <span>{t("repo.editor.delete_this_file")}</span>
              </a>
            </li>
          ) : null}
          {prevFilePath ? (
            <>
              <li role="presentation">
                <hr className="my-1 border-t border-(--color-border)" />
              </li>
              <li className="px-2 py-1.5 text-xs text-(--color-muted-foreground)">
                {t("repo.renamed_from")} <span className="font-mono">{prevFilePath}</span>
              </li>
            </>
          ) : null}
        </ul>
      </PopoverContent>
    </Popover>
  );
}

// Pierre's `renderHeaderMetadata` callback returns the node into a slot inside
// its rendered header. Radix's `asChild` needs the trigger to accept a ref so
// the popover can attach its trigger refs and ARIA wiring; on React 19 that is
// a regular `ref` prop.
// Pierre's `<diffs-container>` attaches gesture handlers higher up the tree
// that interpret clicks anywhere inside the file header. Without stopping
// propagation, clicking the kebab triggers Pierre's header click path,
// which steals focus and bubbles through to the next focusable navbar link.
// Stop the events at the source so the menu trigger behaves like a normal
// button in isolation.
function MenuTrigger({
  onPointerDown,
  onClick,
  onMouseDown,
  ref,
  ...rest
}: ButtonHTMLAttributes<HTMLButtonElement> & { ref?: Ref<HTMLButtonElement> }) {
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
}
