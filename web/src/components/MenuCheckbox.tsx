import { Check } from "lucide-react";
import type { ReactNode } from "react";

// MenuCheckbox is an item of a menu in a popover that is on or off, marked
// with a check.
export function MenuCheckbox({
  checked,
  onSelect,
  children,
}: {
  checked: boolean;
  onSelect: () => void;
  children: ReactNode;
}) {
  return (
    <button
      type="button"
      role="menuitemcheckbox"
      aria-checked={checked}
      onClick={onSelect}
      className="flex w-full cursor-pointer items-center gap-2 rounded px-2 py-1.5 text-sm hover:bg-(--color-surface)"
    >
      <span className="grid size-4 place-items-center">
        {checked ? <Check className="size-3.5 text-(--color-primary)" aria-hidden /> : null}
      </span>
      <span>{children}</span>
    </button>
  );
}
