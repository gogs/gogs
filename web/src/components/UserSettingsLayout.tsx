import { Link } from "@tanstack/react-router";
import {
  BookMarked,
  Building2,
  Key,
  KeyRound,
  type LucideIcon,
  Mail,
  ShieldCheck,
  TriangleAlert,
  User,
} from "lucide-react";
import type { ReactNode } from "react";
import { useTranslation } from "react-i18next";

import { SettingsLayout, SettingsNavDivider, SettingsNavGroup, SettingsNavItem } from "@/components/SettingsLayout";
import { usePageTitle } from "@/lib/page-title";

export type UserSettingsPage =
  | "profile"
  | "email"
  | "ssh"
  | "security"
  | "repositories"
  | "organizations"
  | "applications"
  | "delete";

type NavItem = { page: UserSettingsPage; path: string; label: string; icon: LucideIcon };

// NAV lists the settings pages in groups. The first group has no heading,
// and the danger zone sits apart at the end.
const NAV: { title?: string; items: NavItem[] }[] = [
  { items: [{ page: "profile", path: "", label: "settings.profile", icon: User }] },
  {
    title: "settings.group_access",
    items: [
      { page: "email", path: "/email", label: "settings.emails", icon: Mail },
      { page: "security", path: "/security", label: "settings.authentication", icon: ShieldCheck },
      { page: "ssh", path: "/ssh", label: "settings.ssh_keys", icon: KeyRound },
    ],
  },
  {
    title: "settings.group_integrations",
    items: [{ page: "applications", path: "/applications", label: "settings.access_tokens", icon: Key }],
  },
  {
    title: "settings.group_memberships",
    items: [
      { page: "repositories", path: "/repositories", label: "settings.repos", icon: BookMarked },
      { page: "organizations", path: "/organizations", label: "settings.orgs", icon: Building2 },
    ],
  },
];

const DANGER_ZONE = "repo.settings.danger_zone";

// UserSettingsLayout heads the settings pages of the viewer, with links
// between them. Pages under a settings page, such as enabling two-factor
// authentication under security, mark that page as the current one.
export function UserSettingsLayout({ active, children }: { active: UserSettingsPage; children: ReactNode }) {
  const { t } = useTranslation();
  const label =
    active === "delete" ? DANGER_ZONE : NAV.flatMap((group) => group.items).find((item) => item.page === active)?.label;
  const current = t(label ?? "settings");
  usePageTitle(current);
  return (
    <main className="flex-1">
      <SettingsLayout
        label={t("settings")}
        current={current}
        nav={
          <>
            {NAV.map((group, i) => (
              <SettingsNavGroup key={i} title={group.title ? t(group.title) : undefined}>
                {group.items.map((item) => (
                  <SettingsNavItem key={item.page} active={item.page === active} icon={item.icon}>
                    <Link to={`/user/settings${item.path}`} activeOptions={{ exact: true }}>
                      {t(item.label)}
                    </Link>
                  </SettingsNavItem>
                ))}
              </SettingsNavGroup>
            ))}
            <SettingsNavDivider />
            <SettingsNavItem active={active === "delete"} icon={TriangleAlert} destructive>
              <Link to="/user/settings/delete" activeOptions={{ exact: true }}>
                {t(DANGER_ZONE)}
              </Link>
            </SettingsNavItem>
          </>
        }
      >
        {children}
      </SettingsLayout>
    </main>
  );
}
