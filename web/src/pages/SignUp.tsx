import { getRouteApi, useNavigate } from "@tanstack/react-router";
import { useRef, useState } from "react";
import { useTranslation } from "react-i18next";

import { PasswordInput } from "@/components/PasswordInput";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { usePageTitle } from "@/lib/page-title";
import { subUrl } from "@/lib/url";

export interface SignUpPage {
  registrationDisabled: boolean;
  captchaEnabled: boolean;
}

interface SignUpResponse {
  emailConfirmationRequired?: boolean;
  email?: string;
  hours?: number;
}

interface SignUpErrorResponse {
  error?: string;
  fields?: Record<string, string | null>;
}

const FIELD_ORDER = ["userName", "email", "password", "confirmPassword", "captcha"] as const;

const route = getRouteApi("/user/sign-up");

export function SignUp() {
  const { t } = useTranslation();
  usePageTitle(t("register"));
  const navigate = useNavigate();
  const { registrationDisabled, captchaEnabled } = route.useLoaderData();

  const [userName, setUserName] = useState("");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [captcha, setCaptcha] = useState("");
  const [captchaRefresh, setCaptchaRefresh] = useState(0);
  const [showPassword, setShowPassword] = useState(false);
  const [showConfirmPassword, setShowConfirmPassword] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [sent, setSent] = useState<SignUpResponse | null>(null);
  const [formError, setFormError] = useState<string | null>(null);
  const [fieldErrors, setFieldErrors] = useState<Record<string, string | null>>({});
  const userNameRef = useRef<HTMLInputElement>(null);
  const emailRef = useRef<HTMLInputElement>(null);
  const passwordRef = useRef<HTMLInputElement>(null);
  const confirmPasswordRef = useRef<HTMLInputElement>(null);
  const captchaRef = useRef<HTMLInputElement>(null);

  function refreshCaptcha() {
    setCaptcha("");
    setCaptchaRefresh((value) => value + 1);
  }

  function onSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (registrationDisabled) return;

    setFormError(null);
    if (password !== confirmPassword) {
      setFieldErrors({ password: null, confirmPassword: t("password_mismatch") });
      requestAnimationFrame(() => confirmPasswordRef.current?.focus());
      return;
    }
    setFieldErrors({});
    setSubmitting(true);
    void (async () => {
      try {
        const res = await fetch(subUrl("/api/web/user/sign-up"), {
          method: "POST",
          credentials: "same-origin",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ userName, email, password, captcha }),
        });
        if (!res.ok) {
          const body = (await res.json().catch(() => ({}))) as SignUpErrorResponse;
          if (body.error) setFormError(body.error);
          let focusField: (typeof FIELD_ORDER)[number] | undefined;
          if (body.fields) {
            setFieldErrors(body.fields);
            focusField = FIELD_ORDER.find((f) => f in (body.fields ?? {}));
          }
          if (!body.error && !body.fields) {
            setFormError(t("sign_up_failed"));
          }
          setSubmitting(false);
          if (captchaEnabled) refreshCaptcha();
          requestAnimationFrame(() => {
            if (focusField === "userName") userNameRef.current?.focus();
            else if (focusField === "email") emailRef.current?.focus();
            else if (focusField === "password") passwordRef.current?.focus();
            else if (focusField === "confirmPassword") confirmPasswordRef.current?.focus();
            else if (focusField === "captcha") captchaRef.current?.focus();
          });
          return;
        }

        const data = (await res.json()) as SignUpResponse;
        if (data.emailConfirmationRequired) {
          setSent(data);
          setSubmitting(false);
          return;
        }
        await navigate({ to: "/user/sign-in" });
      } catch {
        setFormError(t("sign_up_failed"));
        setSubmitting(false);
        if (captchaEnabled) refreshCaptcha();
      }
    })();
  }

  return (
    <main className="flex flex-1 items-center justify-center px-4 py-10 sm:px-6 sm:py-16">
      <Card className="w-full max-w-md">
        <CardHeader className="items-center text-center">
          <CardTitle>{t("register")}</CardTitle>
        </CardHeader>
        <CardContent className="pt-2">{renderContent()}</CardContent>
      </Card>
    </main>
  );

  function renderContent() {
    if (registrationDisabled) {
      return (
        <p role="alert" className="text-center text-sm text-(--color-destructive)">
          {t("disable_register_prompt")}
        </p>
      );
    }
    if (sent) {
      return (
        <div className="flex flex-col gap-4 text-center">
          <p role="status" className="text-sm text-(--color-foreground)">
            {t("confirmation_email_sent")
              .replace(/<[^>]+>/g, "")
              .replace("%s", sent.email!)
              .replace("%d", String(sent.hours))}
          </p>
          <Button variant="link" size="inline" asChild className="self-center">
            <a href={subUrl("/user/sign-in")}>{t("back_to_sign_in")}</a>
          </Button>
        </div>
      );
    }

    return (
      <form onSubmit={onSubmit} noValidate>
        <fieldset disabled={submitting} className="contents">
          {formError && (
            <div
              role="alert"
              className="mb-4 rounded-md border border-(--color-destructive) bg-(--color-destructive)/10 px-3 py-2 text-sm text-(--color-destructive)"
            >
              {formError}
            </div>
          )}

          <div className="flex flex-col gap-4">
            <div className="flex flex-col gap-1.5">
              <Label htmlFor="userName">{t("username")}</Label>
              <Input
                ref={userNameRef}
                id="userName"
                name="userName"
                type="text"
                autoComplete="username"
                required
                autoFocus
                tabIndex={1}
                placeholder={t("new_username_placeholder")}
                value={userName}
                onChange={(e) => setUserName(e.target.value)}
                aria-invalid={"userName" in fieldErrors ? true : undefined}
                aria-describedby={fieldErrors.userName ? "userName-error" : undefined}
              />
              {fieldErrors.userName && (
                <p id="userName-error" className="text-sm text-(--color-destructive)">
                  {fieldErrors.userName}
                </p>
              )}
            </div>

            <div className="flex flex-col gap-1.5">
              <Label htmlFor="email">{t("email")}</Label>
              <Input
                ref={emailRef}
                id="email"
                name="email"
                type="email"
                autoComplete="email"
                required
                tabIndex={2}
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

            <div className="flex flex-col gap-1.5">
              <Label htmlFor="password">{t("password")}</Label>
              <PasswordInput
                inputRef={passwordRef}
                id="password"
                value={password}
                tabIndex={3}
                placeholder={t("password_placeholder")}
                show={showPassword}
                onToggleShow={() => setShowPassword((v) => !v)}
                disabled={submitting}
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
              <Label htmlFor="confirmPassword">{t("confirm_password")}</Label>
              <PasswordInput
                inputRef={confirmPasswordRef}
                id="confirmPassword"
                value={confirmPassword}
                tabIndex={5}
                placeholder={t("confirm_password_placeholder")}
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

            {captchaEnabled && (
              <div className="flex flex-col gap-2">
                <Label htmlFor="captcha">{t("captcha")}</Label>
                <div className="group relative">
                  <button
                    type="button"
                    tabIndex={7}
                    disabled={submitting}
                    onClick={refreshCaptcha}
                    aria-label={t("refresh_captcha")}
                    className="block w-full cursor-pointer overflow-hidden rounded-md border border-(--color-border) bg-(--color-surface) outline-none focus-visible:ring-1 focus-visible:ring-(--color-ring) disabled:cursor-not-allowed disabled:opacity-60"
                  >
                    <img
                      src={subUrl("/captcha/image.jpeg") + "?refresh=true&v=" + captchaRefresh}
                      alt={t("captcha_image_alt")}
                      className="block h-20 w-full object-fill"
                    />
                  </button>
                  <span
                    role="tooltip"
                    className="pointer-events-none absolute top-1/2 left-1/2 z-10 -translate-x-1/2 -translate-y-1/2 rounded-md bg-(--color-foreground) px-2 py-1 text-xs font-medium text-(--color-background) opacity-0 shadow transition-opacity duration-150 group-hover:opacity-90 group-focus-within:opacity-90"
                  >
                    {t("click_to_refresh_captcha")}
                  </span>
                </div>
                <Input
                  ref={captchaRef}
                  id="captcha"
                  name="captcha"
                  type="text"
                  autoComplete="off"
                  required
                  tabIndex={8}
                  placeholder={t("captcha_placeholder")}
                  value={captcha}
                  onChange={(e) => setCaptcha(e.target.value)}
                  aria-invalid={"captcha" in fieldErrors ? true : undefined}
                  aria-describedby={fieldErrors.captcha ? "captcha-error" : undefined}
                />
                {fieldErrors.captcha && (
                  <p id="captcha-error" className="text-sm text-(--color-destructive)">
                    {fieldErrors.captcha}
                  </p>
                )}
              </div>
            )}

            <div className="mt-2 flex flex-col gap-3">
              <Button type="submit" disabled={submitting} tabIndex={9} className="w-full">
                {submitting ? t("sign_up_submitting") : t("create_new_account")}
              </Button>
              <Button variant="link" size="inline" asChild className="self-center">
                <a
                  href={subUrl("/user/sign-in")}
                  tabIndex={submitting ? -1 : 10}
                  aria-disabled={submitting || undefined}
                  className={submitting ? "pointer-events-none opacity-50" : undefined}
                  onClick={(e) => {
                    if (submitting) e.preventDefault();
                  }}
                >
                  {t("register_hepler_msg")}
                </a>
              </Button>
            </div>
          </div>
        </fieldset>
      </form>
    );
  }
}
