import { useMutation } from "@tanstack/react-query";
import { Link, useNavigate, useRouter } from "@tanstack/react-router";
import { Check, Copy, KeyRound, X } from "lucide-react";
import { type FormEvent, type ReactNode, useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";

import { ConfirmButton } from "@/components/ConfirmButton";
import { FormAlert, FormField } from "@/components/FormField";
import { RichText } from "@/components/RichText";
import { UserSettingsLayout } from "@/components/UserSettingsLayout";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { webContext } from "@/lib/context";
import { formErrors } from "@/lib/queries/request";
import {
  type Credential,
  type RecoveryCode,
  type SSHKey,
  type TwoFactorEnrollment,
  type UserEmail,
  addSSHKey,
  addUserEmail,
  createAccessToken,
  deleteAccessToken,
  deleteSSHKey,
  deleteUserEmail,
  disableTwoFactor,
  enableTwoFactor,
  regenerateRecoveryCodes,
  setPrimaryEmail,
} from "@/lib/queries/settings";
import { subUrl } from "@/lib/url";
import { useUserInfo } from "@/lib/use-user-info";

const DATE_FMT = new Intl.DateTimeFormat(webContext.lang, { year: "numeric", month: "short", day: "2-digit" });

const ROW_CLASS = "flex flex-wrap items-center gap-x-3 gap-y-2 border-b border-(--color-border) py-3";
const TEXTAREA_CLASS =
  "block w-full rounded-md border border-(--color-input) bg-(--color-background) px-3 py-2 font-mono text-sm text-(--color-foreground) outline-none transition-colors focus-visible:border-(--color-ring) focus-visible:ring-1 focus-visible:ring-(--color-ring) disabled:cursor-not-allowed disabled:opacity-60 aria-invalid:border-(--color-destructive)";

// fill puts values into a translated message in the order its Go
// placeholders, such as `%s`, `%d` or `%[1]s`, appear.
function fill(message: string, ...values: (string | number)[]) {
  let i = 0;
  return message.replace(/%(?:\[\d+\])?[sdv]/g, () => String(values[i++] ?? ""));
}

// focusOnError moves focus to the first field of a failed form that has an
// error.
function focusOnError(err: Error, order: readonly string[]) {
  const { fields } = formErrors(err);
  const first = order.find((f) => f in fields);
  if (first) requestAnimationFrame(() => document.getElementById(first)?.focus());
}

function SectionHeading({ title, description }: { title: string; description?: ReactNode }) {
  return (
    <>
      <h1 className={description ? "mb-2 text-xl font-semibold" : "mb-4 text-xl font-semibold"}>{title}</h1>
      {description ? <p className="mb-4 text-sm text-(--color-muted-foreground)">{description}</p> : null}
    </>
  );
}

// UserEmailSettings lists the email addresses of the viewer, adds new ones,
// and changes which one is primary. activated is true right after the viewer
// confirmed an address through the link sent to it.
export function UserEmailSettings({ emails, activated }: { emails: UserEmail[]; activated: boolean }) {
  const { t } = useTranslation();
  const router = useRouter();
  const navigate = useNavigate();
  const [email, setEmail] = useState("");

  useEffect(() => {
    if (!activated) return;
    toast.success(t("settings.add_email_success"));
    void navigate({ to: "/user/settings/email", search: {}, replace: true });
  }, [activated, navigate, t]);

  const add = useMutation({
    mutationFn: () => addUserEmail(email),
    onSuccess: async (added) => {
      if (added.confirmationSent) {
        toast.info(fill(t("settings.add_email_confirmation_sent"), email, added.codeLifetimeHours ?? 0));
      } else {
        toast.success(t("settings.add_email_success"));
      }
      setEmail("");
      await router.invalidate();
    },
    onError: (err) => focusOnError(err, ["email"]),
  });
  const { fields, formError } = formErrors(add.error);

  const onSubmit = (e: FormEvent) => {
    e.preventDefault();
    add.mutate();
  };

  return (
    <UserSettingsLayout active="email">
      <SectionHeading title={t("settings.manage_emails")} description={t("settings.email_desc")} />
      <ul>
        {emails.map((item) => (
          <EmailRow key={item.email} item={item} />
        ))}
      </ul>
      <form onSubmit={onSubmit} noValidate className="mt-8">
        <fieldset disabled={add.isPending} className="contents">
          <FormAlert message={formError} />
          <div className="flex flex-col gap-3">
            <FormField id="email" label={t("settings.add_new_email")} error={fields.email} invalid={"email" in fields}>
              <Input
                id="email"
                name="email"
                type="email"
                autoComplete="email"
                required
                value={email}
                onChange={(e) => setEmail(e.target.value)}
              />
            </FormField>
            <div>
              <Button type="submit" variant="outline">
                {t("settings.add_email")}
              </Button>
            </div>
          </div>
        </fieldset>
      </form>
    </UserSettingsLayout>
  );
}

function EmailRow({ item }: { item: UserEmail }) {
  const { t } = useTranslation();
  const router = useRouter();
  const primary = useMutation({
    mutationFn: () => setPrimaryEmail(item.email),
    onSuccess: () => router.invalidate(),
    onError: (err) => toast.error(err.message),
  });
  return (
    <li className={ROW_CLASS}>
      <span className="flex min-w-0 flex-1 basis-48 flex-wrap items-center gap-2">
        <span className="min-w-0 font-semibold wrap-anywhere">{item.email}</span>
        {item.isPrimary ? (
          <span className="rounded-full border border-(--color-border) px-2 py-0.5 text-xs text-(--color-muted-foreground)">
            {t("settings.primary")}
          </span>
        ) : null}
      </span>
      {item.isPrimary ? null : (
        <span className="flex shrink-0 flex-wrap gap-2">
          {item.isActivated ? (
            <Button
              variant="outline"
              size="sm"
              aria-label={`${t("settings.primary_email")} ${item.email}`}
              disabled={primary.isPending}
              onClick={() => primary.mutate()}
            >
              {t("settings.primary_email")}
            </Button>
          ) : null}
          <ConfirmButton
            label={t("settings.delete_email")}
            ariaLabel={`${t("settings.delete_email")} ${item.email}`}
            title={t("settings.email_deletion")}
            description={t("settings.email_deletion_desc")}
            size="sm"
            action={() => deleteUserEmail(item.email)}
            onDone={async () => {
              toast.success(t("settings.email_deletion_success"));
              await router.invalidate();
            }}
          />
        </span>
      )}
    </li>
  );
}

// CredentialRow shows an SSH key or an access token with when it was added
// and last used, and deletes it.
function CredentialRow({
  item,
  detail,
  recentLabel,
  deletion,
}: {
  item: Credential;
  detail?: string;
  recentLabel: string;
  deletion: { label: string; title: string; description: string; success: string; action: () => Promise<void> };
}) {
  const { t } = useTranslation();
  const router = useRouter();
  return (
    <li className={ROW_CLASS}>
      <span className="flex min-w-0 flex-1 basis-56 items-start gap-3">
        <KeyRound className="mt-0.5 size-5 shrink-0 text-(--color-muted-foreground)" aria-hidden />
        <span className="min-w-0">
          <span className="flex items-center gap-2">
            <span className="min-w-0 font-semibold wrap-anywhere">{item.name}</span>
            {item.isRecentlyUsed ? (
              <Tooltip>
                <TooltipTrigger asChild>
                  <span
                    role="img"
                    aria-label={recentLabel}
                    tabIndex={0}
                    className="size-2.5 shrink-0 rounded-full bg-(--color-success)"
                  />
                </TooltipTrigger>
                <TooltipContent>{recentLabel}</TooltipContent>
              </Tooltip>
            ) : null}
          </span>
          {detail ? (
            <span className="block font-mono text-xs wrap-anywhere text-(--color-muted-foreground)">{detail}</span>
          ) : null}
          <span className="mt-1 block text-xs text-(--color-muted-foreground)">
            {t("settings.add_on")} {DATE_FMT.format(new Date(item.createdAt))}
            {" · "}
            {item.lastUsedAt
              ? `${t("settings.last_used")} ${DATE_FMT.format(new Date(item.lastUsedAt))}`
              : t("settings.no_activity")}
          </span>
        </span>
      </span>
      <ConfirmButton
        label={deletion.label}
        ariaLabel={`${deletion.label} ${item.name}`}
        title={deletion.title}
        description={deletion.description}
        size="sm"
        action={deletion.action}
        onDone={async () => {
          toast.success(deletion.success);
          await router.invalidate();
        }}
      />
    </li>
  );
}

const SSH_KEY_FIELDS = ["title", "content"] as const;

// UserSSHKeySettings lists the SSH keys of the viewer and adds new ones.
export function UserSSHKeySettings({ keys }: { keys: SSHKey[] }) {
  const { t } = useTranslation();
  const router = useRouter();
  const [form, setForm] = useState({ title: "", content: "" });
  const add = useMutation({
    mutationFn: () => addSSHKey(form),
    onSuccess: async () => {
      toast.success(fill(t("settings.add_key_success"), form.title));
      setForm({ title: "", content: "" });
      await router.invalidate();
    },
    onError: (err) => focusOnError(err, SSH_KEY_FIELDS),
  });
  const { fields, formError } = formErrors(add.error);

  const onSubmit = (e: FormEvent) => {
    e.preventDefault();
    add.mutate();
  };

  return (
    <UserSettingsLayout active="ssh">
      <SectionHeading title={t("settings.manage_ssh_keys")} description={t("settings.ssh_desc")} />
      <ul>
        {keys.map((key) => (
          <CredentialRow
            key={key.id}
            item={key}
            detail={key.fingerprint}
            recentLabel={t("settings.key_state_desc")}
            deletion={{
              label: t("settings.delete_key"),
              title: t("settings.ssh_key_deletion"),
              description: t("settings.ssh_key_deletion_desc"),
              success: t("settings.ssh_key_deletion_success"),
              action: () => deleteSSHKey(key.id),
            }}
          />
        ))}
      </ul>
      <form onSubmit={onSubmit} noValidate className="mt-8">
        <fieldset disabled={add.isPending} className="contents">
          <h2 className="mb-4 text-base font-semibold">{t("settings.add_new_key")}</h2>
          <FormAlert message={formError} />
          <div className="flex flex-col gap-5">
            <FormField id="title" label={t("settings.key_name")} error={fields.title} invalid={"title" in fields}>
              <Input
                id="title"
                name="title"
                autoComplete="off"
                required
                value={form.title}
                onChange={(e) => setForm({ ...form, title: e.target.value })}
              />
            </FormField>
            <FormField
              id="content"
              label={t("settings.key_content")}
              error={fields.content}
              invalid={"content" in fields}
            >
              <textarea
                id="content"
                name="content"
                rows={5}
                required
                spellCheck={false}
                placeholder="ssh-ed25519 AAAA…"
                className={TEXTAREA_CLASS}
                value={form.content}
                onChange={(e) => setForm({ ...form, content: e.target.value })}
              />
            </FormField>
            <div>
              <Button type="submit" variant="outline">
                {t("settings.add_key")}
              </Button>
            </div>
          </div>
        </fieldset>
      </form>
      <p className="mt-8 text-sm text-(--color-muted-foreground)">
        <RichText
          text={t("settings.ssh_helper")}
          links={["https://help.github.com/articles/generating-ssh-keys", "https://help.github.com/ssh-issues/"]}
        />
      </p>
    </UserSettingsLayout>
  );
}

// UserSecuritySettings shows whether two-factor authentication is on, and
// turns it on or off.
export function UserSecuritySettings({ enabled }: { enabled: boolean }) {
  const { t } = useTranslation();
  const router = useRouter();
  const user = useUserInfo();
  const cloneURL = new URL(subUrl(`/${user?.username ?? "user"}/repo.git`), window.location.origin);
  const cloneExample = `${cloneURL.protocol}//{token}:x-access-token@${cloneURL.host}${cloneURL.pathname}`;
  return (
    <UserSettingsLayout active="security">
      <SectionHeading title={t("settings.two_factor")} />
      <div className="flex flex-wrap items-center justify-between gap-3 rounded-md border border-(--color-border) px-4 py-3">
        <p className="flex items-center gap-2 text-sm">
          <span className="font-semibold">{t("settings.two_factor_status")}</span>
          {enabled ? (
            <span className="inline-flex items-center gap-1 font-semibold text-(--color-success)">
              <Check className="size-4" aria-hidden />
              {t("settings.two_factor_on")}
            </span>
          ) : (
            <span className="inline-flex items-center gap-1 font-semibold text-(--color-destructive)">
              <X className="size-4" aria-hidden />
              {t("settings.two_factor_off")}
            </span>
          )}
        </p>
        {enabled ? (
          <ConfirmButton
            label={t("settings.two_factor_disable")}
            title={t("settings.two_factor_disable_title")}
            description={t("settings.two_factor_disable_desc")}
            size="sm"
            action={disableTwoFactor}
            onDone={async () => {
              toast.success(t("settings.two_factor_disable_success"));
              await router.invalidate();
            }}
          />
        ) : (
          <Button asChild variant="outline" size="sm">
            <Link to="/user/settings/security/two_factor_enable">{t("settings.two_factor_enable")}</Link>
          </Button>
        )}
      </div>
      {enabled ? (
        <div className="mt-6 flex flex-col gap-3 text-sm">
          <p>
            <RichText
              text={t("settings.two_factor_view_recovery_codes")}
              links={["/user/settings/security/two_factor_recovery_codes"]}
            />
          </p>
          <p>
            <RichText
              text={t("settings.two_factor_http")}
              links={["/user/settings/applications"]}
              code={cloneExample}
            />
          </p>
          <p className="text-(--color-muted-foreground)">{t("settings.access_token_tips")}</p>
        </div>
      ) : null}
    </UserSettingsLayout>
  );
}

// UserTwoFactorEnable enrolls the viewer in two-factor authentication: they
// add the secret to an authentication app and confirm it with a passcode.
export function UserTwoFactorEnable({ enrollment }: { enrollment: TwoFactorEnrollment }) {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const [passcode, setPasscode] = useState("");
  const inputRef = useRef<HTMLInputElement>(null);
  const enable = useMutation({
    mutationFn: () => enableTwoFactor(passcode),
    onSuccess: async () => {
      toast.success(t("settings.two_factor_enable_success"));
      await navigate({ to: "/user/settings/security/two_factor_recovery_codes" });
    },
    onError: () => requestAnimationFrame(() => inputRef.current?.focus()),
  });
  const { fields, formError } = formErrors(enable.error);

  const onSubmit = (e: FormEvent) => {
    e.preventDefault();
    enable.mutate();
  };

  return (
    <UserSettingsLayout active="security">
      <SectionHeading title={t("settings.two_factor_enable_title")} />
      <p className="text-sm">{t("settings.two_factor_scan_qr")}</p>
      <img
        src={enrollment.qrCodeURL}
        alt={enrollment.secret}
        width={240}
        height={240}
        className="my-4 size-48 rounded-md border border-(--color-border) bg-white p-2"
      />
      <p className="text-sm">
        {t("settings.two_factor_or_enter_secret")}{" "}
        <code className="rounded bg-(--color-surface) px-1 font-mono font-semibold wrap-anywhere select-all">
          {enrollment.secret}
        </code>
      </p>
      <form onSubmit={onSubmit} noValidate className="mt-6">
        <fieldset disabled={enable.isPending} className="contents">
          <FormAlert message={formError} />
          <div className="flex flex-col gap-3">
            <FormField
              id="passcode"
              label={t("settings.two_factor_then_enter_passcode")}
              error={fields.passcode}
              invalid={"passcode" in fields}
            >
              <Input
                ref={inputRef}
                id="passcode"
                name="passcode"
                inputMode="numeric"
                autoComplete="one-time-code"
                required
                className="max-w-48 font-mono"
                value={passcode}
                onChange={(e) => setPasscode(e.target.value.trim())}
              />
            </FormField>
            <div>
              <Button type="submit" variant="outline">
                {t("settings.two_factor_verify")}
              </Button>
            </div>
          </div>
        </fieldset>
      </form>
    </UserSettingsLayout>
  );
}

// UserRecoveryCodes shows the recovery codes of the viewer, who can sign in
// with one when they lose their authentication app, and replaces them.
export function UserRecoveryCodes({ codes: initialCodes }: { codes: RecoveryCode[] }) {
  const { t } = useTranslation();
  const [codes, setCodes] = useState(initialCodes);
  const regenerate = useMutation({
    mutationFn: regenerateRecoveryCodes,
    onSuccess: (next) => {
      setCodes(next);
      toast.success(t("settings.two_factor_regenerate_recovery_codes_success"));
    },
    onError: (err) => toast.error(err.message),
  });
  return (
    <UserSettingsLayout active="security">
      <SectionHeading title={t("settings.two_factor_recovery_codes_title")} />
      <p className="text-sm">
        <RichText text={t("settings.two_factor_recovery_codes_desc")} />
      </p>
      <ul className="my-4 grid grid-cols-1 gap-2 rounded-md border border-(--color-border) p-4 font-mono text-sm min-[360px]:grid-cols-2">
        {codes.map((code) =>
          code.isUsed ? (
            <li key={code.code} className="flex items-baseline gap-2">
              <del className="text-(--color-muted-foreground)">{code.code}</del>
              <span className="font-sans text-xs text-(--color-muted-foreground)">
                {t("settings.two_factor_recovery_code_used")}
              </span>
            </li>
          ) : (
            <li key={code.code}>{code.code}</li>
          ),
        )}
      </ul>
      <Button variant="outline" disabled={regenerate.isPending} onClick={() => regenerate.mutate()}>
        {t("settings.two_factor_regenerate_recovery_codes")}
      </Button>
    </UserSettingsLayout>
  );
}

// UserTokenSettings lists the personal access tokens of the viewer and
// generates new ones, which are only shown once.
export function UserTokenSettings({ tokens }: { tokens: Credential[] }) {
  const { t } = useTranslation();
  const router = useRouter();
  const [name, setName] = useState("");
  const [created, setCreated] = useState<string | null>(null);
  const inputRef = useRef<HTMLInputElement>(null);
  const generate = useMutation({
    mutationFn: () => createAccessToken(name),
    onSuccess: async (token) => {
      setCreated(token);
      setName("");
      await router.invalidate();
    },
    onError: () => requestAnimationFrame(() => inputRef.current?.focus()),
  });
  const { fields, formError } = formErrors(generate.error);

  const onSubmit = (e: FormEvent) => {
    e.preventDefault();
    generate.mutate();
  };

  return (
    <UserSettingsLayout active="applications">
      <SectionHeading title={t("settings.manage_access_token")} description={t("settings.tokens_desc")} />
      {created ? <NewToken token={created} /> : null}
      <ul>
        {tokens.map((token) => (
          <CredentialRow
            key={token.id}
            item={token}
            recentLabel={t("settings.token_state_desc")}
            deletion={{
              label: t("settings.delete_token"),
              title: t("settings.access_token_deletion"),
              description: t("settings.access_token_deletion_desc"),
              success: t("settings.delete_token_success"),
              action: () => deleteAccessToken(token.id),
            }}
          />
        ))}
      </ul>
      <form onSubmit={onSubmit} noValidate className="mt-8">
        <fieldset disabled={generate.isPending} className="contents">
          <h2 className="mb-2 text-base font-semibold">{t("settings.generate_new_token")}</h2>
          <p className="mb-4 text-sm text-(--color-muted-foreground)">{t("settings.new_token_desc")}</p>
          <FormAlert message={formError} />
          <div className="flex flex-col gap-3">
            <FormField id="name" label={t("settings.token_name")} error={fields.name} invalid={"name" in fields}>
              <Input
                ref={inputRef}
                id="name"
                name="name"
                autoComplete="off"
                required
                value={name}
                onChange={(e) => setName(e.target.value)}
              />
            </FormField>
            <div>
              <Button type="submit" variant="outline">
                {t("settings.generate_token")}
              </Button>
            </div>
          </div>
        </fieldset>
      </form>
      <p className="mt-8 text-sm text-(--color-muted-foreground)">{t("settings.access_token_tips")}</p>
    </UserSettingsLayout>
  );
}

// NewToken shows a token right after it was generated, the only time it can
// be copied.
function NewToken({ token }: { token: string }) {
  const { t } = useTranslation();
  const [copied, setCopied] = useState(false);
  const copy = async () => {
    try {
      await navigator.clipboard.writeText(token);
      setCopied(true);
      window.setTimeout(() => setCopied(false), 1500);
    } catch {
      toast.error(t("copy_failed"));
    }
  };
  return (
    <div
      role="status"
      className="mb-6 rounded-md border border-(--color-success) bg-(--color-success)/10 px-4 py-3 text-sm"
    >
      <p>{t("settings.generate_token_succees")}</p>
      <div className="mt-3 flex flex-wrap items-center gap-2">
        <code className="min-w-0 flex-1 basis-48 rounded bg-(--color-background) px-2 py-1 font-mono wrap-anywhere select-all">
          {token}
        </code>
        <Button type="button" variant="outline" size="sm" onClick={() => void copy()}>
          <Copy className="size-3.5" aria-hidden />
          {copied ? t("repo.copy_link_success") : t("repo.copy_link")}
        </Button>
      </div>
    </div>
  );
}
