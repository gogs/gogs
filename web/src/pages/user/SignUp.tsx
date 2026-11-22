import { Link, getRouteApi, useNavigate } from "@tanstack/react-router";
import { LoaderCircle } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { Trans, useTranslation } from "react-i18next";

import { PasswordInput } from "@/components/PasswordInput";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { type AltchaSolution, solveAltcha } from "@/lib/altcha";
import { usePageTitle } from "@/lib/page-title";
import { subUrl } from "@/lib/url";
import { cn } from "@/lib/utils";

export interface SignUpPage {
  registrationDisabled: boolean;
  // antiBotVerification is empty when sign-ups are not verified.
  antiBotVerification: "" | "altcha" | "captcha";
}

type AltchaState = "idle" | "verifying" | "verified" | "error";

interface SignUpResponse {
  emailConfirmationRequired?: boolean;
  email?: string;
  hours?: number;
}

interface SignUpErrorResponse {
  error?: string;
  fields?: Record<string, string | null>;
}

const FIELD_ORDER = ["userName", "email", "password", "confirmPassword", "antiBotPayload"] as const;

const route = getRouteApi("/user/sign-up");

export function SignUp() {
  const { t } = useTranslation();
  usePageTitle(t("register"));
  const navigate = useNavigate();
  const { registrationDisabled, antiBotVerification } = route.useLoaderData();
  const altchaEnabled = antiBotVerification === "altcha";
  const captchaEnabled = antiBotVerification === "captcha";

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
  const [altchaState, setAltchaState] = useState<AltchaState>("idle");
  const formFilled =
    userName.trim() !== "" &&
    email.trim() !== "" &&
    password !== "" &&
    confirmPassword !== "" &&
    (!captchaEnabled || captcha.trim() !== "") &&
    (!altchaEnabled || altchaState === "verified");
  const altchaSolution = useRef<AltchaSolution | null>(null);
  const altchaRef = useRef<HTMLButtonElement>(null);

  useEffect(() => {
    if (altchaState !== "verified" || !altchaSolution.current) return;
    // Start over once the solution expires, so a stale one is never sent.
    const timer = setTimeout(() => {
      altchaSolution.current = null;
      setAltchaState("idle");
    }, altchaSolution.current.expiresAt - Date.now());
    return () => clearTimeout(timer);
  }, [altchaState]);

  function resetAntiBotVerification() {
    if (altchaEnabled) {
      resetAltcha();
      return;
    }
    setCaptcha("");
    setCaptchaRefresh((value) => value + 1);
  }

  function resetAltcha() {
    altchaSolution.current = null;
    setAltchaState("idle");
  }

  // verifyAltcha solves a proof-of-work challenge and keeps the solution for
  // the next submit.
  function verifyAltcha() {
    setAltchaState("verifying");
    solveAltcha().then(
      (solved) => {
        altchaSolution.current = solved;
        setAltchaState("verified");
      },
      () => {
        altchaSolution.current = null;
        setAltchaState("error");
      },
    );
  }

  function onSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const solution = altchaSolution.current;
    if (registrationDisabled || !formFilled || (altchaEnabled && !(solution && solution.expiresAt > Date.now())))
      return;

    setFormError(null);
    if (password !== confirmPassword) {
      setFieldErrors({ password: null, confirmPassword: t("auth.password_mismatch") });
      requestAnimationFrame(() => confirmPasswordRef.current?.focus());
      return;
    }
    setFieldErrors({});
    setSubmitting(true);
    void (async () => {
      try {
        // The server accepts an altcha solution only once.
        if (altchaEnabled) altchaSolution.current = null;
        const res = await fetch(subUrl("/api/web/user/sign-up"), {
          method: "POST",
          credentials: "same-origin",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({
            userName,
            email,
            password,
            antiBotPayload: altchaEnabled ? solution?.payload : captcha,
          }),
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
            setFormError(t("auth.sign_up_failed"));
          }
          setSubmitting(false);
          if (antiBotVerification) resetAntiBotVerification();
          requestAnimationFrame(() => {
            if (focusField === "userName") userNameRef.current?.focus();
            else if (focusField === "email") emailRef.current?.focus();
            else if (focusField === "password") passwordRef.current?.focus();
            else if (focusField === "confirmPassword") confirmPasswordRef.current?.focus();
            else if (focusField === "antiBotPayload") (altchaEnabled ? altchaRef : captchaRef).current?.focus();
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
        setFormError(t("auth.sign_up_failed"));
        setSubmitting(false);
        if (antiBotVerification) resetAntiBotVerification();
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
          {t("auth.disable_register_prompt")}
        </p>
      );
    }
    if (sent) {
      return (
        <div className="flex flex-col gap-4 text-center">
          <p role="status" className="text-sm text-(--color-foreground)">
            <Trans
              i18nKey="auth.activation_email_sent"
              values={{ email: sent.email, hours: sent.hours }}
              components={{ email: <b />, hours: <b /> }}
            />
          </p>
          <Button variant="link" size="inline" asChild className="self-center">
            <Link activeOptions={{ exact: true }} to="/user/sign-in">
              {t("auth.back_to_sign_in")}
            </Link>
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
              <Label htmlFor="confirmPassword">{t("auth.confirm_password")}</Label>
              <PasswordInput
                inputRef={confirmPasswordRef}
                id="confirmPassword"
                value={confirmPassword}
                tabIndex={5}
                placeholder={t("auth.confirm_password_placeholder")}
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

            {altchaEnabled && renderAltcha()}

            {captchaEnabled && (
              <div className="flex flex-col gap-2">
                <Label htmlFor="captcha">{t("captcha")}</Label>
                <div className="group relative">
                  <button
                    type="button"
                    tabIndex={7}
                    disabled={submitting}
                    onClick={resetAntiBotVerification}
                    aria-label={t("refresh_captcha")}
                    className="block w-full cursor-pointer overflow-hidden rounded-md border border-(--color-border) bg-(--color-surface) outline-none focus-visible:ring-1 focus-visible:ring-(--color-ring) disabled:cursor-not-allowed disabled:opacity-60"
                  >
                    <img
                      src={subUrl("/-/captcha/image.png") + "?refresh=true&v=" + captchaRefresh}
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
                  aria-invalid={"antiBotPayload" in fieldErrors ? true : undefined}
                  aria-describedby={fieldErrors.antiBotPayload ? "antiBotPayload-error" : undefined}
                />
                {fieldErrors.antiBotPayload && (
                  <p id="antiBotPayload-error" className="text-sm text-(--color-destructive)">
                    {fieldErrors.antiBotPayload}
                  </p>
                )}
              </div>
            )}

            <div className="mt-2 flex flex-col gap-3">
              <Button type="submit" disabled={submitting || !formFilled} tabIndex={9} className="w-full">
                {submitting ? t("auth.sign_up_submitting") : t("auth.create_new_account")}
              </Button>
              <Button variant="link" size="inline" asChild className="self-center">
                <Link
                  activeOptions={{ exact: true }}
                  to="/user/sign-in"
                  tabIndex={submitting ? -1 : 10}
                  aria-disabled={submitting || undefined}
                  className={submitting ? "pointer-events-none opacity-50" : undefined}
                  onClick={(e) => {
                    if (submitting) e.preventDefault();
                  }}
                >
                  {t("auth.register_hepler_msg")}
                </Link>
              </Button>
            </div>
          </div>
        </fieldset>
      </form>
    );
  }

  function renderAltcha() {
    const labels: Record<AltchaState, string> = {
      idle: t("altcha_label"),
      verifying: t("altcha_verifying"),
      verified: t("altcha_verified"),
      error: t("altcha_failed"),
    };
    // Every state's message is stacked in the same grid cell and only the
    // current one is visible, so the box keeps the size of its largest state.
    const message = (state: AltchaState, current: boolean) => (
      <div
        key={state}
        aria-hidden={current ? undefined : true}
        className={cn("col-start-1 row-start-1 flex flex-col gap-0.5 self-center", !current && "invisible")}
      >
        <span
          id={current ? "altcha-label" : undefined}
          className={cn(
            "text-sm font-medium",
            state === "error" ? "text-(--color-destructive)" : "text-(--color-foreground)",
          )}
        >
          {labels[state]}
        </span>
        {state === "error" && (
          <span id={current ? "altcha-note" : undefined} className="text-xs text-(--color-muted-foreground)">
            {t("altcha_retry")}
          </span>
        )}
      </div>
    );
    return (
      <div className="flex flex-col gap-2">
        <div
          className={cn(
            "flex items-center gap-3 rounded-md border bg-(--color-surface) px-3 py-2.5 transition-[border-color,box-shadow]",
            altchaState === "error" || "antiBotPayload" in fieldErrors
              ? "border-(--color-destructive) ring-1 ring-(--color-destructive)"
              : altchaState === "verified"
                ? "border-(--color-success) ring-1 ring-(--color-success)"
                : "border-(--color-border)",
          )}
        >
          <button
            ref={altchaRef}
            type="button"
            role="checkbox"
            id="altcha"
            tabIndex={7}
            aria-checked={altchaState === "verified"}
            aria-busy={altchaState === "verifying" || undefined}
            aria-labelledby="altcha-label"
            aria-describedby={
              cn(altchaState === "error" && "altcha-note", fieldErrors.antiBotPayload && "antiBotPayload-error") ||
              undefined
            }
            aria-invalid={"antiBotPayload" in fieldErrors ? true : undefined}
            disabled={submitting}
            onClick={() => {
              if (altchaState !== "verifying" && altchaState !== "verified") verifyAltcha();
            }}
            className={cn(
              "flex size-6 shrink-0 cursor-pointer items-center justify-center rounded-sm border-2 border-(--color-input) bg-(--color-field) outline-none transition-colors",
              "focus-visible:ring-2 focus-visible:ring-(--color-ring) focus-visible:ring-offset-2 focus-visible:ring-offset-(--color-surface)",
              "disabled:cursor-not-allowed disabled:opacity-60",
              altchaState === "verifying" && "cursor-progress border-transparent bg-transparent",
              altchaState === "verified" && "cursor-default border-transparent bg-transparent",
              altchaState === "error" && "border-(--color-destructive)",
            )}
          >
            {altchaState === "verifying" && (
              <LoaderCircle className="size-5 text-(--color-primary) motion-safe:animate-spin" aria-hidden />
            )}
            {altchaState === "verified" && (
              // Lucide's ShieldCheck, with the shield filled so the check mark reads on it.
              <svg
                viewBox="0 0 24 24"
                className="size-7 shrink-0"
                fill="none"
                strokeWidth={2}
                strokeLinecap="round"
                strokeLinejoin="round"
                aria-hidden
              >
                <path
                  d="M20 13c0 5-3.5 7.5-7.66 8.95a1 1 0 0 1-.67-.01C7.5 20.5 4 18 4 13V6a1 1 0 0 1 1-1c2 0 4.5-1.2 6.24-2.72a1.17 1.17 0 0 1 1.52 0C14.51 3.81 17 5 19 5a1 1 0 0 1 1 1z"
                  className="fill-(--color-success) stroke-(--color-success)"
                />
                <path d="m9 12 2 2 4-4" strokeWidth={2.5} className="stroke-(--color-background)" />
              </svg>
            )}
          </button>
          <div className="grid min-w-0">
            {(Object.keys(labels) as AltchaState[]).map((state) => message(state, state === altchaState))}
          </div>
          {/* A live region only announces text that changes inside it, so it
              stays mounted and its text follows the state. */}
          <span className="sr-only" aria-live="polite">
            {altchaState === "idle"
              ? ""
              : altchaState === "error"
                ? `${labels.error}. ${t("altcha_retry")}`
                : labels[altchaState]}
          </span>
        </div>
        {fieldErrors.antiBotPayload && (
          <p id="antiBotPayload-error" className="text-sm text-(--color-destructive)">
            {fieldErrors.antiBotPayload}
          </p>
        )}
      </div>
    );
  }
}
