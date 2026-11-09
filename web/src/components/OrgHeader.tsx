import { Link } from "@tanstack/react-router";
import { User, Users } from "lucide-react";
import { useTranslation } from "react-i18next";

import type { OrgProfileData } from "@/lib/queries/user";
import { cn } from "@/lib/utils";

// OrgHeader heads the pages of an organization that only its members see,
// with tabs for its people and its teams.
export function OrgHeader({ org, activeTab }: { org: OrgProfileData; activeTab: "people" | "teams" }) {
  const { t } = useTranslation();
  const tabs = [
    { key: "people", to: "/org/$org/members", icon: User, label: t("org.people"), count: org.memberCount },
    { key: "teams", to: "/org/$org/teams", icon: Users, label: t("org.teams"), count: org.teamCount },
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
        <nav aria-label={org.fullName || org.name} className="-mb-px flex overflow-x-auto">
          {tabs.map(({ key, to, icon: Icon, label, count }) => {
            const active = key === activeTab;
            return (
              <Link
                key={key}
                to={to}
                params={{ org: org.name }}
                aria-current={active ? "page" : undefined}
                className={cn(
                  "flex items-center gap-2 border-b-2 px-3 py-2 text-sm whitespace-nowrap",
                  active
                    ? "border-(--color-primary) font-semibold text-(--color-foreground)"
                    : "border-transparent text-(--color-muted-foreground) hover:border-(--color-border) hover:text-(--color-foreground)",
                )}
              >
                <Icon className="size-4" aria-hidden />
                {label}
                {count !== undefined ? (
                  <span className="rounded-full bg-(--color-secondary) px-2 text-xs/5 font-medium tabular-nums text-(--color-secondary-foreground)">
                    {count}
                  </span>
                ) : null}
              </Link>
            );
          })}
        </nav>
      </div>
    </div>
  );
}
