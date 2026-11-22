import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Link, useRouter } from "@tanstack/react-router";
import { type FormEvent, useState } from "react";
import { Trans, useTranslation } from "react-i18next";
import { toast } from "sonner";

import { AvatarForm } from "@/components/AvatarForm";
import { ConfirmButton } from "@/components/ConfirmButton";
import { DeleteForm } from "@/components/DeleteForm";
import { FormAlert, FormField } from "@/components/FormField";
import { UserSettingsLayout } from "@/components/UserSettingsLayout";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { removeOrgMember } from "@/lib/queries/org";
import { FormError, formErrors } from "@/lib/queries/request";
import {
  type SettingsOrg,
  type SettingsRepo,
  type UserPasswordForm,
  type UserProfileForm,
  type UserSettingsData,
  changePassword,
  deleteAccount,
  deleteUserAvatar,
  leaveRepo,
  updateUserProfile,
  uploadUserAvatar,
} from "@/lib/queries/settings";
import { userProfileQuery } from "@/lib/queries/user";
import { subUrl } from "@/lib/url";
import { useRefreshUserInfo, useUserInfo } from "@/lib/use-user-info";
import { RepoIcon } from "@/pages/user/Dashboard";

const PROFILE_FIELDS = ["name", "fullName", "website", "location"] as const;

// UserProfileSettings changes the username, the public profile and the avatar
// of the viewer.
export function UserProfileSettings({ settings }: { settings: UserSettingsData }) {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const router = useRouter();
  const avatarChanged = useAvatarChanged();
  const [form, setForm] = useState<UserProfileForm>({
    name: settings.name,
    fullName: settings.fullName,
    website: settings.website,
    location: settings.location,
  });

  const save = useMutation({
    mutationFn: () => updateUserProfile(form),
    onSuccess: async (name) => {
      if (name !== settings.name) {
        // Every page shows the username, so load them all again.
        window.location.assign(subUrl("/user/settings"));
        return;
      }
      toast.success(t("settings.update_profile_success"));
      await queryClient.invalidateQueries({ queryKey: userProfileQuery(name).queryKey });
      await router.invalidate();
    },
    onError: (err) => {
      const fields = err instanceof FormError ? err.fields : {};
      const first = PROFILE_FIELDS.find((f) => f in fields);
      if (first) requestAnimationFrame(() => document.getElementById(first)?.focus());
    },
  });
  const { fields, formError } = formErrors(save.error);
  const renaming = form.name.toLowerCase() !== settings.name.toLowerCase();

  const onSubmit = (e: FormEvent) => {
    e.preventDefault();
    save.mutate();
  };
  const field = (key: keyof UserProfileForm) => ({
    id: key,
    name: key,
    value: form[key],
    onChange: (e: { target: { value: string } }) => setForm({ ...form, [key]: e.target.value }),
  });

  return (
    <UserSettingsLayout active="profile">
      <form onSubmit={onSubmit} noValidate>
        <fieldset disabled={save.isPending} className="contents">
          <h1 className="mb-2 text-xl font-semibold">{t("settings.public_profile")}</h1>
          <p className="mb-4 text-sm text-(--color-muted-foreground)">{t("settings.profile_desc")}</p>
          <FormAlert message={formError} />
          <div className="flex flex-col gap-5">
            <FormField
              id="name"
              label={t("username")}
              helper={
                !settings.isLocal
                  ? t("settings.password_username_disabled")
                  : renaming
                    ? // Renaming changes every link to the account.
                      t("settings.change_username_prompt")
                    : undefined
              }
              error={fields.name}
              invalid={"name" in fields}
            >
              <Input autoComplete="username" required readOnly={!settings.isLocal} {...field("name")} />
            </FormField>
            <FormField
              id="fullName"
              label={t("settings.full_name")}
              error={fields.fullName}
              invalid={"fullName" in fields}
            >
              <Input autoComplete="name" {...field("fullName")} />
            </FormField>
            <FormField id="website" label={t("settings.website")} error={fields.website} invalid={"website" in fields}>
              <Input type="url" autoComplete="url" {...field("website")} />
            </FormField>
            <FormField
              id="location"
              label={t("settings.location")}
              error={fields.location}
              invalid={"location" in fields}
            >
              <Input autoComplete="off" {...field("location")} />
            </FormField>
            <div className="border-t border-(--color-border) pt-5">
              <Button type="submit">{t("settings.update_profile")}</Button>
            </div>
          </div>
        </fieldset>
      </form>
      <section className="mt-8 border-t border-(--color-border) pt-6">
        <h2 className="mb-4 text-base font-semibold">{t("settings.avatar")}</h2>
        <AvatarForm
          avatarURL={settings.avatarURL}
          round
          hasCustomAvatar={settings.hasCustomAvatar}
          upload={uploadUserAvatar}
          remove={deleteUserAvatar}
          onChanged={avatarChanged}
        />
      </section>
    </UserSettingsLayout>
  );
}

// useAvatarChanged reloads the avatar of the viewer wherever it shows, e.g.,
// in the navigation bar, after a change. It keeps the page, so unsaved edits
// of the profile stay.
function useAvatarChanged() {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const router = useRouter();
  const refreshUserInfo = useRefreshUserInfo();
  const user = useUserInfo();
  return async () => {
    toast.success(t("settings.update_avatar_success"));
    await Promise.all([
      refreshUserInfo(),
      user ? queryClient.invalidateQueries({ queryKey: userProfileQuery(user.username).queryKey }) : null,
      router.invalidate(),
    ]);
  };
}

const PASSWORD_FIELDS = ["oldPassword", "password", "retype"] as const;
const EMPTY_PASSWORD_FORM: UserPasswordForm = { oldPassword: "", password: "", retype: "" };

// PasswordSection changes the password of local users.
export function PasswordSection({ isLocal }: { isLocal: boolean }) {
  const { t } = useTranslation();
  const [form, setForm] = useState<UserPasswordForm>(EMPTY_PASSWORD_FORM);
  const save = useMutation({
    mutationFn: () => changePassword(form),
    onSuccess: () => {
      toast.success(t("settings.change_password_success"));
      setForm(EMPTY_PASSWORD_FORM);
    },
    onError: (err) => {
      const fields = err instanceof FormError ? err.fields : {};
      const first = PASSWORD_FIELDS.find((f) => f in fields);
      if (first) requestAnimationFrame(() => document.getElementById(first)?.focus());
    },
  });
  const { fields, formError } = formErrors(save.error);

  const onSubmit = (e: FormEvent) => {
    e.preventDefault();
    save.mutate();
  };
  const field = (key: keyof UserPasswordForm) => ({
    id: key,
    name: key,
    type: "password",
    required: true,
    value: form[key],
    onChange: (e: { target: { value: string } }) => setForm({ ...form, [key]: e.target.value }),
  });

  return (
    <section>
      <h2 className="mb-4 text-base font-semibold">{t("settings.change_password")}</h2>
      {isLocal ? (
        <form onSubmit={onSubmit} noValidate>
          <fieldset disabled={save.isPending} className="contents">
            <FormAlert message={formError} />
            <div className="flex flex-col gap-5">
              <FormField
                id="oldPassword"
                label={t("settings.old_password")}
                error={fields.oldPassword}
                invalid={"oldPassword" in fields}
              >
                <Input autoComplete="current-password" {...field("oldPassword")} />
              </FormField>
              <FormField
                id="password"
                label={t("settings.new_password")}
                error={fields.password}
                invalid={"password" in fields}
              >
                <Input autoComplete="new-password" {...field("password")} />
              </FormField>
              <FormField
                id="retype"
                label={t("settings.retype_new_password")}
                error={fields.retype}
                invalid={"retype" in fields}
              >
                <Input autoComplete="new-password" {...field("retype")} />
              </FormField>
              <div className="border-t border-(--color-border) pt-5">
                <Button type="submit">{t("settings.change_password")}</Button>
              </div>
            </div>
          </fieldset>
        </form>
      ) : (
        <p className="rounded-md border border-(--color-border) bg-(--color-surface) px-3 py-2 text-sm">
          {t("settings.password_change_disabled")}
        </p>
      )}
    </section>
  );
}

// UserRepoSettings lists the repositories of the viewer and the ones they
// collaborate on, which they can leave.
export function UserRepoSettings({ repos }: { repos: SettingsRepo[] }) {
  const { t } = useTranslation();
  const router = useRouter();
  return (
    <UserSettingsLayout active="repositories">
      <h1 className="mb-2 text-xl font-semibold">{t("settings.repos")}</h1>
      <ul>
        {repos.map((repo) => {
          const fullName = `${repo.owner}/${repo.name}`;
          return (
            <li
              key={repo.id}
              className="flex flex-wrap items-center gap-x-3 gap-y-2 border-b border-(--color-border) py-3"
            >
              <span className="flex min-w-0 flex-1 basis-48 items-center gap-3">
                <RepoIcon repo={repo} />
                <Link
                  activeOptions={{ exact: true }}
                  to={`/${fullName}`}
                  className="min-w-0 font-semibold wrap-anywhere text-(--color-primary) hover:underline"
                >
                  {fullName}
                </Link>
                <span className="shrink-0 text-xs text-(--color-muted-foreground)">{repo.size}</span>
              </span>
              {repo.isOwner ? null : (
                <ConfirmButton
                  label={t("settings.repos.leave")}
                  ariaLabel={`${t("settings.repos.leave")} ${fullName}`}
                  title={t("settings.repos.leave_title")}
                  description={t("settings.repos.leave_desc")}
                  size="sm"
                  action={() => leaveRepo(repo.id)}
                  onDone={async () => {
                    toast.success(t("settings.repos.leave_success", { repo: fullName }));
                    await router.invalidate();
                  }}
                />
              )}
            </li>
          );
        })}
      </ul>
    </UserSettingsLayout>
  );
}

// UserOrgSettings lists the organizations the viewer is a member of, which
// they can leave.
export function UserOrgSettings({ orgs }: { orgs: SettingsOrg[] }) {
  const { t } = useTranslation();
  const user = useUserInfo();
  const queryClient = useQueryClient();
  const router = useRouter();
  return (
    <UserSettingsLayout active="organizations">
      <div className="mb-2 flex flex-wrap items-center justify-between gap-3">
        <h1 className="text-xl font-semibold">{t("settings.orgs")}</h1>
        {user?.canCreateOrganization ? (
          <Button asChild variant="outline" size="sm">
            <Link to="/org/create">{t("new_org")}</Link>
          </Button>
        ) : null}
      </div>
      {orgs.length === 0 ? (
        <p className="py-3 text-sm text-(--color-muted-foreground)">{t("settings.orgs.none")}</p>
      ) : (
        <ul>
          {orgs.map((org) => (
            <li
              key={org.name}
              className="flex flex-wrap items-center gap-x-3 gap-y-2 border-b border-(--color-border) py-3"
            >
              <span className="flex min-w-0 flex-1 basis-48 items-center gap-3">
                <img
                  src={org.avatarURL}
                  alt=""
                  className="size-8 shrink-0 rounded-md border border-(--color-border) object-cover"
                />
                <Link
                  to="/$username"
                  params={{ username: org.name }}
                  search={{}}
                  className="min-w-0 font-semibold wrap-anywhere text-(--color-primary) hover:underline"
                >
                  {org.name}
                </Link>
              </span>
              <ConfirmButton
                label={t("org.members.leave")}
                ariaLabel={`${t("org.members.leave")} ${org.name}`}
                title={t("settings.orgs.leave_title")}
                description={t("settings.orgs.leave_desc")}
                size="sm"
                action={() => removeOrgMember(org.name, user?.username ?? "")}
                onDone={async () => {
                  // The organization is gone from every page the viewer had
                  // loaded.
                  await queryClient.invalidateQueries();
                  await router.invalidate();
                }}
              />
            </li>
          ))}
        </ul>
      )}
    </UserSettingsLayout>
  );
}

// UserDelete deletes the account of the viewer once they confirm their
// password.
export function UserDelete() {
  const { t } = useTranslation();
  return (
    <UserSettingsLayout active="delete">
      <h1 className="mb-4 text-xl font-semibold">{t("settings.delete_account")}</h1>
      <DeleteForm
        prompt={<Trans i18nKey="settings.delete_prompt" />}
        confirmLabel={t("settings.confirm_delete_account")}
        remove={deleteAccount}
        // The account is gone, so start over as a visitor.
        onDeleted={() => window.location.assign(subUrl("/"))}
      />
    </UserSettingsLayout>
  );
}
