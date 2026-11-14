import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Link, useRouter } from "@tanstack/react-router";
import { type FormEvent, type ReactNode, useRef, useState } from "react";
import { Trans, useTranslation } from "react-i18next";
import { toast } from "sonner";

import { AvatarForm } from "@/components/AvatarForm";
import { ConfirmButton } from "@/components/ConfirmButton";
import { DeleteForm } from "@/components/DeleteForm";
import { FormField } from "@/components/FormField";
import { SettingsLayout, SettingsNavItem } from "@/components/SettingsLayout";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { usePageTitle } from "@/lib/page-title";
import { removeOrgMember } from "@/lib/queries/org";
import { FormError } from "@/lib/queries/request";
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
  setGravatarEmail,
  updateUserProfile,
  uploadUserAvatar,
} from "@/lib/queries/settings";
import { userProfileQuery } from "@/lib/queries/user";
import { subUrl } from "@/lib/url";
import { useUserInfo } from "@/lib/use-user-info";
import { RepoIcon } from "@/pages/user/Dashboard";

type SettingsPage = "profile" | "avatar" | "password" | "repositories" | "organizations" | "delete";

// NAV lists the settings pages in the order of the server-rendered menu. The
// ones without a page are still served by the server-rendered pages.
const NAV: { path: string; label: string; page?: SettingsPage }[] = [
  { path: "", label: "settings.profile", page: "profile" },
  { path: "/avatar", label: "settings.avatar", page: "avatar" },
  { path: "/password", label: "settings.password", page: "password" },
  { path: "/email", label: "settings.emails" },
  { path: "/ssh", label: "settings.ssh_keys" },
  { path: "/security", label: "settings.security" },
  { path: "/repositories", label: "settings.repos", page: "repositories" },
  { path: "/organizations", label: "settings.orgs", page: "organizations" },
  { path: "/applications", label: "settings.applications" },
  { path: "/delete", label: "settings.delete", page: "delete" },
];

const ALERT_CLASS =
  "mb-4 rounded-md border border-(--color-destructive) bg-(--color-destructive)/10 px-3 py-2 text-sm text-(--color-destructive)";

// UserSettingsLayout heads the settings pages of the viewer, with links
// between them.
function UserSettingsLayout({ active, children }: { active: SettingsPage; children: ReactNode }) {
  const { t } = useTranslation();
  const current = t(NAV.find((item) => item.page === active)?.label ?? "settings");
  usePageTitle(current);
  return (
    <main className="flex-1">
      <SettingsLayout
        label={t("settings")}
        current={current}
        nav={NAV.map((item) => (
          <SettingsNavItem key={item.path} active={item.page === active} destructive={item.page === "delete"}>
            {item.page ? (
              <Link to={`/user/settings${item.path}`} activeOptions={{ exact: true }}>
                {t(item.label)}
              </Link>
            ) : (
              <a href={subUrl(`/user/settings${item.path}`)}>{t(item.label)}</a>
            )}
          </SettingsNavItem>
        ))}
      >
        {children}
      </SettingsLayout>
    </main>
  );
}

// formErrors returns the messages for the fields of a failed form, and the
// message for the whole form when no field is to blame.
function formErrors(error: Error | null) {
  const fields = error instanceof FormError ? error.fields : {};
  const formError = error && Object.keys(fields).length === 0 ? error.message : null;
  return { fields, formError };
}

const PROFILE_FIELDS = ["name", "fullName", "website", "location"] as const;

// UserProfileSettings changes the username and the public profile of the
// viewer.
export function UserProfileSettings({ settings }: { settings: UserSettingsData }) {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const router = useRouter();
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
          {formError ? (
            <div role="alert" className={ALERT_CLASS}>
              {formError}
            </div>
          ) : null}
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
              <Button type="submit" variant="outline">
                {t("settings.update_profile")}
              </Button>
            </div>
          </div>
        </fieldset>
      </form>
    </UserSettingsLayout>
  );
}

// The navigation bar shows the avatar too, so load the page again after a
// change.
const reloadPage = () => window.location.reload();

// UserAvatarSettings uploads or deletes the avatar of the viewer, or looks it
// up by email on sites that use Gravatar.
export function UserAvatarSettings({ settings }: { settings: UserSettingsData }) {
  const { t } = useTranslation();
  return (
    <UserSettingsLayout active="avatar">
      <h1 className="mb-4 text-xl font-semibold">{t("settings.avatar")}</h1>
      <AvatarForm
        avatarURL={settings.avatarURL}
        hasCustomAvatar={settings.hasCustomAvatar}
        upload={uploadUserAvatar}
        remove={deleteUserAvatar}
        onChanged={reloadPage}
      />
      {settings.avatarEmail !== undefined ? <GravatarForm email={settings.avatarEmail} /> : null}
    </UserSettingsLayout>
  );
}

function GravatarForm({ email: initialEmail }: { email: string }) {
  const { t } = useTranslation();
  const [email, setEmail] = useState(initialEmail);
  const inputRef = useRef<HTMLInputElement>(null);
  const save = useMutation({
    mutationFn: () => setGravatarEmail(email),
    onSuccess: reloadPage,
    onError: () => requestAnimationFrame(() => inputRef.current?.focus()),
  });
  const { fields, formError } = formErrors(save.error);

  const onSubmit = (e: FormEvent) => {
    e.preventDefault();
    save.mutate();
  };

  return (
    <form onSubmit={onSubmit} noValidate className="mt-8 border-t border-(--color-border) pt-6">
      <fieldset disabled={save.isPending} className="contents">
        {formError ? (
          <div role="alert" className={ALERT_CLASS}>
            {formError}
          </div>
        ) : null}
        <div className="flex flex-col gap-3">
          <FormField
            id="avatarEmail"
            label={t("settings.avatar_email")}
            error={fields.email}
            invalid={"email" in fields}
          >
            <Input
              ref={inputRef}
              id="avatarEmail"
              name="email"
              type="email"
              autoComplete="email"
              required
              value={email}
              onChange={(e) => setEmail(e.target.value)}
            />
          </FormField>
          <div>
            <Button type="submit" variant="outline" size="sm">
              {t("settings.lookup_avatar_by_mail")}
            </Button>
          </div>
        </div>
      </fieldset>
    </form>
  );
}

const PASSWORD_FIELDS = ["oldPassword", "password", "retype"] as const;
const EMPTY_PASSWORD_FORM: UserPasswordForm = { oldPassword: "", password: "", retype: "" };

// UserPasswordSettings changes the password of local users.
export function UserPasswordSettings({ isLocal }: { isLocal: boolean }) {
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
    <UserSettingsLayout active="password">
      <h1 className="mb-4 text-xl font-semibold">{t("settings.change_password")}</h1>
      {isLocal ? (
        <form onSubmit={onSubmit} noValidate>
          <fieldset disabled={save.isPending} className="contents">
            {formError ? (
              <div role="alert" className={ALERT_CLASS}>
                {formError}
              </div>
            ) : null}
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
                <Button type="submit" variant="outline">
                  {t("settings.change_password")}
                </Button>
              </div>
            </div>
          </fieldset>
        </form>
      ) : (
        <p className="rounded-md border border-(--color-border) bg-(--color-surface) px-3 py-2 text-sm">
          {t("settings.password_change_disabled")}
        </p>
      )}
    </UserSettingsLayout>
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
                <a
                  href={subUrl(`/${fullName}`)}
                  className="min-w-0 font-semibold wrap-anywhere text-(--color-primary) hover:underline"
                >
                  {fullName}
                </a>
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
                    toast.success(t("settings.repos.leave_success").replace(/%(?:\[1\])?s/, fullName));
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
