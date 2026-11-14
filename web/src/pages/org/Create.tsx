import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { type FormEvent, useRef, useState } from "react";
import { useTranslation } from "react-i18next";

import { FormField } from "@/components/FormField";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { usePageTitle } from "@/lib/page-title";
import { FormError, createOrg } from "@/lib/queries/org";

// OrgCreate creates an organization owned by the viewer.
export function OrgCreate() {
  const { t } = useTranslation();
  usePageTitle(t("new_org"));
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const [orgName, setOrgName] = useState("");
  const inputRef = useRef<HTMLInputElement>(null);
  const mutation = useMutation({
    mutationFn: () => createOrg(orgName.trim()),
    onSuccess: async (name) => {
      // The dashboards list the organizations of the viewer.
      await queryClient.invalidateQueries();
      await navigate({ to: "/org/$org/dashboard", params: { org: name } });
    },
    onError: () => requestAnimationFrame(() => inputRef.current?.focus()),
  });

  const fields = mutation.error instanceof FormError ? mutation.error.fields : {};
  const formError = mutation.error && Object.keys(fields).length === 0 ? mutation.error.message : null;
  const onSubmit = (e: FormEvent) => {
    e.preventDefault();
    mutation.mutate();
  };

  return (
    <main className="mx-auto w-full max-w-xl flex-1 px-4 py-10 sm:px-6">
      <form onSubmit={onSubmit} noValidate>
        <fieldset disabled={mutation.isPending} className="contents">
          <h1 className="mb-4 text-xl font-semibold">{t("new_org")}</h1>
          {formError ? (
            <div
              role="alert"
              className="mb-4 rounded-md border border-(--color-destructive) bg-(--color-destructive)/10 px-3 py-2 text-sm text-(--color-destructive)"
            >
              {formError}
            </div>
          ) : null}
          <div className="flex flex-col gap-5">
            <FormField
              id="orgName"
              label={t("org.org_name_holder")}
              helper={t("org.org_name_helper")}
              error={fields.orgName}
              invalid={"orgName" in fields}
            >
              <Input
                ref={inputRef}
                id="orgName"
                name="orgName"
                autoComplete="off"
                autoFocus
                required
                value={orgName}
                onChange={(e) => setOrgName(e.target.value)}
              />
            </FormField>
            <div className="border-t border-(--color-border) pt-5">
              <Button type="submit" variant="outline">
                {t("org.create_org")}
              </Button>
            </div>
          </div>
        </fieldset>
      </form>
    </main>
  );
}
