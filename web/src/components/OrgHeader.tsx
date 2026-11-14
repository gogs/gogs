import { Link } from "@tanstack/react-router";
import { User, Users } from "lucide-react";
import { useTranslation } from "react-i18next";

import { PageTabs } from "@/components/PageTabs";
import type { OrgProfileData } from "@/lib/queries/user";

const ORG_TAB_PATHS = {
  people: "/org/$org/members",
  teams: "/org/$org/teams",
} as const;

// OrgHeader heads the pages of an organization that only its members see,
// with tabs for its people and its teams.
export function OrgHeader({ org, activeTab }: { org: OrgProfileData; activeTab: "people" | "teams" }) {
  const { t } = useTranslation();
  const tabs = [
    { key: "people", icon: User, label: t("org.people"), count: org.memberCount },
    { key: "teams", icon: Users, label: t("org.teams"), count: org.teamCount },
  ] as const;

  return (
    <div className="border-b border-(--color-border)">
      <div className="mx-auto w-full max-w-6xl px-4 pt-6 sm:px-6">
        <h1 className="flex min-w-0 items-center gap-3 pb-4">
          <img
            src={org.avatarURL}
            alt=""
            className="size-10 shrink-0 rounded-md border border-(--color-border) object-cover"
          />
          <Link
            to="/$username"
            params={{ username: org.name }}
            search={{}}
            className="min-w-0 text-xl font-semibold wrap-anywhere hover:underline"
          >
            {org.fullName || org.name}
          </Link>
        </h1>
        <PageTabs
          label={org.fullName || org.name}
          tabs={tabs}
          active={activeTab}
          // Both tabs with their counts fit at 320px.
          inline={2}
          renderLink={(tab, props) => <Link to={ORG_TAB_PATHS[tab.key]} params={{ org: org.name }} {...props} />}
        />
      </div>
    </div>
  );
}
