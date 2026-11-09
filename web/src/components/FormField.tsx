import { type AriaAttributes, type ReactElement, type ReactNode, cloneElement } from "react";

import { Checkbox } from "@/components/ui/checkbox";
import { Label } from "@/components/ui/label";

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
  helper?: string;
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

// CheckboxField is a checkbox with its label beside it.
export function CheckboxField({
  id,
  label,
  checked,
  onChange,
}: {
  id: string;
  label: ReactNode;
  checked: boolean;
  onChange: (checked: boolean) => void;
}) {
  return (
    <label htmlFor={id} className="flex cursor-pointer items-start gap-2.5 text-sm">
      <Checkbox id={id} className="mt-0.5" checked={checked} onCheckedChange={(c) => onChange(c === true)} />
      <span>{label}</span>
    </label>
  );
}
