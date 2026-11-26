import { LoaderCircle } from "lucide-react";
import { useState } from "react";
import { useTranslation } from "react-i18next";

import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { type AltchaState, type AntiBot } from "@/lib/anti-bot";
import { subUrl } from "@/lib/url";
import { cn } from "@/lib/utils";

let captchaMounts = 0;

interface AntiBotFieldProps {
  antiBot: AntiBot;
  disabled: boolean;
  // error is the server's message about the "antiBotPayload" field, and
  // invalid is whether the server rejected it.
  error?: string | null;
  invalid: boolean;
  // tabIndex is of the altcha checkbox, or of the captcha image, followed by
  // its input.
  tabIndex: number;
}

// AntiBotField asks for the anti-bot verification of a form: the altcha
// checkbox or the captcha image. It renders nothing when the form is not
// verified.
export function AntiBotField({ antiBot, disabled, error, invalid, tabIndex }: AntiBotFieldProps) {
  const { t } = useTranslation();
  // The browser shows an image it already loaded for the same URL without
  // asking the server again, whose text may have been used up since, e.g.,
  // when logging in right after signing up. So every mount gets its own URL.
  const [captchaMount] = useState(() => ++captchaMounts);
  const errorMessage = error && (
    <p id="antiBotPayload-error" className="text-sm text-(--color-destructive)">
      {error}
    </p>
  );

  if (antiBot.strategy === "captcha") {
    return (
      <div className="flex flex-col gap-2">
        <Label htmlFor="captcha">{t("captcha")}</Label>
        <div className="group relative">
          <button
            type="button"
            tabIndex={tabIndex}
            disabled={disabled}
            onClick={antiBot.reset}
            aria-label={t("refresh_captcha")}
            className="block w-full cursor-pointer overflow-hidden rounded-md border border-(--color-border) bg-(--color-surface) outline-none focus-visible:ring-1 focus-visible:ring-(--color-ring) disabled:cursor-not-allowed disabled:opacity-60"
          >
            <img
              src={`${subUrl("/-/captcha/image.png")}?refresh=true&m=${captchaMount}&v=${antiBot.captchaRefresh}`}
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
          id="captcha"
          name="captcha"
          type="text"
          autoComplete="off"
          required
          tabIndex={tabIndex + 1}
          placeholder={t("captcha_placeholder")}
          value={antiBot.captcha}
          onChange={(e) => antiBot.setCaptcha(e.target.value)}
          aria-invalid={invalid ? true : undefined}
          aria-describedby={error ? "antiBotPayload-error" : undefined}
        />
        {errorMessage}
      </div>
    );
  }
  if (antiBot.strategy !== "altcha") return null;

  const altchaState = antiBot.altchaState;
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
          altchaState === "error" || invalid
            ? "border-(--color-destructive) ring-1 ring-(--color-destructive)"
            : altchaState === "verified"
              ? "border-(--color-success) ring-1 ring-(--color-success)"
              : "border-(--color-border)",
        )}
      >
        <button
          type="button"
          role="checkbox"
          id="altcha"
          tabIndex={tabIndex}
          aria-checked={altchaState === "verified"}
          aria-busy={altchaState === "verifying" || undefined}
          aria-labelledby="altcha-label"
          aria-describedby={cn(altchaState === "error" && "altcha-note", error && "antiBotPayload-error") || undefined}
          aria-invalid={invalid ? true : undefined}
          disabled={disabled}
          onClick={() => {
            if (altchaState !== "verifying" && altchaState !== "verified") antiBot.verifyAltcha();
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
      {errorMessage}
    </div>
  );
}
