import { type AriaAttributes, type ReactElement, type ReactNode, cloneElement } from "react";

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
