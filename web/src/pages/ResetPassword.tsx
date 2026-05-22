import { getRouteApi, useNavigate } from "@tanstack/react-router";
import { Eye, EyeOff } from "lucide-react";
import { useRef, useState } from "react";
import { useTranslation } from "react-i18next";

import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { usePageTitle } from "@/lib/page-title";
import { subUrl } from "@/lib/url";

export interface ResetPasswordPage {
  code: string;
  valid: boolean;
}

interface ResetPasswordErrorResponse {
  error?: string;
  fields?: Record<string, string | null>;
}

const route = getRouteApi("/user/reset-password");

export function ResetPassword() {
  const { t } = useTranslation();
  usePageTitle(t("reset_password"));
  const navigate = useNavigate();
  const { code, valid } = route.useLoaderData();

  const [password, setPassword] = useState("");
  const [showPassword, setShowPassword] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [formError, setFormError] = useState<string | null>(null);
  const [fieldErrors, setFieldErrors] = useState<Record<string, string | null>>({});
  const passwordRef = useRef<HTMLInputElement>(null);

  function onSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!valid) return;

    setFormError(null);
    setFieldErrors({});
    setSubmitting(true);
    void (async () => {
      try {
        const res = await fetch(subUrl("/api/web/user/reset-password"), {
          method: "POST",
          credentials: "same-origin",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ code, password }),
        });
        if (!res.ok) {
          const body = (await res.json().catch(() => ({}))) as ResetPasswordErrorResponse;
          if (body.error) setFormError(body.error);
          if (body.fields) setFieldErrors(body.fields);
          if (!body.error && !body.fields) {
            setFormError(t("reset_password_failed"));
          }
          setSubmitting(false);
          requestAnimationFrame(() => passwordRef.current?.focus());
          return;
        }
        await navigate({ to: "/user/sign-in" });
      } catch {
        setFormError(t("reset_password_failed"));
        setSubmitting(false);
      }
    })();
  }

  return (
    <main className="flex flex-1 items-center justify-center px-4 py-10 sm:px-6 sm:py-16">
      <Card className="w-full max-w-md">
        <CardHeader className="items-center text-center">
          <CardTitle>{t("reset_password")}</CardTitle>
        </CardHeader>
        <CardContent className="pt-2">
          {!valid ? (
            <div className="flex flex-col gap-4 text-center">
              <p role="alert" className="text-sm text-(--color-destructive)">
                {t("invalid_code")}
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
                    <Label htmlFor="password">{t("password")}</Label>
                    <div className="relative">
                      <Input
                        ref={passwordRef}
                        id="password"
                        name="password"
                        type={showPassword ? "text" : "password"}
                        autoComplete="new-password"
                        required
                        autoFocus
                        tabIndex={1}
                        placeholder={t("password_placeholder")}
                        value={password}
                        onChange={(e) => setPassword(e.target.value)}
                        aria-invalid={"password" in fieldErrors ? true : undefined}
                        aria-describedby={fieldErrors.password ? "password-error" : undefined}
                        className="pr-10"
                      />
                      <button
                        type="button"
                        tabIndex={2}
                        disabled={submitting}
                        onClick={() => setShowPassword((v) => !v)}
                        aria-label={showPassword ? t("hide_password") : t("show_password")}
                        aria-pressed={showPassword}
                        className="absolute inset-y-0 right-0 flex w-10 cursor-pointer items-center justify-center rounded-r-md text-(--color-muted-foreground) outline-none hover:text-(--color-foreground) focus-visible:text-(--color-foreground) focus-visible:ring-1 focus-visible:ring-(--color-ring) disabled:cursor-not-allowed disabled:opacity-50"
                      >
                        {showPassword ? (
                          <EyeOff className="size-4" aria-hidden />
                        ) : (
                          <Eye className="size-4" aria-hidden />
                        )}
                      </button>
                    </div>
                    {fieldErrors.password && (
                      <p id="password-error" className="text-sm text-(--color-destructive)">
                        {fieldErrors.password}
                      </p>
                    )}
                  </div>

                  <div className="mt-2 flex flex-col gap-3">
                    <Button type="submit" disabled={submitting} tabIndex={3} className="w-full">
                      {submitting ? t("reset_password_submitting") : t("reset_password_helper")}
                    </Button>
                    <Button variant="link" size="inline" asChild className="self-center">
                      <a
                        href={subUrl("/user/sign-in")}
                        tabIndex={submitting ? -1 : 4}
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
