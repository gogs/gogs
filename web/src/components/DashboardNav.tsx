import { Link } from "@tanstack/react-router";
import { Activity, Check, ChevronDown, CircleDot, GitPullRequest, Plus } from "lucide-react";
import { type ReactNode, useState } from "react";
import { useTranslation } from "react-i18next";

import { PageTabs } from "@/components/PageTabs";
import { Button } from "@/components/ui/button";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import type { DashboardContextData } from "@/lib/queries/user";
import { subUrl } from "@/lib/url";
import { useUserInfo } from "@/lib/use-user-info";
import { cn } from "@/lib/utils";

// DashboardPage is one of the pages every dashboard owner has.
export type DashboardPage = "dashboard" | "issues" | "pulls";

const USER_PATHS = { dashboard: "/", issues: "/issues", pulls: "/pulls" } as const;
const ORG_PATHS = {
  dashboard: "/org/$org/dashboard",
  issues: "/org/$org/issues",
  pulls: "/org/$org/pulls",
} as const;

// ContextLinkTarget is a dashboard page of the user or of an organization.
// It is narrower than LinkProps, which grows too deep for the type checker
// with the number of routes.
type ContextLinkTarget =
  | { to: (typeof USER_PATHS)[DashboardPage] }
  | { to: (typeof ORG_PATHS)[DashboardPage]; params: { org: string } };

// ContextSwitcher switches the current page between the dashboards of the
// signed-in user and of their organizations.
export function ContextSwitcher({ data, page }: { data: DashboardContextData; page: DashboardPage }) {
  const { t } = useTranslation();
  const user = useUserInfo();
  const [open, setOpen] = useState(false);
  if (!user) return null;

  const close = () => setOpen(false);
  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger className="flex w-full cursor-pointer items-center gap-2 rounded-md border border-(--color-border) px-2 py-1.5 text-sm hover:bg-(--color-surface)">
        <img src={data.avatarURL} alt="" className="size-6 shrink-0 rounded-md object-cover" />
        <span className="sr-only">{t("home.switch_dashboard_context")}: </span>
        <span className="min-w-0 flex-1 truncate text-left font-semibold">{data.name}</span>
        <ChevronDown className="size-4 shrink-0 text-(--color-muted-foreground)" aria-hidden />
      </PopoverTrigger>
      <PopoverContent align="start" className="w-(--radix-popover-trigger-width) min-w-56 p-1">
        <p className="px-2 py-1.5 text-xs text-(--color-muted-foreground)">{t("home.switch_dashboard_context")}</p>
        <ul className="max-h-80 overflow-y-auto">
          <li>
            <ContextLink
              linkProps={{ to: USER_PATHS[page] }}
              current={data.kind === "user"}
              avatarURL={user.avatarURL}
              onSelect={close}
            >
              {user.username}
            </ContextLink>
          </li>
          {data.organizations.map((org) => (
            <li key={org.name}>
              <ContextLink
                linkProps={{ to: ORG_PATHS[page], params: { org: org.name } }}
                current={data.kind === "organization" && data.name === org.name}
                avatarURL={org.avatarURL}
                onSelect={close}
              >
                {org.name}
              </ContextLink>
            </li>
          ))}
        </ul>
        {user.canCreateOrganization ? (
          <div className="mt-1 border-t border-(--color-border) pt-1">
            <a
              href={subUrl("/org/create")}
              className="flex items-center gap-2 rounded-sm px-2 py-1.5 text-sm hover:bg-(--color-surface)"
            >
              <Plus className="size-4" aria-hidden />
              {t("new_org")}
            </a>
          </div>
        ) : null}
      </PopoverContent>
    </Popover>
  );
}

function ContextLink({
  linkProps,
  current,
  avatarURL,
  onSelect,
  children,
}: {
  linkProps: ContextLinkTarget;
  current: boolean;
  avatarURL: string;
  onSelect: () => void;
  children: ReactNode;
}) {
  return (
    <Link
      {...linkProps}
      onClick={onSelect}
      aria-current={current ? "page" : undefined}
      className="flex items-center gap-2 rounded-sm px-2 py-1.5 text-sm hover:bg-(--color-surface)"
    >
      <img src={avatarURL} alt="" className="size-5 shrink-0 rounded-sm object-cover" />
      <span className={cn("min-w-0 flex-1 truncate", current && "font-semibold")}>{children}</span>
      {current ? <Check className="size-4 shrink-0" aria-hidden /> : null}
    </Link>
  );
}

// OrgNav links the dashboard pages of an organization, and its home page.
export function OrgNav({ name, page }: { name: string; page: DashboardPage }) {
  const { t } = useTranslation();
  const tabs = [
    { key: "dashboard", label: t("activities"), icon: Activity },
    { key: "issues", label: t("issues"), icon: CircleDot },
    { key: "pulls", label: t("pull_requests"), icon: GitPullRequest },
  ] as const;
  return (
    // The button wraps above the tabs when both do not fit in one row.
    <div className="flex flex-wrap-reverse items-end justify-end gap-x-3 gap-y-2 border-b border-(--color-border)">
      <PageTabs
        label={name}
        tabs={tabs}
        active={page}
        // Only the active tab fits next to the menu at 320px.
        inline={1}
        renderLink={(tab, props) => <Link to={ORG_PATHS[tab.key]} params={{ org: name }} {...props} />}
        className="mr-auto max-w-full"
      />
      <Button asChild variant="outline" size="sm" className="mb-1.5 shrink-0">
        <Link to="/$username" params={{ username: name }}>
          {t("home.view_home").replace(/%(?:\[1\])?s/, name)}
        </Link>
      </Button>
    </div>
  );
}
