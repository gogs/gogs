import { getRouteApi } from "@tanstack/react-router";
import { useRef, useState } from "react";
import { useTranslation } from "react-i18next";

import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { usePageTitle } from "@/lib/page-title";
import { subUrl } from "@/lib/url";

export interface ForgotPasswordPage {
  emailEnabled: boolean;
}

interface ForgotPasswordResponse {
  hours: number;
  resendLimited?: boolean;
}

interface ForgotPasswordErrorResponse {
  error?: string;
  fields?: Record<string, string | null>;
}

const route = getRouteApi("/user/forgot-password");

export function ForgotPassword() {
  const { t } = useTranslation();
  usePageTitle(t("forgot_password"));
  const { emailEnabled } = route.useLoaderData();

  const [email, setEmail] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [sent, setSent] = useState<ForgotPasswordResponse | null>(null);
  const [formError, setFormError] = useState<string | null>(null);
  const [fieldErrors, setFieldErrors] = useState<Record<string, string | null>>({});
  const emailRef = useRef<HTMLInputElement>(null);

  function onSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!emailEnabled) return;

    setFormError(null);
    setFieldErrors({});
    setSubmitting(true);
    void (async () => {
      try {
        const res = await fetch(subUrl("/api/web/user/forgot-password"), {
          method: "POST",
          credentials: "same-origin",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ email }),
        });
        if (!res.ok) {
          const body = (await res.json().catch(() => ({}))) as ForgotPasswordErrorResponse;
          if (body.error) setFormError(body.error);
          if (body.fields) setFieldErrors(body.fields);
          if (!body.error && !body.fields) {
            setFormError(t("forgot_password_failed"));
          }
          setSubmitting(false);
          requestAnimationFrame(() => emailRef.current?.focus());
          return;
        }

        setSent((await res.json()) as ForgotPasswordResponse);
        setSubmitting(false);
      } catch {
        setFormError(t("forgot_password_failed"));
        setSubmitting(false);
      }
    })();
  }

  return (
    <main className="flex flex-1 items-center justify-center px-4 py-10 sm:px-6 sm:py-16">
      <Card className="w-full max-w-md">
        <CardHeader className="items-center text-center">
          <CardTitle>{t("forgot_password")}</CardTitle>
        </CardHeader>
        <CardContent className="pt-2">
          {!emailEnabled ? (
            <p role="alert" className="text-center text-sm text-(--color-destructive)">
              {t("disable_register_mail")}
            </p>
          ) : sent ? (
            <div className="flex flex-col gap-4 text-center">
              <p role="status" className="text-sm text-(--color-foreground)">
                {sent.resendLimited
                  ? t("resent_limit_prompt")
                  : t("reset_mail_sent_prompt", { email, hours: sent.hours })}
              </p>
              <Button variant="link" size="inline" asChild className="self-center">
                <a href={subUrl("/user/sign-in")}>{t("back_to_sign_in")}</a>
              </Button>
            </div>
          ) : (
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

                  <div className="mt-2 flex flex-col gap-3">
                    <Button type="submit" disabled={submitting} tabIndex={2} className="w-full">
                      {submitting ? t("forgot_password_submitting") : t("send_reset_mail")}
                    </Button>
                    <Button variant="link" size="inline" asChild className="self-center">
                      <a
                        href={subUrl("/user/sign-in")}
                        tabIndex={submitting ? -1 : 3}
                        aria-disabled={submitting || undefined}
                        className={submitting ? "pointer-events-none opacity-50" : undefined}
                        onClick={(e) => {
                          if (submitting) e.preventDefault();
                        }}
                      >
                        {t("back_to_sign_in")}
                      </a>
                    </Button>
                  </div>
                </div>
              </fieldset>
            </form>
          )}
        </CardContent>
      </Card>
    </main>
  );
}
