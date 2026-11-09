import { Link } from "@tanstack/react-router";
import { Clock, Link as LinkIcon, Mail, MapPin } from "lucide-react";
import type { ComponentType, ReactNode } from "react";
import { useTranslation } from "react-i18next";

import { Pagination } from "@/components/Pagination";
import { webContext } from "@/lib/context";
import { cn } from "@/lib/utils";

export interface UserListItemData {
  name: string;
  fullName?: string;
  avatarURL: string;
  location?: string;
  email?: string;
  website?: string;
  description?: string;
  createdAt?: string;
}

export interface UserListPage {
  items: UserListItemData[];
  page: number;
  totalPages: number;
}

const JOINED_DATE_FMT = new Intl.DateTimeFormat(webContext.lang, { year: "numeric", month: "short", day: "2-digit" });

// UserListItem is one row of a list of users or organizations.
export function UserListItem({
  user,
  organization = false,
  action,
}: {
  user: UserListItemData;
  organization?: boolean;
  // A control shown at the end of the row, e.g., a button to remove the user.
  action?: ReactNode;
}) {
  const { t } = useTranslation();
  const isWebsiteLink = user.website ? /^https?:\/\//i.test(user.website) : false;
  return (
    <li className="flex gap-3 border-b border-(--color-border) py-4">
      <img
        src={user.avatarURL}
        alt=""
        className={cn(
          "mt-1.25 size-10 shrink-0 border border-(--color-border) object-cover",
          organization ? "rounded-md" : "rounded-full",
        )}
      />
      <div className="min-w-0 flex-1">
        <p className="flex flex-wrap items-baseline gap-x-2">
          <Link
            to="/$username"
            params={{ username: user.name }}
            className="font-semibold text-(--color-primary) hover:underline"
          >
            {user.name}
          </Link>
          {user.fullName ? <span className="text-sm text-(--color-muted-foreground)">{user.fullName}</span> : null}
        </p>
        {organization ? (
          user.description ? (
            <p className="mt-1 text-sm break-words text-(--color-foreground)">{user.description}</p>
          ) : (
            <p className="mt-1 text-sm text-(--color-muted-foreground) italic">{t("repo.no_desc")}</p>
          )
        ) : null}
        <p className="mt-1 flex flex-wrap gap-x-4 gap-y-1 text-xs text-(--color-muted-foreground)">
          {user.location ? (
            <Meta icon={MapPin} label={t("settings.location")}>
              {user.location}
            </Meta>
          ) : null}
          {user.email ? (
            <Meta icon={Mail} label={t("email")}>
              <a
                href={`mailto:${user.email}`}
                rel="nofollow"
                className="hover:text-(--color-foreground) hover:underline"
              >
                {user.email}
              </a>
            </Meta>
          ) : null}
          {user.website ? (
            <Meta icon={LinkIcon} label={t("website")}>
              {isWebsiteLink ? (
                <a href={user.website} rel="nofollow" className="hover:text-(--color-foreground) hover:underline">
                  {user.website}
                </a>
              ) : (
                user.website
              )}
            </Meta>
          ) : null}
          {user.createdAt ? (
            <Meta icon={Clock}>
              {t("user.join_on")}{" "}
              <time dateTime={user.createdAt}>{JOINED_DATE_FMT.format(new Date(user.createdAt))}</time>
            </Meta>
          ) : null}
        </p>
      </div>
      {action ? <div className="shrink-0 self-center">{action}</div> : null}
    </li>
  );
}

function Meta({
  icon: Icon,
  label,
  children,
}: {
  icon: ComponentType<{ className?: string }>;
  label?: string;
  children: ReactNode;
}) {
  return (
    <span className="inline-flex min-w-0 items-center gap-1 break-all">
      <Icon className="size-3.5 shrink-0" aria-hidden />
      {label ? <span className="sr-only">{label}</span> : null}
      {children}
    </span>
  );
}

// UserList is a page of users with pagination, or a note when there are none.
export function UserList({ data, action }: { data: UserListPage; action?: (user: UserListItemData) => ReactNode }) {
  const { t } = useTranslation();
  if (data.items.length === 0) {
    return <p className="py-8 text-center text-sm text-(--color-muted-foreground)">{t("repo.pulls.no_results")}</p>;
  }
  return (
    <>
      <ul>
        {data.items.map((user) => (
          <UserListItem key={user.name} user={user} action={action?.(user)} />
        ))}
      </ul>
      {data.totalPages > 1 ? (
        <div className="mt-4">
          <Pagination page={data.page} totalPages={data.totalPages} />
        </div>
      ) : null}
    </>
  );
}
