import { Link } from "@tanstack/react-router";
import type { ReactNode } from "react";
import { useTranslation } from "react-i18next";

import { SettingsLayout, SettingsNavItem } from "@/components/SettingsLayout";
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
  | "monitor";

// NAV lists the pages of the admin panel.
const NAV: { page: AdminPage; path: string; label: string }[] = [
  { page: "dashboard", path: "", label: "admin.dashboard" },
  { page: "operations", path: "/operations", label: "admin.dashboard.operations" },
  { page: "users", path: "/users", label: "admin.users" },
  { page: "orgs", path: "/orgs", label: "admin.organizations" },
  { page: "repos", path: "/repos", label: "admin.repositories" },
  { page: "auths", path: "/auths", label: "admin.authentication" },
  { page: "config", path: "/config", label: "admin.config" },
  { page: "notices", path: "/notices", label: "admin.notices" },
  { page: "monitor", path: "/monitor", label: "admin.monitor" },
];

// AdminLayout heads the pages of the admin panel, with links between them.
// Lists are wide, forms are not.
export function AdminLayout({
  active,
  wide = false,
  children,
}: {
  active: AdminPage;
  wide?: boolean;
  children: ReactNode;
}) {
  const { t } = useTranslation();
  const current = t(NAV.find((item) => item.page === active)?.label ?? "admin_panel");
  usePageTitle(current);
  return (
    <main className="flex-1">
      <SettingsLayout
        label={t("admin_panel")}
        current={current}
        wide={wide}
        nav={NAV.map((item) => (
          <SettingsNavItem key={item.page} active={item.page === active}>
            <Link to={`/admin${item.path}`} activeOptions={{ exact: true }}>
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

// AdminListHeading names a list of the admin panel with the number of all its
// items, and holds an action for the list, e.g., creating an item.
export function AdminListHeading({ title, total, action }: { title: string; total: number; action?: ReactNode }) {
  const { t } = useTranslation();
  return (
    <div className="mb-4 flex flex-wrap items-end justify-between gap-3">
      <div>
        <h1 className="text-xl font-semibold">{title}</h1>
        <p className="mt-1 text-sm text-(--color-muted-foreground)">
          {t("admin.total").replace(/%(?:\[1\])?d/, String(total))}
        </p>
      </div>
      {action}
    </div>
  );
}
