import { useMutation } from "@tanstack/react-query";
import { type FormEvent, type ReactNode, useRef, useState } from "react";
import { useTranslation } from "react-i18next";

import { FormField } from "@/components/FormField";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { FormError } from "@/lib/queries/request";

// DeleteForm deletes an account once the viewer confirms their password.
export function DeleteForm({
  prompt,
  confirmLabel,
  remove,
  onDeleted,
}: {
  prompt: ReactNode;
  confirmLabel: string;
  remove: (password: string) => Promise<void>;
  onDeleted: () => Promise<void> | void;
}) {
  const { t } = useTranslation();
  const [password, setPassword] = useState("");
  const inputRef = useRef<HTMLInputElement>(null);
  const mutation = useMutation({
    mutationFn: () => remove(password),
    onSuccess: onDeleted,
    onError: () => requestAnimationFrame(() => inputRef.current?.focus()),
  });
  const fields = mutation.error instanceof FormError ? mutation.error.fields : {};
  const formError = mutation.error && Object.keys(fields).length === 0 ? mutation.error.message : null;

  const onSubmit = (e: FormEvent) => {
    e.preventDefault();
    mutation.mutate();
  };

  return (
    <form onSubmit={onSubmit} noValidate>
      <fieldset disabled={mutation.isPending} className="contents">
        <p
          role="note"
          className="mb-4 rounded-md border border-(--color-destructive) bg-(--color-destructive)/10 px-3 py-2 text-sm text-(--color-destructive)"
        >
          {prompt}
        </p>
        {formError ? (
          <div
            role="alert"
            className="mb-4 rounded-md border border-(--color-destructive) bg-(--color-destructive)/10 px-3 py-2 text-sm text-(--color-destructive)"
          >
            {formError}
          </div>
        ) : null}
        <div className="flex flex-col gap-5">
          <FormField id="password" label={t("password")} error={fields.password} invalid={"password" in fields}>
            <Input
              ref={inputRef}
              id="password"
              name="password"
              type="password"
              autoComplete="current-password"
              required
              autoFocus
              value={password}
              onChange={(e) => setPassword(e.target.value)}
            />
          </FormField>
          <div className="border-t border-(--color-border) pt-5">
            <Button type="submit" variant="destructive">
              {confirmLabel}
            </Button>
          </div>
        </div>
      </fieldset>
    </form>
  );
}
