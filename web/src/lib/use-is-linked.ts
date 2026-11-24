import { useRouter, useRouterState } from "@tanstack/react-router";
import { useEffect } from "react";

// useIsLinked reports whether the URL links to the anchor, e.g., to a comment,
// which is then highlighted like on GitHub. Escape drops the anchor from the
// URL, which removes the highlight.
export function useIsLinked(anchor: string) {
  const router = useRouter();
  const linked = useRouterState({ select: (state) => state.location.hash === anchor });
  useEffect(() => {
    if (!linked) return;
    const onKeyDown = (e: KeyboardEvent) => {
      // An open menu or dialog closes on the same key first.
      if (
        e.key !== "Escape" ||
        e.defaultPrevented ||
        document.querySelector('[role="dialog"][data-state="open"], [role="menu"][data-state="open"]')
      ) {
        return;
      }
      void router.navigate({ to: ".", hash: "", params: true, search: true, replace: true, resetScroll: false });
    };
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [linked, router]);
  return linked;
}
