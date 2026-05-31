import { getRouteApi, useNavigate } from "@tanstack/react-router";
import { useRef, useState } from "react";
import { Trans, useTranslation } from "react-i18next";

import { PasswordInput } from "@/components/PasswordInput";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { usePageTitle } from "@/lib/page-title";
import { subUrl } from "@/lib/url";

export interface ResetPasswordPage {
  code: string;
  emailEnabled: boolean;
  valid: boolean;
}

interface ResetPasswordResponse {
  hours?: number;
  resendLimited?: boolean;
}

interface ResetPasswordErrorResponse {
  error?: string;
  fields?: Record<string, string | null>;
}

const route = getRouteApi("/user/reset-password");

function FormActions({
  submitLabel,
  submitTabIndex,
  submitting,
  backLabel,
  backHref,
}: {
  submitLabel: string;
  submitTabIndex: number;
  submitting: boolean;
  backLabel: string;
  backHref: string;
}) {
  return (
    <div className="mt-2 flex flex-col gap-3">
      <Button type="submit" disabled={submitting} tabIndex={submitTabIndex} className="w-full">
        {submitLabel}
      </Button>
      <Button variant="link" size="inline" asChild className="self-center">
        <a
          href={backHref}
          tabIndex={submitting ? -1 : submitTabIndex + 1}
          aria-disabled={submitting || undefined}
          className={submitting ? "pointer-events-none opacity-50" : undefined}
          onClick={(e) => {
            if (submitting) e.preventDefault();
          }}
        >
          {backLabel}
        </a>
      </Button>
    </div>
  );
}

export function ResetPassword() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const { code, emailEnabled, valid } = route.useLoaderData();
  const isResetForm = code !== "";
  usePageTitle(t("auth.reset_password"));

  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [showPassword, setShowPassword] = useState(false);
  const [showConfirmPassword, setShowConfirmPassword] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [sent, setSent] = useState<ResetPasswordResponse | null>(null);
  const [formError, setFormError] = useState<string | null>(null);
  const [fieldErrors, setFieldErrors] = useState<Record<string, string | null>>({});
  const emailRef = useRef<HTMLInputElement>(null);
  const passwordRef = useRef<HTMLInputElement>(null);
  const confirmPasswordRef = useRef<HTMLInputElement>(null);

  function onSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (isResetForm && !valid) return;
    if (!isResetForm && !emailEnabled) return;

    if (isResetForm && password !== confirmPassword) {
      setFormError(null);
      setFieldErrors({ password: null, confirmPassword: t("auth.password_mismatch") });
      requestAnimationFrame(() => confirmPasswordRef.current?.focus());
      return;
    }

    setFormError(null);
    setFieldErrors({});
    setSubmitting(true);
    void (async () => {
      try {
        const res = await fetch(
          subUrl(isResetForm ? "/api/web/user/reset-password/complete" : "/api/web/user/reset-password"),
          {
            method: "POST",
            credentials: "same-origin",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify(isResetForm ? { code, password } : { email }),
          },
        );
        if (!res.ok) {
          const body = (await res.json().catch(() => ({}))) as ResetPasswordErrorResponse;
          if (body.error) setFormError(body.error);
          if (body.fields) setFieldErrors(body.fields);
          if (!body.error && !body.fields) {
            setFormError(t(isResetForm ? "reset_password_failed" : "reset_password_email_failed"));
          }
          setSubmitting(false);
          requestAnimationFrame(() => {
            if (isResetForm) passwordRef.current?.focus();
            else emailRef.current?.focus();
          });
          return;
        }

        if (isResetForm) {
          await navigate({ to: "/user/sign-in" });
          return;
        }
        setSent((await res.json()) as ResetPasswordResponse);
        setSubmitting(false);
      } catch {
        setFormError(t(isResetForm ? "reset_password_failed" : "reset_password_email_failed"));
        setSubmitting(false);
      }
    })();
  }

  const title = t("auth.reset_password");

  return (
    <main className="flex flex-1 items-center justify-center px-4 py-10 sm:px-6 sm:py-16">
      <Card className="w-full max-w-md">
        <CardHeader className="items-center text-center">
          <CardTitle>{title}</CardTitle>
        </CardHeader>
        <CardContent className="pt-2">{isResetForm ? renderResetContent() : renderRequestContent()}</CardContent>
      </Card>
    </main>
  );

  function renderRequestContent() {
    if (!emailEnabled) {
      return (
        <p role="alert" className="text-center text-sm text-(--color-destructive)">
          {t("auth.disable_register_mail")}
        </p>
      );
    }
    if (sent) {
      return (
        <div className="flex flex-col gap-4 text-center">
          <p role="status" className="text-sm text-(--color-foreground)">
            {sent.resendLimited ? (
              t("auth.reset_password_resend_limited")
            ) : (
              <Trans
                i18nKey="auth.reset_password_email_sent"
                values={{ email, hours: sent.hours }}
                components={{ email: <b />, hours: <b /> }}
              />
            )}
          </p>
          <Button variant="link" size="inline" asChild className="self-center">
            <a href={subUrl("/user/sign-in")}>{t("auth.back_to_sign_in")}</a>
          </Button>
        </div>
      );
    }

    return (
      <form onSubmit={onSubmit} noValidate>
        <fieldset disabled={submitting} className="contents">
          {renderFormError()}
          <div className="flex flex-col gap-4">
            <div className="flex flex-col gap-1.5">
              <Label htmlFor="email">{t("email")}</Label>
              <Input
                ref={emailRef}
                id="email"
                name="email"
                type="email"
                autoComplete="email"
                required
                autoFocus
                tabIndex={1}
                placeholder={t("email_placeholder")}
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                aria-invalid={"email" in fieldErrors ? true : undefined}
                aria-describedby={fieldErrors.email ? "email-error" : undefined}
              />
              {fieldErrors.email && (
                <p id="email-error" className="text-sm text-(--color-destructive)">
                  {fieldErrors.email}
                </p>
              )}
            </div>
            <FormActions
              submitLabel={submitting ? t("auth.reset_password_email_submitting") : t("auth.send_reset_email")}
              submitTabIndex={3}
              submitting={submitting}
              backLabel={t("auth.back_to_sign_in")}
              backHref={subUrl("/user/sign-in")}
            />
          </div>
        </fieldset>
      </form>
    );
  }

  function renderResetContent() {
    if (!valid) {
      return (
        <div className="flex flex-col gap-4 text-center">
          <p role="alert" className="text-sm text-(--color-destructive)">
            {t("auth.invalid_code")}
          </p>
          <Button variant="link" size="inline" asChild className="self-center">
            <a href={subUrl("/user/sign-in")}>{t("auth.back_to_sign_in")}</a>
          </Button>
        </div>
      );
    }

    return (
      <form onSubmit={onSubmit} noValidate>
        <fieldset disabled={submitting} className="contents">
          {renderFormError()}
          <div className="flex flex-col gap-4">
            <div className="flex flex-col gap-1.5">
              <Label htmlFor="password">{t("auth.new_password")}</Label>
              <PasswordInput
                inputRef={passwordRef}
                id="password"
                value={password}
                tabIndex={1}
                placeholder={t("auth.new_password_placeholder")}
                show={showPassword}
                onToggleShow={() => setShowPassword((v) => !v)}
                disabled={submitting}
                autoFocus
                describedBy={fieldErrors.password ? "password-error" : undefined}
                invalid={"password" in fieldErrors}
                onChange={setPassword}
              />
              {fieldErrors.password && (
                <p id="password-error" className="text-sm text-(--color-destructive)">
                  {fieldErrors.password}
                </p>
              )}
            </div>
            <div className="flex flex-col gap-1.5">
              <Label htmlFor="confirmPassword">{t("auth.confirm_new_password")}</Label>
              <PasswordInput
                inputRef={confirmPasswordRef}
                id="confirmPassword"
                value={confirmPassword}
                tabIndex={3}
                placeholder={t("auth.confirm_new_password_placeholder")}
                show={showConfirmPassword}
                onToggleShow={() => setShowConfirmPassword((v) => !v)}
                disabled={submitting}
                describedBy={fieldErrors.confirmPassword ? "confirmPassword-error" : undefined}
                invalid={"confirmPassword" in fieldErrors}
                onChange={setConfirmPassword}
              />
              {fieldErrors.confirmPassword && (
                <p id="confirmPassword-error" className="text-sm text-(--color-destructive)">
                  {fieldErrors.confirmPassword}
                </p>
              )}
            </div>
            <FormActions
              submitLabel={submitting ? t("auth.reset_password_submitting") : t("auth.reset_password_submit")}
              submitTabIndex={5}
              submitting={submitting}
              backLabel={t("auth.back_to_sign_in")}
              backHref={subUrl("/user/sign-in")}
            />
          </div>
        </fieldset>
      </form>
    );
  }

  function renderFormError() {
    if (!formError) return null;
    return (
      <div
        role="alert"
        className="mb-4 rounded-md border border-(--color-destructive) bg-(--color-destructive)/10 px-3 py-2 text-sm text-(--color-destructive)"
      >
        {formError}
      </div>
    );
  }
}
