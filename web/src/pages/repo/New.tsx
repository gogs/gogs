import { useMutation } from "@tanstack/react-query";
import { ChevronDown } from "lucide-react";
import { type FormEvent, type ReactNode, useState } from "react";
import { useTranslation } from "react-i18next";

import { CheckboxField, FormAlert, FormField } from "@/components/FormField";
import { RichText } from "@/components/RichText";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { usePageTitle } from "@/lib/page-title";
import {
  type RepoCreateForm,
  type RepoMigrateForm,
  type RepoNewOptions,
  createRepo,
  migrateRepo,
} from "@/lib/queries/repo";
import { FormError, formErrors } from "@/lib/queries/request";
import { subUrl } from "@/lib/url";

const DESCRIPTION_MAX = 512;
const NO_LICENSE = "none";
const TEXTAREA_CLASS =
  "block w-full rounded-md border border-(--color-input) bg-(--color-background) px-3 py-2 text-sm text-(--color-foreground) outline-none transition-colors focus-visible:border-(--color-ring) focus-visible:ring-1 focus-visible:ring-(--color-ring) disabled:cursor-not-allowed disabled:opacity-60 aria-invalid:border-(--color-destructive)";

type Fields = Record<string, string | null>;
type Shared = Pick<RepoCreateForm, "ownerID" | "name" | "description" | "private" | "unlisted">;

// useNewRepoMutation submits a new repository form, focuses the first field
// the server rejects, and opens the repository once it exists. The repository
// pages are still served by the server.
function useNewRepoMutation(
  submit: () => Promise<{ owner: string; name: string }>,
  fieldOrder: string[],
  onFieldErrors?: (fields: Fields) => void,
) {
  return useMutation({
    mutationFn: submit,
    onSuccess: (repo) => window.location.assign(subUrl(`/${repo.owner}/${repo.name}`)),
    onError: (err) => {
      const fields = err instanceof FormError ? err.fields : {};
      onFieldErrors?.(fields);
      const first = fieldOrder.find((f) => f in fields);
      if (first) requestAnimationFrame(() => document.getElementById(first)?.focus());
    },
  });
}

// SharedFields are the owner, name, visibility and description of a new
// repository, which creating and migrating both ask for.
function SharedFields({
  options,
  form,
  setForm,
  fields,
  nameHelper,
  children,
}: {
  options: RepoNewOptions;
  form: Shared;
  setForm: (form: Shared) => void;
  fields: Fields;
  nameHelper?: string;
  // Settings shown after the visibility, e.g., whether a migration mirrors.
  children?: ReactNode;
}) {
  const { t } = useTranslation();
  const remaining = DESCRIPTION_MAX - Array.from(form.description).length;
  return (
    <>
      <FormField id="ownerID" label={t("repo.owner")} error={fields.ownerID} invalid={"ownerID" in fields}>
        <Select value={String(form.ownerID)} onValueChange={(v) => setForm({ ...form, ownerID: Number(v) })}>
          <SelectTrigger id="ownerID" className="sm:max-w-xs">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {options.owners.map((owner) => (
              <SelectItem key={owner.id} value={String(owner.id)}>
                <span className="flex min-w-0 items-center gap-2">
                  <img src={owner.avatarURL} alt="" className="size-5 shrink-0 rounded-sm" />
                  <span className="truncate">{owner.name}</span>
                </span>
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </FormField>
      <FormField
        id="name"
        label={t("repo.repo_name")}
        helper={nameHelper}
        error={fields.name}
        invalid={"name" in fields}
      >
        <Input
          id="name"
          name="name"
          autoComplete="off"
          required
          maxLength={100}
          value={form.name}
          onChange={(e) => setForm({ ...form, name: e.target.value })}
        />
      </FormField>
      <fieldset className="flex flex-col gap-3">
        <legend className="mb-3 text-sm font-medium">{t("repo.visibility")}</legend>
        {options.forcePrivate ? (
          <label htmlFor="private" className="flex items-start gap-2.5 text-sm">
            <Checkbox id="private" className="mt-0.5" checked disabled />
            <span>
              <RichText text={t("repo.visiblity_helper_forced")} />
            </span>
          </label>
        ) : (
          <CheckboxField
            id="private"
            label={<RichText text={t("repo.visiblity_helper")} />}
            checked={form.private}
            onChange={(checked) => setForm({ ...form, private: checked })}
          />
        )}
        <CheckboxField
          id="unlisted"
          label={<RichText text={t("repo.unlisted_helper")} />}
          checked={form.unlisted}
          onChange={(checked) => setForm({ ...form, unlisted: checked })}
        />
      </fieldset>
      {children}
      <FormField
        id="description"
        label={t("repo.repo_desc")}
        helper={`${t("repo.repo_description_helper")} ${t("repo.repo_description_length")}: ${remaining}`}
        error={fields.description}
        invalid={"description" in fields}
      >
        <textarea
          id="description"
          name="description"
          rows={3}
          className={TEXTAREA_CLASS}
          value={form.description}
          onChange={(e) =>
            setForm({ ...form, description: Array.from(e.target.value).slice(0, DESCRIPTION_MAX).join("") })
          }
        />
      </FormField>
    </>
  );
}

function initialShared(options: RepoNewOptions): Shared {
  return {
    ownerID: options.ownerID,
    name: "",
    description: "",
    private: options.private || options.forcePrivate,
    unlisted: false,
  };
}

function NewRepoPage({
  title,
  onSubmit,
  pending,
  formError,
  submitLabel,
  children,
}: {
  title: string;
  onSubmit: () => void;
  pending: boolean;
  formError: string | null;
  submitLabel: string;
  children: ReactNode;
}) {
  usePageTitle(title);
  const submit = (e: FormEvent) => {
    e.preventDefault();
    onSubmit();
  };
  return (
    <main className="mx-auto w-full max-w-2xl flex-1 px-4 py-10 sm:px-6">
      <form onSubmit={submit} noValidate>
        <fieldset disabled={pending} className="contents">
          <h1 className="mb-4 text-xl font-semibold">{title}</h1>
          <FormAlert message={formError} />
          <div className="flex flex-col gap-5">
            {children}
            <div className="border-t border-(--color-border) pt-5">
              <Button type="submit" variant="outline">
                {submitLabel}
              </Button>
            </div>
          </div>
        </fieldset>
      </form>
    </main>
  );
}

// GitignorePicker chooses any number of .gitignore templates from a list that
// can be searched, since there are over a hundred of them.
function GitignorePicker({
  options,
  value,
  onChange,
  ...aria
}: {
  options: string[];
  value: string[];
  onChange: (value: string[]) => void;
  "aria-invalid"?: boolean;
  "aria-describedby"?: string;
}) {
  const { t } = useTranslation();
  const [query, setQuery] = useState("");
  const shown = options.filter((name) => name.toLowerCase().includes(query.trim().toLowerCase()));
  return (
    <Popover onOpenChange={() => setQuery("")}>
      <PopoverTrigger
        id="gitignores"
        className="flex h-10 w-full cursor-pointer items-center justify-between gap-2 rounded-md border border-(--color-input) bg-(--color-background) px-3 py-2 text-left text-sm outline-none transition-colors focus-visible:border-(--color-ring) focus-visible:ring-1 focus-visible:ring-(--color-ring) disabled:cursor-not-allowed disabled:opacity-60 aria-invalid:border-(--color-destructive)"
        {...aria}
      >
        {value.length > 0 ? (
          <span className="truncate">{value.join(", ")}</span>
        ) : (
          <span className="truncate text-(--color-muted-foreground)">{t("repo.repo_gitignore_helper")}</span>
        )}
        <ChevronDown className="size-4 shrink-0 text-(--color-muted-foreground)" aria-hidden />
      </PopoverTrigger>
      <PopoverContent align="start" className="w-(--radix-popover-trigger-width) p-2">
        <Input
          aria-label={t("explore.search")}
          placeholder={t("explore.search")}
          value={query}
          onChange={(e) => setQuery(e.target.value)}
        />
        <ul className="mt-2 flex max-h-64 flex-col gap-2 overflow-y-auto px-1 py-1">
          {shown.map((name) => (
            <li key={name}>
              <CheckboxField
                id={`gitignore-${options.indexOf(name)}`}
                label={name}
                checked={value.includes(name)}
                onChange={(checked) =>
                  onChange(
                    checked ? options.filter((o) => o === name || value.includes(o)) : value.filter((v) => v !== name),
                  )
                }
              />
            </li>
          ))}
        </ul>
      </PopoverContent>
    </Popover>
  );
}

const CREATE_FIELDS = ["ownerID", "name", "description", "objectFormat", "gitignores", "license", "readme"];

// RepoCreate creates a repository for the viewer or an organization they own,
// optionally with a first commit of the chosen files.
export function RepoCreate({ options }: { options: RepoNewOptions }) {
  const { t } = useTranslation();
  const [form, setForm] = useState<RepoCreateForm>(() => ({
    ...initialShared(options),
    autoInit: false,
    gitignores: [],
    license: "",
    readme: options.readmes.includes("Default") ? "Default" : (options.readmes[0] ?? ""),
    objectFormat: "sha1",
  }));
  const save = useNewRepoMutation(() => createRepo(form), CREATE_FIELDS);
  const { fields, formError } = formErrors(save.error);

  return (
    <NewRepoPage
      title={t("new_repo")}
      onSubmit={() => save.mutate()}
      pending={save.isPending || save.isSuccess}
      formError={formError}
      submitLabel={save.isPending || save.isSuccess ? t("repo.create_repo_submitting") : t("repo.create_repo")}
    >
      <SharedFields
        options={options}
        form={form}
        setForm={(shared) => setForm({ ...form, ...shared })}
        fields={fields}
        nameHelper={t("repo.repo_name_helper")}
      />
      <FormField
        id="objectFormat"
        label={t("repo.object_format")}
        helper={t("repo.object_format_helper")}
        error={fields.objectFormat}
        invalid={"objectFormat" in fields}
      >
        <Select
          value={form.objectFormat}
          onValueChange={(v) => setForm({ ...form, objectFormat: v === "sha256" ? "sha256" : "sha1" })}
        >
          <SelectTrigger id="objectFormat" className="sm:max-w-xs">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="sha1">SHA-1</SelectItem>
            <SelectItem value="sha256">SHA-256</SelectItem>
          </SelectContent>
        </Select>
      </FormField>
      <div className="flex flex-col gap-5 border-t border-(--color-border) pt-5">
        <CheckboxField
          id="autoInit"
          label={t("repo.auto_init")}
          checked={form.autoInit}
          onChange={(checked) => setForm({ ...form, autoInit: checked })}
        />
        {form.autoInit ? (
          <div className="flex flex-col gap-5 border-l-2 border-(--color-border) pl-4">
            <FormField id="gitignores" label=".gitignore" error={fields.gitignores} invalid={"gitignores" in fields}>
              <GitignorePicker
                options={options.gitignores}
                value={form.gitignores}
                onChange={(gitignores) => setForm({ ...form, gitignores })}
              />
            </FormField>
            <FormField id="license" label={t("repo.license")} error={fields.license} invalid={"license" in fields}>
              <Select
                value={form.license || NO_LICENSE}
                onValueChange={(v) => setForm({ ...form, license: v === NO_LICENSE ? "" : v })}
              >
                <SelectTrigger id="license">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value={NO_LICENSE}>{t("repo.no_license")}</SelectItem>
                  {options.licenses.map((name) => (
                    <SelectItem key={name} value={name}>
                      {name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </FormField>
            <FormField id="readme" label={t("repo.readme")} error={fields.readme} invalid={"readme" in fields}>
              <Select value={form.readme} onValueChange={(readme) => setForm({ ...form, readme })}>
                <SelectTrigger id="readme">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {options.readmes.map((name) => (
                    <SelectItem key={name} value={name}>
                      {name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </FormField>
          </div>
        ) : null}
      </div>
    </NewRepoPage>
  );
}

const MIGRATE_FIELDS = ["cloneAddr", "authUsername", "authPassword", "ownerID", "name", "description"];

// RepoMigrate copies a repository from another server, or from a path on this
// one for users allowed to, optionally keeping it as a mirror.
export function RepoMigrate({ options, mirror }: { options: RepoNewOptions; mirror: boolean }) {
  const { t } = useTranslation();
  const [form, setForm] = useState<RepoMigrateForm>(() => ({
    ...initialShared(options),
    cloneAddr: "",
    authUsername: "",
    authPassword: "",
    mirror,
  }));
  const [authOpen, setAuthOpen] = useState(false);
  const save = useNewRepoMutation(
    () => migrateRepo(form),
    MIGRATE_FIELDS,
    (fields) => {
      if ("authUsername" in fields || "authPassword" in fields) setAuthOpen(true);
    },
  );
  const { fields, formError } = formErrors(save.error);
  const pending = save.isPending || save.isSuccess;

  return (
    <NewRepoPage
      title={t("new_migrate")}
      onSubmit={() => save.mutate()}
      pending={pending}
      formError={formError}
      submitLabel={pending ? t("repo.migrate_repo_submitting") : t("repo.migrate_repo")}
    >
      <FormField
        id="cloneAddr"
        label={t("repo.migrate.clone_address")}
        helper={
          options.canImportLocal
            ? `${t("repo.migrate.clone_address_desc")} ${t("repo.migrate.clone_address_desc_import_local")}`
            : t("repo.migrate.clone_address_desc")
        }
        error={fields.cloneAddr}
        invalid={"cloneAddr" in fields}
      >
        <Input
          id="cloneAddr"
          name="cloneAddr"
          inputMode="url"
          autoComplete="off"
          autoFocus
          required
          value={form.cloneAddr}
          onChange={(e) => setForm({ ...form, cloneAddr: e.target.value })}
        />
      </FormField>
      <div className="flex flex-col gap-5">
        <button
          type="button"
          aria-expanded={authOpen}
          aria-controls="migrate-auth"
          onClick={() => setAuthOpen(!authOpen)}
          className="flex w-fit cursor-pointer items-center gap-1.5 rounded-sm text-sm font-medium outline-none focus-visible:ring-1 focus-visible:ring-(--color-ring)"
        >
          <ChevronDown className={`size-4 transition-transform ${authOpen ? "" : "-rotate-90"}`} aria-hidden />
          {t("repo.need_auth")}
        </button>
        {authOpen ? (
          <div id="migrate-auth" className="flex flex-col gap-5 border-l-2 border-(--color-border) pl-4">
            <FormField
              id="authUsername"
              label={t("username")}
              error={fields.authUsername}
              invalid={"authUsername" in fields}
            >
              <Input
                id="authUsername"
                name="authUsername"
                autoComplete="off"
                value={form.authUsername}
                onChange={(e) => setForm({ ...form, authUsername: e.target.value })}
              />
            </FormField>
            <FormField
              id="authPassword"
              label={t("password")}
              error={fields.authPassword}
              invalid={"authPassword" in fields}
            >
              <Input
                id="authPassword"
                name="authPassword"
                type="password"
                autoComplete="new-password"
                value={form.authPassword}
                onChange={(e) => setForm({ ...form, authPassword: e.target.value })}
              />
            </FormField>
          </div>
        ) : null}
      </div>
      <div className="flex flex-col gap-5 border-t border-(--color-border) pt-5">
        <SharedFields
          options={options}
          form={form}
          setForm={(shared) => setForm({ ...form, ...shared })}
          fields={fields}
        >
          <fieldset>
            <legend className="mb-3 text-sm font-medium">{t("repo.migrate_type")}</legend>
            <CheckboxField
              id="mirror"
              label={<RichText text={t("repo.migrate_type_helper")} />}
              checked={form.mirror}
              onChange={(checked) => setForm({ ...form, mirror: checked })}
            />
          </fieldset>
        </SharedFields>
      </div>
    </NewRepoPage>
  );
}
