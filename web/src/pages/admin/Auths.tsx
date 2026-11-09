import { useMutation } from "@tanstack/react-query";
import { useNavigate, useRouter } from "@tanstack/react-router";
import { type FormEvent, useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";

import { AdminLayout } from "@/components/AdminLayout";
import { ConfirmButton } from "@/components/ConfirmButton";
import { CheckboxField, FormAlert, FormField } from "@/components/FormField";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import {
  AUTH_SOURCE_TYPES,
  type AdminAuthSourceDetail,
  type AdminAuthSourceForm,
  LDAP_SECURITY_PROTOCOLS,
  SMTP_AUTH_TYPES,
  createAdminAuthSource,
  deleteAdminAuthSource,
  updateAdminAuthSource,
} from "@/lib/queries/admin";
import { FormError, formErrors } from "@/lib/queries/request";

const LDAP = 2;
const SMTP = 3;
const PAM = 4;
const DLDAP = 5;
const GITHUB = 6;

// FIELD_ORDER is the display order of the fields, so the first one the server
// rejects gets focus.
const FIELD_ORDER = [
  "type",
  "name",
  "securityProtocol",
  "host",
  "port",
  "bindDN",
  "bindPassword",
  "userBase",
  "userDN",
  "filter",
  "adminFilter",
  "attributeUsername",
  "attributeName",
  "attributeSurname",
  "attributeMail",
  "groupDN",
  "groupFilter",
  "groupMemberUID",
  "userUID",
  "smtpAuth",
  "smtpHost",
  "smtpPort",
  "allowedDomains",
  "pamServiceName",
  "githubAPIEndpoint",
] as const;

const NEW_SOURCE: AdminAuthSourceForm = {
  type: LDAP,
  name: "",
  isActive: true,
  isDefault: true,
  bindPassword: "",
  securityProtocol: 0,
  host: "",
  port: 0,
  bindDN: "",
  userBase: "",
  userDN: "",
  filter: "",
  adminFilter: "",
  attributeUsername: "",
  attributeName: "",
  attributeSurname: "",
  attributeMail: "",
  attributesInBind: false,
  groupEnabled: false,
  groupDN: "",
  groupFilter: "",
  groupMemberUID: "",
  userUID: "",
  smtpAuth: "PLAIN",
  smtpHost: "",
  smtpPort: 0,
  allowedDomains: "",
  tls: false,
  pamServiceName: "",
  githubAPIEndpoint: "",
  skipVerify: false,
};

type TextKey = {
  [K in keyof AdminAuthSourceForm]: AdminAuthSourceForm[K] extends string ? K : never;
}[keyof AdminAuthSourceForm];

// AuthSourceForm creates an authentication source, or changes one when
// `source` is given. The fields shown depend on the type of the source.
function AuthSourceForm({ source }: { source?: AdminAuthSourceDetail }) {
  const { t } = useTranslation();
  const router = useRouter();
  const navigate = useNavigate();
  const [form, setForm] = useState<AdminAuthSourceForm>(() => {
    if (!source) return NEW_SOURCE;
    // Copy only the form fields, so read-only details such as the ID stay out
    // of update requests.
    const fields = Object.keys(NEW_SOURCE).map((key) => [key, source[key as keyof AdminAuthSourceDetail]]);
    return { ...(Object.fromEntries(fields) as AdminAuthSourceForm), bindPassword: "" };
  });
  const save = useMutation({
    mutationFn: async () => {
      if (!source) return createAdminAuthSource(form);
      await updateAdminAuthSource(source.id, form);
      return undefined;
    },
    onSuccess: async (created) => {
      if (created) {
        toast.success(t("admin.auths.new_success").replace(/%(?:\[1\])?s/, created.name));
        await navigate({ to: "/admin/auths" });
        return;
      }
      toast.success(t("admin.auths.update_success"));
      setForm({ ...form, bindPassword: "" });
      await router.invalidate();
    },
    onError: (err) => {
      const fields = err instanceof FormError ? err.fields : {};
      const first = FIELD_ORDER.find((f) => f in fields);
      if (first) requestAnimationFrame(() => document.getElementById(first)?.focus());
    },
  });
  const { fields, formError } = formErrors(save.error);

  const isLDAP = form.type === LDAP || form.type === DLDAP;
  const usesTLS = form.type === SMTP || form.type === GITHUB || (isLDAP && form.securityProtocol > 0);

  const onSubmit = (e: FormEvent) => {
    e.preventDefault();
    save.mutate();
  };
  const textField = (key: TextKey, label: string, opts: { helper?: string; placeholder?: string } = {}) => (
    <FormField id={key} label={label} helper={opts.helper} error={fields[key]} invalid={key in fields}>
      <Input
        id={key}
        name={key}
        autoComplete="off"
        placeholder={opts.placeholder}
        value={form[key]}
        onChange={(e) => setForm({ ...form, [key]: e.target.value })}
      />
    </FormField>
  );
  const portField = (key: "port" | "smtpPort", label: string, placeholder: string) => (
    <FormField id={key} label={label} error={fields[key]} invalid={key in fields}>
      <Input
        id={key}
        name={key}
        type="number"
        inputMode="numeric"
        min={1}
        max={65535}
        className="max-w-40"
        placeholder={placeholder}
        value={form[key] || ""}
        onChange={(e) => setForm({ ...form, [key]: Number(e.target.value) })}
      />
    </FormField>
  );
  const flag = (key: "attributesInBind" | "groupEnabled" | "tls" | "skipVerify" | "isActive" | "isDefault") => ({
    id: key,
    checked: form[key],
    onChange: (checked: boolean) => setForm({ ...form, [key]: checked }),
  });

  return (
    <form onSubmit={onSubmit} noValidate>
      <fieldset disabled={save.isPending} className="contents">
        <FormAlert message={formError} />
        <div className="flex flex-col gap-5">
          {source ? (
            <div className="flex flex-col gap-1.5 text-sm">
              <span className="font-medium">{t("admin.auths.auth_type")}</span>
              <span className="text-(--color-muted-foreground)">{source.typeName}</span>
            </div>
          ) : (
            <FormField id="type" label={t("admin.auths.auth_type")} error={fields.type} invalid={"type" in fields}>
              <Select value={String(form.type)} onValueChange={(v) => setForm({ ...form, type: Number(v) })}>
                <SelectTrigger id="type">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {AUTH_SOURCE_TYPES.map((s) => (
                    <SelectItem key={s.type} value={String(s.type)}>
                      {s.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </FormField>
          )}
          {textField("name", t("admin.auths.auth_name"))}

          {isLDAP ? (
            <>
              <FormField
                id="securityProtocol"
                label={t("admin.auths.security_protocol")}
                error={fields.securityProtocol}
                invalid={"securityProtocol" in fields}
              >
                <Select
                  value={String(form.securityProtocol)}
                  onValueChange={(v) => setForm({ ...form, securityProtocol: Number(v) })}
                >
                  <SelectTrigger id="securityProtocol">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {LDAP_SECURITY_PROTOCOLS.map((p) => (
                      <SelectItem key={p.value} value={String(p.value)}>
                        {p.name}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </FormField>
              {textField("host", t("admin.auths.host"), { placeholder: "mydomain.com" })}
              {portField("port", t("admin.auths.port"), "636")}
              {form.type === LDAP ? (
                <>
                  {textField("bindDN", t("admin.auths.bind_dn"), {
                    helper: t("admin.auths.bind_dn_helper"),
                    placeholder: "cn=Search,dc=mydomain,dc=com",
                  })}
                  <FormField
                    id="bindPassword"
                    label={t("admin.auths.bind_password")}
                    helper={
                      source?.hasBindPassword
                        ? `${t("admin.users.password_helper")} ${t("admin.auths.bind_password_helper")}`
                        : t("admin.auths.bind_password_helper")
                    }
                    error={fields.bindPassword}
                    invalid={"bindPassword" in fields}
                  >
                    <Input
                      id="bindPassword"
                      name="bindPassword"
                      type="password"
                      autoComplete="new-password"
                      value={form.bindPassword}
                      onChange={(e) => setForm({ ...form, bindPassword: e.target.value })}
                    />
                  </FormField>
                  {textField("userBase", t("admin.auths.user_base"), { placeholder: "ou=Users,dc=mydomain,dc=com" })}
                </>
              ) : (
                textField("userDN", t("admin.auths.user_dn"), { placeholder: "uid=%s,ou=Users,dc=mydomain,dc=com" })
              )}
              {textField("filter", t("admin.auths.filter"), { placeholder: "(&(objectClass=posixAccount)(uid=%s))" })}
              {textField("adminFilter", t("admin.auths.admin_filter"))}
              {textField("attributeUsername", t("admin.auths.attribute_username"), {
                helper: t("admin.auths.attribute_username_placeholder"),
              })}
              {textField("attributeName", t("admin.auths.attribute_name"))}
              {textField("attributeSurname", t("admin.auths.attribute_surname"))}
              {textField("attributeMail", t("admin.auths.attribute_mail"), { placeholder: "mail" })}
              <CheckboxField label={t("admin.auths.verify_group_membership")} {...flag("groupEnabled")} />
              {form.groupEnabled ? (
                <div className="flex flex-col gap-5 border-l-2 border-(--color-border) pl-4">
                  {textField("groupDN", t("admin.auths.group_search_base_dn"), {
                    placeholder: "ou=group,dc=mydomain,dc=com",
                  })}
                  {textField("groupFilter", t("admin.auths.group_filter"), {
                    placeholder: "(|(cn=gogs_users)(cn=admins))",
                  })}
                  {textField("groupMemberUID", t("admin.auths.group_attribute_contain_user_list"), {
                    placeholder: "memberUid",
                  })}
                  {textField("userUID", t("admin.auths.user_attribute_listed_in_group"), { placeholder: "uid" })}
                </div>
              ) : null}
              {form.type === LDAP ? (
                <CheckboxField label={t("admin.auths.attributes_in_bind")} {...flag("attributesInBind")} />
              ) : null}
            </>
          ) : null}

          {form.type === SMTP ? (
            <>
              <FormField
                id="smtpAuth"
                label={t("admin.auths.smtp_auth")}
                error={fields.smtpAuth}
                invalid={"smtpAuth" in fields}
              >
                <Select value={form.smtpAuth} onValueChange={(v) => setForm({ ...form, smtpAuth: v })}>
                  <SelectTrigger id="smtpAuth">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {SMTP_AUTH_TYPES.map((a) => (
                      <SelectItem key={a} value={a}>
                        {a}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </FormField>
              {textField("smtpHost", t("admin.auths.smtphost"), { placeholder: "smtp.mydomain.com" })}
              {portField("smtpPort", t("admin.auths.smtpport"), "587")}
              {textField("allowedDomains", t("admin.auths.allowed_domains"), {
                helper: t("admin.auths.allowed_domains_helper"),
              })}
              <CheckboxField label={t("admin.auths.enable_tls")} {...flag("tls")} />
            </>
          ) : null}

          {form.type === PAM
            ? textField("pamServiceName", t("admin.auths.pam_service_name"), { placeholder: "system-auth" })
            : null}

          {form.type === GITHUB
            ? textField("githubAPIEndpoint", t("admin.auths.github_api_endpoint"), {
                placeholder: "https://api.github.com/",
              })
            : null}

          <div className="flex flex-col gap-3 border-t border-(--color-border) pt-5">
            {usesTLS ? <CheckboxField label={t("admin.auths.skip_tls_verify")} {...flag("skipVerify")} /> : null}
            <CheckboxField label={t("admin.auths.activated")} {...flag("isActive")} />
            <CheckboxField label={t("admin.auths.default_auth")} {...flag("isDefault")} />
          </div>

          <div className="flex flex-wrap items-center gap-2 border-t border-(--color-border) pt-5">
            <Button type="submit" variant="outline">
              {source ? t("admin.auths.update") : t("admin.auths.new")}
            </Button>
            {source && !source.fromFile ? (
              <ConfirmButton
                label={t("admin.auths.delete")}
                title={t("admin.auths.delete_auth_title")}
                description={t("admin.auths.delete_auth_desc")}
                align="start"
                action={() => deleteAdminAuthSource(source.id)}
                onDone={async () => {
                  toast.success(t("admin.auths.deletion_success"));
                  await navigate({ to: "/admin/auths" });
                }}
              />
            ) : null}
          </div>
        </div>
      </fieldset>
    </form>
  );
}

// AdminAuthSourceNew adds an authentication source.
export function AdminAuthSourceNew() {
  const { t } = useTranslation();
  return (
    <AdminLayout active="auths">
      <h1 className="mb-4 text-xl font-semibold">{t("admin.auths.new")}</h1>
      <AuthSourceForm />
    </AdminLayout>
  );
}

// AdminAuthSourceEdit changes or deletes an authentication source.
export function AdminAuthSourceEdit({ source }: { source: AdminAuthSourceDetail }) {
  const { t } = useTranslation();
  return (
    <AdminLayout active="auths">
      <h1 className="mb-4 text-xl font-semibold">{t("admin.auths.edit")}</h1>
      <AuthSourceForm source={source} />
    </AdminLayout>
  );
}
