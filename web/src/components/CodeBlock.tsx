import type { SupportedLanguages } from "@pierre/diffs";
import { File } from "@pierre/diffs/react";
import { Check, Copy } from "lucide-react";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";

import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { resolveTheme, useTheme } from "@/lib/theme-context";

// CodeBlock shows a block of code highlighted like the files of a commit,
// wrapping long lines, with a button to copy it where pointers can hover.
export function CodeBlock({ code, lang }: { code: string; lang: SupportedLanguages }) {
  const { t } = useTranslation();
  const { theme } = useTheme();
  const [copied, setCopied] = useState(false);
  const [tooltipOpen, setTooltipOpen] = useState(false);
  const copy = async () => {
    try {
      await navigator.clipboard.writeText(code);
      setCopied(true);
      window.setTimeout(() => setCopied(false), 1500);
    } catch {
      toast.error(t("copy_failed"));
    }
  };
  const label = copied ? t("copied") : t("repo.copy_link");
  return (
    // The library styles its own element with a square border, so the frame
    // goes on a wrapper instead.
    <div className="group relative overflow-hidden rounded-md border border-(--color-border) text-xs">
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
      {/* The tooltip closes on click, so it stays open while it confirms the copy. */}
      <Tooltip open={tooltipOpen || copied} onOpenChange={setTooltipOpen}>
        <TooltipTrigger asChild>
          {/* Pointers that can hover reveal the button on hover. Touch screens
              hide it, since it would cover the end of the first line. */}
          <button
            type="button"
            onClick={() => void copy()}
            aria-label={label}
            className="absolute top-1.5 right-1.5 grid size-7 cursor-pointer place-items-center rounded-md border border-(--color-border) bg-(--color-background) text-(--color-muted-foreground) group-hover:opacity-100 hover:text-(--color-foreground) focus-visible:opacity-100 pointer-coarse:hidden pointer-fine:opacity-0"
          >
            {copied ? (
              <Check className="size-3.5 text-(--color-success)" aria-hidden />
            ) : (
              <Copy className="size-3.5" aria-hidden />
            )}
          </button>
        </TooltipTrigger>
        <TooltipContent>{label}</TooltipContent>
      </Tooltip>
    </div>
  );
}
