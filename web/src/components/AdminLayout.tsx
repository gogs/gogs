import { Link } from "@tanstack/react-router";
import {
  Activity,
  Bell,
  BookMarked,
  Building2,
  Clock,
  HeartPulse,
  LayoutDashboard,
  type LucideIcon,
  Play,
  Settings,
  ShieldCheck,
  User,
} from "lucide-react";
import type { ReactNode } from "react";
import { useTranslation } from "react-i18next";

import { SettingsLayout, SettingsNavGroup, SettingsNavItem } from "@/components/SettingsLayout";
import { usePageTitle } from "@/lib/page-title";

export type AdminPage =
  | "dashboard"
  | "operations"
  | "users"
  | "orgs"
  | "repos"
  | "auths"
  | "config"
  | "notices"
  | "jobs"
  | "health"
  | "traces";

type NavItem = { page: AdminPage; path: string; label: string; icon: LucideIcon };

// NAV lists the pages of the admin panel in groups. The first group has no
// heading.
const NAV: { title?: string; items: NavItem[] }[] = [
  {
    items: [
      { page: "dashboard", path: "", label: "admin.overview", icon: LayoutDashboard },
      { page: "users", path: "/users", label: "admin.users", icon: User },
      { page: "orgs", path: "/orgs", label: "admin.organizations", icon: Building2 },
      { page: "repos", path: "/repos", label: "admin.repositories", icon: BookMarked },
    ],
  },
  {
    title: "admin.group_settings",
    items: [
      { page: "auths", path: "/auths", label: "admin.authentication", icon: ShieldCheck },
      { page: "config", path: "/config", label: "admin.config", icon: Settings },
    ],
  },
  {
    title: "admin.group_monitoring",
    items: [
      { page: "health", path: "/health", label: "admin.health", icon: HeartPulse },
      { page: "traces", path: "/traces", label: "admin.traces", icon: Activity },
      { page: "notices", path: "/notices", label: "admin.notices", icon: Bell },
    ],
  },
  {
    title: "admin.group_maintenance",
    items: [
      { page: "operations", path: "/operations", label: "admin.dashboard.operations", icon: Play },
      { page: "jobs", path: "/jobs", label: "admin.monitor.cron", icon: Clock },
    ],
  },
];

// AdminLayout heads the pages of the admin panel, with links between them.
export function AdminLayout({ active, children }: { active: AdminPage; children: ReactNode }) {
  const { t } = useTranslation();
  const label = NAV.flatMap((group) => group.items).find((item) => item.page === active)?.label;
  const current = t(label ?? "admin_panel");
  usePageTitle(current);
  return (
    <main className="flex-1">
      <SettingsLayout
        label={t("admin_panel")}
        current={current}
        nav={NAV.map((group, i) => (
          <SettingsNavGroup key={i} title={group.title ? t(group.title) : undefined}>
            {group.items.map((item) => (
              <SettingsNavItem key={item.page} active={item.page === active} icon={item.icon}>
                <Link to={`/admin${item.path}`} activeOptions={{ exact: true }}>
                  {t(item.label)}
                </Link>
              </SettingsNavItem>
            ))}
          </SettingsNavGroup>
        ))}
      >
        {children}
      </SettingsLayout>
    </main>
  );
}

// AdminListHeading names a list of the admin panel with the number of all its
// items, and holds an action for the list, e.g., creating an item.
export function AdminListHeading({ title, total, action }: { title: string; total: number; action?: ReactNode }) {
  const { t } = useTranslation();
  return (
    <div className="mb-4 flex flex-wrap items-end justify-between gap-3">
      <div>
        <h1 className="text-xl font-semibold">{title}</h1>
        <p className="mt-1 text-sm text-(--color-muted-foreground)">{t("admin.total", { count: total })}</p>
      </div>
      {action}
    </div>
  );
}
