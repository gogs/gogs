import { type KeyboardEvent, useId, useRef } from "react";

const FOCUSABLE = 'a[href], button:not(:disabled), input:not(:disabled), [tabindex]:not([tabindex="-1"])';

// useMenuFocus leads the keyboard through a popover menu. It returns the props
// of the trigger and of the content of the popover.
//
// The menu itself takes the focus when it opens, so Tab goes on to its first
// control. Focusing that control instead would ring it after a click, and an
// Enter right after opening would press it. Leaving the focus on the trigger
// would send Tab past the menu.
export function useMenuFocus() {
  const id = useId();
  const trigger = useRef<HTMLButtonElement>(null);
  const closedOutside = useRef(false);
  return {
    trigger: { id, ref: trigger },
    content: {
      // The menu is a dialog, which the trigger names.
      "aria-labelledby": id,
      tabIndex: -1,
      onOpenAutoFocus: (e: Event) => {
        e.preventDefault();
        (e.currentTarget as HTMLElement | null)?.focus();
      },
      // Shift+Tab from the menu itself would go to whatever is before it in the document, which is
      // the end of the page.
      onKeyDown: (e: KeyboardEvent<HTMLElement>) => {
        if (e.key !== "Tab" || !e.shiftKey || e.target !== e.currentTarget) return;
        const last = [...e.currentTarget.querySelectorAll<HTMLElement>(FOCUSABLE)].at(-1);
        if (!last) return;
        e.preventDefault();
        last.focus();
      },
      onInteractOutside: () => {
        closedOutside.current = true;
      },
      // The focus goes back to the trigger unless the viewer went on to something else. Without
      // `preventScroll`, Chromium scrolls a page to its top to show a trigger in a sticky bar.
      onCloseAutoFocus: (e: Event) => {
        e.preventDefault();
        if (!closedOutside.current) trigger.current?.focus({ preventScroll: true });
        closedOutside.current = false;
      },
    },
  };
}
