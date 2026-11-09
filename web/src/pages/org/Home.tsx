import { ChevronRight, Link as LinkIcon, MapPin, Plus, Settings } from "lucide-react";
import type { ReactNode } from "react";
import { useTranslation } from "react-i18next";

import { Pagination } from "@/components/Pagination";
import { RepoListItem } from "@/components/RepoListItem";
import { Button } from "@/components/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import type { OrgProfileData, OwnerReposPage } from "@/lib/queries/user";
import { subUrl } from "@/lib/url";

// OrgHome is the home page of an organization, at the same path as the
// profiles of individual users.
export function OrgHome({ org, repos }: { org: OrgProfileData; repos: OwnerReposPage }) {
  const { t } = useTranslation();
  const orgLink = subUrl(`/org/${org.name}`);
  const isWebsiteLink = org.website ? /^https?:\/\//i.test(org.website) : false;

  return (
    <main className="flex-1">
      <div className="border-b border-(--color-border)">
        <div className="mx-auto flex w-full max-w-6xl items-start gap-4 px-4 py-6 sm:px-6">
          <img
            src={org.avatarURL}
            alt=""
            className="size-16 shrink-0 rounded-md border border-(--color-border) object-cover sm:size-25"
          />
          <div className="min-w-0">
            <div className="flex items-center gap-2">
              <h1 className="min-w-0 text-2xl/[30px] font-semibold break-words">{org.fullName || org.name}</h1>
              {org.viewerIsOwner ? (
                <Tooltip>
                  <TooltipTrigger asChild>
                    <a
                      href={`${orgLink}/settings`}
                      aria-label={t("org.settings")}
                      className="grid size-8 shrink-0 place-items-center rounded-md text-(--color-muted-foreground) hover:bg-(--color-surface) hover:text-(--color-foreground)"
                    >
                      <Settings className="size-4" aria-hidden />
                    </a>
                  </TooltipTrigger>
                  <TooltipContent>{t("org.settings")}</TooltipContent>
                </Tooltip>
              ) : null}
            </div>
            {org.description ? (
              <p className="mt-1 break-words text-(--color-muted-foreground)">{org.description}</p>
            ) : null}
            {org.location || org.website ? (
              <ul className="mt-2 flex flex-wrap gap-x-4 gap-y-1 text-sm text-(--color-muted-foreground)">
                {org.location ? (
                  <Meta icon={<MapPin className="size-4 shrink-0" aria-hidden />} label={t("settings.location")}>
                    {org.location}
                  </Meta>
                ) : null}
                {org.website ? (
                  <Meta icon={<LinkIcon className="size-4 shrink-0" aria-hidden />} label={t("website")}>
                    {isWebsiteLink ? (
                      <a
                        href={org.website}
                        target="_blank"
                        rel="noopener noreferrer nofollow"
                        className="text-(--color-primary) hover:underline"
                      >
                        {org.website}
                      </a>
                    ) : (
                      org.website
                    )}
                  </Meta>
                ) : null}
              </ul>
            ) : null}
          </div>
        </div>
      </div>

      <div className="mx-auto grid w-full max-w-6xl gap-6 px-4 py-6 sm:px-6 md:grid-cols-[minmax(0,1fr)_256px] lg:grid-cols-[minmax(0,1fr)_296px]">
        <section className="min-w-0" aria-label={t("user.repositories")}>
          {org.viewerIsOwner ? (
            <div className="mb-4 flex justify-end">
              <Button asChild variant="outline" size="sm">
                <a href={subUrl(`/repo/create?org=${org.id}`)}>
                  <Plus className="size-4" aria-hidden />
                  {t("new_repo")}
                </a>
              </Button>
            </div>
          ) : null}
          <ul className="border-t border-(--color-border)">
            {repos.items.map((repo) => (
              <RepoListItem key={repo.name} repo={repo} />
            ))}
          </ul>
          {repos.totalPages > 1 ? (
            <div className="mt-4">
              <Pagination page={repos.page} totalPages={repos.totalPages} />
            </div>
          ) : null}
        </section>

        <aside className="min-w-0">
          {org.members.length > 0 || org.viewerIsOwner ? (
            <section>
              <SectionHeader
                title={t("org.people")}
                count={org.memberCount}
                countHref={`${orgLink}/members`}
                countLabel={`${org.memberCount} ${t("org.lower_members")}`}
              />
              <ul className="flex flex-wrap gap-1.5">
                {org.members.map((member) => (
                  <li key={member.name}>
                    <Tooltip>
                      <TooltipTrigger asChild>
                        <a href={subUrl(`/${member.name}`)} aria-label={member.name} className="block rounded-full">
                          <img
                            src={member.avatarURL}
                            alt=""
                            className="size-8 rounded-full border border-(--color-border) object-cover"
                          />
                        </a>
                      </TooltipTrigger>
                      <TooltipContent>
                        {member.fullName ? `${member.name} (${member.fullName})` : member.name}
                      </TooltipContent>
                    </Tooltip>
                  </li>
                ))}
              </ul>
              {org.viewerIsOwner ? (
                <Button asChild variant="outline" size="sm" className="mt-3">
                  <a href={`${orgLink}/invitations/new`}>{t("org.invite_someone")}</a>
                </Button>
              ) : null}
            </section>
          ) : null}

          {org.teams ? (
            <section className="mt-4 border-t border-(--color-border) pt-4">
              <SectionHeader
                title={t("org.teams")}
                count={org.teamCount}
                countHref={`${orgLink}/teams`}
                countLabel={`${org.teamCount} ${t("org.teams")}`}
              />
              <ul className="grid gap-3">
                {org.teams.map((team) => (
                  <li key={team.lowerName} className="text-sm">
                    <a
                      href={`${orgLink}/teams/${team.lowerName}`}
                      className="font-semibold wrap-anywhere text-(--color-primary) hover:underline"
                    >
                      {team.name}
                    </a>
                    <p className="text-(--color-muted-foreground)">
                      {team.memberCount} {t("org.lower_members")} · {team.repoCount} {t("org.lower_repositories")}
                    </p>
                  </li>
                ))}
              </ul>
              {org.viewerIsOwner ? (
                <Button asChild variant="outline" size="sm" className="mt-3">
                  <a href={`${orgLink}/teams/new`}>{t("org.create_new_team")}</a>
                </Button>
              ) : null}
            </section>
          ) : null}
        </aside>
      </div>
    </main>
  );
}

// SectionHeader titles a sidebar section. The count, only sent to members,
// links to the full list.
function SectionHeader({
  title,
  count,
  countHref,
  countLabel,
}: {
  title: string;
  count?: number;
  countHref: string;
  countLabel: string;
}) {
  return (
    <div className="mb-2 flex items-center justify-between gap-2">
      <h2 className="text-sm font-semibold">{title}</h2>
      {count !== undefined ? (
        <a
          href={countHref}
          aria-label={countLabel}
          className="inline-flex items-center gap-0.5 rounded-md px-1.5 py-0.5 text-sm tabular-nums text-(--color-muted-foreground) hover:bg-(--color-surface) hover:text-(--color-foreground)"
        >
          {count}
          <ChevronRight className="size-4" aria-hidden />
        </a>
      ) : null}
    </div>
  );
}

function Meta({ icon, label, children }: { icon: ReactNode; label: string; children: ReactNode }) {
  return (
    <li className="flex min-w-0 items-center gap-1.5 wrap-anywhere">
      {icon}
      <span className="sr-only">{label}</span>
      {children}
    </li>
  );
}
