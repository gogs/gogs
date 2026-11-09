import { useMutation, useQueryClient, useSuspenseQuery } from "@tanstack/react-query";
import { Link, useNavigate, useRouter } from "@tanstack/react-router";
import { type FormEvent, type ReactNode, useRef, useState } from "react";
import { Trans, useTranslation } from "react-i18next";
import { toast } from "sonner";

import { AvatarForm } from "@/components/AvatarForm";
import { DeleteForm } from "@/components/DeleteForm";
import { FormField } from "@/components/FormField";
import { OrgHeader } from "@/components/OrgHeader";
import { SettingsLayout, SettingsNavItem } from "@/components/SettingsLayout";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { usePageTitle } from "@/lib/page-title";
import {
  type OrgSettingsData,
  type OrgSettingsForm,
  deleteOrg,
  deleteOrgAvatar,
  updateOrgSettings,
  uploadOrgAvatar,
} from "@/lib/queries/org";
import { FormError } from "@/lib/queries/request";
import { userProfileQuery } from "@/lib/queries/user";
import { subUrl } from "@/lib/url";

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

  return (
    <main className="flex-1">
      <OrgHeader org={org} activeTab="settings" />
      <SettingsLayout
        label={t("org.settings")}
        current={active === "options" ? t("org.settings.options") : t("org.settings.delete")}
        nav={
          <>
            <SettingsNavItem active={active === "options"}>
              <Link to="/org/$org/settings" params={{ org: org.name }} activeOptions={{ exact: true }}>
                {t("org.settings.options")}
              </Link>
            </SettingsNavItem>
            {/* Webhooks are still served by the server-rendered pages. */}
            <SettingsNavItem active={false}>
              <a href={subUrl(`/org/${org.name}/settings/hooks`)}>{t("repo.settings.hooks")}</a>
            </SettingsNavItem>
            <SettingsNavItem active={active === "delete"} destructive>
              <Link to="/org/$org/settings/delete" params={{ org: org.name }}>
                {t("org.settings.delete")}
              </Link>
            </SettingsNavItem>
          </>
        }
      >
        {children}
      </SettingsLayout>
    </main>
  );
}

// OrgSettings changes the name, the profile and the avatar of an
// organization.
export function OrgSettings({ org, settings }: { org: string; settings: OrgSettingsData }) {
  return (
    <OrgSettingsLayout org={org} active="options">
      <OptionsForm org={org} settings={settings} />
      <OrgAvatarForm org={org} settings={settings} />
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

function OrgAvatarForm({ org, settings }: { org: string; settings: OrgSettingsData }) {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const router = useRouter();
  return (
    <div className="mt-8 border-t border-(--color-border) pt-6">
      <AvatarForm
        avatarURL={settings.avatarURL}
        hasCustomAvatar={settings.hasCustomAvatar}
        upload={(file) => uploadOrgAvatar(org, file)}
        remove={() => deleteOrgAvatar(org)}
        onChanged={async (uploaded) => {
          if (uploaded) toast.success(t("org.settings.update_avatar_success"));
          await queryClient.invalidateQueries({ queryKey: userProfileQuery(org).queryKey });
          await router.invalidate();
        }}
      />
    </div>
  );
}

// OrgDelete deletes an organization once the owner confirms their password.
export function OrgDelete({ org }: { org: string }) {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  return (
    <OrgSettingsLayout org={org} active="delete">
      <h2 className="mb-4 text-xl font-semibold">{t("org.settings.delete_account")}</h2>
      <DeleteForm
        prompt={<Trans i18nKey="org.settings.delete_prompt" />}
        confirmLabel={t("org.settings.confirm_delete_account")}
        remove={(password) => deleteOrg(org, password)}
        onDeleted={async () => {
          queryClient.clear();
          await navigate({ to: "/" });
        }}
      />
    </OrgSettingsLayout>
  );
}
