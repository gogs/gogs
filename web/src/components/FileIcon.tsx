import { createFileTreeIconResolver, getBuiltInSpriteSheet } from "@pierre/trees";

import { cn } from "@/lib/utils";

// The same icons as the diff file tree (DiffFileTree), so a file looks the
// same in the code browser and in diffs.
const ICON_SET = "standard";
const resolver = createFileTreeIconResolver({ set: ICON_SET, colored: true });

// The tree colors its icons inside its shadow DOM with light-dark(), which
// follows the system rather than the app's theme, so the palette is copied
// here as light and dark pairs.
const PALETTE: Record<string, [light: string, dark: string]> = {
  gray: ["#84848a", "#adadb1"],
  red: ["#d52c36", "#ff6762"],
  vermilion: ["#ff8c5b", "#d5512f"],
  orange: ["#d47628", "#ffa359"],
  yellow: ["#d5a910", "#ffd452"],
  green: ["#199f43", "#5ecc71"],
  teal: ["#17a5af", "#64d1db"],
  cyan: ["#1ca1c7", "#68cdf2"],
  blue: ["#1a85d4", "#69b1ff"],
  indigo: ["#693acf", "#9d6afb"],
  purple: ["#a631be", "#d568ea"],
  pink: ["#d32a61", "#ff678d"],
  mauve: ["#594c5b", "#79697b"],
};
const TOKEN_COLORS: Record<string, keyof typeof PALETTE> = {
  astro: "purple",
  babel: "yellow",
  bash: "green",
  biome: "blue",
  bootstrap: "indigo",
  browserslist: "yellow",
  bun: "mauve",
  c: "blue",
  cpp: "blue",
  claude: "orange",
  css: "indigo",
  database: "purple",
  docker: "blue",
  eslint: "indigo",
  git: "vermilion",
  go: "cyan",
  graphql: "pink",
  html: "orange",
  image: "pink",
  javascript: "yellow",
  json: "orange",
  markdown: "green",
  mcp: "teal",
  npm: "red",
  oxc: "cyan",
  postcss: "red",
  prettier: "teal",
  python: "blue",
  react: "cyan",
  ruby: "red",
  rust: "orange",
  sass: "pink",
  svg: "orange",
  svelte: "red",
  svgo: "green",
  swift: "orange",
  table: "teal",
  tailwind: "cyan",
  terraform: "indigo",
  typescript: "blue",
  vite: "purple",
  vscode: "blue",
  vue: "green",
  wasm: "indigo",
  webpack: "blue",
  yml: "red",
  zig: "orange",
  zip: "orange",
};

const SPRITE_ID = "file-icon-sprite";
if (!document.getElementById(SPRITE_ID)) {
  const sprite = document.createElement("div");
  sprite.id = SPRITE_ID;
  sprite.className = "absolute size-0 overflow-hidden";
  sprite.innerHTML = getBuiltInSpriteSheet(ICON_SET);
  document.body.prepend(sprite);
}

// FileIcon shows the icon of a file by its name, e.g., the Go gopher for
// "main.go".
export function FileIcon({ name, className }: { name: string; className?: string }) {
  const icon = resolver.resolveIcon("file-tree-icon-file", name);
  const [light, dark] = PALETTE[TOKEN_COLORS[icon.token ?? ""] ?? "gray"];
  return (
    <svg
      viewBox={icon.viewBox ?? "0 0 16 16"}
      aria-hidden
      className={cn("size-4 shrink-0 text-(--file-icon-light) dark:text-(--file-icon-dark)", className)}
      style={{ "--file-icon-light": light, "--file-icon-dark": dark } as React.CSSProperties}
    >
      <use href={`#${icon.name}`} />
    </svg>
  );
}
