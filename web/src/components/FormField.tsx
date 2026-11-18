import { type AriaAttributes, type ReactElement, type ReactNode, cloneElement } from "react";

import { Checkbox } from "@/components/ui/checkbox";
import { Label } from "@/components/ui/label";
import { cn } from "@/lib/utils";

// FormField labels an input and shows its error, or its helper text when there
// is no error, wiring both to the input for assistive tech.
export function FormField({
  id,
  label,
  helper,
  error,
  invalid,
  children,
}: {
  id: string;
  label: ReactNode;
  helper?: ReactNode;
  error?: string | null;
  invalid: boolean;
  children: ReactElement<AriaAttributes>;
}) {
  const describedBy = error ? `${id}-error` : helper ? `${id}-helper` : undefined;
  return (
    <div className="flex flex-col gap-1.5">
      <Label htmlFor={id}>{label}</Label>
      {cloneElement(children, { "aria-invalid": invalid || undefined, "aria-describedby": describedBy })}
      {error ? (
        <p id={`${id}-error`} className="text-sm text-(--color-destructive)">
          {error}
        </p>
      ) : helper ? (
        <p id={`${id}-helper`} className="text-sm text-(--color-muted-foreground)">
          {helper}
        </p>
      ) : null}
    </div>
  );
}

// FormAlert shows the message for a whole form above its fields.
export function FormAlert({ message }: { message: string | null }) {
  if (!message) return null;
  return (
    <div
      role="alert"
      className="mb-4 rounded-md border border-(--color-destructive) bg-(--color-destructive)/10 px-3 py-2 text-sm text-(--color-destructive)"
    >
      {message}
    </div>
  );
}

// CheckboxField is a checkbox with its label beside it, and an optional
// description of what it does below the label.
export function CheckboxField({
  id,
  label,
  description,
  checked,
  onChange,
  disabled,
}: {
  id: string;
  label: ReactNode;
  description?: ReactNode;
  checked: boolean;
  onChange: (checked: boolean) => void;
  disabled?: boolean;
}) {
  return (
    <label
      htmlFor={id}
      className={cn("flex items-start gap-2.5 text-sm", disabled ? "cursor-not-allowed" : "cursor-pointer")}
    >
      <Checkbox
        id={id}
        className="mt-0.5"
        checked={checked}
        disabled={disabled}
        onCheckedChange={(c) => onChange(c === true)}
        aria-describedby={description ? `${id}-description` : undefined}
      />
      <span className="flex flex-col gap-0.5">
        <span className={description ? "font-medium" : undefined}>{label}</span>
        {description ? (
          <span id={`${id}-description`} className="text-(--color-muted-foreground)">
            {description}
          </span>
        ) : null}
      </span>
    </label>
  );
}

// RadioField is a radio button with its label beside it.
export function RadioField({
  id,
  name,
  checked,
  onChange,
  label,
  disabled,
  description,
  descriptionId,
}: {
  id: string;
  name: string;
  checked: boolean;
  onChange: () => void;
  label: ReactNode;
  disabled?: boolean;
  // description shows below the label, e.g., why the choice is disabled.
  description?: ReactNode;
  descriptionId?: string;
}) {
  return (
    <div className="flex flex-col gap-1">
      <label
        htmlFor={id}
        className={cn(
          "flex items-start gap-2.5 text-sm",
          disabled ? "cursor-not-allowed opacity-60" : "cursor-pointer",
        )}
      >
        <input
          id={id}
          type="radio"
          name={name}
          checked={checked}
          onChange={onChange}
          disabled={disabled}
          aria-describedby={description ? descriptionId : undefined}
          className="mt-0.5 size-4 shrink-0 cursor-pointer accent-(--color-primary) disabled:cursor-not-allowed"
        />
        <span>{label}</span>
      </label>
      {description ? (
        <p id={descriptionId} className="ml-6.5 text-sm text-(--color-destructive)">
          {description}
        </p>
      ) : null}
    </div>
  );
}
