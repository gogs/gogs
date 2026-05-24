import { getRouteApi } from "@tanstack/react-router";
import { useEffect, useRef, useState } from "react";
import { Trans, useTranslation } from "react-i18next";

import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { usePageTitle } from "@/lib/page-title";
import { subUrl } from "@/lib/url";

export interface ActivatePage {
  code: string;
  signedIn: boolean;
  email: string;
  serviceNotEnabled: boolean;
  hours: number;
}

interface ActivateResponse {
  resendLimited?: boolean;
  hours?: number;
}

interface ActivateErrorResponse {
  error?: string;
}

const route = getRouteApi("/user/activate");

export function Activate() {
  const { t } = useTranslation();
  const { code, signedIn, email, serviceNotEnabled, hours } = route.useLoaderData();
  usePageTitle(t("active_your_account"));

  const isVerifying = code !== "";
  const [verifyFailed, setVerifyFailed] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [resent, setResent] = useState<ActivateResponse | null>(null);
  const [formError, setFormError] = useState<string | null>(null);
  const verifyOnceRef = useRef(false);

  useEffect(() => {
    if (!isVerifying || verifyOnceRef.current) return;
    verifyOnceRef.current = true;
    void (async () => {
      try {
        const res = await fetch(subUrl("/api/web/user/activate/complete"), {
          method: "POST",
          credentials: "same-origin",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ code }),
        });
        if (!res.ok) {
          setVerifyFailed(true);
          return;
        }
        window.location.assign(subUrl("/"));
      } catch {
        setVerifyFailed(true);
      }
    })();
  }, [isVerifying, code]);

  function onResend(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setFormError(null);
    setSubmitting(true);
    void (async () => {
      try {
        const res = await fetch(subUrl("/api/web/user/activate"), {
          method: "POST",
          credentials: "same-origin",
        });
        if (!res.ok) {
          const body = (await res.json().catch(() => ({}))) as ActivateErrorResponse;
          setFormError(body.error ?? t("activate_resend_failed"));
          setSubmitting(false);
          return;
        }
        setResent((await res.json()) as ActivateResponse);
        setSubmitting(false);
      } catch {
        setFormError(t("activate_resend_failed"));
        setSubmitting(false);
      }
    })();
  }

  return (
    <main className="flex flex-1 items-center justify-center px-4 py-10 sm:px-6 sm:py-16">
      <Card className="w-full max-w-md">
        <CardHeader className="items-center text-center">
          <CardTitle>{t("active_your_account")}</CardTitle>
        </CardHeader>
        <CardContent className="pt-2">{renderContent()}</CardContent>
      </Card>
    </main>
  );

  function renderContent() {
    if (isVerifying) {
      if (verifyFailed) {
        return (
          <div className="flex flex-col gap-4 text-center">
            <p role="alert" className="text-sm text-(--color-destructive)">
              {t("invalid_code")}
            </p>
            <Button variant="link" size="inline" asChild className="self-center">
              <a href={subUrl("/user/sign-in")}>{t("back_to_sign_in")}</a>
            </Button>
          </div>
        );
      }
      return (
        <p role="status" className="text-center text-sm text-(--color-foreground)">
          {t("activate_verifying")}
        </p>
      );
    }

    if (serviceNotEnabled) {
      return (
        <p role="alert" className="text-center text-sm text-(--color-destructive)">
          {t("disable_register_mail")}
        </p>
      );
    }

    if (!signedIn) {
      return (
        <div className="flex flex-col gap-4 text-center">
          <p className="text-sm text-(--color-foreground)">{t("activate_check_email")}</p>
          <Button variant="link" size="inline" asChild className="self-center">
            <a href={subUrl("/user/sign-in")}>{t("back_to_sign_in")}</a>
          </Button>
        </div>
      );
    }

    if (resent) {
      return (
        <p role="status" className="text-center text-sm text-(--color-foreground)">
          {resent.resendLimited ? (
            t("resent_limit_prompt")
          ) : (
            <Trans
              i18nKey="activate_email_sent"
              values={{ email, hours: resent.hours }}
              components={{ email: <b />, hours: <b /> }}
            />
          )}
        </p>
      );
    }

    return (
      <form onSubmit={onResend} noValidate>
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
            <p className="text-sm text-(--color-foreground)">
              <Trans
                i18nKey="activate_resend_prompt"
                values={{ email, hours }}
                components={{ email: <b />, hours: <b /> }}
              />
            </p>
            <Button type="submit" disabled={submitting} className="w-full">
              {submitting ? t("activate_resending") : t("resend_mail")}
            </Button>
          </div>
        </fieldset>
      </form>
    );
  }
}
