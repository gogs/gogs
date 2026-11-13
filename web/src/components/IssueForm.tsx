import { useQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { Check, Paperclip, Settings, X } from "lucide-react";
import { type ReactNode, useId, useRef, useState } from "react";
import { useTranslation } from "react-i18next";

import { CodeEditor } from "@/components/CodeEditor";
import { Markdown } from "@/components/Markdown";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { type IssueMetaOptions, type RepoIssueData, renderRepoMarkdown } from "@/lib/queries/repo";
import type { AttachmentUploads } from "@/lib/use-attachment-uploads";
import { cn } from "@/lib/utils";

// The parts of the forms that open and change issues, shared by the issue page
// and the form to open one.

interface RepoRef {
  owner: string;
  name: string;
}

// MarkdownEditor is a text area for Markdown with a tab that previews it the
// way the server renders it.
export function MarkdownEditor({
  repo,
  label,
  value,
  onChange,
  error,
  autoFocus,
  disabled,
  uploads,
  minLines,
}: {
  repo: RepoRef;
  label: string;
  value: string;
  onChange: (value: string) => void;
  error?: string | null;
  autoFocus?: boolean;
  disabled?: boolean;
  // uploads attaches the files pasted or dropped into the editor.
  uploads?: AttachmentUploads;
  minLines?: number;
}) {
  const { t } = useTranslation();
  const id = useId();
  const [tab, setTab] = useState<"write" | "preview">("write");
  const preview = useQuery({
    queryKey: ["markdown", repo.owner, repo.name, value],
    queryFn: () => renderRepoMarkdown(repo.owner, repo.name, value),
    enabled: tab === "preview" && value.trim() !== "",
    staleTime: Infinity,
  });
  const tabs = [
    { key: "write", label: t("repo.release.write") },
    { key: "preview", label: t("repo.release.preview") },
  ] as const;

  return (
    <div>
      <div role="tablist" aria-label={label} className="mb-2 flex gap-1">
        {tabs.map((item) => (
          <button
            key={item.key}
            type="button"
            role="tab"
            id={`${id}-${item.key}-tab`}
            aria-selected={tab === item.key}
            aria-controls={`${id}-${item.key}`}
            onClick={() => setTab(item.key)}
            className={cn(
              "cursor-pointer rounded-md px-3 py-1.5 text-sm",
              tab === item.key
                ? "bg-(--color-surface) font-semibold text-(--color-foreground)"
                : "text-(--color-muted-foreground) hover:text-(--color-foreground)",
            )}
          >
            {item.label}
          </button>
        ))}
      </div>
      {tab === "write" ? (
        <div role="tabpanel" id={`${id}-write`} aria-labelledby={`${id}-write-tab`}>
          <CodeEditor
            id={`${id}-input`}
            label={label}
            lang="markdown"
            value={value}
            onChange={onChange}
            autoFocus={autoFocus}
            disabled={disabled}
            minLines={minLines}
            onFiles={uploads?.settings ? uploads.add : undefined}
            aria-invalid={Boolean(error) || undefined}
            aria-describedby={error ? `${id}-error` : undefined}
          />
          {error ? (
            <p id={`${id}-error`} className="mt-1.5 text-sm text-(--color-destructive)">
              {error}
            </p>
          ) : null}
        </div>
      ) : (
        <div
          role="tabpanel"
          id={`${id}-preview`}
          aria-labelledby={`${id}-preview-tab`}
          aria-busy={preview.isFetching}
          className="min-h-32 rounded-md border border-(--color-border) px-4 py-3"
        >
          {value.trim() === "" ? (
            <p className="text-sm text-(--color-muted-foreground) italic">{t("repo.issues.no_content")}</p>
          ) : preview.data !== undefined ? (
            <Markdown html={preview.data} />
          ) : preview.isError ? (
            <p className="text-sm text-(--color-destructive)">{preview.error.message}</p>
          ) : (
            <p className="text-sm text-(--color-muted-foreground)">{t("repo.release.loading")}</p>
          )}
        </div>
      )}
    </div>
  );
}

// PendingFiles lists the files picked to attach, which can be removed once
// they are uploaded.
export function PendingFiles({ uploads }: { uploads: AttachmentUploads }) {
  const { t } = useTranslation();
  if (!uploads.files.length) return null;
  return (
    <ul className="mt-2 flex flex-col gap-1">
      {uploads.files.map((f) => (
        <li key={f.key} className="flex items-center gap-2 text-sm">
          <Paperclip className="size-4 shrink-0 text-(--color-muted-foreground)" aria-hidden />
          <span className={cn("min-w-0 flex-1 truncate", !f.uuid && "text-(--color-muted-foreground)")}>{f.name}</span>
          {f.uuid ? (
            <Button
              type="button"
              variant="ghost"
              size="icon"
              aria-label={`${t("dropzone.remove_file")}: ${f.name}`}
              className="size-7 text-(--color-muted-foreground)"
              onClick={() => uploads.remove(f.key)}
            >
              <X className="size-4" aria-hidden />
            </Button>
          ) : (
            <span className="text-xs text-(--color-muted-foreground)">{t("repo.release.loading")}</span>
          )}
        </li>
      ))}
    </ul>
  );
}

// AttachFilesButton picks files to attach, or holds the place of the button
// when attachments are disabled.
export function AttachFilesButton({ uploads }: { uploads: AttachmentUploads }) {
  const { t } = useTranslation();
  const fileInput = useRef<HTMLInputElement>(null);
  const settings = uploads.settings;
  if (!settings) return <span />;
  return (
    <>
      <input
        ref={fileInput}
        type="file"
        multiple
        hidden
        accept={settings.allowedTypes.includes("*/*") ? undefined : settings.allowedTypes.join(",")}
        onChange={(e) => {
          if (e.target.files) uploads.add(e.target.files);
          e.target.value = "";
        }}
      />
      <Button
        type="button"
        variant="ghost"
        size="sm"
        className="text-(--color-muted-foreground)"
        disabled={uploads.files.length >= settings.maxFiles}
        onClick={() => fileInput.current?.click()}
      >
        <Paperclip className="size-4" aria-hidden />
        {t("repo.issues.attach_files")}
      </Button>
    </>
  );
}

// SidebarSection is a part of the sidebar with a heading, which writers can
// change through the popover of its button.
function SidebarSection({
  title,
  editor,
  children,
}: {
  title: string;
  editor?: (close: () => void) => ReactNode;
  children: ReactNode;
}) {
  const [open, setOpen] = useState(false);
  return (
    <section className="border-b border-(--color-border) pb-4">
      <div className="mb-2 flex min-h-8 items-center justify-between gap-2">
        <h2 className="text-sm font-semibold text-(--color-muted-foreground)">{title}</h2>
        {editor ? (
          <Popover open={open} onOpenChange={setOpen}>
            <Tooltip>
              <TooltipTrigger asChild>
                <PopoverTrigger asChild>
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon"
                    aria-label={title}
                    className="size-8 text-(--color-muted-foreground)"
                  >
                    <Settings className="size-4" aria-hidden />
                  </Button>
                </PopoverTrigger>
              </TooltipTrigger>
              <TooltipContent>{title}</TooltipContent>
            </Tooltip>
            <PopoverContent align="end" collisionPadding={16} className="w-64 max-w-[calc(100vw-2rem)] p-1">
              {editor(() => setOpen(false))}
            </PopoverContent>
          </Popover>
        ) : null}
      </div>
      {children}
    </section>
  );
}

const OPTION_CLASS =
  "flex w-full cursor-pointer items-center gap-2 rounded-sm px-2 py-1.5 text-left text-sm hover:bg-(--color-surface) disabled:cursor-not-allowed disabled:opacity-60";

// OptionList lists the choices in the popover of a sidebar section: a button
// that clears the choice, then the options, marking the current one.
function OptionList<T extends { id: number }>({
  clearLabel,
  groups,
  current,
  disabled,
  render,
  onPick,
}: {
  clearLabel: string;
  groups: { label?: string; items: T[] }[];
  current?: number;
  disabled: boolean;
  render: (item: T) => ReactNode;
  onPick: (id: number) => void;
}) {
  return (
    <div className="max-h-80 overflow-y-auto">
      <button type="button" className={OPTION_CLASS} disabled={disabled || !current} onClick={() => onPick(0)}>
        <X className="size-4 shrink-0" aria-hidden />
        {clearLabel}
      </button>
      {groups.map((group, i) =>
        group.items.length ? (
          <div key={i} role="group" aria-label={group.label}>
            {group.label ? (
              <p className="px-2 pt-2 pb-1 text-xs font-semibold text-(--color-muted-foreground)">{group.label}</p>
            ) : null}
            {group.items.map((item) => (
              <button
                key={item.id}
                type="button"
                aria-pressed={item.id === current}
                className={OPTION_CLASS}
                disabled={disabled}
                onClick={() => onPick(item.id)}
              >
                <Check className={cn("size-4 shrink-0", item.id !== current && "invisible")} aria-hidden />
                {render(item)}
              </button>
            ))}
          </div>
        ) : null,
      )}
    </div>
  );
}

function LabelChip({ label }: { label: RepoIssueData["labels"][number] }) {
  return (
    <span
      className="inline-block max-w-full truncate rounded-full px-2 text-xs/5 font-medium"
      style={{ backgroundColor: label.color, color: label.textColor }}
    >
      {label.name}
    </span>
  );
}

// IssueMeta shows the labels, the milestone and the assignee of an issue, and
// lets writers, who get the options, change them. Children follow them, e.g.,
// the participants.
export function IssueMeta({
  repo,
  value,
  options,
  pending,
  onLabels,
  onMilestone,
  onAssignee,
  linkChoices = true,
  children,
}: {
  repo: RepoRef;
  value: Pick<RepoIssueData, "labels" | "milestone" | "assignee">;
  options: IssueMetaOptions;
  // pending disables the options while a change is saved.
  pending: boolean;
  onLabels: (ids: number[]) => void;
  // onMilestone and onAssignee set the choice, or clear it with 0, and settle
  // once it is saved.
  onMilestone: (id: number) => Promise<unknown>;
  onAssignee: (id: number) => Promise<unknown>;
  // linkChoices links the labels and the milestone to the issues that have
  // them, and the assignee to their profile, which a form being filled in
  // would lose.
  linkChoices?: boolean;
  children?: ReactNode;
}) {
  const { t } = useTranslation();
  const params = { owner: repo.owner, repo: repo.name };
  const checked = new Set(value.labels.map((l) => l.id));
  const toggleLabel = (id: number) =>
    onLabels(checked.has(id) ? [...checked].filter((l) => l !== id) : [...checked, id]);
  // The popover closes once the choice is saved, and stays open to try again
  // when it fails.
  const pickThenClose = (pick: (id: number) => Promise<unknown>, close: () => void) => (id: number) =>
    void pick(id).then(close, () => undefined);
  const emptyText = "text-sm text-(--color-muted-foreground)";

  return (
    <aside className="flex min-w-0 flex-col gap-4">
      <SidebarSection
        title={t("repo.issues.new.labels")}
        editor={
          options.labelOptions
            ? () => (
                <div className="max-h-80 overflow-y-auto">
                  <button
                    type="button"
                    className={OPTION_CLASS}
                    disabled={pending || checked.size === 0}
                    onClick={() => onLabels([])}
                  >
                    <X className="size-4 shrink-0" aria-hidden />
                    {t("repo.issues.new.clear_labels")}
                  </button>
                  {options.labelOptions?.map((label) => (
                    <label key={label.id} className={cn(OPTION_CLASS, pending && "opacity-60")}>
                      <Checkbox
                        checked={checked.has(label.id)}
                        disabled={pending}
                        onCheckedChange={() => toggleLabel(label.id)}
                      />
                      <span
                        className="size-3 shrink-0 rounded-full"
                        style={{ backgroundColor: label.color }}
                        aria-hidden
                      />
                      <span className="min-w-0 truncate">{label.name}</span>
                    </label>
                  ))}
                </div>
              )
            : undefined
        }
      >
        {value.labels.length ? (
          <ul className="flex flex-wrap gap-1">
            {value.labels.map((label) => (
              <li key={label.id} className="max-w-full">
                {linkChoices ? (
                  <Link
                    to="/$owner/$repo/issues"
                    params={params}
                    search={{ labels: label.id }}
                    className="hover:opacity-80"
                  >
                    <LabelChip label={label} />
                  </Link>
                ) : (
                  <LabelChip label={label} />
                )}
              </li>
            ))}
          </ul>
        ) : (
          <p className={emptyText}>{t("repo.issues.new.no_label")}</p>
        )}
      </SidebarSection>

      <SidebarSection
        title={t("repo.issues.new.milestone")}
        editor={
          options.milestoneOptions
            ? (close) => (
                <OptionList
                  clearLabel={t("repo.issues.new.clear_milestone")}
                  groups={[
                    {
                      label: t("repo.issues.new.open_milestone"),
                      items: options.milestoneOptions?.filter((m) => !m.isClosed) ?? [],
                    },
                    {
                      label: t("repo.issues.new.closed_milestone"),
                      items: options.milestoneOptions?.filter((m) => m.isClosed) ?? [],
                    },
                  ]}
                  current={value.milestone?.id}
                  disabled={pending}
                  render={(m) => <span className="min-w-0 truncate">{m.name}</span>}
                  onPick={pickThenClose(onMilestone, close)}
                />
              )
            : undefined
        }
      >
        {value.milestone && !linkChoices ? (
          <p className="text-sm font-medium wrap-anywhere">{value.milestone.name}</p>
        ) : value.milestone ? (
          <Link
            to="/$owner/$repo/issues"
            params={params}
            search={{ milestone: value.milestone.id }}
            className="text-sm font-medium wrap-anywhere hover:text-(--color-primary) hover:underline"
          >
            {value.milestone.name}
          </Link>
        ) : (
          <p className={emptyText}>{t("repo.issues.new.no_milestone")}</p>
        )}
      </SidebarSection>

      <SidebarSection
        title={t("repo.issues.new.assignee")}
        editor={
          options.assigneeOptions
            ? (close) => (
                <OptionList
                  clearLabel={t("repo.issues.new.clear_assignee")}
                  groups={[{ items: options.assigneeOptions ?? [] }]}
                  current={value.assignee?.id}
                  disabled={pending}
                  render={(u) => (
                    <>
                      <img src={u.avatarURL} alt="" className="size-5 shrink-0 rounded-full" />
                      <span className="min-w-0 truncate">{u.name}</span>
                    </>
                  )}
                  onPick={pickThenClose(onAssignee, close)}
                />
              )
            : undefined
        }
      >
        {value.assignee && !linkChoices ? (
          <p className="inline-flex max-w-full items-center gap-2 text-sm font-medium">
            <img src={value.assignee.avatarURL} alt="" className="size-5 shrink-0 rounded-full" />
            <span className="truncate">{value.assignee.name}</span>
          </p>
        ) : value.assignee ? (
          <Link
            to="/$username"
            params={{ username: value.assignee.name }}
            className="inline-flex max-w-full items-center gap-2 text-sm font-medium hover:text-(--color-primary) hover:underline"
          >
            <img src={value.assignee.avatarURL} alt="" className="size-5 shrink-0 rounded-full" />
            <span className="truncate">{value.assignee.name}</span>
          </Link>
        ) : (
          <p className={emptyText}>{t("repo.issues.new.no_assignee")}</p>
        )}
      </SidebarSection>

      {children}
    </aside>
  );
}
