import { useMutation, useQueryClient, useSuspenseQuery } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { type FormEvent, useRef, useState } from "react";
import { useTranslation } from "react-i18next";

import { OrgHeader } from "@/components/OrgHeader";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { usePageTitle } from "@/lib/page-title";
import { inviteOrgMember } from "@/lib/queries/org";
import { userProfileQuery } from "@/lib/queries/user";

// OrgInvite adds a user to an organization. Only owners get here.
export function OrgInvite({ org: orgName }: { org: string }) {
  const { t } = useTranslation();
  const { data: org } = useSuspenseQuery(userProfileQuery(orgName));
  usePageTitle(`${t("org.invite_someone")} - ${org.fullName || org.name}`);
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const [username, setUsername] = useState("");
  const inputRef = useRef<HTMLInputElement>(null);
  const mutation = useMutation({
    mutationFn: () => inviteOrgMember(orgName, username.trim()),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: userProfileQuery(orgName).queryKey });
      await navigate({ to: "/org/$org/members", params: { org: orgName } });
    },
    onError: () => requestAnimationFrame(() => inputRef.current?.focus()),
  });
  if (org.kind !== "organization") return null;

  const onSubmit = (e: FormEvent) => {
    e.preventDefault();
    if (username.trim()) mutation.mutate();
  };
  const error = mutation.error?.message;

  return (
    <main className="flex-1">
      <OrgHeader org={org} activeTab="people" />
      <div className="mx-auto w-full max-w-6xl px-4 py-6 sm:px-6">
        <form onSubmit={onSubmit} noValidate className="max-w-xl">
          <fieldset disabled={mutation.isPending} className="contents">
            <h2 className="mb-4 text-xl font-semibold">{t("org.invite_someone")}</h2>
            <div className="flex flex-col gap-1.5">
              <Label htmlFor="username">{t("username")}</Label>
              <div className="flex flex-wrap gap-2">
                <Input
                  ref={inputRef}
                  id="username"
                  name="username"
                  autoComplete="off"
                  autoFocus
                  required
                  placeholder={t("repo.settings.search_user_placeholder")}
                  value={username}
                  onChange={(e) => setUsername(e.target.value)}
                  aria-invalid={error ? true : undefined}
                  aria-describedby={error ? "username-error" : undefined}
                  className="min-w-0 flex-1 basis-56"
                />
                <Button type="submit" variant="outline">
                  {t("org.members.invite_now")}
                </Button>
              </div>
              {error ? (
                <p id="username-error" className="text-sm text-(--color-destructive)">
                  {error}
                </p>
              ) : null}
            </div>
          </fieldset>
        </form>
      </div>
    </main>
  );
}
