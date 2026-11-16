import { useMutation, useSuspenseQuery } from "@tanstack/react-query";
import { Link, useNavigate } from "@tanstack/react-router";
import { ExternalLink } from "lucide-react";
import { type FormEvent, useState } from "react";
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
  type IssueTemplateField,
  type RepoNewIssueOptions,
  createRepoIssue,
  repoHeaderQuery,
} from "@/lib/queries/repo";
import { formErrors } from "@/lib/queries/request";
import { useAttachmentUploads } from "@/lib/use-attachment-uploads";
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
          <NewIssueForm owner={repo.owner} name={repo.name} options={options} search={search} />
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

function NewIssueForm({
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
  const repoRef = { owner, name };
  const template = options.template;
  const form = template?.form;
  const [title, setTitle] = useState(search.title ?? template?.title ?? "");
  // Like the server-rendered page, the issue template wins over the content
  // in the URL.
  const [content, setContent] = useState(template ? template.body : options.content || (search.content ?? ""));
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
      createRepoIssue(owner, name, {
        title,
        content: form ? "" : content,
        labelIDs,
        milestoneID,
        assigneeID,
        attachments: uploads.uploaded,
        template: template?.file,
        fields: form ? fieldValues : undefined,
      }),
    onSuccess: ({ index }) =>
      navigate({
        to: "/$owner/$repo/issues/$index",
        params: { owner, repo: name, index: String(index) },
      }),
  });
  const { fields, formError } = formErrors(mutation.error);
  const onSubmit = (e: FormEvent) => {
    e.preventDefault();
    mutation.mutate();
  };
  const milestone = options.milestoneOptions?.find((m) => m.id === milestoneID);
  const assignee = options.assigneeOptions?.find((u) => u.id === assigneeID);
  // The choices are kept in the form until it is submitted.
  const pick = (set: (id: number) => void) => (id: number) => {
    set(id);
    return Promise.resolve();
  };
  const setFieldValue = (index: number, value: string[]) =>
    setFieldValues((values) => values.map((v, i) => (i === index ? value : v)));

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
              aria-label={t("repo.milestones.title")}
              placeholder={t("repo.milestones.title")}
              value={title}
              maxLength={255}
              autoFocus
              aria-invalid={Boolean(fields.title) || undefined}
              aria-describedby={fields.title ? "new-issue-title-error" : undefined}
              onChange={(e) => setTitle(e.target.value)}
            />
            {fields.title ? (
              <p id="new-issue-title-error" className="text-sm text-(--color-destructive)">
                {fields.title}
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
                  error={fields[`field-${i}`]}
                />
              ))}
              <div className="flex justify-end">
                <Button type="submit" size="sm">
                  {t("repo.issues.create")}
                </Button>
              </div>
            </div>
          ) : (
            <div className="mt-4 rounded-md border border-(--color-border) p-3">
              <MarkdownEditor
                repo={repoRef}
                label={t("repo.issues.new")}
                value={content}
                onChange={setContent}
                disabled={mutation.isPending}
                uploads={uploads}
              />
              <PendingFiles uploads={uploads} />
              <div className="mt-3 flex flex-wrap items-center justify-between gap-2">
                <AttachFilesButton uploads={uploads} />
                <Button type="submit" size="sm" disabled={uploads.uploading}>
                  {t("repo.issues.create")}
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
}: {
  id: string;
  field: IssueTemplateField;
  value: string[];
  onChange: (value: string[]) => void;
  error?: string | null;
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
        value={value[0] ?? (field.required ? undefined : NO_OPTION)}
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
