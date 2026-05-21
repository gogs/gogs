import { getRouteApi } from "@tanstack/react-router";
import { useRef, useState } from "react";
import { useTranslation } from "react-i18next";

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
  twoFactor?: boolean;
  redirectTo?: string;
}

interface SignInErrorResponse {
  error?: string;
  errors?: Record<string, string | null>;
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
          if (body.errors) {
            setFieldErrors(body.errors);
            const first = FIELD_ORDER.find((f) => f in (body.errors ?? {}));
            if (first === "username") usernameRef.current?.focus();
            else if (first === "password") passwordRef.current?.focus();
          }
          if (!body.error && !body.errors) {
            setFormError(t("sign_in_failed"));
          }
          return;
        }
        const data = (await res.json()) as SignInResponse;
        if (data.twoFactor) {
          window.location.assign(subUrl("/user/login/two_factor"));
          return;
        }
        window.location.assign(data.redirectTo || subUrl("/"));
      } catch {
        setFormError(t("sign_in_failed"));
      } finally {
        setSubmitting(false);
      }
    })();
  }

  return (
    <main className="flex flex-1 items-center justify-center px-4 py-10 sm:px-6 sm:py-16">
      <div className="w-full max-w-md">
        <h1 className="mb-6 text-center text-2xl font-semibold text-(--color-foreground)">{t("sign_in")}</h1>
        <form
          onSubmit={onSubmit}
          className="rounded-lg border border-(--color-border) bg-(--color-card) p-5 shadow-xs sm:p-6"
          noValidate
        >
          {formError && (
            <div
              role="alert"
              className="mb-4 rounded-md border border-(--color-destructive) bg-(--color-destructive)/10 px-3 py-2 text-sm text-(--color-destructive)"
            >
              {formError}
            </div>
          )}

          <div className="mb-4">
            <label htmlFor="username" className="mb-1 block text-sm font-medium text-(--color-foreground)">
              {t("username")}
            </label>
            <input
              ref={usernameRef}
              id="username"
              name="username"
              type="text"
              autoComplete="username"
              required
              autoFocus
              value={username}
              onChange={(e) => setUsername(e.target.value)}
              aria-invalid={"username" in fieldErrors ? true : undefined}
              aria-describedby={fieldErrors.username ? "username-error" : undefined}
              className={`block w-full rounded-md border bg-(--color-background) px-3 py-2 text-sm text-(--color-foreground) outline-none focus-visible:ring-2 focus-visible:ring-(--color-ring) ${
                "username" in fieldErrors ? "border-(--color-destructive)" : "border-(--color-input)"
              }`}
            />
            {fieldErrors.username && (
              <p id="username-error" className="mt-1 text-sm text-(--color-destructive)">
                {fieldErrors.username}
              </p>
            )}
          </div>

          <div className="mb-4">
            <label htmlFor="password" className="mb-1 block text-sm font-medium text-(--color-foreground)">
              {t("password")}
            </label>
            <input
              ref={passwordRef}
              id="password"
              name="password"
              type="password"
              autoComplete="current-password"
              required
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              aria-invalid={"password" in fieldErrors ? true : undefined}
              aria-describedby={fieldErrors.password ? "password-error" : undefined}
              className={`block w-full rounded-md border bg-(--color-background) px-3 py-2 text-sm text-(--color-foreground) outline-none focus-visible:ring-2 focus-visible:ring-(--color-ring) ${
                "password" in fieldErrors ? "border-(--color-destructive)" : "border-(--color-input)"
              }`}
            />
            {fieldErrors.password && (
              <p id="password-error" className="mt-1 text-sm text-(--color-destructive)">
                {fieldErrors.password}
              </p>
            )}
          </div>

          {loginSources.length > 0 && (
            <div className="mb-4">
              <label htmlFor="login_source" className="mb-1 block text-sm font-medium text-(--color-foreground)">
                {t("auth_source")}
              </label>
              <select
                id="login_source"
                name="login_source"
                value={loginSource}
                onChange={(e) => setLoginSource(Number(e.target.value))}
                className="block w-full rounded-md border border-(--color-input) bg-(--color-background) px-3 py-2 text-sm text-(--color-foreground) outline-none focus-visible:ring-2 focus-visible:ring-(--color-ring)"
              >
                <option value={0}>{t("local")}</option>
                {loginSources.map((s) => (
                  <option key={s.id} value={s.id}>
                    {s.name}
                  </option>
                ))}
              </select>
            </div>
          )}

          <label className="mb-5 flex items-center gap-2 text-sm text-(--color-foreground)">
            <input
              type="checkbox"
              checked={remember}
              onChange={(e) => setRemember(e.target.checked)}
              className="size-4 rounded border-(--color-input) text-(--color-primary) focus-visible:ring-2 focus-visible:ring-(--color-ring)"
            />
            {t("remember_me")}
          </label>

          <div className="flex flex-wrap items-center gap-x-4 gap-y-3 text-sm text-(--color-foreground)">
            <button
              type="submit"
              disabled={submitting}
              className="inline-flex cursor-pointer items-center justify-center rounded-md bg-(--color-primary) px-4 py-2 text-sm font-medium text-(--color-primary-foreground) hover:opacity-90 focus-visible:ring-2 focus-visible:ring-(--color-ring) disabled:cursor-not-allowed disabled:opacity-60"
            >
              {submitting ? t("sign_in_submitting") : t("sign_in")}
            </button>
            <a href={subUrl("/user/forget_password")} className="rounded-sm hover:underline">
              {t("forget_password")}
            </a>
            <a href={subUrl("/user/sign_up")} className="ml-auto rounded-sm hover:underline">
              {t("sign_up_now")}
            </a>
          </div>
        </form>
      </div>
    </main>
  );
}
