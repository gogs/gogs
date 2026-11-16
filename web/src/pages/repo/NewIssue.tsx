import { useMutation, useSuspenseQuery } from "@tanstack/react-query";
import { Link, useNavigate } from "@tanstack/react-router";
import { ExternalLink, Upload } from "lucide-react";
import { type FormEvent, useEffect, useEffectEvent, useRef, useState } from "react";
import { useTranslation } from "react-i18next";

import { CheckboxField, FormAlert } from "@/components/FormField";
import { AttachFilesButton, IssueMeta, MarkdownEditor, PendingFiles } from "@/components/IssueForm";
import { Markdown } from "@/components/Markdown";
import { RepoHeader } from "@/components/RepoHeader";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { usePageTitle } from "@/lib/page-title";
import {
  type IssueAttachmentSettings,
  type IssueMetaOptions,
  type IssueTemplate,
  type IssueTemplateField,
  type RepoNewIssueForm,
  type RepoNewIssueOptions,
  createRepoIssue,
  repoHeaderQuery,
} from "@/lib/queries/repo";
import { formErrors } from "@/lib/queries/request";
import { linkedUploads, useAttachmentUploads } from "@/lib/use-attachment-uploads";
import { cn } from "@/lib/utils";
import type { RepoNewIssueSearch } from "@/pages/repo/NewIssue.search";

const TEXTAREA_CLASS =
  "block w-full rounded-md border border-(--color-input) bg-(--color-background) px-3 py-2 text-sm text-(--color-foreground) placeholder:text-(--color-muted-foreground) outline-none transition-colors focus-visible:border-(--color-ring) focus-visible:ring-1 focus-visible:ring-(--color-ring) disabled:cursor-not-allowed disabled:opacity-60 aria-invalid:border-(--color-destructive)";

// The value a single-choice dropdown has when no option is chosen, since a
// select item cannot have an empty value.
const NO_OPTION = "\u0000";

// RepoNewIssue is the form to open an issue. Writers also set its labels,
// milestone and assignee. When the repository has issue templates, they are
// offered first.
export function RepoNewIssue({
  owner,
  name,
  options,
  search,
}: {
  owner: string;
  name: string;
  options: RepoNewIssueOptions;
  search: RepoNewIssueSearch;
}) {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const { data: repo } = useSuspenseQuery(repoHeaderQuery(owner, name));
  usePageTitle(`${t("repo.issues.new")} - ${repo.owner}/${repo.name}`);
  const choosing =
    !options.template &&
    (options.templates.length > 0 || options.contactLinks.length > 0) &&
    (!search.blank || !options.blankIssuesEnabled);

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <RepoHeader repo={repo} activeTab="issues" />
      <main className="mx-auto w-full max-w-7xl flex-1 px-4 py-6 sm:px-6">
        <h1 className="mb-4 text-2xl font-semibold">{t("repo.issues.new")}</h1>
        {choosing ? (
          <TemplateChooser owner={repo.owner} name={repo.name} options={options} />
        ) : (
          <NewIssueForm
            owner={repo.owner}
            name={repo.name}
            options={options}
            template={options.template}
            label={t("repo.issues.new")}
            submitLabel={t("repo.issues.create")}
            title={search.title ?? options.template?.title ?? ""}
            // Like the server-rendered page, the issue template wins over the
            // content in the URL.
            content={options.template ? options.template.body : options.content || (search.content ?? "")}
            create={(form) => createRepoIssue(repo.owner, repo.name, form)}
            onCreated={(index) =>
              navigate({
                to: "/$owner/$repo/issues/$index",
                params: { owner: repo.owner, repo: repo.name, index: String(index) },
              })
            }
          />
        )}
      </main>
    </div>
  );
}

function TemplateChooser({ owner, name, options }: { owner: string; name: string; options: RepoNewIssueOptions }) {
  const { t } = useTranslation();
  const params = { owner, repo: name };
  const rowClass =
    "flex flex-col gap-3 border-b border-(--color-border) px-4 py-3 last:border-b-0 sm:flex-row sm:items-center sm:justify-between";
  return (
    <div>
      <p className="mb-3 text-sm text-(--color-muted-foreground)">{t("repo.issues.new.choose_template")}</p>
      <ul className="rounded-md border border-(--color-border)">
        {options.templates.map((tmpl) => (
          <li key={tmpl.file} className={rowClass}>
            <div className="min-w-0">
              <p className="font-semibold wrap-anywhere">{tmpl.name}</p>
              {tmpl.about ? (
                <p className="text-sm text-(--color-muted-foreground) wrap-anywhere">{tmpl.about}</p>
              ) : null}
            </div>
            <Button asChild size="sm" className="shrink-0 self-start sm:self-auto">
              <Link to="/$owner/$repo/issues/new" params={params} search={{ template: tmpl.file }}>
                {t("repo.issues.new.get_started")}
                <span className="sr-only">: {tmpl.name}</span>
              </Link>
            </Button>
          </li>
        ))}
        {options.contactLinks.map((link) => (
          <li key={link.url + link.name} className={rowClass}>
            <div className="min-w-0">
              <p className="font-semibold wrap-anywhere">{link.name}</p>
              {link.about ? (
                <p className="text-sm text-(--color-muted-foreground) wrap-anywhere">{link.about}</p>
              ) : null}
            </div>
            <Button asChild size="sm" variant="outline" className="shrink-0 self-start sm:self-auto">
              <a href={link.url} target="_blank" rel="noopener noreferrer">
                {t("repo.issues.new.open_link")}
                <span className="sr-only">: {link.name}</span>
                <ExternalLink className="size-4" aria-hidden />
              </a>
            </Button>
          </li>
        ))}
      </ul>
      {options.blankIssuesEnabled ? (
        <p className="mt-3 text-sm">
          <Link
            to="/$owner/$repo/issues/new"
            params={params}
            search={{ blank: true }}
            className="text-(--color-primary) underline underline-offset-2"
          >
            {t("repo.issues.new.open_blank")}
          </Link>
        </p>
      ) : null}
    </div>
  );
}

// initialFieldValues returns what each element of an issue form starts with.
function initialFieldValues(form: IssueTemplateField[]) {
  return form.map((field): string[] => {
    if ((field.type === "input" || field.type === "textarea") && field.value) return [field.value];
    if (field.type === "dropdown" && field.default !== undefined && field.options?.[field.default]) {
      return [field.options[field.default].label];
    }
    return [];
  });
}

// NewIssueForm is the form to open an issue or a pull request, starting with
// the title and the content, or the fields of an issue form. Writers also set
// its labels, milestone and assignee.
export function NewIssueForm({
  owner,
  name,
  options,
  label,
  submitLabel,
  template,
  title: initialTitle,
  content: initialContent,
  create,
  onCreated,
}: {
  owner: string;
  name: string;
  options: IssueMetaOptions & { attachment?: IssueAttachmentSettings };
  // template is the issue template the form is made from, if any.
  template?: IssueTemplate;
  label: string;
  submitLabel: string;
  title: string;
  content: string;
  create: (form: RepoNewIssueForm) => Promise<{ index: number }>;
  onCreated: (index: number) => Promise<void>;
}) {
  const { t } = useTranslation();
  const repoRef = { owner, name };
  const form = template?.form;
  const [title, setTitle] = useState(initialTitle);
  const [content, setContent] = useState(initialContent);
  const [fieldValues, setFieldValues] = useState(() => initialFieldValues(form ?? []));
  // Writers start with the labels and the assignee of the template, and can
  // change them.
  const [labelIDs, setLabelIDs] = useState<number[]>(
    () =>
      options.labelOptions
        ?.filter((l) => template?.labels.some((n) => n.toLowerCase() === l.name.toLowerCase()))
        .map((l) => l.id) ?? [],
  );
  const [milestoneID, setMilestoneID] = useState(0);
  const [assigneeID, setAssigneeID] = useState(() => {
    for (const n of template?.assignees ?? []) {
      const user = options.assigneeOptions?.find((u) => u.name.toLowerCase() === n.toLowerCase());
      if (user) return user.id;
    }
    return 0;
  });
  const uploads = useAttachmentUploads(owner, name, options.attachment);

  const mutation = useMutation({
    mutationFn: () =>
      create({
        title,
        content: form ? "" : content,
        labelIDs,
        milestoneID,
        assigneeID,
        attachments: form ? [] : linkedUploads(uploads.uploaded, content),
        template: template?.file,
        fields: form ? fieldValues : undefined,
      }),
    onSuccess: ({ index }) => onCreated(index),
    // The first invalid field may be far above the submit button, so it is
    // brought into view below the sticky header.
    onError: (err) => {
      const { fields } = formErrors(err);
      const ids = ["title", ...(form ?? []).map((_, i) => `field-${i}`)].filter((key) => key in fields);
      if (ids.length === 0) return;
      const id = `new-issue-${ids[0]}`;
      requestAnimationFrame(() => {
        const el = document.getElementById(id) ?? document.getElementById(`${id}-0`);
        el?.scrollIntoView({ block: "center" });
        el?.focus({ preventScroll: true });
      });
    },
  });
  const { fields, formError } = formErrors(mutation.error);
  // edited has the fields changed since the last submit, whose errors are
  // hidden until the form is submitted again.
  const [edited, setEdited] = useState<string[]>([]);
  const markEdited = (key: string) => setEdited((keys) => (keys.includes(key) ? keys : [...keys, key]));
  const fieldError = (key: string) => (edited.includes(key) ? undefined : fields[key]);
  const titleError = fieldError("title");
  const onSubmit = (e: FormEvent) => {
    e.preventDefault();
    setEdited([]);
    mutation.mutate();
  };
  const milestone = options.milestoneOptions?.find((m) => m.id === milestoneID);
  const assignee = options.assigneeOptions?.find((u) => u.id === assigneeID);
  // The choices are kept in the form until it is submitted.
  const pick = (set: (id: number) => void) => (id: number) => {
    set(id);
    return Promise.resolve();
  };
  const setFieldValue = (index: number, value: string[]) => {
    setFieldValues((values) => values.map((v, i) => (i === index ? value : v)));
    markEdited(`field-${index}`);
  };
  // fieldsUploading has the upload fields whose files are still uploading.
  const [fieldsUploading, setFieldsUploading] = useState<number[]>([]);
  const setFieldUploading = (index: number, uploading: boolean) =>
    setFieldsUploading((list) => (uploading ? [...new Set([...list, index])] : list.filter((i) => i !== index)));

  return (
    <form onSubmit={onSubmit} className="grid content-start gap-8 md:grid-cols-[minmax(0,1fr)_16rem]">
      <fieldset disabled={mutation.isPending} className="contents">
        <div className="min-w-0">
          <FormAlert message={formError} />
          {template ? (
            <div className="mb-4 flex flex-wrap items-baseline justify-between gap-2 text-sm">
              <p className="font-semibold wrap-anywhere">{template.name}</p>
              <Link
                to="/$owner/$repo/issues/new"
                params={{ owner, repo: name }}
                className="text-(--color-primary) underline underline-offset-2"
              >
                {t("repo.issues.new.change_template")}
              </Link>
            </div>
          ) : null}
          <div className="flex flex-col gap-1.5">
            <Input
              id="new-issue-title"
              aria-label={t("repo.milestones.title")}
              placeholder={t("repo.milestones.title")}
              value={title}
              maxLength={255}
              autoFocus
              aria-invalid={Boolean(titleError) || undefined}
              aria-describedby={titleError ? "new-issue-title-error" : undefined}
              onChange={(e) => {
                setTitle(e.target.value);
                markEdited("title");
              }}
            />
            {titleError ? (
              <p id="new-issue-title-error" className="text-sm text-(--color-destructive)">
                {titleError}
              </p>
            ) : null}
          </div>
          {form ? (
            <div className="mt-4 flex flex-col gap-6">
              {form.map((field, i) => (
                <FormElement
                  key={i}
                  id={`new-issue-field-${i}`}
                  field={field}
                  value={fieldValues[i]}
                  onChange={(value) => setFieldValue(i, value)}
                  error={fieldError(`field-${i}`)}
                  upload={{ owner, name, settings: options.attachment }}
                  onUploading={(uploading) => setFieldUploading(i, uploading)}
                />
              ))}
              <div className="flex justify-end">
                <Button type="submit" size="sm" disabled={fieldsUploading.length > 0}>
                  {submitLabel}
                </Button>
              </div>
            </div>
          ) : (
            <div className="mt-4 rounded-md border border-(--color-border) p-3">
              <MarkdownEditor
                repo={repoRef}
                label={label}
                value={content}
                onChange={setContent}
                disabled={mutation.isPending}
                uploads={uploads}
                linkFiles
              />
              <div className="mt-3 flex flex-wrap items-center justify-between gap-2">
                <AttachFilesButton uploads={uploads} />
                <Button type="submit" size="sm" disabled={uploads.uploading}>
                  {submitLabel}
                </Button>
              </div>
            </div>
          )}
        </div>
        <IssueMeta
          repo={repoRef}
          value={{
            labels: options.labelOptions?.filter((l) => labelIDs.includes(l.id)) ?? [],
            milestone,
            assignee,
          }}
          options={options}
          pending={mutation.isPending}
          onLabels={setLabelIDs}
          onMilestone={pick(setMilestoneID)}
          onAssignee={pick(setAssigneeID)}
          linkChoices={false}
        />
      </fieldset>
    </form>
  );
}

// FormElement is an element of an issue form. The server checks the values,
// so a missing required value shows the error it answers with.
function FormElement({
  id,
  field,
  value,
  onChange,
  error,
  upload,
  onUploading,
}: {
  id: string;
  field: IssueTemplateField;
  value: string[];
  onChange: (value: string[]) => void;
  error?: string | null;
  // upload is where the files of an upload element go.
  upload: { owner: string; name: string; settings?: IssueAttachmentSettings };
  onUploading: (uploading: boolean) => void;
}) {
  const { t } = useTranslation();
  if (field.type === "markdown") {
    return <Markdown html={field.html ?? ""} />;
  }

  const describedBy = [field.descriptionHTML ? `${id}-description` : "", error ? `${id}-error` : ""]
    .filter(Boolean)
    .join(" ");
  const aria = {
    "aria-invalid": Boolean(error) || undefined,
    "aria-describedby": describedBy || undefined,
    "aria-required": field.required || undefined,
  };
  const label = (
    <>
      {field.label}
      {field.required ? (
        <span className="text-(--color-destructive)" title={t("repo.issues.new.required")}>
          {" *"}
          <span className="sr-only">{t("repo.issues.new.required")}</span>
        </span>
      ) : null}
    </>
  );
  const description = field.descriptionHTML ? (
    <div id={`${id}-description`} className="text-sm text-(--color-muted-foreground)">
      <Markdown html={field.descriptionHTML} />
    </div>
  ) : null;
  const errorText = error ? (
    <p id={`${id}-error`} className="text-sm text-(--color-destructive)">
      {error}
    </p>
  ) : null;

  if (field.type === "upload") {
    return (
      <div className="flex flex-col gap-1.5">
        <Label htmlFor={id}>{label}</Label>
        {description}
        <UploadField
          id={id}
          accept={field.accept}
          upload={upload}
          onChange={onChange}
          onUploading={onUploading}
          aria={aria}
        />
        {errorText}
      </div>
    );
  }

  // Checkboxes and multiple-choice dropdowns are a group of checkboxes.
  if (field.type === "checkboxes" || (field.type === "dropdown" && field.multiple)) {
    const toggle = (option: string, checked: boolean) =>
      onChange(checked ? [...value, option] : value.filter((v) => v !== option));
    return (
      <fieldset className="flex flex-col gap-2" {...aria}>
        <legend className="mb-1.5 text-sm font-medium">{label}</legend>
        {description}
        {field.options?.map((option, j) => (
          <CheckboxField
            key={j}
            id={`${id}-${j}`}
            checked={value.includes(option.label)}
            onChange={(checked) => toggle(option.label, checked)}
            label={
              <>
                {option.labelHTML ? (
                  <span
                    className="[&_a]:text-(--color-primary) [&_a]:underline [&_a]:underline-offset-2 [&_code]:rounded [&_code]:bg-(--color-surface) [&_code]:px-1 [&_code]:font-mono [&_code]:text-[0.9em]"
                    // The server renders and sanitizes the Markdown.
                    dangerouslySetInnerHTML={{ __html: option.labelHTML }}
                  />
                ) : (
                  option.label
                )}
                {option.required ? (
                  <span className="text-(--color-destructive)">
                    {" *"}
                    <span className="sr-only">{t("repo.issues.new.required")}</span>
                  </span>
                ) : null}
              </>
            }
          />
        ))}
        {errorText}
      </fieldset>
    );
  }

  let control;
  if (field.type === "dropdown") {
    control = (
      <Select
        // An empty value shows the placeholder and keeps the select controlled.
        value={value[0] ?? (field.required ? "" : NO_OPTION)}
        onValueChange={(v) => onChange(v === NO_OPTION ? [] : [v])}
        aria-invalid={aria["aria-invalid"]}
        aria-describedby={aria["aria-describedby"]}
      >
        <SelectTrigger id={id}>
          <SelectValue placeholder={t("repo.issues.new.select_option")} />
        </SelectTrigger>
        <SelectContent>
          {field.required ? null : <SelectItem value={NO_OPTION}>{t("repo.issues.new.no_option")}</SelectItem>}
          {field.options?.map((option, j) => (
            <SelectItem key={j} value={option.label}>
              {option.label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    );
  } else if (field.type === "textarea") {
    control = (
      <textarea
        id={id}
        rows={field.render ? 6 : 4}
        className={field.render ? `${TEXTAREA_CLASS} font-mono` : TEXTAREA_CLASS}
        placeholder={field.placeholder}
        value={value[0] ?? ""}
        onChange={(e) => onChange([e.target.value])}
        {...aria}
      />
    );
  } else {
    control = (
      <Input
        id={id}
        placeholder={field.placeholder}
        value={value[0] ?? ""}
        onChange={(e) => onChange([e.target.value])}
        {...aria}
      />
    );
  }
  return (
    <div className="flex flex-col gap-1.5">
      <Label htmlFor={id}>{label}</Label>
      {description}
      {control}
      {errorText}
    </div>
  );
}

// UploadField takes the files of an upload element, dropped on it or picked,
// and uploads them right away. Its value is the IDs of the uploaded files.
function UploadField({
  id,
  accept,
  upload,
  onChange,
  onUploading,
  aria,
}: {
  id: string;
  accept?: string;
  upload: { owner: string; name: string; settings?: IssueAttachmentSettings };
  onChange: (value: string[]) => void;
  onUploading: (uploading: boolean) => void;
  aria: { "aria-invalid"?: boolean; "aria-describedby"?: string; "aria-required"?: boolean };
}) {
  const { t } = useTranslation();
  const uploads = useAttachmentUploads(upload.owner, upload.name, upload.settings);
  const fileInput = useRef<HTMLInputElement>(null);
  const [dragging, setDragging] = useState(false);
  const report = useEffectEvent((uuids: string[], uploading: boolean) => {
    onChange(uuids);
    onUploading(uploading);
  });
  const uploadedKey = uploads.uploaded.join(",");
  useEffect(() => {
    report(uploadedKey ? uploadedKey.split(",") : [], uploads.uploading);
  }, [uploadedKey, uploads.uploading]);

  if (!upload.settings) {
    return (
      <p id={id} className="text-sm text-(--color-muted-foreground) italic">
        {t("repo.issues.new.uploads_disabled")}
      </p>
    );
  }
  const full = uploads.files.length >= upload.settings.maxFiles;
  return (
    <div>
      <input
        ref={fileInput}
        type="file"
        multiple
        hidden
        accept={
          accept || (upload.settings.allowedTypes.includes("*/*") ? undefined : upload.settings.allowedTypes.join(","))
        }
        onChange={(e) => {
          if (e.target.files) uploads.add(e.target.files);
          e.target.value = "";
        }}
      />
      <button
        id={id}
        type="button"
        disabled={full}
        onClick={() => fileInput.current?.click()}
        onDragOver={(e) => {
          e.preventDefault();
          setDragging(true);
        }}
        onDragLeave={() => setDragging(false)}
        onDrop={(e) => {
          e.preventDefault();
          setDragging(false);
          if (e.dataTransfer.files.length) uploads.add(e.dataTransfer.files);
        }}
        className={cn(
          "flex w-full cursor-pointer flex-col items-center gap-1.5 rounded-md border border-dashed border-(--color-input) px-4 py-6 text-sm text-(--color-muted-foreground) outline-none transition-colors hover:bg-(--color-surface) focus-visible:border-(--color-ring) focus-visible:ring-1 focus-visible:ring-(--color-ring) disabled:cursor-not-allowed disabled:opacity-60 aria-invalid:border-(--color-destructive)",
          dragging && "border-(--color-primary) bg-(--color-surface)",
        )}
        {...aria}
      >
        <Upload className="size-5" aria-hidden />
        {t("dropzone.default_message")}
      </button>
      <PendingFiles uploads={uploads} />
    </div>
  );
}
