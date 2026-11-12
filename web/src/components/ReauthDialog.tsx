import { useMutation, useQuery } from "@tanstack/react-query";
import { KeyRound, LoaderCircle } from "lucide-react";
import { type FormEvent, useEffect, useState, useSyncExternalStore } from "react";
import { useTranslation } from "react-i18next";

import { FormAlert, FormField } from "@/components/FormField";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { type RequestOptionsJSON, getPasskey, passkeysSupported } from "@/lib/passkey";
import { fetchJSON, formErrors, submit } from "@/lib/queries/request";
import { finishConfirmIdentity, isConfirmingIdentity, subscribeConfirmIdentity } from "@/lib/reauth";
import { subUrl } from "@/lib/url";

type Method = "passkey" | "password" | "passcode";

interface ReauthMethods {
  passkey: boolean;
  passcode: boolean;
  minutes: number;
}

function reauthURL(path = "") {
  return subUrl(`/api/web/user/reauth${path}`);
}

// ReauthDialog asks the viewer to confirm who they are when an action needs
// it, see confirmIdentity. It is mounted once for the whole app.
export function ReauthDialog() {
  const open = useSyncExternalStore(subscribeConfirmIdentity, isConfirmingIdentity);
  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        if (!next) finishConfirmIdentity(false);
      }}
    >
      {open ? (
        <DialogContent>
          <ReauthForm />
        </DialogContent>
      ) : null}
    </Dialog>
  );
}

function ReauthForm() {
  const { t } = useTranslation();
  const methods = useQuery({
    queryKey: ["reauth-methods"],
    queryFn: () => fetchJSON<ReauthMethods>(reauthURL()),
    staleTime: 0,
  });
  const canUsePasskey = !!methods.data?.passkey && passkeysSupported();
  const [chosen, setChosen] = useState<Method | null>(null);
  const method: Method = chosen ?? (canUsePasskey ? "passkey" : "password");
  const [password, setPassword] = useState("");
  const [passcode, setPasscode] = useState("");

  // Bumped to ask for the passkey again.
  const [passkeyRun, setPasskeyRun] = useState(0);
  const passkey = useMutation({
    mutationFn: async (signal: AbortSignal) => {
      const options = await submit<RequestOptionsJSON>("POST", reauthURL("/passkey/challenge"), {});
      const credential = await getPasskey(options, signal);
      await submit<void>("POST", reauthURL("/passkey"), credential);
    },
    onSuccess: () => finishConfirmIdentity(true),
  });
  const { mutate: confirmWithPasskey } = passkey;
  const secret = useMutation({
    mutationFn: () =>
      method === "passcode"
        ? submit<void>("POST", reauthURL("/passcode"), { passcode })
        : submit<void>("POST", reauthURL(), { password }),
    onSuccess: () => finishConfirmIdentity(true),
  });

  // Ask for the passkey as soon as it is the method, as signing in does. The
  // viewer just took the action, so browsers that need a click for passkeys
  // usually allow it. Choosing another method or closing the dialog cancels
  // the browser prompt, which would otherwise keep the next one from opening.
  useEffect(() => {
    if (method !== "passkey") return;
    const controller = new AbortController();
    confirmWithPasskey(controller.signal);
    return () => controller.abort();
  }, [method, passkeyRun, confirmWithPasskey]);

  const choose = (next: Method) => {
    setChosen(next);
    secret.reset();
  };

  const onSubmit = (e: FormEvent) => {
    e.preventDefault();
    secret.mutate();
  };

  // A DOMException is the browser giving up on the passkey: the viewer
  // cancelled or the dialog did, which need no message, or no passkey
  // answered.
  const passkeyError =
    passkey.error instanceof DOMException
      ? passkey.error.name === "NotAllowedError" || passkey.error.name === "AbortError"
        ? null
        : t("auth.reauth_passkey_failed")
      : (passkey.error?.message ?? null);
  const { fields, formError } = formErrors(secret.error);
  const others = (["passkey", "password", "passcode"] as const).filter(
    (m) =>
      m !== method &&
      (m === "password" || (m === "passkey" && canUsePasskey) || (m === "passcode" && methods.data?.passcode)),
  );
  const otherLabels: Record<Method, string> = {
    passkey: t("auth.reauth_use_passkey"),
    password: t("auth.reauth_use_password"),
    passcode: t("auth.reauth_use_passcode"),
  };

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-col gap-1.5">
        <DialogTitle>{t("auth.reauth_title")}</DialogTitle>
        <DialogDescription>
          {methods.data ? t("auth.reauth_desc", { minutes: methods.data.minutes }) : null}
        </DialogDescription>
      </div>

      {methods.isPending ? (
        <LoaderCircle
          className="size-5 self-center text-(--color-muted-foreground) motion-safe:animate-spin"
          aria-hidden
        />
      ) : method === "passkey" ? (
        <div className="flex flex-col gap-3">
          <FormAlert message={passkeyError} />
          {passkey.isPending ? (
            <p role="status" className="flex items-center gap-2 text-sm">
              <LoaderCircle className="size-4 shrink-0 motion-safe:animate-spin" aria-hidden />
              {t("auth.reauth_waiting")}
            </p>
          ) : (
            <Button type="button" className="w-full" onClick={() => setPasskeyRun((n) => n + 1)}>
              <KeyRound className="size-4" aria-hidden />
              {t("auth.reauth_use_passkey")}
            </Button>
          )}
        </div>
      ) : (
        <form onSubmit={onSubmit} noValidate>
          <fieldset disabled={secret.isPending} className="contents">
            <FormAlert message={formError} />
            <div className="flex flex-col gap-3">
              {method === "password" ? (
                <FormField
                  id="reauth-password"
                  label={t("password")}
                  error={fields.password}
                  invalid={"password" in fields}
                >
                  <Input
                    id="reauth-password"
                    type="password"
                    autoComplete="current-password"
                    required
                    autoFocus
                    value={password}
                    onChange={(e) => setPassword(e.target.value)}
                  />
                </FormField>
              ) : (
                <FormField
                  id="reauth-passcode"
                  label={t("auth.mfa_passcode")}
                  error={fields.passcode}
                  invalid={"passcode" in fields}
                >
                  <Input
                    id="reauth-passcode"
                    inputMode="numeric"
                    autoComplete="one-time-code"
                    required
                    autoFocus
                    placeholder={t("auth.mfa_passcode_placeholder")}
                    value={passcode}
                    onChange={(e) => setPasscode(e.target.value.trim())}
                  />
                </FormField>
              )}
              <Button type="submit" className="w-full">
                {secret.isPending ? t("auth.reauth_confirming") : t("auth.reauth_confirm")}
              </Button>
            </div>
          </fieldset>
        </form>
      )}

      {methods.data && others.length > 0 ? (
        <div className="flex flex-col items-center gap-2">
          {others.map((m) => (
            <Button key={m} type="button" variant="link" size="inline" onClick={() => choose(m)}>
              {otherLabels[m]}
            </Button>
          ))}
        </div>
      ) : null}
    </div>
  );
}
