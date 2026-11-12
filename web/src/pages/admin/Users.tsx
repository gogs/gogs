import { useMutation } from "@tanstack/react-query";
import { useNavigate, useRouter } from "@tanstack/react-router";
import { type FormEvent, type ReactNode, useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";

import { AdminLayout } from "@/components/AdminLayout";
import { ConfirmButton } from "@/components/ConfirmButton";
import { CheckboxField, FormAlert, FormField } from "@/components/FormField";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { passwordHint } from "@/lib/password";
import {
  type AdminUserCreateForm,
  type AdminUserDetail,
  type AdminUserForm,
  type AdminUserOptions,
  createAdminUser,
  deleteAdminUser,
  updateAdminUser,
} from "@/lib/queries/admin";
import { FormError, formErrors } from "@/lib/queries/request";

// focusFirstError focuses the first field, in display order, that the server
// rejected.
function focusFirstError(err: Error, order: readonly string[]) {
  const fields = err instanceof FormError ? err.fields : {};
  const first = order.find((f) => f in fields);
  if (first) requestAnimationFrame(() => document.getElementById(first)?.focus());
}

// LoginSourceSelect picks how a user signs in: with a local password or
// through an authentication source.
function LoginSourceSelect({
  options,
  value,
  error,
  onChange,
}: {
  options: AdminUserOptions;
  value: number;
  error?: string | null;
  onChange: (value: number) => void;
}) {
  const { t } = useTranslation();
  return (
    <FormField id="loginSource" label={t("admin.users.auth_source")} error={error} invalid={error !== undefined}>
      <Select value={String(value)} onValueChange={(v) => onChange(Number(v))}>
        <SelectTrigger id="loginSource">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="0">{t("admin.users.local")}</SelectItem>
          {options.loginSources.map((s) => (
            <SelectItem key={s.id} value={String(s.id)}>
              {s.name}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </FormField>
  );
}

function FormActions({ children }: { children: ReactNode }) {
  return <div className="flex flex-wrap items-center gap-2 border-t border-(--color-border) pt-5">{children}</div>;
}

const CREATE_FIELDS = ["loginSource", "loginName", "name", "email", "password"] as const;

// AdminUserNew creates an account, which signs in with a password or through
// an authentication source.
export function AdminUserNew({ options }: { options: AdminUserOptions }) {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const [form, setForm] = useState<AdminUserCreateForm>({
    loginSource: 0,
    loginName: "",
    name: "",
    email: "",
    password: "",
    sendNotify: false,
  });
  const create = useMutation({
    // Only the fields of the chosen kind of account are sent.
    mutationFn: () => createAdminUser(form.loginSource === 0 ? { ...form, loginName: "" } : { ...form, password: "" }),
    onSuccess: async (user) => {
      toast.success(t("admin.users.new_success").replace(/%(?:\[1\])?s/, user.name));
      await navigate({ to: "/admin/users/$id", params: { id: String(user.id) } });
    },
    onError: (err) => focusFirstError(err, CREATE_FIELDS),
  });
  const { fields, formError } = formErrors(create.error);
  const isLocal = form.loginSource === 0;

  const onSubmit = (e: FormEvent) => {
    e.preventDefault();
    create.mutate();
  };
  const field = (key: "loginName" | "name" | "email" | "password") => ({
    id: key,
    name: key,
    value: form[key],
    onChange: (e: { target: { value: string } }) => setForm({ ...form, [key]: e.target.value }),
  });

  return (
    <AdminLayout active="users">
      <h1 className="mb-4 text-xl font-semibold">{t("admin.users.new_account")}</h1>
      <form onSubmit={onSubmit} noValidate>
        <fieldset disabled={create.isPending} className="contents">
          <FormAlert message={formError} />
          <div className="flex flex-col gap-5">
            <LoginSourceSelect
              options={options}
              value={form.loginSource}
              error={fields.loginSource}
              onChange={(loginSource) => setForm({ ...form, loginSource })}
            />
            {isLocal ? null : (
              <FormField
                id="loginName"
                label={t("admin.users.auth_login_name")}
                error={fields.loginName}
                invalid={"loginName" in fields}
              >
                <Input autoComplete="off" required {...field("loginName")} />
              </FormField>
            )}
            <FormField id="name" label={t("username")} error={fields.name} invalid={"name" in fields}>
              <Input autoComplete="off" required autoFocus {...field("name")} />
            </FormField>
            <FormField id="email" label={t("email")} error={fields.email} invalid={"email" in fields}>
              <Input type="email" autoComplete="off" required {...field("email")} />
            </FormField>
            {isLocal ? (
              <FormField
                id="password"
                label={t("password")}
                helper={passwordHint(t)}
                error={fields.password}
                invalid={"password" in fields}
              >
                <Input type="password" autoComplete="new-password" required {...field("password")} />
              </FormField>
            ) : null}
            {options.canSendEmail ? (
              <CheckboxField
                id="sendNotify"
                label={t("admin.users.send_register_notify")}
                checked={form.sendNotify}
                onChange={(sendNotify) => setForm({ ...form, sendNotify })}
              />
            ) : null}
            <FormActions>
              <Button type="submit" variant="outline">
                {t("admin.users.new_account")}
              </Button>
            </FormActions>
          </div>
        </fieldset>
      </form>
    </AdminLayout>
  );
}

const EDIT_FIELDS = [
  "loginSource",
  "loginName",
  "fullName",
  "email",
  "password",
  "website",
  "location",
  "maxRepoCreation",
] as const;

// AdminUserEdit changes the profile, the sign-in and the permissions of an
// account, or deletes it.
export function AdminUserEdit({ user, options }: { user: AdminUserDetail; options: AdminUserOptions }) {
  const { t } = useTranslation();
  const router = useRouter();
  const navigate = useNavigate();
  const [form, setForm] = useState<AdminUserForm>({
    loginSource: user.loginSource,
    loginName: user.loginName,
    fullName: user.fullName,
    email: user.email,
    password: "",
    website: user.website,
    location: user.location,
    maxRepoCreation: user.maxRepoCreation,
    isActive: user.isActive,
    isAdmin: user.isAdmin,
    allowGitHook: user.allowGitHook,
    allowImportLocal: user.allowImportLocal,
    prohibitLogin: user.prohibitLogin,
  });
  const save = useMutation({
    // Only local accounts sign in with a password.
    mutationFn: () => updateAdminUser(user.id, form.loginSource === 0 ? form : { ...form, password: "" }),
    onSuccess: async () => {
      toast.success(t("admin.users.update_profile_success"));
      setForm({ ...form, password: "" });
      await router.invalidate();
    },
    onError: (err) => focusFirstError(err, EDIT_FIELDS),
  });
  const { fields, formError } = formErrors(save.error);
  const isLocal = form.loginSource === 0;

  const onSubmit = (e: FormEvent) => {
    e.preventDefault();
    save.mutate();
  };
  const field = (key: "loginName" | "fullName" | "email" | "password" | "website" | "location") => ({
    id: key,
    name: key,
    value: form[key],
    onChange: (e: { target: { value: string } }) => setForm({ ...form, [key]: e.target.value }),
  });
  const flag = (key: "isActive" | "prohibitLogin" | "isAdmin" | "allowGitHook" | "allowImportLocal") => ({
    id: key,
    checked: form[key],
    onChange: (checked: boolean) => setForm({ ...form, [key]: checked }),
  });

  return (
    <AdminLayout active="users">
      <h1 className="mb-4 text-xl font-semibold">{t("admin.users.edit_account")}</h1>
      <div className="mb-5 flex min-w-0 items-center gap-3">
        <img
          src={user.avatarURL}
          alt=""
          className="size-10 shrink-0 rounded-full border border-(--color-border) object-cover"
        />
        <div className="min-w-0">
          <p className="font-semibold wrap-anywhere">{user.name}</p>
          <p className="text-xs text-(--color-muted-foreground) tabular-nums">ID {user.id}</p>
        </div>
      </div>
      <form onSubmit={onSubmit} noValidate>
        <fieldset disabled={save.isPending} className="contents">
          <FormAlert message={formError} />
          <div className="flex flex-col gap-5">
            <LoginSourceSelect
              options={options}
              value={form.loginSource}
              error={fields.loginSource}
              onChange={(loginSource) => setForm({ ...form, loginSource })}
            />
            {isLocal ? null : (
              <FormField
                id="loginName"
                label={t("admin.users.auth_login_name")}
                error={fields.loginName}
                invalid={"loginName" in fields}
              >
                <Input autoComplete="off" required {...field("loginName")} />
              </FormField>
            )}
            <FormField
              id="fullName"
              label={t("settings.full_name")}
              error={fields.fullName}
              invalid={"fullName" in fields}
            >
              <Input autoComplete="off" {...field("fullName")} />
            </FormField>
            <FormField id="email" label={t("email")} error={fields.email} invalid={"email" in fields}>
              <Input type="email" autoComplete="off" required {...field("email")} />
            </FormField>
            {isLocal ? (
              <FormField
                id="password"
                label={t("password")}
                helper={t("admin.users.password_helper")}
                error={fields.password}
                invalid={"password" in fields}
              >
                <Input type="password" autoComplete="new-password" {...field("password")} />
              </FormField>
            ) : null}
            <FormField id="website" label={t("settings.website")} error={fields.website} invalid={"website" in fields}>
              <Input type="url" autoComplete="off" {...field("website")} />
            </FormField>
            <FormField
              id="location"
              label={t("settings.location")}
              error={fields.location}
              invalid={"location" in fields}
            >
              <Input autoComplete="off" {...field("location")} />
            </FormField>
            <FormField
              id="maxRepoCreation"
              label={t("admin.users.max_repo_creation")}
              helper={t("admin.users.max_repo_creation_desc")}
              error={fields.maxRepoCreation}
              invalid={"maxRepoCreation" in fields}
            >
              <Input
                id="maxRepoCreation"
                name="maxRepoCreation"
                type="number"
                min={-1}
                className="max-w-40"
                value={form.maxRepoCreation}
                onChange={(e) => setForm({ ...form, maxRepoCreation: Number(e.target.value) })}
              />
            </FormField>
            <div className="flex flex-col gap-3 border-t border-(--color-border) pt-5">
              <CheckboxField label={t("admin.users.is_activated")} {...flag("isActive")} />
              <CheckboxField label={t("admin.users.prohibit_login")} {...flag("prohibitLogin")} />
              <CheckboxField label={t("admin.users.is_admin")} {...flag("isAdmin")} />
              <CheckboxField label={t("admin.users.allow_git_hook")} {...flag("allowGitHook")} />
              {options.localPathMigrationEnabled ? (
                <CheckboxField label={t("admin.users.allow_import_local")} {...flag("allowImportLocal")} />
              ) : null}
            </div>
            <FormActions>
              <Button type="submit" variant="outline">
                {t("admin.users.update_profile")}
              </Button>
              <ConfirmButton
                label={t("admin.users.delete_account")}
                title={t("settings.delete_account_title")}
                description={t("settings.delete_account_desc")}
                align="start"
                action={() => deleteAdminUser(user.id)}
                onDone={async () => {
                  toast.success(t("admin.users.deletion_success"));
                  await navigate({ to: "/admin/users" });
                }}
              />
            </FormActions>
          </div>
        </fieldset>
      </form>
    </AdminLayout>
  );
}
