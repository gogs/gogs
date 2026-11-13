import { Check, ChevronDown, GitBranch, GitCommitHorizontal, Search, Tag } from "lucide-react";
import { type ReactNode, useId, useState } from "react";
import { useTranslation } from "react-i18next";

import { Button } from "@/components/ui/button";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import { cn } from "@/lib/utils";

export type RefType = "branch" | "tag" | "commit";

const ITEM_CLASS =
  "flex w-full items-center gap-2 rounded-sm px-2 py-1.5 text-left text-sm hover:bg-(--color-surface) focus-visible:bg-(--color-surface) focus-visible:outline-none";

// RefPicker switches the branch or the tag a page shows. renderLink renders
// the link to the same page for another ref, with the class and the content
// of the item. Without tags, it only picks branches, under the label.
export function RefPicker({
  current,
  type,
  branches,
  tags,
  renderLink,
  label,
}: {
  current: string;
  type: RefType;
  branches: string[];
  tags?: string[];
  label?: string;
  renderLink: (ref: string, className: string, children: ReactNode) => ReactNode;
}) {
  const { t } = useTranslation();
  const id = useId();
  const [open, setOpen] = useState(false);
  const [tab, setTab] = useState<"branches" | "tags">(type === "tag" ? "tags" : "branches");
  const [filter, setFilter] = useState("");
  const refs = (tab === "branches" || !tags ? branches : tags).filter((r) =>
    r.toLowerCase().includes(filter.trim().toLowerCase()),
  );
  const Icon = type === "branch" ? GitBranch : type === "tag" ? Tag : GitCommitHorizontal;
  const tabs = [
    { key: "branches", label: t("repo.branches") },
    { key: "tags", label: t("repo.tags") },
  ] as const;

  return (
    <Popover
      open={open}
      onOpenChange={(next) => {
        setOpen(next);
        if (!next) setFilter("");
      }}
    >
      <PopoverTrigger asChild>
        <Button variant="outline" size="sm" className="max-w-full min-w-0">
          <Icon className="size-4 shrink-0" aria-hidden />
          <span className="text-(--color-muted-foreground)">
            {label ?? (type === "branch" ? t("repo.branch") : type === "tag" ? t("repo.tag") : t("repo.tree"))}:
          </span>
          <span className="min-w-0 truncate font-semibold">{type === "commit" ? current.slice(0, 10) : current}</span>
          <ChevronDown className="size-4 shrink-0" aria-hidden />
        </Button>
      </PopoverTrigger>
      <PopoverContent align="start" collisionPadding={16} className="w-80 max-w-[calc(100vw-2rem)] p-0">
        <div className="relative border-b border-(--color-border) p-2">
          <Search
            className="pointer-events-none absolute top-1/2 left-4 size-4 -translate-y-1/2 text-(--color-muted-foreground)"
            aria-hidden
          />
          <input
            value={filter}
            onChange={(e) => setFilter(e.target.value)}
            placeholder={`${t(tags ? "repo.filter_branch_and_tag" : "repo.pulls.filter_branch")}...`}
            aria-label={t(tags ? "repo.filter_branch_and_tag" : "repo.pulls.filter_branch")}
            autoFocus
            className="block w-full rounded-md border border-(--color-input) bg-(--color-background) py-1.5 pr-3 pl-8 text-sm outline-none placeholder:text-(--color-muted-foreground) focus-visible:border-(--color-ring) focus-visible:ring-1 focus-visible:ring-(--color-ring)"
          />
        </div>
        {tags ? (
          <div
            role="tablist"
            aria-label={t("repo.filter_branch_and_tag")}
            className="flex gap-1 border-b border-(--color-border) px-2 py-1.5"
          >
            {tabs.map((item) => (
              <button
                key={item.key}
                type="button"
                role="tab"
                id={`${id}-${item.key}-tab`}
                aria-selected={tab === item.key}
                aria-controls={`${id}-list`}
                onClick={() => setTab(item.key)}
                className={cn(
                  "cursor-pointer rounded-md px-3 py-1 text-sm",
                  tab === item.key
                    ? "bg-(--color-surface) font-semibold text-(--color-foreground)"
                    : "text-(--color-muted-foreground) hover:text-(--color-foreground)",
                )}
              >
                {item.label}
              </button>
            ))}
          </div>
        ) : null}
        <div
          role={tags ? "tabpanel" : undefined}
          id={`${id}-list`}
          aria-labelledby={tags ? `${id}-${tab}-tab` : undefined}
          className="max-h-72 overflow-y-auto p-1"
        >
          {refs.length ? (
            <ul>
              {refs.map((ref) => (
                <li key={ref} onClick={() => setOpen(false)}>
                  {renderLink(
                    ref,
                    ITEM_CLASS,
                    <>
                      <Check
                        className={cn("size-4 shrink-0", (ref !== current || type === "commit") && "invisible")}
                        aria-hidden
                      />
                      <span className="min-w-0 break-all">{ref}</span>
                    </>,
                  )}
                </li>
              ))}
            </ul>
          ) : (
            <p className="px-2 py-4 text-center text-sm text-(--color-muted-foreground)">
              {t("repo.pulls.no_results")}
            </p>
          )}
        </div>
      </PopoverContent>
    </Popover>
  );
}
