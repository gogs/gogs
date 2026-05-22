import { getRouteApi } from "@tanstack/react-router";
import { Eye, EyeOff } from "lucide-react";
import { useRef, useState } from "react";
import { useTranslation } from "react-i18next";

import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { usePageTitle } from "@/lib/page-title";
import { subUrl } from "@/lib/url";

export interface LoginSource {
  id: number;
  name: string;
  isDefault: boolean;
}

export interface SignInPage {
  loginSources: LoginSource[];
}

interface SignInResponse {
  mfa?: boolean;
  redirectTo?: string;
}

interface SignInErrorResponse {
  error?: string;
  fields?: Record<string, string | null>;
}

// Field display order; the first key with a server-side error gets focus.
const FIELD_ORDER = ["username", "password"] as const;

const route = getRouteApi("/user/sign-in");

export function SignIn() {
  const { t } = useTranslation();
  usePageTitle(t("sign_in"));
  const { loginSources } = route.useLoaderData();
  const defaultSource = loginSources.find((s) => s.isDefault);

  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [loginSource, setLoginSource] = useState<number>(defaultSource?.id ?? 0);
  const [remember, setRemember] = useState(false);
  const [showPassword, setShowPassword] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [formError, setFormError] = useState<string | null>(null);
  const [fieldErrors, setFieldErrors] = useState<Record<string, string | null>>({});
  const usernameRef = useRef<HTMLInputElement>(null);
  const passwordRef = useRef<HTMLInputElement>(null);

  function onSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setFormError(null);
    setFieldErrors({});
    setSubmitting(true);
    void (async () => {
      try {
        const redirectTo = new URLSearchParams(window.location.search).get("redirect_to") ?? "";
        const res = await fetch(subUrl("/api/web/user/sign-in"), {
          method: "POST",
          credentials: "same-origin",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ username, password, loginSource, remember, redirectTo }),
        });
        if (!res.ok) {
          const body = (await res.json().catch(() => ({}))) as SignInErrorResponse;
          if (body.error) setFormError(body.error);
          else setFormError(null);
          if (body.fields) {
            setFieldErrors(body.fields);
            const first = FIELD_ORDER.find((f) => f in (body.fields ?? {}));
            if (first === "username") usernameRef.current?.focus();
            else if (first === "password") passwordRef.current?.focus();
          }
          if (!body.error && !body.fields) {
            setFormError(t("sign_in_failed"));
          }
          setSubmitting(false);
          return;
        }
        const data = (await res.json()) as SignInResponse;
        if (data.mfa) {
          const search = window.location.search;
          window.location.assign(subUrl("/user/mfa") + search);
          return;
        }
        window.location.assign(data.redirectTo || subUrl("/"));
      } catch {
        setFormError(t("sign_in_failed"));
        setSubmitting(false);
      }
    })();
  }

  return (
    <main className="flex flex-1 items-center justify-center px-4 py-10 sm:px-6 sm:py-16">
      <Card className="w-full max-w-md">
        <CardHeader className="items-center text-center">
          <CardTitle>{t("sign_in")}</CardTitle>
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
                <div className="flex flex-col gap-1.5">
                  <Label htmlFor="username">{t("username")}</Label>
                  <Input
                    ref={usernameRef}
                    id="username"
                    name="username"
                    type="text"
                    autoComplete="username"
                    required
                    autoFocus
                    tabIndex={1}
                    placeholder={t("username_placeholder")}
                    value={username}
                    onChange={(e) => setUsername(e.target.value)}
                    aria-invalid={"username" in fieldErrors ? true : undefined}
                    aria-describedby={fieldErrors.username ? "username-error" : undefined}
                  />
                  {fieldErrors.username && (
                    <p id="username-error" className="text-sm text-(--color-destructive)">
                      {fieldErrors.username}
                    </p>
                  )}
                </div>

                <div className="flex flex-col gap-1.5">
                  <div className="flex items-center justify-between gap-3">
                    <Label htmlFor="password">{t("password")}</Label>
                    <Button variant="link" size="inline" asChild>
                      <a
                        href={subUrl("/user/forget_password")}
                        tabIndex={submitting ? -1 : 7}
                        aria-disabled={submitting || undefined}
                        className={submitting ? "pointer-events-none opacity-50" : undefined}
                        onClick={(e) => {
                          if (submitting) e.preventDefault();
                        }}
                      >
                        {t("forget_password")}
                      </a>
                    </Button>
                  </div>
                  <div className="relative">
                    <Input
                      ref={passwordRef}
                      id="password"
                      name="password"
                      type={showPassword ? "text" : "password"}
                      autoComplete="current-password"
                      required
                      tabIndex={2}
                      placeholder={t("password_placeholder")}
                      value={password}
                      onChange={(e) => setPassword(e.target.value)}
                      aria-invalid={"password" in fieldErrors ? true : undefined}
                      aria-describedby={fieldErrors.password ? "password-error" : undefined}
                      className="pr-10"
                    />
                    <button
                      type="button"
                      tabIndex={3}
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

                {loginSources.length > 0 && (
                  <div className="flex flex-col gap-1.5">
                    <Label htmlFor="login_source">{t("auth_source")}</Label>
                    <Select
                      value={String(loginSource)}
                      onValueChange={(v) => setLoginSource(Number(v))}
                      disabled={submitting}
                    >
                      <SelectTrigger id="login_source" tabIndex={4}>
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="0">{t("local")}</SelectItem>
                        {loginSources.map((s) => (
                          <SelectItem key={s.id} value={String(s.id)}>
                            {s.name}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  </div>
                )}

                <div className="flex items-center gap-2">
                  <Checkbox
                    id="remember"
                    tabIndex={5}
                    checked={remember}
                    onCheckedChange={(v) => setRemember(v === true)}
                  />
                  <Label htmlFor="remember" className="cursor-pointer font-normal">
                    {t("remember_me")}
                  </Label>
                </div>

                <div className="mt-2 flex flex-col gap-3">
                  <Button type="submit" disabled={submitting} tabIndex={6} className="w-full">
                    {submitting ? t("sign_in_submitting") : t("sign_in")}
                  </Button>
                  <Button variant="link" size="inline" asChild className="self-center">
                    <a
                      href={subUrl("/user/sign_up")}
                      tabIndex={submitting ? -1 : 8}
                      aria-disabled={submitting || undefined}
                      className={submitting ? "pointer-events-none opacity-50" : undefined}
                      onClick={(e) => {
                        if (submitting) e.preventDefault();
                      }}
                    >
                      {t("sign_up_now")}
                    </a>
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
