import { useMutation, useQueryClient, useSuspenseQuery } from "@tanstack/react-query";
import { Link, useNavigate } from "@tanstack/react-router";
import { type ComponentProps, type FormEvent, type ReactElement, cloneElement, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";

import { OrgHeader } from "@/components/OrgHeader";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import { usePageTitle } from "@/lib/page-title";
import {
  FormError,
  type OrgTeamDetail,
  type OrgTeamForm,
  type TeamPermission,
  createOrgTeam,
  deleteOrgTeam,
  updateOrgTeam,
} from "@/lib/queries/org";
import { userProfileQuery } from "@/lib/queries/user";

const PERMISSIONS: TeamPermission[] = ["read", "write", "admin"];

// TeamForm creates a team, or changes or deletes one when given. Only owners
// get here.
export function TeamForm({ org: orgName, team }: { org: string; team?: OrgTeamDetail }) {
  const { t } = useTranslation();
  const { data: org } = useSuspenseQuery(userProfileQuery(orgName));
  const title = team ? t("org.teams.settings") : t("org.create_new_team");
  usePageTitle(`${team ? `${team.name} ${title}` : title} - ${org.fullName || org.name}`);
  const queryClient = useQueryClient();
  const navigate = useNavigate();

  const [form, setForm] = useState<OrgTeamForm>({
    teamName: team?.name ?? "",
    description: team?.description ?? "",
    permission: team && team.permission !== "owner" ? team.permission : "read",
  });
  const teamNameRef = useRef<HTMLInputElement>(null);
  const descriptionRef = useRef<HTMLInputElement>(null);

  const save = useMutation({
    mutationFn: () => (team ? updateOrgTeam(orgName, team.lowerName, form) : createOrgTeam(orgName, form)),
    onSuccess: async (lowerName) => {
      await queryClient.invalidateQueries({ queryKey: userProfileQuery(orgName).queryKey });
      await navigate({ to: "/org/$org/teams/$team", params: { org: orgName, team: lowerName } });
    },
    onError: (err) => {
      const fields = err instanceof FormError ? err.fields : {};
      requestAnimationFrame(() => {
        if ("teamName" in fields) teamNameRef.current?.focus();
        else if ("description" in fields) descriptionRef.current?.focus();
      });
    },
  });
  if (org.kind !== "organization") return null;

  const fieldErrors = save.error instanceof FormError ? save.error.fields : {};
  const formError = save.error && Object.keys(fieldErrors).length === 0 ? save.error.message : null;
  // The owners team keeps its name and its full access.
  const isOwnerTeam = team?.isOwnerTeam ?? false;

  const onSubmit = (e: FormEvent) => {
    e.preventDefault();
    save.mutate();
  };

  return (
    <main className="flex-1">
      <OrgHeader org={org} activeTab="teams" />
      <div className="mx-auto w-full max-w-6xl px-4 py-6 sm:px-6">
        <form onSubmit={onSubmit} noValidate className="max-w-xl">
          <fieldset disabled={save.isPending} className="contents">
            <h2 className="mb-4 text-xl font-semibold wrap-anywhere">
              {team ? (
                <>
                  <Link
                    to="/org/$org/teams/$team"
                    params={{ org: orgName, team: team.lowerName }}
                    className="text-(--color-primary) hover:underline"
                  >
                    {team.name}
                  </Link>
                  <span className="text-(--color-muted-foreground)" aria-hidden>
                    {" / "}
                  </span>
                </>
              ) : null}
              {title}
            </h2>
            {formError ? (
              <div
                role="alert"
                className="mb-4 rounded-md border border-(--color-destructive) bg-(--color-destructive)/10 px-3 py-2 text-sm text-(--color-destructive)"
              >
                {formError}
              </div>
            ) : null}
            <div className="flex flex-col gap-5">
              <Field
                id="teamName"
                label={t("org.team_name")}
                helper={t("org.team_name_helper")}
                error={fieldErrors.teamName}
                invalid={"teamName" in fieldErrors}
              >
                <Input
                  ref={teamNameRef}
                  id="teamName"
                  name="teamName"
                  autoComplete="off"
                  required
                  autoFocus={!isOwnerTeam}
                  readOnly={isOwnerTeam}
                  value={form.teamName}
                  onChange={(e) => setForm({ ...form, teamName: e.target.value })}
                  className={isOwnerTeam ? "bg-(--color-surface) text-(--color-muted-foreground)" : undefined}
                />
              </Field>
              <Field
                id="description"
                label={t("org.team_desc")}
                helper={t("org.team_desc_helper")}
                error={fieldErrors.description}
                invalid={"description" in fieldErrors}
              >
                <Input
                  ref={descriptionRef}
                  id="description"
                  name="description"
                  autoComplete="off"
                  autoFocus={isOwnerTeam}
                  value={form.description}
                  onChange={(e) => setForm({ ...form, description: e.target.value })}
                />
              </Field>
              {isOwnerTeam ? null : (
                <fieldset className="flex flex-col gap-3">
                  <legend className="mb-3 text-sm font-medium">{t("org.team_permission_desc")}</legend>
                  {PERMISSIONS.map((permission) => (
                    <label key={permission} className="flex cursor-pointer items-start gap-3">
                      <input
                        type="radio"
                        name="permission"
                        value={permission}
                        checked={form.permission === permission}
                        onChange={() => setForm({ ...form, permission })}
                        aria-describedby={`permission-${permission}-helper`}
                        className="mt-0.5 size-4 shrink-0 cursor-pointer accent-(--color-primary)"
                      />
                      <span className="text-sm">
                        <span className="font-medium">{t(`org.teams.${permission}_access`)}</span>
                        <span
                          id={`permission-${permission}-helper`}
                          className="mt-0.5 block text-(--color-muted-foreground)"
                        >
                          {t(`org.teams.${permission}_access_helper`)}
                        </span>
                      </span>
                    </label>
                  ))}
                </fieldset>
              )}
              <div className="flex flex-wrap items-center gap-2 border-t border-(--color-border) pt-5">
                <Button type="submit" variant="outline">
                  {team ? t("org.teams.update_settings") : t("org.create_new_team")}
                </Button>
                {team && !isOwnerTeam ? <DeleteTeamButton org={orgName} team={team} /> : null}
              </div>
            </div>
          </fieldset>
        </form>
      </div>
    </main>
  );
}

function Field({
  id,
  label,
  helper,
  error,
  invalid,
  children,
}: {
  id: string;
  label: string;
  helper: string;
  error?: string | null;
  invalid: boolean;
  children: ReactElement<ComponentProps<"input">>;
}) {
  const describedBy = error ? `${id}-error` : `${id}-helper`;
  return (
    <div className="flex flex-col gap-1.5">
      <Label htmlFor={id}>{label}</Label>
      {cloneElement(children, { "aria-invalid": invalid || undefined, "aria-describedby": describedBy })}
      {error ? (
        <p id={`${id}-error`} className="text-sm text-(--color-destructive)">
          {error}
        </p>
      ) : (
        <p id={`${id}-helper`} className="text-sm text-(--color-muted-foreground)">
          {helper}
        </p>
      )}
    </div>
  );
}

// DeleteTeamButton asks before deleting the team, since its members may lose
// access to its repositories.
function DeleteTeamButton({ org, team }: { org: string; team: OrgTeamDetail }) {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const [open, setOpen] = useState(false);
  const mutation = useMutation({
    mutationFn: () => deleteOrgTeam(org, team.lowerName),
    onSuccess: async () => {
      toast.success(t("org.teams.delete_team_success"));
      await queryClient.invalidateQueries({ queryKey: userProfileQuery(org).queryKey });
      await navigate({ to: "/org/$org/teams", params: { org } });
    },
    onError: (err) => toast.error(err.message),
  });
  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button type="button" variant="outline" className="text-(--color-destructive)">
          {t("org.teams.delete_team")}
        </Button>
      </PopoverTrigger>
      <PopoverContent align="start" collisionPadding={16} className="w-80 max-w-[calc(100vw-2rem)] p-4">
        <p className="font-semibold">{t("org.teams.delete_team_title")}</p>
        <p className="mt-1 text-sm text-(--color-muted-foreground)">{t("org.teams.delete_team_desc")}</p>
        <div className="mt-4 flex justify-end gap-2">
          <Button type="button" variant="outline" size="sm" onClick={() => setOpen(false)}>
            {t("cancel")}
          </Button>
          <Button
            type="button"
            variant="destructive"
            size="sm"
            disabled={mutation.isPending}
            onClick={() => mutation.mutate()}
          >
            {t("org.teams.delete_team_title")}
          </Button>
        </div>
      </PopoverContent>
    </Popover>
  );
}
