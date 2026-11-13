import { Check, Copy } from "lucide-react";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import type { RepoCloneLinks } from "@/lib/queries/repo";
import { cn } from "@/lib/utils";

// The server-rendered pages remember the protocol under the same key.
const PROTOCOL_KEY = "repo-clone-protocol";

// CloneBox shows the clone URL of a repository in the protocol the viewer
// last picked, with a button to copy it. label names the URL for assistive
// technology.
export function CloneBox({ links, label }: { links: RepoCloneLinks; label: string }) {
  const { t } = useTranslation();
  const protocols = [
    ...(links.https
      ? [{ key: "https", label: links.https.startsWith("https:") ? "HTTPS" : "HTTP", url: links.https }]
      : []),
    ...(links.ssh ? [{ key: "ssh", label: "SSH", url: links.ssh }] : []),
  ];
  const [selected, setSelected] = useState(() => {
    const stored = window.localStorage.getItem(PROTOCOL_KEY);
    return protocols.some((p) => p.key === stored) ? stored : protocols[0]?.key;
  });
  const [copied, setCopied] = useState(false);
  if (!protocols.length) return null;
  const url = protocols.find((p) => p.key === selected)?.url ?? protocols[0].url;
  const copy = async () => {
    try {
      await navigator.clipboard.writeText(url);
      setCopied(true);
      window.setTimeout(() => setCopied(false), 1500);
    } catch {
      toast.error(t("copy_failed"));
    }
  };
  return (
    <div className="flex flex-col gap-2">
      {protocols.length > 1 ? (
        <div role="group" aria-label={label} className="flex gap-1">
          {protocols.map((p) => (
            <button
              key={p.key}
              type="button"
              aria-pressed={p.key === selected}
              onClick={() => {
                setSelected(p.key);
                window.localStorage.setItem(PROTOCOL_KEY, p.key);
              }}
              className={cn(
                "min-h-7 cursor-pointer rounded-md px-2.5 text-xs",
                p.key === selected
                  ? "bg-(--color-surface) font-semibold text-(--color-foreground)"
                  : "text-(--color-muted-foreground) hover:text-(--color-foreground)",
              )}
            >
              {p.label}
            </button>
          ))}
        </div>
      ) : null}
      <div className="flex gap-1.5">
        <Input
          readOnly
          value={url}
          aria-label={label}
          onFocus={(e) => e.target.select()}
          className="h-8 min-w-0 flex-1 font-mono text-xs"
        />
        <Button type="button" variant="outline" size="sm" className="h-8 shrink-0" onClick={() => void copy()}>
          {copied ? <Check className="size-3.5" aria-hidden /> : <Copy className="size-3.5" aria-hidden />}
          <span aria-live="polite">{copied ? t("repo.copy_link_success") : t("repo.copy_link")}</span>
        </Button>
      </div>
    </div>
  );
}
