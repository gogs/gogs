import { useQueryClient, useSuspenseQuery } from "@tanstack/react-query";
import { Link, useRouter } from "@tanstack/react-router";
import { Plus } from "lucide-react";
import { useTranslation } from "react-i18next";

import { OrgHeader } from "@/components/OrgHeader";
import { Button } from "@/components/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { usePageTitle } from "@/lib/page-title";
import { type OrgTeamSummary, addOrgTeamMember, removeOrgTeamMember } from "@/lib/queries/org";
import { userProfileQuery } from "@/lib/queries/user";
import { subUrl } from "@/lib/url";
import { useOrgChange } from "@/lib/use-org-change";
import { useUserInfo } from "@/lib/use-user-info";
import { cn } from "@/lib/utils";

// OrgTeams lists the teams of an organization: every team for owners, their
// own teams for other members.
export function OrgTeams({ org: orgName, teams }: { org: string; teams: OrgTeamSummary[] }) {
  const { t } = useTranslation();
  const { data: org } = useSuspenseQuery(userProfileQuery(orgName));
  usePageTitle(org.fullName || org.name);
  if (org.kind !== "organization") return null;

  return (
    <main className="flex-1">
      <OrgHeader org={org} activeTab="teams" />
      <div className="mx-auto w-full max-w-6xl px-4 py-6 sm:px-6">
        {org.viewerIsOwner ? (
          <div className="mb-4 flex justify-end">
            <Button asChild variant="outline" size="sm">
              <a href={subUrl(`/org/${org.name}/teams/new`)}>
                <Plus className="size-4" aria-hidden />
                {t("org.create_new_team")}
              </a>
            </Button>
          </div>
        ) : null}
        {teams.length > 0 ? (
          <ul className="grid gap-4 md:grid-cols-2">
            {teams.map((team) => (
              <TeamCard key={team.lowerName} org={org.name} team={team} viewerIsOwner={org.viewerIsOwner} />
            ))}
          </ul>
        ) : (
          <p className="py-8 text-center text-sm text-(--color-muted-foreground)">{t("repo.pulls.no_results")}</p>
        )}
      </div>
    </main>
  );
}

function TeamCard({ org, team, viewerIsOwner }: { org: string; team: OrgTeamSummary; viewerIsOwner: boolean }) {
  return (
    <li className="flex flex-col rounded-md border border-(--color-border)">
      <div className="flex items-center justify-between gap-3 border-b border-(--color-border) px-4 py-3">
        <Link
          to="/org/$org/teams/$team"
          params={{ org, team: team.lowerName }}
          className="min-w-0 font-semibold wrap-anywhere hover:underline"
        >
          {team.name}
        </Link>
        <TeamMembershipButton org={org} team={team} viewerIsOwner={viewerIsOwner} />
      </div>
      <ul className="flex min-h-14 flex-1 flex-wrap content-start gap-1.5 px-4 py-3">
        {team.members.map((member) => (
          <li key={member.name}>
            <Tooltip>
              <TooltipTrigger asChild>
                <Link
                  to="/$username"
                  params={{ username: member.name }}
                  search={{}}
                  aria-label={member.name}
                  className="block rounded-full"
                >
                  <img
                    src={member.avatarURL}
                    alt=""
                    className="size-8 rounded-full border border-(--color-border) object-cover"
                  />
                </Link>
              </TooltipTrigger>
              <TooltipContent>{member.fullName ? `${member.name} (${member.fullName})` : member.name}</TooltipContent>
            </Tooltip>
          </li>
        ))}
      </ul>
      <TeamCounts org={org} team={team} className="border-t border-(--color-border) px-4 py-2" />
    </li>
  );
}

// TeamCounts links to the members and the repositories of a team.
export function TeamCounts({
  org,
  team,
  className,
}: {
  org: string;
  team: { lowerName: string; memberCount: number; repoCount: number };
  className?: string;
}) {
  const { t } = useTranslation();
  return (
    <p className={cn("text-sm text-(--color-muted-foreground)", className)}>
      <Link
        to="/org/$org/teams/$team"
        params={{ org, team: team.lowerName }}
        className="hover:text-(--color-primary) hover:underline"
      >
        <strong className="font-semibold text-(--color-foreground) tabular-nums">{team.memberCount}</strong>{" "}
        {t("org.lower_members")}
      </Link>
      {" · "}
      <Link
        to="/org/$org/teams/$team/repositories"
        params={{ org, team: team.lowerName }}
        className="hover:text-(--color-primary) hover:underline"
      >
        <strong className="font-semibold text-(--color-foreground) tabular-nums">{team.repoCount}</strong>{" "}
        {t("org.lower_repositories")}
      </Link>
    </p>
  );
}

// TeamMembershipButton lets members leave a team, and owners join one, like
// the server-rendered pages.
export function TeamMembershipButton({
  org,
  team,
  viewerIsOwner,
}: {
  org: string;
  team: { name: string; lowerName: string; viewerIsMember: boolean };
  viewerIsOwner: boolean;
}) {
  const { t } = useTranslation();
  const user = useUserInfo();
  const queryClient = useQueryClient();
  const router = useRouter();
  const mutation = useOrgChange(
    org,
    () =>
      team.viewerIsMember
        ? removeOrgTeamMember(org, team.lowerName, user?.username ?? "")
        : addOrgTeamMember(org, team.lowerName, user?.username ?? ""),
    // Members other than owners only see their own teams, and owners who
    // leave the owners team are no longer owners, so the team page may be
    // gone once the viewer leaves.
    team.viewerIsMember
      ? async () => {
          await queryClient.invalidateQueries({ queryKey: userProfileQuery(org).queryKey });
          await router.navigate({ to: "/org/$org/teams", params: { org } });
          await router.invalidate();
        }
      : undefined,
  );
  if (!user || (!team.viewerIsMember && !viewerIsOwner)) return null;

  const label = team.viewerIsMember ? t("org.teams.leave") : t("org.teams.join");
  return (
    <Button
      variant="outline"
      size="sm"
      aria-label={`${label} ${team.name}`}
      className="shrink-0"
      disabled={mutation.isPending}
      onClick={() => mutation.mutate()}
    >
      {label}
    </Button>
  );
}
