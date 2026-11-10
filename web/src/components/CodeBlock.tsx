import type { SupportedLanguages } from "@pierre/diffs";
import { File } from "@pierre/diffs/react";

import { resolveTheme, useTheme } from "@/lib/theme-context";

// CodeBlock shows a block of code highlighted like the files of a commit,
// wrapping long lines.
export function CodeBlock({ code, lang }: { code: string; lang: SupportedLanguages }) {
  const { theme } = useTheme();
  return (
    // The library styles its own element with a square border, so the frame
    // goes on a wrapper instead.
    <div className="overflow-hidden rounded-md border border-(--color-border) text-xs">
      <File
        file={{ name: `code.${lang}`, contents: code, lang }}
        style={{ border: "none" }}
        options={{
          theme: { light: "pierre-light", dark: "pierre-dark" },
          themeType: resolveTheme(theme),
          disableFileHeader: true,
          disableLineNumbers: true,
          overflow: "wrap",
        }}
      />
    </div>
  );
}
