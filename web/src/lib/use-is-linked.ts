import { useRouterState } from "@tanstack/react-router";

// useIsLinked reports whether the URL links to the anchor, e.g., to a comment,
// which is then highlighted like on GitHub.
export function useIsLinked(anchor: string) {
  return useRouterState({ select: (state) => state.location.hash === anchor });
}
