import {
  Check,
  ChevronDown,
  Maximize2,
  Minimize2,
  PanelLeftClose,
  PanelLeftOpen,
  Settings2,
  SlidersHorizontal,
} from "lucide-react";
import { type ReactNode, forwardRef } from "react";

import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";
import type { DiffFileStatus } from "@/pages/repo/Commit.search";

export type DiffStyle = "unified" | "split";

// Whitespace UX vocabulary, including the explicit "show" state that the URL
// represents as absence. See `repo/Commit.search.ts` for the URL-side type.
export type WhitespaceMode = "show" | "ignore-all" | "ignore-change";

export interface DiffToolbarStats {
  fileCount: number;
  additions: number;
  deletions: number;
}

export interface DiffToolbarSettings {
  diffStyle: DiffStyle;
  wrapLines: boolean;
  statusFilter: Record<DiffFileStatus, boolean>;
}

export interface DiffToolbarProps {
  stats: DiffToolbarStats;
  settings: DiffToolbarSettings;
  onSettingsChange: (next: DiffToolbarSettings) => void;
  whitespace: WhitespaceMode;
  onWhitespaceChange: (next: WhitespaceMode) => void;
  onExpandAll: () => void;
  onCollapseAll: () => void;
  // Mobile sheet trigger: opens the slide-over file tree below `lg` since
  // the desktop sidebar doesn't render at that breakpoint.
  onShowTreeMobile?: () => void;
  // Desktop sidebar toggle: shows when the sidebar is collapsed, hides
  // when it's open. Same icon slot as the mobile trigger; CSS picks which
  // one shows at each breakpoint.
  onToggleTreeDesktop?: () => void;
  desktopTreeOpen?: boolean;
}

const STATUS_LABELS: Record<DiffFileStatus, string> = {
  added: "Added",
  modified: "Modified",
  deleted: "Deleted",
  renamed: "Renamed",
};

export function DiffToolbar({
  stats,
  settings,
  onSettingsChange,
  whitespace,
  onWhitespaceChange,
  onExpandAll,
  onCollapseAll,
  onShowTreeMobile,
  onToggleTreeDesktop,
  desktopTreeOpen,
}: DiffToolbarProps) {
  const setStyle = (diffStyle: DiffStyle) => onSettingsChange({ ...settings, diffStyle });
  const setWrap = (wrapLines: boolean) => onSettingsChange({ ...settings, wrapLines });
  const setStatus = (key: DiffFileStatus, value: boolean) =>
    onSettingsChange({
      ...settings,
      statusFilter: { ...settings.statusFilter, [key]: value },
    });

  return (
    <div className="flex flex-wrap items-center justify-between gap-3 border-b border-(--color-border) bg-(--color-background) px-4 py-2 text-sm">
      <div className="flex items-center gap-3">
        <span className="flex items-center gap-1.5 text-(--color-foreground)">
          {onShowTreeMobile ? (
            <Tooltip>
              <TooltipTrigger asChild>
                <button
                  type="button"
                  onClick={onShowTreeMobile}
                  aria-label="Show file tree"
                  className="grid size-6 cursor-pointer place-items-center rounded text-(--color-muted-foreground) hover:bg-(--color-surface) hover:text-(--color-foreground) lg:hidden"
                >
                  <PanelLeftOpen className="size-4" aria-hidden />
                </button>
              </TooltipTrigger>
              <TooltipContent>Show file tree</TooltipContent>
            </Tooltip>
          ) : null}
          {onToggleTreeDesktop ? (
            <Tooltip>
              <TooltipTrigger asChild>
                <button
                  type="button"
                  onClick={onToggleTreeDesktop}
                  aria-label={desktopTreeOpen ? "Hide file tree" : "Show file tree"}
                  aria-pressed={desktopTreeOpen}
                  className="hidden size-6 cursor-pointer place-items-center rounded text-(--color-muted-foreground) hover:bg-(--color-surface) hover:text-(--color-foreground) lg:grid"
                >
                  {desktopTreeOpen ? (
                    <PanelLeftClose className="size-4" aria-hidden />
                  ) : (
                    <PanelLeftOpen className="size-4" aria-hidden />
                  )}
                </button>
              </TooltipTrigger>
              <TooltipContent>{desktopTreeOpen ? "Hide file tree" : "Show file tree"}</TooltipContent>
            </Tooltip>
          ) : null}
          <span>
            Showing{" "}
            <strong className="font-semibold">
              {stats.fileCount} changed {stats.fileCount === 1 ? "file" : "files"}
            </strong>
          </span>
        </span>
        <span className="flex items-center gap-1 text-(--color-muted-foreground)">
          <span aria-hidden className="size-1.5 rounded-full bg-(--color-diff-added)" />
          <span className="tabular-nums">+{stats.additions.toLocaleString()} additions</span>
        </span>
        <span className="flex items-center gap-1 text-(--color-muted-foreground)">
          <span aria-hidden className="size-1.5 rounded-full bg-(--color-diff-removed)" />
          <span className="tabular-nums">-{stats.deletions.toLocaleString()} deletions</span>
        </span>
      </div>

      <div className="flex flex-wrap items-center gap-2">
        <div className="inline-flex h-7 items-stretch overflow-hidden rounded-md border border-(--color-border) text-xs">
          <SegmentButton active={settings.diffStyle === "unified"} onClick={() => setStyle("unified")}>
            Unified
          </SegmentButton>
          <SegmentButton active={settings.diffStyle === "split"} onClick={() => setStyle("split")}>
            Split
          </SegmentButton>
        </div>

        <Popover>
          <PopoverTrigger asChild>
            <ToolbarButton icon={SlidersHorizontal} label="Diff settings" />
          </PopoverTrigger>
          <PopoverContent align="end" className="w-64 p-2">
            <div className="px-2 pb-1 text-xs font-semibold tracking-wide text-(--color-muted-foreground) uppercase">
              Whitespace
            </div>
            <MenuRadio checked={whitespace === "show"} onSelect={() => onWhitespaceChange("show")}>
              Show whitespace
            </MenuRadio>
            <MenuRadio checked={whitespace === "ignore-change"} onSelect={() => onWhitespaceChange("ignore-change")}>
              Ignore whitespace changes
            </MenuRadio>
            <MenuRadio checked={whitespace === "ignore-all"} onSelect={() => onWhitespaceChange("ignore-all")}>
              Ignore all whitespace
            </MenuRadio>
            <div className="my-1 h-px bg-(--color-border)" />
            <div className="px-2 pb-1 text-xs font-semibold tracking-wide text-(--color-muted-foreground) uppercase">
              Display
            </div>
            <MenuCheckbox checked={settings.wrapLines} onSelect={() => setWrap(!settings.wrapLines)}>
              Wrap long lines
            </MenuCheckbox>
          </PopoverContent>
        </Popover>

        <Popover>
          <PopoverTrigger asChild>
            <ToolbarButton icon={Settings2} label="Filter files" />
          </PopoverTrigger>
          <PopoverContent align="end" className="w-64 p-2">
            <div className="px-2 pb-1 text-xs font-semibold tracking-wide text-(--color-muted-foreground) uppercase">
              By status
            </div>
            {(Object.keys(STATUS_LABELS) as DiffFileStatus[]).map((key) => (
              <MenuCheckbox
                key={key}
                checked={settings.statusFilter[key]}
                onSelect={() => setStatus(key, !settings.statusFilter[key])}
              >
                {STATUS_LABELS[key]}
              </MenuCheckbox>
            ))}
          </PopoverContent>
        </Popover>

        <div className="inline-flex h-7 items-stretch overflow-hidden rounded-md border border-(--color-border) text-xs">
          <IconActionButton
            onClick={onExpandAll}
            label="Expand all files"
            icon={<Maximize2 className="size-3.5" aria-hidden />}
          />
          <IconActionButton
            onClick={onCollapseAll}
            label="Collapse all files"
            icon={<Minimize2 className="size-3.5" aria-hidden />}
          />
        </div>
      </div>
    </div>
  );
}

const ToolbarButton = forwardRef<
  HTMLButtonElement,
  {
    icon: typeof Settings2;
    label: string;
  } & Omit<React.ButtonHTMLAttributes<HTMLButtonElement>, "type">
>(function ToolbarButton({ icon: Icon, label, className, ...rest }, ref) {
  return (
    <button
      ref={ref}
      type="button"
      {...rest}
      className={cn(
        "inline-flex h-7 cursor-pointer items-center gap-1.5 rounded-md border border-(--color-border) px-2 text-xs hover:bg-(--color-surface)",
        className,
      )}
    >
      <Icon className="size-3.5" aria-hidden />
      <span>{label}</span>
      <ChevronDown className="size-3 text-(--color-muted-foreground)" aria-hidden />
    </button>
  );
});

function SegmentButton({ active, onClick, children }: { active: boolean; onClick: () => void; children: ReactNode }) {
  return (
    <button
      type="button"
      onClick={onClick}
      aria-pressed={active}
      className={cn(
        "cursor-pointer px-2.5 text-xs",
        active
          ? "bg-(--color-surface) font-semibold text-(--color-foreground)"
          : "text-(--color-muted-foreground) hover:bg-(--color-surface)/60",
      )}
    >
      {children}
    </button>
  );
}

function IconActionButton({ onClick, label, icon }: { onClick?: () => void; label: string; icon: ReactNode }) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <button
          type="button"
          onClick={onClick}
          aria-label={label}
          className="flex cursor-pointer items-center px-2 text-(--color-muted-foreground) hover:bg-(--color-surface) hover:text-(--color-foreground) [&:not(:first-child)]:border-l [&:not(:first-child)]:border-(--color-border)"
        >
          {icon}
        </button>
      </TooltipTrigger>
      <TooltipContent>{label}</TooltipContent>
    </Tooltip>
  );
}

function MenuRadio({ checked, onSelect, children }: { checked: boolean; onSelect: () => void; children: ReactNode }) {
  return (
    <button
      type="button"
      role="menuitemradio"
      aria-checked={checked}
      onClick={onSelect}
      className="flex w-full cursor-pointer items-center gap-2 rounded px-2 py-1.5 text-sm hover:bg-(--color-surface)"
    >
      <span
        aria-hidden
        className={cn(
          "grid size-4 place-items-center rounded-full border border-(--color-border)",
          checked && "border-(--color-primary)",
        )}
      >
        {checked ? <span className="size-2 rounded-full bg-(--color-primary)" /> : null}
      </span>
      <span>{children}</span>
    </button>
  );
}

function MenuCheckbox({
  checked,
  onSelect,
  children,
}: {
  checked: boolean;
  onSelect: () => void;
  children: ReactNode;
}) {
  return (
    <button
      type="button"
      role="menuitemcheckbox"
      aria-checked={checked}
      onClick={onSelect}
      className="flex w-full cursor-pointer items-center gap-2 rounded px-2 py-1.5 text-sm hover:bg-(--color-surface)"
    >
      <span className="grid size-4 place-items-center">
        {checked ? <Check className="size-3.5 text-(--color-primary)" aria-hidden /> : null}
      </span>
      <span>{children}</span>
    </button>
  );
}
