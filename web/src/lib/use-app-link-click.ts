import { useNavigate } from "@tanstack/react-router";
import type { MouseEvent } from "react";

import { routePath } from "./url";

// useAppLinkClick returns a click handler for an element that holds HTML from
// the server, so the links in it that lead to pages of the app navigate like
// router links instead of reloading the document. Links to other sites, and
// clicks that open a link elsewhere, e.g., with Ctrl, are left to the browser.
export function useAppLinkClick() {
  const navigate = useNavigate();
  return (e: MouseEvent) => {
    if (e.button !== 0 || e.metaKey || e.ctrlKey || e.shiftKey || e.altKey) return;
    const link = e.target instanceof Element ? e.target.closest("a") : null;
    if (!link || link.origin !== window.location.origin) return;
    e.preventDefault();
    // The router reads a "$" in a path as a parameter, and the server leaves
    // it unescaped in branch names.
    void navigate({ to: routePath(link.pathname).replaceAll("$", "%24") + link.search + link.hash });
  };
}
