import { Eye, EyeOff } from "lucide-react";
import { useTranslation } from "react-i18next";

import { Input } from "@/components/ui/input";

export function PasswordInput({
  inputRef,
  id,
  value,
  tabIndex,
  placeholder,
  show,
  onToggleShow,
  disabled,
  describedBy,
  invalid,
  autoFocus,
  onChange,
}: {
  inputRef: React.RefObject<HTMLInputElement | null>;
  id: string;
  value: string;
  tabIndex: number;
  placeholder: string;
  show: boolean;
  onToggleShow: () => void;
  disabled: boolean;
  describedBy?: string;
  invalid: boolean;
  autoFocus?: boolean;
  onChange: (value: string) => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="relative">
      <Input
        ref={inputRef}
        id={id}
        name={id}
        type={show ? "text" : "password"}
        autoComplete="new-password"
        required
        autoFocus={autoFocus}
        tabIndex={tabIndex}
        placeholder={placeholder}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        aria-invalid={invalid ? true : undefined}
        aria-describedby={describedBy}
        className="pr-10"
      />
      <button
        type="button"
        tabIndex={tabIndex + 1}
        disabled={disabled}
        onClick={onToggleShow}
        aria-label={show ? t("auth.hide_password") : t("auth.show_password")}
        aria-pressed={show}
        className="absolute inset-y-0 right-0 flex w-10 cursor-pointer items-center justify-center rounded-r-md text-(--color-muted-foreground) outline-none hover:text-(--color-foreground) focus-visible:text-(--color-foreground) focus-visible:ring-1 focus-visible:ring-(--color-ring) disabled:cursor-not-allowed disabled:opacity-50"
      >
        {show ? <EyeOff className="size-4" aria-hidden /> : <Eye className="size-4" aria-hidden />}
      </button>
    </div>
  );
}
