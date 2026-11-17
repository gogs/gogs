import type { CSSProperties } from "react";

// TREE_THEME_STYLE bridges the app tokens into the shadow root of the
// @pierre/trees file trees.
export const TREE_THEME_STYLE: CSSProperties = {
  // @ts-expect-error -- CSS custom properties are valid in style objects.
  "--trees-fg-override": "var(--color-foreground)",
  "--trees-fg-muted-override": "var(--color-muted-foreground)",
  "--trees-bg-override": "var(--glass-pane, var(--color-background))",
  "--trees-bg-muted-override": "var(--color-surface)",
  "--trees-accent-override": "var(--color-primary)",
  "--trees-border-color-override": "var(--color-border)",
  "--trees-selected-bg-override": "var(--color-surface)",
  "--trees-focus-ring-color-override": "var(--color-ring)",
  // `--trees-input-bg` uses `light-dark()` off the shadow host's
  // `color-scheme`, which follows the OS, not the app's class-based theme.
  "--trees-search-bg-override": "var(--color-surface)",
  "--trees-search-fg-override": "var(--color-foreground)",
};
