import { useMutation, useSuspenseQuery } from "@tanstack/react-query";
import { Link, useRouter } from "@tanstack/react-router";
import { Check, CircleDot, CircleHelp, Milestone, Plus, Tag } from "lucide-react";
import { type FormEvent, type ReactNode, useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";

import { ConfirmButton } from "@/components/ConfirmButton";
import { FormAlert } from "@/components/FormField";
import { RepoHeader } from "@/components/RepoHeader";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { usePageTitle } from "@/lib/page-title";
import {
  type RepoLabel,
  type RepoLabelForm,
  type RepoLabelsData,
  createRepoLabel,
  deleteRepoLabel,
  initializeRepoLabels,
  repoHeaderQuery,
  updateRepoLabel,
} from "@/lib/queries/repo";
import { formErrors } from "@/lib/queries/request";
import { subUrl } from "@/lib/url";
import { cn } from "@/lib/utils";

// The colors offered next to the color picker, the same as on the
// server-rendered page.
const PRESET_COLORS = [
  "#e11d21",
  "#eb6420",
  "#fbca04",
  "#009800",
  "#006b75",
  "#207de5",
  "#0052cc",
  "#5319e7",
  "#f6c6c7",
  "#fad8c7",
  "#fef2c0",
  "#bfe5bf",
  "#bfdadc",
  "#c7def8",
  "#bfd4f2",
  "#d4c5f9",
];
const HEX_COLOR = /^#[0-9a-f]{6}$/i;

// IssueTrackerLayout heads the labels and milestones pages of a repository,
// with links between them.
export function IssueTrackerLayout({
  owner,
  name,
  active,
  title,
  action,
  children,
}: {
  owner: string;
  name: string;
  active: "labels" | "milestones";
  title: string;
  action?: ReactNode;
  children: ReactNode;
}) {
  const { t } = useTranslation();
  const { data: repo } = useSuspenseQuery(repoHeaderQuery(owner, name));
  usePageTitle(`${title} - ${repo.owner}/${repo.name}`);
  const params = { owner: repo.owner, repo: repo.name };
  const tabs = [
    { key: "labels", to: "/$owner/$repo/labels", label: t("repo.labels"), icon: Tag },
    { key: "milestones", to: "/$owner/$repo/milestones", label: t("repo.milestones"), icon: Milestone },
  ] as const;

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <RepoHeader repo={repo} activeTab="issues" />
      <main className="mx-auto w-full max-w-7xl flex-1 px-4 py-6 sm:px-6">
        <h1 className="sr-only">{title}</h1>
        <div className="mb-4 flex flex-wrap items-center justify-between gap-2">
          <nav aria-label={t("issues")} className="inline-flex rounded-md border border-(--color-input)">
            {tabs.map(({ key, to, label, icon: Icon }) => (
              <Link
                key={key}
                to={to}
                params={params}
                aria-current={key === active ? "page" : undefined}
                className={cn(
                  "inline-flex items-center gap-1.5 px-3 py-1.5 text-sm first:rounded-l-md last:rounded-r-md not-first:border-l not-first:border-(--color-input) hover:bg-(--color-surface)",
                  key === active
                    ? "bg-(--color-surface) font-semibold text-(--color-foreground)"
                    : "text-(--color-muted-foreground)",
                )}
              >
                <Icon className="size-4" aria-hidden />
                {label}
              </Link>
            ))}
          </nav>
          {action}
        </div>
        {children}
      </main>
    </div>
  );
}

// labelTextColor picks black or white text for the color of a label, the same
// way as the server-rendered pages.
function labelTextColor(color: string) {
  if (!HEX_COLOR.test(color)) return "#000";
  const n = parseInt(color.slice(1), 16);
  const luminance = (0.2126 * ((n >> 16) & 0xff) + 0.7152 * ((n >> 8) & 0xff) + 0.0722 * (n & 0xff)) / 255;
  return luminance < 0.66 ? "#fff" : "#000";
}

function LabelChip({ name, color }: { name: string; color: string }) {
  return (
    <span
      className="inline-flex max-w-full items-center gap-1.5 rounded-full px-2.5 py-0.5 text-sm font-medium"
      style={{ backgroundColor: color, color: labelTextColor(color) }}
    >
      <Tag className="size-3.5 shrink-0" aria-hidden />
      <span className="truncate">{name}</span>
    </span>
  );
}

// RepoLabels lists the labels of a repository. Writers can add, change and
// delete them.
export function RepoLabels({ owner, name, data }: { owner: string; name: string; data: RepoLabelsData }) {
  const { t } = useTranslation();
  const router = useRouter();
  const [creating, setCreating] = useState(false);
  const [editing, setEditing] = useState<number | null>(null);

  return (
    <IssueTrackerLayout
      owner={owner}
      name={name}
      active="labels"
      title={t("repo.labels")}
      action={
        data.viewerCanWrite && !creating ? (
          <Button variant="outline" size="sm" onClick={() => setCreating(true)}>
            <Plus className="size-4" aria-hidden />
            {t("repo.issues.new_label")}
          </Button>
        ) : null
      }
    >
      {creating ? (
        <div className="mb-4 rounded-md border border-(--color-border) p-4">
          <LabelForm
            initial={{ name: "", color: "#70c24a" }}
            submitLabel={t("repo.issues.create_label")}
            save={(form) => createRepoLabel(owner, name, form)}
            onClose={() => setCreating(false)}
          />
        </div>
      ) : null}
      {data.viewerCanWrite && data.items.length === 0 && data.templates?.length ? (
        <LabelTemplates owner={owner} name={name} templates={data.templates} />
      ) : null}
      <div className="rounded-md border border-(--color-border)">
        <h2 className="bg-(--color-surface) px-4 py-3 text-sm font-semibold">
          {t("repo.issues.label_count").replace(/%(?:\[1\])?d/, String(data.items.length))}
        </h2>
        {data.items.length ? (
          <ul>
            {data.items.map((label) => (
              <li key={label.id} className="border-t border-(--color-border) px-4 py-3">
                {editing === label.id ? (
                  <LabelForm
                    initial={{ name: label.name, color: label.color }}
                    submitLabel={t("repo.issues.save")}
                    save={(form) => updateRepoLabel(owner, name, label.id, form)}
                    onClose={() => setEditing(null)}
                  />
                ) : (
                  <LabelRow
                    owner={owner}
                    name={name}
                    label={label}
                    canWrite={data.viewerCanWrite}
                    onEdit={() => setEditing(label.id)}
                    onDeleted={() => router.invalidate()}
                  />
                )}
              </li>
            ))}
          </ul>
        ) : null}
      </div>
    </IssueTrackerLayout>
  );
}

function LabelRow({
  owner,
  name,
  label,
  canWrite,
  onEdit,
  onDeleted,
}: {
  owner: string;
  name: string;
  label: RepoLabel;
  canWrite: boolean;
  onEdit: () => void;
  onDeleted: () => Promise<void>;
}) {
  const { t } = useTranslation();
  return (
    <div className="flex flex-wrap items-center gap-x-6 gap-y-2">
      <div className="min-w-0 flex-1 basis-48">
        <LabelChip name={label.name} color={label.color} />
      </div>
      {/* The issue list is still served by the server-rendered pages. */}
      <a
        href={subUrl(`/${owner}/${name}/issues?labels=${label.id}`)}
        className="inline-flex items-center gap-1.5 text-sm text-(--color-muted-foreground) hover:text-(--color-foreground) hover:underline"
      >
        <CircleDot className="size-4" aria-hidden />
        {t("repo.issues.label_open_issues").replace(/%(?:\[1\])?d/, String(label.openIssueCount))}
      </a>
      {canWrite ? (
        <div className="flex gap-2">
          <Button
            variant="outline"
            size="sm"
            aria-label={`${t("repo.issues.label_edit")} ${label.name}`}
            onClick={onEdit}
          >
            {t("repo.issues.label_edit")}
          </Button>
          <ConfirmButton
            size="sm"
            label={t("repo.issues.label_delete")}
            ariaLabel={`${t("repo.issues.label_delete")} ${label.name}`}
            title={t("repo.issues.label_deletion")}
            description={t("repo.issues.label_deletion_desc")}
            action={() => deleteRepoLabel(owner, name, label.id)}
            onDone={async () => {
              toast.success(t("repo.issues.label_deletion_success"));
              await onDeleted();
            }}
          />
        </div>
      ) : null}
    </div>
  );
}

// LabelForm takes the name and the color of a new or changed label.
function LabelForm({
  initial,
  submitLabel,
  save,
  onClose,
}: {
  initial: RepoLabelForm;
  submitLabel: string;
  save: (form: RepoLabelForm) => Promise<void>;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const router = useRouter();
  const [form, setForm] = useState(initial);
  const mutation = useMutation({
    mutationFn: () => save(form),
    onSuccess: async () => {
      await router.invalidate();
      onClose();
    },
  });
  const { fields, formError } = formErrors(mutation.error);
  // While the hex is being typed, show the last color that was valid.
  const shownColor = HEX_COLOR.test(form.color) ? form.color.toLowerCase() : initial.color;

  const onSubmit = (e: FormEvent) => {
    e.preventDefault();
    mutation.mutate();
  };

  return (
    <form onSubmit={onSubmit} noValidate>
      <fieldset disabled={mutation.isPending} className="contents">
        <FormAlert message={formError} />
        <div className="mb-3">
          <LabelChip name={form.name || t("repo.issues.new_label_placeholder")} color={shownColor} />
        </div>
        <div className="flex flex-wrap items-start gap-4">
          <Input
            aria-label={t("repo.issues.label_title")}
            aria-invalid={"name" in fields || undefined}
            aria-describedby={fields.name ? "label-name-error" : undefined}
            placeholder={t("repo.issues.new_label_placeholder")}
            autoComplete="off"
            autoFocus
            required
            maxLength={50}
            value={form.name}
            onChange={(e) => setForm({ ...form, name: e.target.value })}
            className="w-full sm:w-64"
          />
          <div className="flex items-center gap-2">
            <input
              type="color"
              aria-label={t("repo.issues.label_color")}
              value={shownColor}
              onChange={(e) => setForm({ ...form, color: e.target.value })}
              className="size-10 shrink-0 cursor-pointer rounded-md border border-(--color-input) bg-(--color-background) p-1"
            />
            <Input
              aria-label={`${t("repo.issues.label_color")} (hex)`}
              aria-invalid={"color" in fields || undefined}
              aria-describedby={fields.color ? "label-color-error" : undefined}
              autoComplete="off"
              spellCheck={false}
              required
              maxLength={7}
              value={form.color}
              onChange={(e) => setForm({ ...form, color: e.target.value })}
              className="w-28 font-mono"
            />
          </div>
          <ul className="grid grid-cols-8 gap-1" aria-label={t("repo.issues.label_color")}>
            {PRESET_COLORS.map((color) => {
              const selected = color === form.color.toLowerCase();
              return (
                <li key={color}>
                  <button
                    type="button"
                    aria-label={color}
                    aria-pressed={selected}
                    onClick={() => setForm({ ...form, color })}
                    className="flex size-6 cursor-pointer items-center justify-center rounded-sm outline-none focus-visible:ring-2 focus-visible:ring-(--color-ring) focus-visible:ring-offset-1"
                    style={{ backgroundColor: color, color: labelTextColor(color) }}
                  >
                    {selected ? <Check className="size-4" aria-hidden /> : null}
                  </button>
                </li>
              );
            })}
          </ul>
          <div className="flex gap-2 lg:ml-auto">
            <Button type="button" variant="outline" size="sm" onClick={onClose}>
              {t("cancel")}
            </Button>
            <Button type="submit" variant="outline" size="sm">
              {submitLabel}
            </Button>
          </div>
        </div>
        {(["name", "color"] as const).map((field) =>
          fields[field] ? (
            <p key={field} id={`label-${field}-error`} className="mt-2 text-sm text-(--color-destructive)">
              {fields[field]}
            </p>
          ) : null,
        )}
      </fieldset>
    </form>
  );
}

// LabelTemplates offers the predefined sets of labels to a repository that has
// none yet.
function LabelTemplates({ owner, name, templates }: { owner: string; name: string; templates: string[] }) {
  const { t } = useTranslation();
  const router = useRouter();
  const [template, setTemplate] = useState(templates[0]);
  const mutation = useMutation({
    mutationFn: () => initializeRepoLabels(owner, name, template),
    onSuccess: () => router.invalidate(),
    onError: (err) => toast.error(err.message),
  });
  return (
    <section className="mb-4 rounded-md border border-(--color-border) p-4">
      <h2 className="flex items-center gap-1.5 font-semibold">
        {t("repo.issues.label_templates.title")}
        <a
          href="https://discuss.gogs.io/t/how-to-use-predefined-label-templates/599"
          target="_blank"
          rel="noopener noreferrer"
          aria-label={t("help")}
          className="inline-flex size-6 items-center justify-center rounded-md text-(--color-muted-foreground) hover:bg-(--color-surface) hover:text-(--color-foreground)"
        >
          <CircleHelp className="size-4" aria-hidden />
        </a>
      </h2>
      <p className="mt-1 text-sm text-(--color-muted-foreground)">{t("repo.issues.label_templates.info")}</p>
      <div className="mt-4 flex flex-wrap gap-2">
        <Select value={template} onValueChange={setTemplate}>
          <SelectTrigger className="w-full sm:w-64" aria-label={t("repo.issues.label_templates.helper")}>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {templates.map((name) => (
              <SelectItem key={name} value={name}>
                {name}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <Button variant="outline" disabled={mutation.isPending} onClick={() => mutation.mutate()}>
          {t("repo.issues.label_templates.use")}
        </Button>
      </div>
    </section>
  );
}
