import { Link, getRouteApi, useNavigate } from "@tanstack/react-router";
import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";

import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { usePageTitle } from "@/lib/page-title";
import { subUrl } from "@/lib/url";

interface MFAErrorResponse {
  error?: string;
  fields?: Record<string, string | null>;
}

type Mode = "passcode" | "recovery";

const route = getRouteApi("/user/mfa");

export function MFA() {
  const { t } = useTranslation();
  usePageTitle(t("auth.mfa_title"));
  const navigate = useNavigate();
  // When no challenge is pending the loader has already kicked off a full
  // navigation away; the early return keeps this page from flashing.
  const { pending } = route.useLoaderData();

  const [mode, setMode] = useState<Mode>("passcode");
  const [passcode, setPasscode] = useState("");
  const [recoveryCode, setRecoveryCode] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [formError, setFormError] = useState<string | null>(null);
  const [fieldErrors, setFieldErrors] = useState<Record<string, string | null>>({});
  const passcodeRef = useRef<HTMLInputElement>(null);
  const recoveryRef = useRef<HTMLInputElement>(null);

  // Focus the active input on initial render and on every mode swap.
  // `autoFocus` on the JSX doesn't reliably fire when the route mounts via
  // a TanStack navigate or when the conditional swaps which input is in the
  // tree, so we drive focus explicitly off the mode.
  useEffect(() => {
    if (!pending) return;
    if (mode === "passcode") passcodeRef.current?.focus();
    else recoveryRef.current?.focus();
  }, [mode, pending]);

  if (!pending) {
    return null;
  }

  function switchMode(next: Mode) {
    setMode(next);
    setFormError(null);
    setFieldErrors({});
  }

  function onSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setFormError(null);
    setFieldErrors({});
    setSubmitting(true);
    void (async () => {
      try {
        const url = mode === "passcode" ? subUrl("/api/web/user/mfa") : subUrl("/api/web/user/mfa/recovery");
        const body = mode === "passcode" ? JSON.stringify({ passcode }) : JSON.stringify({ recoveryCode });
        const res = await fetch(url, {
          method: "POST",
          credentials: "same-origin",
          headers: { "Content-Type": "application/json" },
          body,
        });
        if (!res.ok) {
          const errBody = (await res.json().catch(() => ({}))) as MFAErrorResponse;
          if (res.status === 401 && !errBody.fields) {
            // Session-expired or missing 2FA session: send the user back to start.
            await navigate({ to: "/user/sign-in" });
            return;
          }
          if (errBody.error) setFormError(errBody.error);
          if (errBody.fields) setFieldErrors(errBody.fields);
          if (!errBody.error && !errBody.fields) {
            setFormError(t("auth.mfa_verify_failed"));
          }
          setSubmitting(false);
          // Focus after React re-enables the fieldset; .focus() is a no-op
          // while the input is still inside a disabled fieldset, so we defer
          // past the commit with rAF.
          requestAnimationFrame(() => {
            if (mode === "passcode") passcodeRef.current?.focus();
            else recoveryRef.current?.focus();
          });
          return;
        }
        const to = new URLSearchParams(window.location.search).get("redirect_to") ?? "";
        window.location.assign(subUrl("/redirect") + "?to=" + encodeURIComponent(to));
      } catch {
        setFormError(t("auth.mfa_verify_failed"));
        setSubmitting(false);
      }
    })();
  }

  const isPasscode = mode === "passcode";
  const inputId = isPasscode ? "passcode" : "recovery_code";
  const inputErrorKey = isPasscode ? "passcode" : "recoveryCode";

  return (
    <main className="flex flex-1 items-center justify-center px-4 py-10 sm:px-6 sm:py-16">
      <Card className="w-full max-w-md">
        <CardHeader className="items-center text-center">
          <CardTitle>{t("auth.mfa_title")}</CardTitle>
        </CardHeader>
        <CardContent className="pt-2">
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
                {isPasscode ? (
                  <div className="flex flex-col gap-1.5">
                    <Label htmlFor={inputId}>{t("auth.mfa_passcode")}</Label>
                    <Input
                      ref={passcodeRef}
                      id={inputId}
                      name="passcode"
                      type="text"
                      inputMode="numeric"
                      autoComplete="one-time-code"
                      required
                      autoFocus
                      tabIndex={1}
                      placeholder={t("auth.mfa_passcode_placeholder")}
                      value={passcode}
                      onChange={(e) => setPasscode(e.target.value)}
                      aria-invalid={inputErrorKey in fieldErrors ? true : undefined}
                      aria-describedby={fieldErrors[inputErrorKey] ? `${inputId}-error` : undefined}
                    />
                    {fieldErrors[inputErrorKey] && (
                      <p id={`${inputId}-error`} className="text-sm text-(--color-destructive)">
                        {fieldErrors[inputErrorKey]}
                      </p>
                    )}
                  </div>
                ) : (
                  <div className="flex flex-col gap-1.5">
                    <Label htmlFor={inputId}>{t("auth.mfa_recovery_code")}</Label>
                    <Input
                      ref={recoveryRef}
                      id={inputId}
                      name="recovery_code"
                      type="text"
                      autoComplete="one-time-code"
                      required
                      autoFocus
                      tabIndex={1}
                      placeholder={t("auth.mfa_recovery_code_placeholder")}
                      value={recoveryCode}
                      onChange={(e) => setRecoveryCode(e.target.value)}
                      aria-invalid={inputErrorKey in fieldErrors ? true : undefined}
                      aria-describedby={fieldErrors[inputErrorKey] ? `${inputId}-error` : undefined}
                    />
                    {fieldErrors[inputErrorKey] && (
                      <p id={`${inputId}-error`} className="text-sm text-(--color-destructive)">
                        {fieldErrors[inputErrorKey]}
                      </p>
                    )}
                  </div>
                )}

                <div className="mt-2 flex flex-col gap-3">
                  <Button type="submit" disabled={submitting} tabIndex={2} className="w-full">
                    {submitting ? t("auth.mfa_verifying") : t("auth.mfa_verify")}
                  </Button>
                  <Button
                    type="button"
                    variant="link"
                    size="inline"
                    tabIndex={3}
                    className="self-center"
                    onClick={() => switchMode(isPasscode ? "recovery" : "passcode")}
                  >
                    {isPasscode ? t("auth.mfa_use_recovery_code") : t("auth.mfa_use_passcode")}
                  </Button>
                  <Button variant="link" size="inline" asChild className="self-center">
                    <Link
                      to="/user/sign-in"
                      tabIndex={submitting ? -1 : 4}
                      aria-disabled={submitting || undefined}
                      className={submitting ? "pointer-events-none opacity-50" : undefined}
                      onClick={(e) => {
                        if (submitting) e.preventDefault();
                      }}
                    >
                      {t("auth.back_to_sign_in")}
                    </Link>
                  </Button>
                </div>
              </div>
            </fieldset>
          </form>
        </CardContent>
      </Card>
    </main>
  );
}
