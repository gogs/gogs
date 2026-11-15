import { Link } from "@tanstack/react-router";
import type { ReactNode } from "react";
import { useTranslation } from "react-i18next";

import { SettingsLayout, SettingsNavItem } from "@/components/SettingsLayout";
import { usePageTitle } from "@/lib/page-title";

export type UserSettingsPage =
  | "profile"
  | "avatar"
  | "password"
  | "email"
  | "ssh"
  | "security"
  | "sessions"
  | "repositories"
  | "organizations"
  | "applications"
  | "delete";

// NAV lists the settings pages in the order of the server-rendered menu.
const NAV: { page: UserSettingsPage; path: string; label: string }[] = [
  { page: "profile", path: "", label: "settings.profile" },
  { page: "avatar", path: "/avatar", label: "settings.avatar" },
  { page: "password", path: "/password", label: "settings.password" },
  { page: "email", path: "/email", label: "settings.emails" },
  { page: "ssh", path: "/ssh", label: "settings.ssh_keys" },
  { page: "security", path: "/security", label: "settings.security" },
  { page: "sessions", path: "/sessions", label: "settings.sessions" },
  { page: "repositories", path: "/repositories", label: "settings.repos" },
  { page: "organizations", path: "/organizations", label: "settings.orgs" },
  { page: "applications", path: "/applications", label: "settings.applications" },
  { page: "delete", path: "/delete", label: "settings.delete" },
];

// UserSettingsLayout heads the settings pages of the viewer, with links
// between them. Pages under a settings page, such as enabling two-factor
// authentication under security, mark that page as the current one.
export function UserSettingsLayout({ active, children }: { active: UserSettingsPage; children: ReactNode }) {
  const { t } = useTranslation();
  const current = t(NAV.find((item) => item.page === active)?.label ?? "settings");
  usePageTitle(current);
  return (
    <main className="flex-1">
      <SettingsLayout
        label={t("settings")}
        current={current}
        nav={NAV.map((item) => (
          <SettingsNavItem key={item.page} active={item.page === active} destructive={item.page === "delete"}>
            <Link to={`/user/settings${item.path}`} activeOptions={{ exact: true }}>
              {t(item.label)}
            </Link>
          </SettingsNavItem>
        ))}
      >
        {children}
      </SettingsLayout>
    </main>
  );
}
