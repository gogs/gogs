import { Check, Copy, ExternalLink, FileCode2, MoreHorizontal } from "lucide-react";
import { type ButtonHTMLAttributes, forwardRef, useCallback, useState } from "react";

import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";

export interface FileHeaderMenuProps {
  filePath: string;
  prevFilePath?: string;
  viewFileHref: string;
  blameHref: string;
  permalinkHref: string;
}

// Per-file overflow menu rendered into Pierre's `renderHeaderMetadata` slot.
// Sits on the right of each file header (next to the +/- stats and collapse
// chevron) and matches GitHub's three-dot pattern.
export function FileHeaderMenu({
  filePath,
  prevFilePath,
  viewFileHref,
  blameHref,
  permalinkHref,
}: FileHeaderMenuProps) {
  const [copiedPath, setCopiedPath] = useState(false);
  const [copiedLink, setCopiedLink] = useState(false);
  const [open, setOpen] = useState(false);

  const copyPath = useCallback(() => {
    void (async () => {
      try {
        await navigator.clipboard.writeText(filePath);
        setCopiedPath(true);
        window.setTimeout(() => {
          setCopiedPath(false);
          setOpen(false);
        }, 800);
      } catch {
        // Clipboard API can fail in insecure contexts. The menu still shows
        // the path on the file header so the user can copy manually.
      }
    })();
  }, [filePath]);

  const copyLink = useCallback(() => {
    void (async () => {
      try {
        const absolute = new URL(permalinkHref, window.location.origin).toString();
        await navigator.clipboard.writeText(absolute);
        setCopiedLink(true);
        window.setTimeout(() => {
          setCopiedLink(false);
          setOpen(false);
        }, 800);
      } catch {
        // See note above.
      }
    })();
  }, [permalinkHref]);

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <MenuTrigger />
      </PopoverTrigger>
      <PopoverContent align="end" sideOffset={4} className="w-56 p-1 text-sm">
        <ul role="menu" className="flex flex-col">
          <li>
            <a
              href={viewFileHref}
              role="menuitem"
              className="flex items-center gap-2 rounded px-2 py-1.5 hover:bg-(--color-surface)"
            >
              <FileCode2 className="size-3.5 shrink-0" aria-hidden />
              <span>View file at this commit</span>
            </a>
          </li>
          <li>
            <a
              href={blameHref}
              role="menuitem"
              className="flex items-center gap-2 rounded px-2 py-1.5 hover:bg-(--color-surface)"
            >
              <ExternalLink className="size-3.5 shrink-0" aria-hidden />
              <span>View blame</span>
            </a>
          </li>
          <li role="separator" className="my-1 h-px bg-(--color-border)" />
          <li>
            <button
              type="button"
              role="menuitem"
              onClick={copyPath}
              className="flex w-full cursor-pointer items-center gap-2 rounded px-2 py-1.5 text-left hover:bg-(--color-surface)"
            >
              {copiedPath ? (
                <Check className="size-3.5 shrink-0 text-(--color-success)" aria-hidden />
              ) : (
                <Copy className="size-3.5 shrink-0" aria-hidden />
              )}
              <span>Copy file path</span>
            </button>
          </li>
          <li>
            <button
              type="button"
              role="menuitem"
              onClick={copyLink}
              className="flex w-full cursor-pointer items-center gap-2 rounded px-2 py-1.5 text-left hover:bg-(--color-surface)"
            >
              {copiedLink ? (
                <Check className="size-3.5 shrink-0 text-(--color-success)" aria-hidden />
              ) : (
                <Copy className="size-3.5 shrink-0" aria-hidden />
              )}
              <span>Copy permalink</span>
            </button>
          </li>
          {prevFilePath ? (
            <>
              <li role="separator" className="my-1 h-px bg-(--color-border)" />
              <li role="menuitem" aria-disabled className="px-2 py-1.5 text-xs text-(--color-muted-foreground)">
                Renamed from <span className="font-mono">{prevFilePath}</span>
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
      aria-label="More actions"
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
