import { MultiFileDiff } from "@pierre/diffs/react";
import { keepPreviousData, useMutation, useQuery, useSuspenseQuery } from "@tanstack/react-query";
import { FileX, GitBranch, Upload } from "lucide-react";
import { type AriaAttributes, type FormEvent, type ReactNode, useId, useRef, useState } from "react";
import { Trans, useTranslation } from "react-i18next";

import { CodeEditor } from "@/components/CodeEditor";
import { FormAlert, FormField, RadioField } from "@/components/FormField";
import { PendingFiles } from "@/components/IssueForm";
import { Markdown } from "@/components/Markdown";
import { RepoHeader } from "@/components/RepoHeader";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { codeLanguageOf } from "@/lib/code-language";
import { usePageTitle } from "@/lib/page-title";
import {
  type RepoEditorCommit,
  type RepoEditorIndent,
  type RepoEditorTarget,
  commitRepoFile,
  deleteRepoEditorUpload,
  deleteRepoFile,
  fetchRepoEditorIndent,
  refPath,
  renderRepoMarkdown,
  repoHeaderQuery,
  uploadRepoFiles,
} from "@/lib/queries/repo";
import { FormError, formErrors } from "@/lib/queries/request";
import { resolveTheme, useTheme } from "@/lib/theme-context";
import { subUrl } from "@/lib/url";
import { useAttachmentUploads } from "@/lib/use-attachment-uploads";
import { useUserInfo } from "@/lib/use-user-info";
import { cn } from "@/lib/utils";

// The pages that change the files of a branch in the browser: editing or
// creating a file, deleting one, and uploading files. Each change is a commit
// to the branch or to a new branch.

const TEXTAREA_CLASS =
  "block w-full rounded-md border border-(--color-input) bg-(--color-background) px-3 py-2 text-sm text-(--color-foreground) placeholder:text-(--color-muted-foreground) outline-none transition-colors focus-visible:border-(--color-ring) focus-visible:ring-1 focus-visible:ring-(--color-ring) disabled:cursor-not-allowed disabled:opacity-60";

function EditorLayout({
  owner,
  name,
  title,
  children,
}: {
  owner: string;
  name: string;
  title: string;
  children: ReactNode;
}) {
  const { data: repo } = useSuspenseQuery(repoHeaderQuery(owner, name));
  usePageTitle(`${title} - ${repo.owner}/${repo.name}`);
  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <RepoHeader repo={repo} activeTab="code" />
      <main className="mx-auto w-full max-w-7xl flex-1 px-4 py-6 sm:px-6">{children}</main>
    </div>
  );
}

// srcURL links to a file or a directory of a branch, which the server still
// renders.
function srcURL(owner: string, name: string, branch: string, path: string) {
  return subUrl(`/${owner}/${name}/src/${refPath(branch)}${path ? `/${refPath(path)}` : ""}`);
}

function splitPath(path: string) {
  return path.split("/").filter(Boolean);
}

// PathInput edits a path one name at a time, like a breadcrumb: typing "/"
// turns the name into a directory, and Backspace at the start of the field
// turns the last directory back into text.
function PathInput({
  id,
  repoName,
  dirs,
  onDirs,
  value,
  onChange,
  placeholder,
  autoFocus,
  ...aria
}: {
  id: string;
  repoName: string;
  dirs: string[];
  onDirs: (dirs: string[]) => void;
  value: string;
  onChange: (value: string) => void;
  placeholder: string;
  autoFocus?: boolean;
} & Pick<AriaAttributes, "aria-invalid" | "aria-describedby">) {
  const input = useRef<HTMLInputElement>(null);
  return (
    <div className="flex flex-wrap items-center gap-x-1 gap-y-2 text-sm">
      <span className="font-semibold wrap-anywhere">{repoName}</span>
      {dirs.map((dir, i) => (
        <span key={i} className="contents">
          <span className="text-(--color-muted-foreground)">/</span>
          <span className="wrap-anywhere">{dir}</span>
        </span>
      ))}
      <span className="text-(--color-muted-foreground)">/</span>
      <Input
        ref={input}
        id={id}
        value={value}
        placeholder={placeholder}
        autoFocus={autoFocus}
        className="w-auto min-w-40 flex-1 sm:max-w-sm"
        onChange={(e) => {
          const parts = e.target.value.split("/");
          if (parts.length > 1) {
            onDirs([...dirs, ...parts.slice(0, -1).filter((p) => p && p !== "." && p !== "..")]);
          }
          onChange(parts.at(-1) ?? "");
        }}
        onKeyDown={(e) => {
          const el = e.currentTarget;
          if (e.key !== "Backspace" || el.selectionStart !== 0 || el.selectionEnd !== 0 || !dirs.length) return;
          e.preventDefault();
          const last = dirs.at(-1) ?? "";
          onDirs(dirs.slice(0, -1));
          onChange(last + value);
          requestAnimationFrame(() => input.current?.setSelectionRange(last.length, last.length));
        }}
        {...aria}
      />
    </div>
  );
}

interface CommitFields {
  summary: string;
  message: string;
  toNewBranch: boolean;
  newBranch: string;
}

function useCommitFields() {
  const user = useUserInfo();
  return useState<CommitFields>({
    summary: "",
    message: "",
    toNewBranch: false,
    newBranch: `${user?.username ?? "patch"}-patch-1`,
  });
}

function commitOf(branch: string, fields: CommitFields): RepoEditorCommit {
  return {
    branch,
    newBranch: fields.toNewBranch ? fields.newBranch : "",
    summary: fields.summary,
    message: fields.message,
  };
}

// CommitForm takes the message of the commit and the branch it goes to, and
// holds the buttons of the page's form.
function CommitForm({
  target,
  value,
  onChange,
  defaultSummary,
  errors,
  cancelURL,
  submitDisabled,
  destructive,
}: {
  target: RepoEditorTarget;
  value: CommitFields;
  onChange: (value: CommitFields) => void;
  defaultSummary: string;
  errors: Record<string, string | null>;
  cancelURL: string;
  submitDisabled?: boolean;
  destructive?: boolean;
}) {
  const { t } = useTranslation();
  const user = useUserInfo();
  const set = (patch: Partial<CommitFields>) => onChange({ ...value, ...patch });
  const [before, after = ""] = t("repo.editor.commit_directly_to_this_branch").split(/<strong[^>]*>%s<\/strong>|%s/);

  return (
    <section
      aria-labelledby="commit-heading"
      className="mt-6 flex gap-4 rounded-md border border-(--color-border) p-4 sm:p-5"
    >
      {user ? <img src={user.avatarURL} alt="" className="hidden size-10 shrink-0 rounded-full sm:block" /> : null}
      <div className="flex min-w-0 flex-1 flex-col gap-4">
        <h2 id="commit-heading" className="text-base font-semibold">
          {t("repo.editor.commit_changes")}
        </h2>
        <FormField
          id="commit-summary"
          label={t("repo.editor.commit_summary")}
          error={errors.summary}
          invalid={Boolean(errors.summary)}
        >
          <Input
            id="commit-summary"
            value={value.summary}
            placeholder={defaultSummary}
            maxLength={100}
            onChange={(e) => set({ summary: e.target.value })}
          />
        </FormField>
        <FormField id="commit-description" label={t("repo.editor.commit_description")} invalid={false}>
          <textarea
            id="commit-description"
            rows={3}
            value={value.message}
            placeholder={t("repo.editor.commit_message_desc")}
            className={TEXTAREA_CLASS}
            onChange={(e) => set({ message: e.target.value })}
          />
        </FormField>
        <div role="radiogroup" aria-labelledby="commit-heading" className="flex flex-col gap-3">
          <RadioField
            id="commit-direct"
            name="commit-to"
            checked={!value.toNewBranch}
            onChange={() => set({ toNewBranch: false })}
            label={
              <>
                {before}
                <strong className="font-semibold wrap-anywhere">{target.branch}</strong>
                {after}
              </>
            }
          />
          <RadioField
            id="commit-new-branch"
            name="commit-to"
            checked={value.toNewBranch}
            onChange={() => set({ toNewBranch: true })}
            label={<Trans i18nKey="repo.editor.create_new_branch" />}
          />
          {value.toNewBranch ? (
            <div className="ml-6.5">
              <div className="relative sm:max-w-sm">
                <GitBranch
                  className="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-(--color-muted-foreground)"
                  aria-hidden
                />
                <Input
                  id="commit-new-branch-name"
                  aria-label={t("repo.editor.new_branch_name")}
                  value={value.newBranch}
                  placeholder={t("repo.editor.new_branch_name_desc")}
                  maxLength={100}
                  required
                  autoFocus
                  className="pl-9"
                  aria-invalid={Boolean(errors.newBranch) || undefined}
                  aria-describedby={errors.newBranch ? "commit-new-branch-error" : undefined}
                  onChange={(e) => set({ newBranch: e.target.value })}
                />
              </div>
              {errors.newBranch ? (
                <p id="commit-new-branch-error" className="mt-1.5 text-sm text-(--color-destructive)">
                  {errors.newBranch}
                </p>
              ) : null}
            </div>
          ) : null}
        </div>
        <div className="flex flex-wrap items-center gap-2 border-t border-(--color-border) pt-4">
          <Button type="submit" variant={destructive ? "destructive" : "default"} disabled={submitDisabled}>
            {t("repo.editor.commit_changes")}
          </Button>
          <Button asChild variant="ghost">
            <a href={cancelURL}>{t("cancel")}</a>
          </Button>
        </div>
      </div>
    </section>
  );
}

const TAB_CLASS = "cursor-pointer rounded-md px-3 py-1.5 text-sm whitespace-nowrap";

function EditorTabs<K extends string>({
  id,
  label,
  tabs,
  current,
  onChange,
}: {
  id: string;
  label: string;
  tabs: { key: K; label: string }[];
  current: K;
  onChange: (key: K) => void;
}) {
  return (
    <div role="tablist" aria-label={label} className="mb-3 flex gap-1 overflow-x-auto">
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
            TAB_CLASS,
            current === tab.key
              ? "bg-(--color-surface) font-semibold text-(--color-foreground)"
              : "text-(--color-muted-foreground) hover:text-(--color-foreground)",
          )}
        >
          {tab.label}
        </button>
      ))}
    </div>
  );
}

// ChangesPreview shows the changes to a file as a diff.
function ChangesPreview({
  oldFile,
  newFile,
  wrap,
}: {
  oldFile: { name: string; contents: string };
  newFile: { name: string; contents: string };
  wrap: boolean;
}) {
  const { t } = useTranslation();
  const { theme } = useTheme();
  if (oldFile.name === newFile.name && oldFile.contents === newFile.contents) {
    return (
      <p className="py-6 text-center text-sm text-(--color-muted-foreground)">{t("repo.editor.no_changes_to_show")}</p>
    );
  }
  return (
    // The library styles its own element with a square border, so the frame
    // goes on a wrapper instead.
    <div className="overflow-hidden rounded-md border border-(--color-border) text-xs">
      <MultiFileDiff
        oldFile={oldFile}
        newFile={newFile}
        style={{ border: "none" }}
        options={{
          theme: { light: "pierre-light", dark: "pierre-dark" },
          themeType: resolveTheme(theme),
          diffStyle: "unified",
          overflow: wrap ? "wrap" : "scroll",
        }}
      />
    </div>
  );
}

// indentationOf returns how to indent with the rules of .editorconfig: one
// level is size spaces, 4 when only the style is set, and 2 without rules.
function indentationOf(indent?: RepoEditorIndent) {
  const tabSize = indent?.tabWidth || indent?.size || 4;
  if (indent?.style === "tab") return { indentUnit: "\t", tabSize };
  return { indentUnit: " ".repeat(indent?.size || (indent?.style ? 4 : 2)), tabSize };
}

// conflictOf returns the message of a commit refused because the file changed
// since the editor opened, and the commit of the branch now.
function conflictOf(error: Error | null) {
  if (!(error instanceof FormError) || typeof error.body.lastCommit !== "string") return null;
  return { html: error.message, lastCommit: error.body.lastCommit };
}

// RepoFileEditor edits a file of a branch, or creates one in a directory.
export function RepoFileEditor({
  owner,
  name,
  target,
  isNew,
}: {
  owner: string;
  name: string;
  target: RepoEditorTarget;
  isNew: boolean;
}) {
  const { t } = useTranslation();
  const id = useId();
  const original = isNew ? "" : (target.content ?? "");
  const [dirs, setDirs] = useState(() => (isNew ? splitPath(target.path) : splitPath(target.path).slice(0, -1)));
  const [fileName, setFileName] = useState(() => (isNew ? "" : (splitPath(target.path).at(-1) ?? "")));
  const [content, setContent] = useState(original);
  const [tab, setTab] = useState<"write" | "preview" | "changes">("write");
  const [commit, setCommit] = useCommitFields();
  const [lastCommit, setLastCommit] = useState(target.lastCommit);
  const path = [...dirs, fileName].join("/");

  const lang = codeLanguageOf(path, target.markdownExtensions);
  const extension = fileName.includes(".") ? fileName.slice(fileName.lastIndexOf(".")) : "";
  const isCode = lang !== "markdown";
  const wrap = !isCode || (extension !== "" && target.lineWrapExtensions.includes(extension));
  // The rules follow the file name as it is typed.
  const indent = useQuery({
    queryKey: ["editor-indent", owner, name, target.branch, path],
    queryFn: () => fetchRepoEditorIndent(owner, name, target.branch, path),
    enabled: fileName !== "",
    staleTime: Infinity,
    placeholderData: keepPreviousData,
  });
  const preview = useQuery({
    queryKey: ["markdown", owner, name, content],
    queryFn: () => renderRepoMarkdown(owner, name, content),
    enabled: tab === "preview" && content.trim() !== "",
    staleTime: Infinity,
  });

  const save = useMutation({
    mutationFn: () =>
      commitRepoFile(owner, name, {
        ...commitOf(target.branch, commit),
        oldPath: isNew ? "" : target.path,
        path,
        content,
        lastCommit,
      }),
    onSuccess: ({ location }) => window.location.assign(location),
    onError: (err) => {
      // Committing again overwrites the changes made since.
      const conflict = conflictOf(err);
      if (conflict) setLastCommit(conflict.lastCommit);
    },
  });
  const conflict = conflictOf(save.error);
  const { fields, formError } = formErrors(conflict ? null : save.error);
  const onSubmit = (e: FormEvent) => {
    e.preventDefault();
    save.mutate();
  };

  const heading = isNew ? t("repo.editor.new_file") : t("repo.editor.edit_file");
  const tabs: { key: typeof tab; label: string }[] = [{ key: "write", label: heading }];
  if (!isCode && target.previewMarkdown) tabs.push({ key: "preview", label: t("repo.release.preview") });
  if (!isNew) tabs.push({ key: "changes", label: t("repo.editor.preview_changes") });
  const currentTab = tabs.some((x) => x.key === tab) ? tab : "write";
  const defaultSummary = (isNew ? t("repo.editor.add") : t("repo.editor.update")).replace(
    "%s",
    path || t("repo.editor.name_your_file"),
  );

  return (
    <EditorLayout owner={owner} name={name} title={path ? `${heading}: ${path}` : heading}>
      <h1 className="sr-only">{heading}</h1>
      <form onSubmit={onSubmit} noValidate>
        <fieldset disabled={save.isPending} className="flex min-w-0 flex-col gap-5">
          <FormAlert message={formError} />
          {conflict ? (
            <div
              role="alert"
              className="rounded-md border border-(--color-destructive) bg-(--color-destructive)/10 px-3 py-2 text-sm text-(--color-destructive) [&_a]:underline"
              dangerouslySetInnerHTML={{ __html: conflict.html }}
            />
          ) : null}
          <FormField
            id="editor-path"
            label={t("repo.editor.file_path")}
            helper={t("repo.editor.filename_help")}
            error={fields.path}
            invalid={Boolean(fields.path)}
          >
            <PathInput
              id="editor-path"
              repoName={name}
              dirs={dirs}
              onDirs={setDirs}
              value={fileName}
              onChange={setFileName}
              placeholder={t("repo.editor.name_your_file")}
              autoFocus={isNew}
            />
          </FormField>
          <div className="min-w-0 rounded-md border border-(--color-border) p-3">
            <EditorTabs id={id} label={heading} tabs={tabs} current={currentTab} onChange={setTab} />
            {currentTab === "write" ? (
              <div role="tabpanel" id={`${id}-write`} aria-labelledby={`${id}-write-tab`}>
                <CodeEditor
                  id="editor-content"
                  label={path || heading}
                  lang={lang}
                  code={isCode}
                  wrap={wrap}
                  {...indentationOf(indent.data)}
                  value={content}
                  onChange={setContent}
                  autoFocus={!isNew}
                  minLines={20}
                  aria-invalid={Boolean(fields.content) || undefined}
                />
              </div>
            ) : currentTab === "preview" ? (
              <div
                role="tabpanel"
                id={`${id}-preview`}
                aria-labelledby={`${id}-preview-tab`}
                aria-busy={preview.isFetching}
                className="min-h-32 rounded-md border border-(--color-border) px-4 py-3"
              >
                {content.trim() === "" ? (
                  <p className="text-sm text-(--color-muted-foreground) italic">{t("repo.issues.no_content")}</p>
                ) : preview.data !== undefined ? (
                  <Markdown html={preview.data} />
                ) : preview.isError ? (
                  <p className="text-sm text-(--color-destructive)">{preview.error.message}</p>
                ) : (
                  <p className="text-sm text-(--color-muted-foreground)">{t("repo.release.loading")}</p>
                )}
              </div>
            ) : (
              <div role="tabpanel" id={`${id}-changes`} aria-labelledby={`${id}-changes-tab`}>
                <ChangesPreview
                  oldFile={{ name: target.path, contents: original }}
                  newFile={{ name: path, contents: content }}
                  wrap={wrap}
                />
              </div>
            )}
          </div>
        </fieldset>
        <fieldset disabled={save.isPending} className="min-w-0">
          <CommitForm
            target={target}
            value={commit}
            onChange={setCommit}
            defaultSummary={defaultSummary}
            errors={fields}
            cancelURL={srcURL(owner, name, target.branch, target.path)}
          />
        </fieldset>
      </form>
    </EditorLayout>
  );
}

// RepoFileDelete deletes a file of a branch.
export function RepoFileDelete({ owner, name, target }: { owner: string; name: string; target: RepoEditorTarget }) {
  const { t } = useTranslation();
  const [commit, setCommit] = useCommitFields();
  const remove = useMutation({
    mutationFn: () => deleteRepoFile(owner, name, { ...commitOf(target.branch, commit), path: target.path }),
    onSuccess: ({ location }) => window.location.assign(location),
  });
  const { fields, formError } = formErrors(remove.error);
  const onSubmit = (e: FormEvent) => {
    e.preventDefault();
    remove.mutate();
  };
  const heading = t("repo.editor.delete_file");

  return (
    <EditorLayout owner={owner} name={name} title={`${heading}: ${target.path}`}>
      <form onSubmit={onSubmit} noValidate>
        <fieldset disabled={remove.isPending} className="min-w-0">
          <h1 className="mb-4 text-xl font-semibold">{heading}</h1>
          <FormAlert message={formError} />
          <p className="flex items-start gap-2 rounded-md border border-(--color-border) bg-(--color-surface) px-4 py-3 font-mono text-sm">
            <FileX className="mt-0.5 size-4 shrink-0 text-(--color-destructive)" aria-hidden />
            <span className="min-w-0 wrap-anywhere">{target.path}</span>
          </p>
          <CommitForm
            target={target}
            value={commit}
            onChange={setCommit}
            defaultSummary={t("repo.editor.delete").replace("%s", target.path)}
            errors={fields}
            cancelURL={srcURL(owner, name, target.branch, target.path)}
            destructive
          />
        </fieldset>
      </form>
    </EditorLayout>
  );
}

// RepoFileUpload commits files picked from the device to a directory of a
// branch.
export function RepoFileUpload({ owner, name, target }: { owner: string; name: string; target: RepoEditorTarget }) {
  const { t } = useTranslation();
  const fileInput = useRef<HTMLInputElement>(null);
  const [dragging, setDragging] = useState(false);
  const [dirs, setDirs] = useState(() => splitPath(target.path));
  const [subdir, setSubdir] = useState("");
  const [commit, setCommit] = useCommitFields();
  const uploads = useAttachmentUploads(owner, name, target.upload, "editor");
  const path = [...dirs, subdir].filter(Boolean).join("/");

  const upload = useMutation({
    mutationFn: () =>
      uploadRepoFiles(owner, name, { ...commitOf(target.branch, commit), path, files: uploads.uploaded }),
    onSuccess: ({ location }) => window.location.assign(location),
  });
  const { fields, formError } = formErrors(upload.error);
  const filesError = uploads.files.length ? null : fields.files;
  const onSubmit = (e: FormEvent) => {
    e.preventDefault();
    upload.mutate();
  };
  // A removed file is deleted from the server too, since it will not be
  // committed.
  const pending = {
    ...uploads,
    remove: (key: string) => {
      const uuid = uploads.files.find((f) => f.key === key)?.uuid;
      if (uuid) void deleteRepoEditorUpload(owner, name, uuid).catch(() => undefined);
      uploads.remove(key);
    },
  };
  const settings = target.upload;
  const heading = t("repo.editor.upload_file");

  return (
    <EditorLayout owner={owner} name={name} title={heading}>
      <form onSubmit={onSubmit} noValidate>
        <fieldset disabled={upload.isPending} className="flex min-w-0 flex-col gap-5">
          <h1 className="text-xl font-semibold">{heading}</h1>
          <FormAlert message={formError} />
          <FormField
            id="upload-path"
            label={t("repo.editor.directory")}
            error={fields.path}
            invalid={Boolean(fields.path)}
          >
            <PathInput
              id="upload-path"
              repoName={name}
              dirs={dirs}
              onDirs={setDirs}
              value={subdir}
              onChange={setSubdir}
              placeholder={t("repo.editor.add_subdir")}
            />
          </FormField>
          <div>
            <input
              ref={fileInput}
              type="file"
              multiple
              hidden
              accept={!settings || settings.allowedTypes.includes("*/*") ? undefined : settings.allowedTypes.join(",")}
              onChange={(e) => {
                if (e.target.files) uploads.add(e.target.files);
                e.target.value = "";
              }}
            />
            <button
              type="button"
              aria-describedby={filesError ? "upload-files-error" : undefined}
              disabled={settings ? uploads.files.length >= settings.maxFiles : true}
              onClick={() => fileInput.current?.click()}
              onDragOver={(e) => {
                e.preventDefault();
                setDragging(true);
              }}
              onDragLeave={() => setDragging(false)}
              onDrop={(e) => {
                e.preventDefault();
                setDragging(false);
                uploads.add(e.dataTransfer.files);
              }}
              className={cn(
                "flex w-full cursor-pointer flex-col items-center gap-2 rounded-md border-2 border-dashed px-4 py-10 text-sm text-(--color-muted-foreground) transition-colors hover:border-(--color-ring) hover:text-(--color-foreground) focus-visible:border-(--color-ring) focus-visible:outline-none disabled:cursor-not-allowed disabled:opacity-60",
                dragging
                  ? "border-(--color-primary) bg-(--color-primary)/5 text-(--color-foreground)"
                  : "border-(--color-border)",
                filesError && "border-(--color-destructive)",
              )}
            >
              <Upload className="size-6" aria-hidden />
              {t("dropzone.default_message")}
            </button>
            {filesError ? (
              <p id="upload-files-error" className="mt-1.5 text-sm text-(--color-destructive)">
                {filesError}
              </p>
            ) : null}
            <PendingFiles uploads={pending} />
          </div>
        </fieldset>
        <fieldset disabled={upload.isPending} className="min-w-0">
          <CommitForm
            target={target}
            value={commit}
            onChange={setCommit}
            defaultSummary={t("repo.editor.upload_files_to_dir").replace("%s", path)}
            errors={fields}
            cancelURL={srcURL(owner, name, target.branch, target.path)}
            submitDisabled={uploads.uploading}
          />
        </fieldset>
      </form>
    </EditorLayout>
  );
}
