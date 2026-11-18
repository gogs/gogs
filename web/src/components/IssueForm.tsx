import { useQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { Check, Paperclip, Pencil, Settings, X } from "lucide-react";
import { type ReactNode, useEffect, useId, useLayoutEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";

import { CodeEditor, type CodeEditorHandle } from "@/components/CodeEditor";
import { Markdown } from "@/components/Markdown";
import { SessionExpired } from "@/components/SessionExpired";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import { ScrollArea } from "@/components/ui/scroll-area";
import { type IssueMetaOptions, type RepoIssueData, renderRepoMarkdown } from "@/lib/queries/repo";
import { isSessionExpired } from "@/lib/queries/request";
import type { RepoRef } from "@/lib/reference-completion";
import type { AttachmentUploads, UploadLinker } from "@/lib/use-attachment-uploads";
import { useUserInfo } from "@/lib/use-user-info";
import { cn } from "@/lib/utils";

// The parts of the forms that open and change issues, shared by the issue page
// and the form to open one.

// uploadingText is what holds the place of a file in the text while it uploads,
// like on GitHub.
function uploadingText(file: File) {
  return `${file.type.startsWith("image/") ? "!" : ""}[Uploading ${file.name}…]()`;
}

// fileLink links a file in Markdown, showing an image in place.
function fileLink(file: File, url: string) {
  const text = file.name.replace(/[\\[\]]/g, "\\$&");
  return `${file.type.startsWith("image/") ? "!" : ""}[${text}](${url})`;
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
  wiki = false,
  linkFiles,
}: {
  repo: RepoRef;
  label: string;
  value: string;
  onChange: (value: string) => void;
  error?: ReactNode;
  autoFocus?: boolean;
  disabled?: boolean;
  // uploads attaches the files pasted or dropped into the editor.
  uploads?: AttachmentUploads;
  minLines?: number;
  // wiki previews relative links the way wiki pages resolve them.
  wiki?: boolean;
  // linkFiles links the files picked, pasted or dropped where the cursor is,
  // like on GitHub, instead of only attaching them.
  linkFiles?: boolean;
}) {
  const { t } = useTranslation();
  const id = useId();
  const [tab, setTab] = useState<"write" | "preview">("write");
  const editor = useRef<CodeEditorHandle>(null);
  // latest is the text to change while the editor is not shown, e.g., on the
  // preview tab when an upload finishes.
  const latest = useRef(value);
  useLayoutEffect(() => {
    latest.current = value;
  }, [value]);
  const setText = (text: string) => {
    latest.current = text;
    onChange(text);
  };
  const linker: UploadLinker = {
    start: (file) => {
      const text = uploadingText(file);
      if (editor.current) editor.current.insert(text);
      else setText(latest.current ? `${latest.current.replace(/\n*$/, "")}\n${text}\n` : `${text}\n`);
    },
    done: (file, url) => {
      const from = uploadingText(file);
      const to = url ? fileLink(file, url) : "";
      if (!editor.current?.replace(from, to)) setText(latest.current.replace(from, to));
    },
  };
  useEffect(() => {
    if (!linkFiles || !uploads) return;
    uploads.setLinker(linker);
    return () => uploads.setLinker(null);
  });
  const preview = useQuery({
    queryKey: ["markdown", repo.owner, repo.name, value, wiki],
    queryFn: () => renderRepoMarkdown(repo.owner, repo.name, value, { wiki }),
    enabled: tab === "preview" && value.trim() !== "",
    staleTime: Infinity,
  });
  const tabs = [
    { key: "write", label: t("repo.release.write") },
    { key: "preview", label: t("repo.release.preview") },
  ] as const;

  return (
    <div>
      <ContentTabs id={id} label={label} tabs={tabs} current={tab} onChange={setTab} />
      {tab === "write" ? (
        <div role="tabpanel" id={`${id}-write`} aria-labelledby={`${id}-write-tab`}>
          <CodeEditor
            editorRef={editor}
            id={`${id}-input`}
            label={label}
            lang="markdown"
            repo={repo}
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
          className="min-h-32 glass rounded-md border border-(--color-border) px-4 py-3"
        >
          {value.trim() === "" ? (
            <p className="text-sm text-(--color-muted-foreground) italic">{t("repo.issues.no_content")}</p>
          ) : preview.data !== undefined ? (
            <Markdown html={preview.data} />
          ) : preview.isError ? (
            <p className="text-sm text-(--color-destructive)">
              {isSessionExpired(preview.error) ? <SessionExpired /> : preview.error.message}
            </p>
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

// SidebarSection is a part of the sidebar with a heading, which writers click
// to change it in a popover.
export function SidebarSection({
  title,
  editor,
  children,
}: {
  title: string;
  editor?: (close: () => void) => ReactNode;
  children: ReactNode;
}) {
  const [open, setOpen] = useState(false);
  const heading = "text-xs font-semibold text-(--color-muted-foreground)";
  return (
    <section className="border-b border-(--color-border) py-4 first:pt-0 last:border-b-0">
      {editor ? (
        <Popover open={open} onOpenChange={setOpen}>
          <h2 className="mb-1">
            <PopoverTrigger asChild>
              <button
                type="button"
                className={cn(
                  heading,
                  "-ml-2 flex min-h-8 w-[calc(100%+0.5rem)] cursor-pointer items-center justify-between gap-2 rounded-md px-2 hover:text-(--color-primary) focus-visible:outline-2 focus-visible:outline-(--color-ring)",
                )}
              >
                {title}
                <Settings className="size-4" aria-hidden />
              </button>
            </PopoverTrigger>
          </h2>
          <PopoverContent
            align="end"
            collisionPadding={16}
            className="flex max-h-(--radix-popover-content-available-height) flex-col w-75 max-w-[calc(100vw-2rem)] p-0"
          >
            {editor(() => setOpen(false))}
          </PopoverContent>
        </Popover>
      ) : (
        <h2 className={cn(heading, "mb-1 flex min-h-8 items-center")}>{title}</h2>
      )}
      {children}
    </section>
  );
}

// Picker is the popover of a sidebar section, like the ones of GitHub: a
// heading, a filter, and the options in groups, each toggled on click.
function Picker<T extends { id: number; name: string }>({
  heading,
  filterLabel,
  groups,
  checked,
  disabled,
  render,
  onToggle,
  footer,
}: {
  heading: string;
  filterLabel: string;
  groups: { label?: string; items: T[] }[];
  checked: Set<number>;
  disabled: boolean;
  render: (item: T) => ReactNode;
  onToggle: (id: number) => void;
  // footer links to where the options are managed.
  footer?: { label: string; to: "/$owner/$repo/labels" | "/$owner/$repo/milestones"; repo: RepoRef };
}) {
  const { t } = useTranslation();
  const [filter, setFilter] = useState("");
  // ordered puts the options checked when the picker opened first in each
  // group, and keeps them in place while they are toggled.
  const [ordered] = useState(() =>
    groups.map((group) => ({
      ...group,
      items: [...group.items.filter((i) => checked.has(i.id)), ...group.items.filter((i) => !checked.has(i.id))],
    })),
  );
  const query = filter.trim().toLowerCase();
  const shown = ordered
    .map((group) => ({
      ...group,
      items: query ? group.items.filter((i) => i.name.toLowerCase().includes(query)) : group.items,
    }))
    .filter((group) => group.items.length);
  return (
    <div className="flex min-h-0 flex-col text-sm">
      <p className="border-b border-(--color-border) px-3 py-2 text-xs font-semibold">{heading}</p>
      <div className="border-b border-(--color-border) p-2">
        <Input
          type="search"
          autoFocus
          value={filter}
          onChange={(e) => setFilter(e.target.value)}
          placeholder={filterLabel}
          aria-label={filterLabel}
          className="h-8"
        />
      </div>
      {/* -mb-px tucks the border of the last row under the edge of the popover. */}
      <ScrollArea className={cn("max-h-80", !footer && "-mb-px")}>
        {shown.length ? (
          shown.map((group, i) => (
            <div key={i} role="group" aria-label={group.label}>
              {group.label ? (
                <p className="border-b border-(--color-border) bg-(--color-surface) px-3 py-1.5 text-xs font-semibold text-(--color-muted-foreground)">
                  {group.label}
                </p>
              ) : null}
              {group.items.map((item) => (
                <button
                  key={item.id}
                  type="button"
                  role="checkbox"
                  aria-checked={checked.has(item.id)}
                  disabled={disabled}
                  onClick={() => onToggle(item.id)}
                  className="flex w-full cursor-pointer items-center gap-2 border-b border-(--color-border) px-3 py-2 text-left hover:bg-(--color-surface) focus-visible:bg-(--color-surface) focus-visible:outline-none disabled:cursor-not-allowed disabled:opacity-60"
                >
                  <Check className={cn("size-4 shrink-0", !checked.has(item.id) && "invisible")} aria-hidden />
                  {render(item)}
                </button>
              ))}
            </div>
          ))
        ) : (
          <p className="border-b border-(--color-border) px-3 py-4 text-center text-(--color-muted-foreground)">
            {t("repo.issues.picker_no_results")}
          </p>
        )}
      </ScrollArea>
      {footer ? (
        <Link
          to={footer.to}
          params={{ owner: footer.repo.owner, repo: footer.repo.name }}
          className="-mt-px flex items-center gap-2 border-t border-(--color-border) px-3 py-2 text-(--color-muted-foreground) hover:bg-(--color-surface) hover:text-(--color-primary)"
        >
          <Pencil className="size-4" aria-hidden />
          {footer.label}
        </Link>
      ) : null}
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

// IssueMeta shows the assignee, the labels and the milestone of an issue, and
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
  value: Pick<RepoIssueData, "labels" | "assignee"> & {
    // completeness is only known for the milestone of an existing issue.
    milestone?: { id: number; name: string; completeness?: number };
  };
  options: IssueMetaOptions;
  // pending disables the options while a change is saved.
  pending: boolean;
  onLabels: (ids: number[]) => void;
  // onMilestone and onAssignee set the choice, or clear it with 0, and settle
  // once it is saved.
  onMilestone: (id: number) => Promise<unknown>;
  onAssignee: (id: number) => Promise<unknown>;
  // linkChoices links the labels and the milestone to the issues that have
  // them, the assignee to their profile, and the pickers to where the labels
  // and milestones are edited, which a form being filled in would lose.
  linkChoices?: boolean;
  children?: ReactNode;
}) {
  const { t } = useTranslation();
  const params = { owner: repo.owner, repo: repo.name };
  const checked = new Set(value.labels.map((l) => l.id));
  const toggleLabel = (id: number) =>
    onLabels(checked.has(id) ? [...checked].filter((l) => l !== id) : [...checked, id]);
  // pickThenClose picks an option, or clears the choice when it is picked
  // again. The popover closes once the choice is saved, and stays open to try
  // again when it fails.
  const pickThenClose =
    (pick: (id: number) => Promise<unknown>, current: number | undefined, close: () => void) => (id: number) =>
      void pick(id === current ? 0 : id).then(close, () => undefined);
  const emptyText = "text-sm text-(--color-muted-foreground)";
  // self is the viewer when they can be assigned, for the shortcut to assign
  // themselves.
  const viewer = useUserInfo();
  const self = viewer ? options.assigneeOptions?.find((u) => u.name === viewer.username) : undefined;
  const assigneeClass = "inline-flex max-w-full items-center gap-2 text-sm font-semibold";

  return (
    <aside className="glass flex min-w-0 flex-col glass-ui:self-start glass-ui:border glass-ui:p-4">
      <SidebarSection
        title={t("repo.issues.new.assignee")}
        editor={
          options.assigneeOptions
            ? (close) => (
                <Picker
                  heading={t("repo.issues.assign_someone")}
                  filterLabel={t("repo.issues.filter_assignees")}
                  groups={[{ items: options.assigneeOptions ?? [] }]}
                  checked={new Set(value.assignee ? [value.assignee.id] : [])}
                  disabled={pending}
                  render={(u) => (
                    <>
                      <img src={u.avatarURL} alt="" className="size-5 shrink-0 rounded-full" />
                      <span className="min-w-0 truncate font-medium">{u.name}</span>
                    </>
                  )}
                  onToggle={pickThenClose(onAssignee, value.assignee?.id, close)}
                />
              )
            : undefined
        }
      >
        {value.assignee && !linkChoices ? (
          <p className={assigneeClass}>
            <img src={value.assignee.avatarURL} alt="" className="size-5 shrink-0 rounded-full" />
            <span className="truncate">{value.assignee.name}</span>
          </p>
        ) : value.assignee ? (
          <Link
            to="/$username"
            params={{ username: value.assignee.name }}
            className={cn(assigneeClass, "hover:text-(--color-primary) hover:underline")}
          >
            <img src={value.assignee.avatarURL} alt="" className="size-5 shrink-0 rounded-full" />
            <span className="truncate">{value.assignee.name}</span>
          </Link>
        ) : (
          <p className={emptyText}>
            {t("repo.issues.new.no_assignee")}
            {self ? (
              <>
                {", "}
                <button
                  type="button"
                  className="cursor-pointer text-(--color-foreground) hover:text-(--color-primary) hover:underline disabled:cursor-not-allowed disabled:opacity-60"
                  disabled={pending}
                  onClick={() => void onAssignee(self.id).catch(() => undefined)}
                >
                  {t("repo.issues.assign_yourself")}
                </button>
              </>
            ) : null}
          </p>
        )}
      </SidebarSection>

      <SidebarSection
        title={t("repo.issues.new.labels")}
        editor={
          options.labelOptions
            ? () => (
                <Picker
                  heading={t("repo.issues.apply_labels")}
                  filterLabel={t("repo.issues.filter_labels")}
                  groups={[{ items: options.labelOptions ?? [] }]}
                  checked={checked}
                  disabled={pending}
                  render={(label) => (
                    <>
                      <span
                        className="size-3.5 shrink-0 rounded-full border border-black/10"
                        style={{ backgroundColor: label.color }}
                        aria-hidden
                      />
                      <span className="min-w-0 truncate font-medium">{label.name}</span>
                    </>
                  )}
                  onToggle={toggleLabel}
                  footer={
                    linkChoices ? { label: t("repo.issues.edit_labels"), to: "/$owner/$repo/labels", repo } : undefined
                  }
                />
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
                <Picker
                  heading={t("repo.issues.set_milestone")}
                  filterLabel={t("repo.issues.filter_milestones")}
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
                  checked={new Set(value.milestone ? [value.milestone.id] : [])}
                  disabled={pending}
                  render={(m) => <span className="min-w-0 truncate font-medium">{m.name}</span>}
                  onToggle={pickThenClose(onMilestone, value.milestone?.id, close)}
                  footer={
                    linkChoices
                      ? { label: t("repo.issues.edit_milestones"), to: "/$owner/$repo/milestones", repo }
                      : undefined
                  }
                />
              )
            : undefined
        }
      >
        {value.milestone ? (
          <div className="flex flex-col gap-2">
            {value.milestone.completeness !== undefined ? (
              <div
                role="progressbar"
                aria-label={value.milestone.name}
                aria-valuenow={value.milestone.completeness}
                aria-valuemin={0}
                aria-valuemax={100}
                className="h-2 overflow-hidden rounded-full bg-(--color-secondary)"
              >
                <div className="h-full bg-(--color-success)" style={{ width: `${value.milestone.completeness}%` }} />
              </div>
            ) : null}
            {linkChoices ? (
              <Link
                to="/$owner/$repo/issues"
                params={params}
                search={{ milestone: value.milestone.id }}
                className="text-sm font-semibold wrap-anywhere hover:text-(--color-primary) hover:underline"
              >
                {value.milestone.name}
              </Link>
            ) : (
              <p className="text-sm font-semibold wrap-anywhere">{value.milestone.name}</p>
            )}
          </div>
        ) : (
          <p className={emptyText}>{t("repo.issues.new.no_milestone")}</p>
        )}
      </SidebarSection>

      {children}
    </aside>
  );
}

// ContentTabs switch what one editor shows, e.g., Write and Preview, as tabs
// underlined like the page tabs, which stand out on any background. Each tab
// controls the panel with the ID of the tab without its "-tab" suffix.
export function ContentTabs<K extends string>({
  id,
  label,
  tabs,
  current,
  onChange,
}: {
  id: string;
  label: string;
  tabs: readonly { key: K; label: string }[];
  current: K;
  onChange: (key: K) => void;
}) {
  return (
    <div role="tablist" aria-label={label} className="mb-3 flex gap-1 overflow-x-auto border-b border-(--color-border)">
      {tabs.map((tab) => (
        <button
          key={tab.key}
          type="button"
          role="tab"
          id={`${id}-${tab.key}-tab`}
          aria-selected={current === tab.key}
          aria-controls={`${id}-${tab.key}`}
          onClick={() => onChange(tab.key)}
          className={cn(
            "-mb-px cursor-pointer border-b-2 px-3 py-2 text-sm whitespace-nowrap outline-none focus-visible:ring-1 focus-visible:ring-(--color-ring)",
            current === tab.key
              ? "border-(--color-primary) font-semibold text-(--color-foreground)"
              : "border-transparent text-(--color-muted-foreground) hover:border-(--color-border) hover:text-(--color-foreground)",
          )}
        >
          {tab.label}
        </button>
      ))}
    </div>
  );
}
