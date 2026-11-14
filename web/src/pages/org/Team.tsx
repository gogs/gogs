import { useSuspenseQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { Settings } from "lucide-react";
import { type FormEvent, useState } from "react";
import { useTranslation } from "react-i18next";

import { OrgHeader } from "@/components/OrgHeader";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { usePageTitle } from "@/lib/page-title";
import {
  type OrgTeamDetail,
  type OrgTeamMember,
  type OrgTeamRepo,
  addOrgTeamMember,
  addOrgTeamRepo,
  removeOrgTeamMember,
  removeOrgTeamRepo,
} from "@/lib/queries/org";
import { userProfileQuery } from "@/lib/queries/user";
import { subUrl } from "@/lib/url";
import { useOrgChange } from "@/lib/use-org-change";
import { TeamCounts, TeamMembershipButton } from "@/pages/org/Teams";
import { RepoIcon } from "@/pages/user/Dashboard";

export type TeamPageData =
  | { tab: "members"; team: OrgTeamDetail; members: OrgTeamMember[] }
  | { tab: "repositories"; team: OrgTeamDetail; repos: OrgTeamRepo[] };

// OrgTeam shows a team of an organization with its members or its
// repositories.
export function OrgTeam({ org: orgName, data }: { org: string; data: TeamPageData }) {
  const { t } = useTranslation();
  const { data: org } = useSuspenseQuery(userProfileQuery(orgName));
  usePageTitle(data.team.name);
  if (org.kind !== "organization") return null;

  const { team } = data;
  return (
    <main className="flex-1">
      <OrgHeader org={org} activeTab="teams" />
      <div className="mx-auto grid w-full max-w-6xl content-start gap-6 px-4 py-6 sm:px-6 md:grid-cols-[256px_minmax(0,1fr)] lg:grid-cols-[296px_minmax(0,1fr)]">
        <TeamSidebar org={org.name} team={team} viewerIsOwner={org.viewerIsOwner} />
        <section className="min-w-0">
          <h2 className="border-b border-(--color-border) pb-2 text-base font-semibold">
            {data.tab === "members" ? t("org.teams.members") : t("org.teams.repositories")}
          </h2>
          {data.tab === "members" ? (
            <TeamMembers org={org.name} team={team} members={data.members} viewerIsOwner={org.viewerIsOwner} />
          ) : (
            <TeamRepos
              org={org.name}
              team={team}
              repos={data.repos}
              // The owners team has every repository.
              canChange={org.viewerIsOwner && !team.isOwnerTeam}
            />
          )}
        </section>
      </div>
    </main>
  );
}

function TeamSidebar({ org, team, viewerIsOwner }: { org: string; team: OrgTeamDetail; viewerIsOwner: boolean }) {
  const { t } = useTranslation();
  return (
    <aside className="min-w-0 rounded-md border border-(--color-border)">
      <div className="flex items-center justify-between gap-3 border-b border-(--color-border) px-4 py-3">
        <h2 className="min-w-0 font-semibold wrap-anywhere">{team.name}</h2>
        <TeamMembershipButton org={org} team={team} viewerIsOwner={viewerIsOwner} />
      </div>
      <div className="grid gap-3 px-4 py-3 text-sm">
        {team.description ? (
          <p className="wrap-anywhere">{team.description}</p>
        ) : (
          <p className="text-(--color-muted-foreground) italic">{t("org.teams.no_desc")}</p>
        )}
        <TeamCounts org={org} team={team} />
        <p
          className="text-(--color-muted-foreground) [&_strong]:font-semibold [&_strong]:text-(--color-foreground)"
          // Sanitized on the server.
          dangerouslySetInnerHTML={{ __html: team.permissionHTML }}
        />
      </div>
      {viewerIsOwner ? (
        <div className="border-t border-(--color-border) px-4 py-3">
          <Button asChild variant="outline" size="sm">
            <a href={subUrl(`/org/${org}/teams/${team.lowerName}/edit`)}>
              <Settings className="size-4" aria-hidden />
              {t("org.teams.settings")}
            </a>
          </Button>
        </div>
      ) : null}
    </aside>
  );
}

function TeamMembers({
  org,
  team,
  members,
  viewerIsOwner,
}: {
  org: string;
  team: OrgTeamDetail;
  members: OrgTeamMember[];
  viewerIsOwner: boolean;
}) {
  const { t } = useTranslation();
  return (
    <>
      <ul>
        {members.map((member) => (
          <li key={member.name} className="flex items-center gap-3 border-b border-(--color-border) py-3">
            <img
              src={member.avatarURL}
              alt=""
              className="size-8 shrink-0 rounded-full border border-(--color-border) object-cover"
            />
            <p className="flex min-w-0 flex-1 flex-wrap items-baseline gap-x-2">
              <Link
                to="/$username"
                params={{ username: member.name }}
                search={{}}
                className="font-semibold wrap-anywhere text-(--color-primary) hover:underline"
              >
                {member.name}
              </Link>
              {member.fullName ? (
                <span className="text-sm wrap-anywhere text-(--color-muted-foreground)">{member.fullName}</span>
              ) : null}
            </p>
            {viewerIsOwner ? (
              <RemoveButton
                org={org}
                label={`${t("org.members.remove")} ${member.name}`}
                change={() => removeOrgTeamMember(org, team.lowerName, member.name)}
              />
            ) : null}
          </li>
        ))}
      </ul>
      {viewerIsOwner ? (
        <AddForm
          org={org}
          label={t("org.teams.add_team_member")}
          placeholder={t("repo.settings.search_user_placeholder")}
          add={(name) => addOrgTeamMember(org, team.lowerName, name)}
        />
      ) : null}
    </>
  );
}

function TeamRepos({
  org,
  team,
  repos,
  canChange,
}: {
  org: string;
  team: OrgTeamDetail;
  repos: OrgTeamRepo[];
  canChange: boolean;
}) {
  const { t } = useTranslation();
  return (
    <>
      <ul>
        {repos.map((repo) => (
          <li key={repo.name} className="flex items-center gap-3 border-b border-(--color-border) py-3">
            <RepoIcon repo={repo} />
            <a
              href={subUrl(`/${org}/${repo.name}`)}
              className="min-w-0 flex-1 font-semibold wrap-anywhere text-(--color-primary) hover:underline"
            >
              {org}/{repo.name}
            </a>
            {canChange ? (
              <RemoveButton
                org={org}
                label={`${t("org.teams.remove_repo")} ${org}/${repo.name}`}
                change={() => removeOrgTeamRepo(org, team.lowerName, repo.name)}
              />
            ) : null}
          </li>
        ))}
      </ul>
      {canChange ? (
        <AddForm
          org={org}
          label={t("org.teams.add_team_repository")}
          placeholder={t("org.teams.search_repo_placeholder")}
          // Like the server-rendered form, accept "owner/name" as well as a name.
          add={(name) => addOrgTeamRepo(org, team.lowerName, name.slice(name.lastIndexOf("/") + 1))}
        />
      ) : null}
    </>
  );
}

function RemoveButton({ org, label, change }: { org: string; label: string; change: () => Promise<void> }) {
  const { t } = useTranslation();
  const mutation = useOrgChange(org, change);
  return (
    <Button
      variant="outline"
      size="sm"
      aria-label={label}
      className="shrink-0 text-(--color-destructive)"
      disabled={mutation.isPending}
      onClick={() => mutation.mutate()}
    >
      {t("org.members.remove")}
    </Button>
  );
}

function AddForm({
  org,
  label,
  placeholder,
  add,
}: {
  org: string;
  label: string;
  placeholder: string;
  add: (name: string) => Promise<void>;
}) {
  const [name, setName] = useState("");
  const mutation = useOrgChange(org, () => add(name.trim()));
  const onSubmit = (e: FormEvent) => {
    e.preventDefault();
    if (!name.trim()) return;
    mutation.mutate(undefined, { onSuccess: () => setName("") });
  };
  return (
    <form onSubmit={onSubmit} className="mt-4">
      <fieldset disabled={mutation.isPending} className="flex flex-wrap gap-2">
        <Input
          aria-label={placeholder}
          placeholder={placeholder}
          value={name}
          onChange={(e) => setName(e.target.value)}
          autoComplete="off"
          required
          className="h-8 min-w-0 flex-1 basis-48 py-1"
        />
        <Button type="submit" variant="outline" size="sm">
          {label}
        </Button>
      </fieldset>
    </form>
  );
}
