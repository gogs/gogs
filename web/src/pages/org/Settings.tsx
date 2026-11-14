import { useMutation, useQueryClient, useSuspenseQuery } from "@tanstack/react-query";
import { Link, useNavigate, useRouter } from "@tanstack/react-router";
import { type FormEvent, type ReactNode, useRef, useState } from "react";
import { Trans, useTranslation } from "react-i18next";
import { toast } from "sonner";

import { FormField } from "@/components/FormField";
import { OrgHeader } from "@/components/OrgHeader";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { usePageTitle } from "@/lib/page-title";
import {
  FormError,
  type OrgSettingsData,
  type OrgSettingsForm,
  deleteOrg,
  deleteOrgAvatar,
  updateOrgSettings,
  uploadOrgAvatar,
} from "@/lib/queries/org";
import { userProfileQuery } from "@/lib/queries/user";
import { subUrl } from "@/lib/url";
import { cn } from "@/lib/utils";

const FIELD_ORDER = ["name", "fullName", "description", "website", "location", "maxRepoCreation"] as const;

// OrgSettingsLayout heads the settings pages of an organization, with links
// between them. Only owners get here.
function OrgSettingsLayout({
  org: orgName,
  active,
  children,
}: {
  org: string;
  active: "options" | "delete";
  children: ReactNode;
}) {
  const { t } = useTranslation();
  const { data: org } = useSuspenseQuery(userProfileQuery(orgName));
  usePageTitle(`${t("org.settings")} - ${org.fullName || org.name}`);
  if (org.kind !== "organization") return null;

  const itemClass = (isActive: boolean) =>
    cn(
      "rounded-md px-3 py-1.5 text-sm whitespace-nowrap hover:bg-(--color-surface)",
      isActive ? "bg-(--color-surface) font-semibold text-(--color-foreground)" : "text-(--color-muted-foreground)",
    );
  return (
    <main className="flex-1">
      <OrgHeader org={org} activeTab="settings" />
      <div className="mx-auto grid w-full max-w-6xl content-start gap-6 px-4 py-6 sm:px-6 md:grid-cols-[200px_minmax(0,1fr)]">
        <nav aria-label={t("org.settings")} className="flex flex-wrap gap-1 md:flex-col">
          <Link
            to="/org/$org/settings"
            params={{ org: org.name }}
            aria-current={active === "options" ? "page" : undefined}
            className={itemClass(active === "options")}
          >
            {t("org.settings.options")}
          </Link>
          {/* Webhooks are still served by the server-rendered pages. */}
          <a href={subUrl(`/org/${org.name}/settings/hooks`)} className={itemClass(false)}>
            {t("repo.settings.hooks")}
          </a>
          <Link
            to="/org/$org/settings/delete"
            params={{ org: org.name }}
            aria-current={active === "delete" ? "page" : undefined}
            className={cn(itemClass(active === "delete"), active !== "delete" && "text-(--color-destructive)")}
          >
            {t("org.settings.delete")}
          </Link>
        </nav>
        <section className="min-w-0 max-w-xl">{children}</section>
      </div>
    </main>
  );
}

// OrgSettings changes the name, the profile and the avatar of an
// organization.
export function OrgSettings({ org, settings }: { org: string; settings: OrgSettingsData }) {
  return (
    <OrgSettingsLayout org={org} active="options">
      <OptionsForm org={org} settings={settings} />
      <AvatarForm org={org} settings={settings} />
    </OrgSettingsLayout>
  );
}

function OptionsForm({ org, settings }: { org: string; settings: OrgSettingsData }) {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const router = useRouter();
  const [form, setForm] = useState<OrgSettingsForm>({
    name: settings.name,
    fullName: settings.fullName,
    description: settings.description,
    website: settings.website,
    location: settings.location,
    maxRepoCreation: settings.maxRepoCreation,
  });
  const refs = useRef<Partial<Record<(typeof FIELD_ORDER)[number], HTMLInputElement | HTMLTextAreaElement | null>>>({});

  const save = useMutation({
    mutationFn: () => updateOrgSettings(org, form),
    onSuccess: async (name) => {
      toast.success(t("org.settings.update_setting_success"));
      if (name !== org) {
        // The old name no longer exists, so drop it rather than refetch it.
        queryClient.removeQueries({ queryKey: userProfileQuery(org).queryKey });
        await navigate({ to: "/org/$org/settings", params: { org: name }, replace: true });
        return;
      }
      await queryClient.invalidateQueries({ queryKey: userProfileQuery(org).queryKey });
      await router.invalidate();
    },
    onError: (err) => {
      const fields = err instanceof FormError ? err.fields : {};
      const first = FIELD_ORDER.find((f) => f in fields);
      if (first) requestAnimationFrame(() => refs.current[first]?.focus());
    },
  });
  const fields = save.error instanceof FormError ? save.error.fields : {};
  const formError = save.error && Object.keys(fields).length === 0 ? save.error.message : null;
  const renaming = form.name.toLowerCase() !== settings.name.toLowerCase();

  const onSubmit = (e: FormEvent) => {
    e.preventDefault();
    save.mutate();
  };
  const field = (key: Exclude<keyof OrgSettingsForm, "maxRepoCreation">) => ({
    id: key,
    name: key,
    value: form[key],
    onChange: (e: { target: { value: string } }) => setForm({ ...form, [key]: e.target.value }),
  });

  return (
    <form onSubmit={onSubmit} noValidate>
      <fieldset disabled={save.isPending} className="contents">
        <h2 className="mb-4 text-xl font-semibold">{t("org.settings.options")}</h2>
        {formError ? (
          <div
            role="alert"
            className="mb-4 rounded-md border border-(--color-destructive) bg-(--color-destructive)/10 px-3 py-2 text-sm text-(--color-destructive)"
          >
            {formError}
          </div>
        ) : null}
        <div className="flex flex-col gap-5">
          <FormField
            id="name"
            label={t("org.org_name_holder")}
            // Renaming changes every link to the organization.
            helper={renaming ? t("org.settings.change_orgname_prompt") : undefined}
            error={fields.name}
            invalid={"name" in fields}
          >
            <Input
              ref={(el) => {
                refs.current.name = el;
              }}
              autoComplete="off"
              required
              {...field("name")}
            />
          </FormField>
          <FormField
            id="fullName"
            label={t("org.org_full_name_holder")}
            error={fields.fullName}
            invalid={"fullName" in fields}
          >
            <Input
              ref={(el) => {
                refs.current.fullName = el;
              }}
              autoComplete="off"
              {...field("fullName")}
            />
          </FormField>
          <FormField
            id="description"
            label={t("org.org_desc")}
            error={fields.description}
            invalid={"description" in fields}
          >
            <textarea
              ref={(el) => {
                refs.current.description = el;
              }}
              rows={3}
              className="block w-full rounded-md border border-(--color-input) bg-(--color-background) px-3 py-2 text-sm text-(--color-foreground) outline-none transition-colors focus-visible:border-(--color-ring) focus-visible:ring-1 focus-visible:ring-(--color-ring) disabled:cursor-not-allowed disabled:opacity-60 aria-invalid:border-(--color-destructive)"
              {...field("description")}
            />
          </FormField>
          <FormField
            id="website"
            label={t("org.settings.website")}
            error={fields.website}
            invalid={"website" in fields}
          >
            <Input
              ref={(el) => {
                refs.current.website = el;
              }}
              type="url"
              autoComplete="url"
              {...field("website")}
            />
          </FormField>
          <FormField
            id="location"
            label={t("org.settings.location")}
            error={fields.location}
            invalid={"location" in fields}
          >
            <Input
              ref={(el) => {
                refs.current.location = el;
              }}
              autoComplete="off"
              {...field("location")}
            />
          </FormField>
          {form.maxRepoCreation !== undefined ? (
            <FormField
              id="maxRepoCreation"
              label={t("admin.users.max_repo_creation")}
              helper={t("admin.users.max_repo_creation_desc")}
              error={fields.maxRepoCreation}
              invalid={"maxRepoCreation" in fields}
            >
              <Input
                ref={(el) => {
                  refs.current.maxRepoCreation = el;
                }}
                id="maxRepoCreation"
                name="maxRepoCreation"
                type="number"
                min={-1}
                className="max-w-40"
                value={form.maxRepoCreation}
                onChange={(e) => setForm({ ...form, maxRepoCreation: Number(e.target.value) })}
              />
            </FormField>
          ) : null}
          <div className="border-t border-(--color-border) pt-5">
            <Button type="submit" variant="outline">
              {t("org.settings.update_settings")}
            </Button>
          </div>
        </div>
      </fieldset>
    </form>
  );
}

function AvatarForm({ org, settings }: { org: string; settings: OrgSettingsData }) {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const router = useRouter();
  const [file, setFile] = useState<File | null>(null);
  const inputRef = useRef<HTMLInputElement>(null);
  // The avatar keeps its URL when it changes, so refetch it after a change.
  const [version, setVersion] = useState(0);
  const onChanged = async () => {
    setFile(null);
    if (inputRef.current) inputRef.current.value = "";
    setVersion((v) => v + 1);
    await queryClient.invalidateQueries({ queryKey: userProfileQuery(org).queryKey });
    await router.invalidate();
  };
  const upload = useMutation({
    mutationFn: () => uploadOrgAvatar(org, file as File),
    onSuccess: async () => {
      toast.success(t("org.settings.update_avatar_success"));
      await onChanged();
    },
    onError: (err) => toast.error(err.message),
  });
  const remove = useMutation({
    mutationFn: () => deleteOrgAvatar(org),
    onSuccess: onChanged,
    onError: (err) => toast.error(err.message),
  });
  const pending = upload.isPending || remove.isPending;
  const avatarURL = version
    ? `${settings.avatarURL}${settings.avatarURL.includes("?") ? "&" : "?"}v=${version}`
    : settings.avatarURL;

  const onSubmit = (e: FormEvent) => {
    e.preventDefault();
    if (file) upload.mutate();
  };

  return (
    <form onSubmit={onSubmit} className="mt-8 border-t border-(--color-border) pt-6">
      <fieldset disabled={pending} className="contents">
        <div className="flex flex-wrap items-start gap-4">
          <img
            src={avatarURL}
            alt=""
            className="size-20 shrink-0 rounded-md border border-(--color-border) object-cover"
          />
          <div className="flex min-w-0 flex-1 basis-56 flex-col gap-3">
            <FormField id="avatar" label={t("settings.choose_new_avatar")} invalid={false}>
              <input
                ref={inputRef}
                id="avatar"
                name="avatar"
                type="file"
                accept="image/png,image/jpeg,image/gif"
                onChange={(e) => setFile(e.target.files?.[0] ?? null)}
                className="block w-full min-w-0 text-sm text-(--color-muted-foreground) file:mr-3 file:cursor-pointer file:rounded-md file:border file:border-(--color-input) file:bg-(--color-background) file:px-3 file:py-1.5 file:text-sm file:font-medium file:text-(--color-foreground) hover:file:bg-(--color-surface)"
              />
            </FormField>
            <div className="flex flex-wrap gap-2">
              <Button type="submit" variant="outline" size="sm" disabled={!file || pending}>
                {t("settings.update_avatar")}
              </Button>
              {settings.hasCustomAvatar ? (
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  className="text-(--color-destructive)"
                  onClick={() => remove.mutate()}
                >
                  {t("settings.delete_current_avatar")}
                </Button>
              ) : null}
            </div>
          </div>
        </div>
      </fieldset>
    </form>
  );
}

// OrgDelete deletes an organization once the owner confirms their password.
export function OrgDelete({ org }: { org: string }) {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const [password, setPassword] = useState("");
  const inputRef = useRef<HTMLInputElement>(null);
  const mutation = useMutation({
    mutationFn: () => deleteOrg(org, password),
    onSuccess: async () => {
      queryClient.clear();
      await navigate({ to: "/" });
    },
    onError: () => requestAnimationFrame(() => inputRef.current?.focus()),
  });
  const fields = mutation.error instanceof FormError ? mutation.error.fields : {};
  const formError = mutation.error && Object.keys(fields).length === 0 ? mutation.error.message : null;

  const onSubmit = (e: FormEvent) => {
    e.preventDefault();
    mutation.mutate();
  };

  return (
    <OrgSettingsLayout org={org} active="delete">
      <form onSubmit={onSubmit} noValidate>
        <fieldset disabled={mutation.isPending} className="contents">
          <h2 className="mb-4 text-xl font-semibold">{t("org.settings.delete_account")}</h2>
          <p
            role="note"
            className="mb-4 rounded-md border border-(--color-destructive) bg-(--color-destructive)/10 px-3 py-2 text-sm text-(--color-destructive)"
          >
            <Trans i18nKey="org.settings.delete_prompt" />
          </p>
          {formError ? (
            <div
              role="alert"
              className="mb-4 rounded-md border border-(--color-destructive) bg-(--color-destructive)/10 px-3 py-2 text-sm text-(--color-destructive)"
            >
              {formError}
            </div>
          ) : null}
          <div className="flex flex-col gap-5">
            <FormField id="password" label={t("password")} error={fields.password} invalid={"password" in fields}>
              <Input
                ref={inputRef}
                id="password"
                name="password"
                type="password"
                autoComplete="current-password"
                required
                autoFocus
                value={password}
                onChange={(e) => setPassword(e.target.value)}
              />
            </FormField>
            <div className="border-t border-(--color-border) pt-5">
              <Button type="submit" variant="destructive">
                {t("org.settings.confirm_delete_account")}
              </Button>
            </div>
          </div>
        </fieldset>
      </form>
    </OrgSettingsLayout>
  );
}
