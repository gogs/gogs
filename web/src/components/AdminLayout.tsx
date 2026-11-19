import { useQuery } from "@tanstack/react-query";
import { Link, Outlet, useRouterState } from "@tanstack/react-router";
import {
  Activity,
  ArrowLeft,
  Bell,
  BookMarked,
  Building2,
  ChevronRight,
  Clock,
  HeartPulse,
  LayoutDashboard,
  type LucideIcon,
  PanelLeft,
  Play,
  Settings,
  ShieldCheck,
  User,
} from "lucide-react";
import { type MouseEvent, type ReactNode, useRef, useState } from "react";
import { useTranslation } from "react-i18next";

import { PaletteButton, UserMenu } from "@/components/Navbar";
import { NoticeBanner } from "@/components/NoticeBanner";
import { SettingsMenu } from "@/components/SettingsMenu";
import { Sheet, SheetContent, SheetTitle } from "@/components/ui/sheet";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { AdminCrumbSlot } from "@/lib/admin-crumb-context";
import { formatNumber } from "@/lib/format";
import { usePageTitle } from "@/lib/page-title";
import { adminNoticesQuery } from "@/lib/queries/admin";
import { subUrl } from "@/lib/url";
import { useUserInfo } from "@/lib/use-user-info";
import { cn } from "@/lib/utils";

type NavItem = { path: string; label: string; icon: LucideIcon };

// NAV lists the pages of the admin console in groups. The first group has no
// heading. Pages below the path of an item, e.g., a user below the users,
// belong to the item.
const NAV: { title?: string; items: NavItem[] }[] = [
  {
    items: [
      { path: "", label: "admin.overview", icon: LayoutDashboard },
      { path: "/users", label: "admin.users", icon: User },
      { path: "/orgs", label: "admin.organizations", icon: Building2 },
      { path: "/repos", label: "admin.repositories", icon: BookMarked },
    ],
  },
  {
    title: "admin.group_settings",
    items: [
      { path: "/auths", label: "admin.authentication", icon: ShieldCheck },
      { path: "/config", label: "admin.config", icon: Settings },
    ],
  },
  {
    title: "admin.group_monitoring",
    items: [
      { path: "/health", label: "admin.health", icon: HeartPulse },
      { path: "/traces", label: "admin.traces", icon: Activity },
      { path: "/notices", label: "admin.notices", icon: Bell },
    ],
  },
  {
    title: "admin.group_maintenance",
    items: [
      { path: "/operations", label: "admin.dashboard.operations", icon: Play },
      { path: "/jobs", label: "admin.monitor.cron", icon: Clock },
    ],
  },
];

const COLLAPSED_KEY = "gogs-admin-sidebar-collapsed";

// Without a stored choice, the sidebar starts as a rail of icons on screens
// where it would leave the page narrow.
function readCollapsed() {
  try {
    const stored = window.localStorage.getItem(COLLAPSED_KEY);
    if (stored !== null) return stored === "true";
  } catch {
    // Storage may be unavailable, e.g., in a private window.
  }
  return window.matchMedia("(max-width: 1023px)").matches;
}

const ICON_BUTTON =
  "size-8 shrink-0 cursor-pointer items-center justify-center rounded-md text-(--color-muted-foreground) outline-none hover:bg-(--color-surface) hover:text-(--color-foreground) focus-visible:ring-1 focus-visible:ring-(--color-ring)";

// The current page gets a bar in the primary color beside its fill, so it does
// not rely on the subtle fill alone.
const NAV_LINK =
  "relative flex h-8 items-center gap-2 rounded-md px-2 text-sm text-(--color-muted-foreground) outline-none hover:bg-(--color-foreground)/5 hover:text-(--color-foreground) focus-visible:ring-1 focus-visible:ring-(--color-ring) aria-[current=page]:bg-(--color-foreground)/10 glass-ui:aria-[current=page]:bg-(--color-surface) aria-[current=page]:font-semibold aria-[current=page]:text-(--color-foreground) aria-[current=page]:before:absolute aria-[current=page]:before:inset-y-1.5 aria-[current=page]:before:-left-2 aria-[current=page]:before:w-1 aria-[current=page]:before:rounded-full aria-[current=page]:before:bg-(--color-primary)";

// Sidebar holds the navigation of the admin console. As a rail it shows only
// icons, each named by a tooltip.
function Sidebar({ rail }: { rail: boolean }) {
  const { t } = useTranslation();
  const user = useUserInfo();
  const notices = useQuery(adminNoticesQuery({}));
  const unread = notices.data?.total ?? 0;

  const link = (item: NavItem) => {
    const label = t(item.label);
    const count = item.path === "/notices" && unread > 0 ? formatNumber(unread) : null;
    const node = (
      <Link
        key={item.path}
        to={`/admin${item.path}`}
        activeOptions={{ exact: item.path === "", includeSearch: false }}
        aria-label={rail ? (count ? `${label} (${count})` : label) : undefined}
        className={cn(NAV_LINK, rail && "justify-center px-0")}
      >
        <item.icon className="size-4 shrink-0" aria-hidden />
        {rail ? null : <span className="min-w-0 flex-1 truncate">{label}</span>}
        {count ? (
          rail ? (
            <span aria-hidden className="absolute top-1 right-1.5 size-2 rounded-full bg-(--color-primary)" />
          ) : (
            <span className="rounded-full bg-(--color-foreground)/10 px-2 text-xs/5 font-medium text-(--color-foreground) tabular-nums">
              {count}
            </span>
          )
        ) : null}
      </Link>
    );
    return rail ? (
      <Tooltip key={item.path}>
        <TooltipTrigger asChild>{node}</TooltipTrigger>
        <TooltipContent side="right">{label}</TooltipContent>
      </Tooltip>
    ) : (
      node
    );
  };

  return (
    <>
      <div className={cn("flex h-10 shrink-0 items-center gap-2", rail ? "justify-center" : "px-2")}>
        <img src={subUrl("/img/favicon.png")} alt="" width="28" height="28" className="size-7" />
        {rail ? null : (
          <div className="min-w-0 leading-tight">
            <div className="truncate text-sm font-semibold">Gogs</div>
            <div className="truncate text-xs text-(--color-muted-foreground)">{t("admin_panel")}</div>
          </div>
        )}
      </div>
      <nav aria-label={t("admin_panel")} className="mt-2 flex min-h-0 flex-1 flex-col overflow-y-auto px-2 pb-2">
        {NAV.map((group, i) => (
          <div
            key={i}
            role="group"
            aria-label={group.title ? t(group.title) : undefined}
            className="flex flex-col gap-0.5"
          >
            {group.title ? (
              rail ? (
                <hr className="my-2 border-(--color-border)" />
              ) : (
                <div className="mt-4 mb-1 px-2 text-xs font-medium text-(--color-muted-foreground)">
                  {t(group.title)}
                </div>
              )
            ) : null}
            {group.items.map(link)}
          </div>
        ))}
      </nav>
      <div className="flex shrink-0 flex-col gap-0.5 px-2">
        {rail ? (
          <Tooltip>
            <TooltipTrigger asChild>
              <a
                href={subUrl("/")}
                aria-label={t("admin.back_to_site")}
                className={cn(NAV_LINK, "justify-center px-0")}
              >
                <ArrowLeft className="size-4 shrink-0" aria-hidden />
              </a>
            </TooltipTrigger>
            <TooltipContent side="right">{t("admin.back_to_site")}</TooltipContent>
          </Tooltip>
        ) : (
          <a href={subUrl("/")} className={NAV_LINK}>
            <ArrowLeft className="size-4 shrink-0" aria-hidden />
            {t("admin.back_to_site")}
          </a>
        )}
        {user ? (
          <UserMenu
            user={user}
            showName={!rail}
            side="top"
            align="start"
            className={cn("hover:bg-(--color-foreground)/5", rail && "justify-center px-0")}
          />
        ) : null}
      </div>
    </>
  );
}

// AdminLayout is the console that every page of the admin panel renders in:
// a sidebar with the links between the pages, which collapses to a rail of
// icons and moves into a sheet on narrow screens, and a bar that names the
// current page above it. The pages use the whole window width.
export function AdminLayout() {
  const { t } = useTranslation();
  const [collapsed, setCollapsed] = useState(readCollapsed);
  const [sheetOpen, setSheetOpen] = useState(false);
  const sheetToggle = useRef<HTMLButtonElement>(null);
  const [crumbSlot, setCrumbSlot] = useState<HTMLElement | null>(null);

  // The deepest route tells the page, e.g., "/admin/users/$id".
  const routePath = useRouterState({ select: (state) => state.matches.at(-1)?.fullPath ?? "" });
  const group = NAV.find((g) => g.items.some((item) => item.path && `${routePath}/`.startsWith(`/admin${item.path}/`)));
  const item = group?.items.find((i) => i.path && `${routePath}/`.startsWith(`/admin${i.path}/`)) ?? NAV[0].items[0];
  const current = t(item.label);
  // A page that is not found or failed to load says so in place of its name.
  const status = useRouterState({ select: (state) => state.matches.at(-1)?.status });
  const problem =
    status === "notFound" ? t("status.page_not_found") : status === "error" ? t("status.internal_server_error") : null;
  usePageTitle(problem ?? current);
  // A page below the page of the item names itself in the breadcrumb.
  const below = item.path !== "" && routePath.replace(/\/$/, "") !== `/admin${item.path}`;

  const toggleCollapsed = () => {
    const next = !collapsed;
    setCollapsed(next);
    try {
      window.localStorage.setItem(COLLAPSED_KEY, next ? "true" : "false");
    } catch {
      // The choice lasts until the page reloads.
    }
  };
  // The sheet closes after a click on a link, which stays in the app.
  const closeOnLink = (e: MouseEvent) => {
    if ((e.target as Element).closest("a")) setSheetOpen(false);
  };

  return (
    <div className="flex flex-1 bg-(--color-surface) glass-ui:bg-transparent">
      <aside
        id="admin-sidebar"
        className={cn(
          "glass-bar sticky top-0 hidden h-dvh shrink-0 flex-col py-2 md:flex glass-ui:border-r glass-ui:border-(--color-border)",
          collapsed ? "w-14" : "w-60",
        )}
      >
        <Sidebar rail={collapsed} />
      </aside>
      <Sheet open={sheetOpen} onOpenChange={setSheetOpen}>
        <SheetContent
          side="left"
          aria-describedby={undefined}
          className="flex w-64 flex-col bg-(--color-surface) py-2"
          onClick={closeOnLink}
          // Radix passes over links when it picks what to focus, which would
          // be the user menu at the foot of the sheet.
          onOpenAutoFocus={(e) => {
            e.preventDefault();
            const sheet = e.currentTarget as HTMLElement;
            (sheet.querySelector<HTMLElement>("nav a[aria-current=page]") ?? sheet.querySelector("a"))?.focus();
          }}
          // The sheet has no trigger of its own to give the focus back to.
          onCloseAutoFocus={(e) => {
            e.preventDefault();
            sheetToggle.current?.focus();
          }}
        >
          <SheetTitle className="sr-only">{t("admin_panel")}</SheetTitle>
          <Sidebar rail={false} />
        </SheetContent>
      </Sheet>
      <div className="flex min-w-0 flex-1 flex-col md:py-2 md:pr-2 glass-ui:md:py-0 glass-ui:md:pr-0">
        <div className="flex min-w-0 flex-1 flex-col bg-(--color-background) md:rounded-xl md:border md:border-(--color-border) glass-ui:bg-transparent glass-ui:md:rounded-none glass-ui:md:border-0">
          <header className="glass-bar sticky top-0 z-10 flex h-12 shrink-0 items-center gap-2 border-b border-(--color-border) bg-(--color-background) px-2 sm:px-3 md:rounded-t-xl glass-ui:md:rounded-none">
            <button
              ref={sheetToggle}
              type="button"
              aria-label={t("admin.toggle_sidebar")}
              onClick={() => setSheetOpen(true)}
              className={cn(ICON_BUTTON, "inline-flex md:hidden")}
            >
              <PanelLeft className="size-4" aria-hidden />
            </button>
            <button
              type="button"
              aria-label={t("admin.toggle_sidebar")}
              aria-expanded={!collapsed}
              aria-controls="admin-sidebar"
              onClick={toggleCollapsed}
              className={cn(ICON_BUTTON, "hidden md:inline-flex")}
            >
              <PanelLeft className="size-4" aria-hidden />
            </button>
            <span aria-hidden className="h-4 w-px shrink-0 bg-(--color-border)" />
            <nav aria-label={t("admin.breadcrumb")} className="min-w-0">
              <ol className="flex min-w-0 items-center gap-1.5 text-sm">
                <Crumb className="hidden xl:flex">
                  <Link to="/admin" activeOptions={{ exact: true }} className={CRUMB_LINK}>
                    {t("admin_panel")}
                  </Link>
                </Crumb>
                {group?.title ? (
                  <Crumb className="hidden lg:flex">
                    <span className="min-w-0 truncate">{t(group.title)}</span>
                  </Crumb>
                ) : null}
                {below || (problem !== null && item.path !== "") ? (
                  <Crumb className="hidden sm:flex">
                    <Link to={`/admin${item.path}`} activeOptions={{ exact: true }} className={CRUMB_LINK}>
                      {current}
                    </Link>
                  </Crumb>
                ) : null}
                {/* The keys keep React from reusing the element a page wrote its name into. */}
                {problem !== null ? (
                  <li key="problem" aria-current="page" className="truncate font-medium">
                    {problem}
                  </li>
                ) : below ? (
                  <li key="slot" ref={setCrumbSlot} aria-current="page" className="truncate font-medium" />
                ) : (
                  <li key="current" aria-current="page" className="truncate font-medium">
                    {current}
                  </li>
                )}
              </ol>
            </nav>
            <div className="ml-auto flex shrink-0 items-center gap-1">
              <PaletteButton />
              <SettingsMenu />
            </div>
          </header>
          <NoticeBanner fullWidth />
          <main className="min-w-0 flex-1 px-4 py-6 sm:px-6">
            <AdminCrumbSlot value={crumbSlot}>
              <Outlet />
            </AdminCrumbSlot>
          </main>
        </div>
      </div>
    </div>
  );
}

// The line height makes the link 24px tall, the least a touch target may be.
const CRUMB_LINK = "min-w-0 truncate leading-6 hover:text-(--color-foreground)";

// Crumb is a page above the current one. Where the bar is narrow it gives up
// its room before the current page does, which the large shrink factor says,
// down to a width that leaves its link 24px wide.
function Crumb({ className, children }: { className?: string; children: ReactNode }) {
  return (
    <li className={cn("min-w-11 shrink-10000 items-center gap-1.5 text-(--color-muted-foreground)", className)}>
      {children}
      <ChevronRight className="size-3.5 shrink-0" aria-hidden />
    </li>
  );
}
