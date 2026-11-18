import { type Ref, useState } from "react";
import { useTranslation } from "react-i18next";

import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { subUrl } from "@/lib/url";

let mounts = 0;

interface CaptchaFieldProps {
  value: string;
  onChange: (value: string) => void;
  // Changing it loads a new image, e.g., after a failed submission used up the
  // current one.
  refreshKey: number;
  onRefresh: () => void;
  disabled: boolean;
  error?: string | null;
  invalid: boolean;
  inputRef: Ref<HTMLInputElement>;
  // Tab indexes of the image button and the input, in the order of the form.
  tabIndex: [number, number];
}

export function CaptchaField({
  value,
  onChange,
  refreshKey,
  onRefresh,
  disabled,
  error,
  invalid,
  inputRef,
  tabIndex,
}: CaptchaFieldProps) {
  const { t } = useTranslation();
  // The browser shows an image it already loaded for the same URL without
  // asking the server again, whose text may have been used up since, e.g.,
  // when signing in right after signing up. So every mount gets its own URL.
  const [mount] = useState(() => ++mounts);
  return (
    <div className="flex flex-col gap-2">
      <Label htmlFor="captcha">{t("captcha")}</Label>
      <div className="group relative">
        <button
          type="button"
          tabIndex={tabIndex[0]}
          disabled={disabled}
          onClick={onRefresh}
          aria-label={t("refresh_captcha")}
          className="block w-full cursor-pointer overflow-hidden rounded-md border border-(--color-border) bg-(--color-surface) outline-none focus-visible:ring-1 focus-visible:ring-(--color-ring) disabled:cursor-not-allowed disabled:opacity-60"
        >
          <img
            src={`${subUrl("/captcha/image.jpeg")}?refresh=true&m=${mount}&v=${refreshKey}`}
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
        ref={inputRef}
        id="captcha"
        name="captcha"
        type="text"
        autoComplete="off"
        required
        tabIndex={tabIndex[1]}
        placeholder={t("captcha_placeholder")}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        aria-invalid={invalid ? true : undefined}
        aria-describedby={error ? "captcha-error" : undefined}
      />
      {error && (
        <p id="captcha-error" className="text-sm text-(--color-destructive)">
          {error}
        </p>
      )}
    </div>
  );
}
