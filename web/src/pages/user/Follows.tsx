import { useSuspenseQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";

import { UserList, type UserListPage } from "@/components/UserListItem";
import { usePageTitle } from "@/lib/page-title";
import { userProfileQuery } from "@/lib/queries/user";
import { cn } from "@/lib/utils";
import { ProfileSidebar } from "@/pages/user/Profile";

// Follows lists the followers of a user, or the users they follow, next to
// their profile.
export function Follows({
  username,
  list,
  data,
}: {
  username: string;
  list: "followers" | "following";
  data: UserListPage;
}) {
  const { t } = useTranslation();
  const { data: profile } = useSuspenseQuery(userProfileQuery(username));
  usePageTitle(profile.fullName || profile.name);
  // The list endpoints turn organizations away, so the loader never gets here
  // for one.
  if (profile.kind !== "user") return null;

  const tabs = [
    { key: "followers", label: t("user.followers"), count: profile.followerCount },
    { key: "following", label: t("user.following"), count: profile.followingCount },
  ] as const;

  return (
    <main className="mx-auto grid w-full max-w-6xl flex-1 gap-6 px-4 py-6 sm:px-6 md:grid-cols-[256px_minmax(0,1fr)] lg:grid-cols-[296px_minmax(0,1fr)]">
      <ProfileSidebar profile={profile} />
      <section className="min-w-0">
        <nav
          aria-label={profile.fullName || profile.name}
          className="flex overflow-x-auto border-b border-(--color-border)"
        >
          {tabs.map(({ key, label, count }) => {
            const active = key === list;
            return (
              <Link
                key={key}
                to={key === "followers" ? "/$username/followers" : "/$username/following"}
                params={{ username }}
                aria-current={active ? "page" : undefined}
                className={cn(
                  "-mb-px flex items-center gap-2 border-b-2 px-3 py-2 text-sm whitespace-nowrap",
                  active
                    ? "border-(--color-primary) font-semibold text-(--color-foreground)"
                    : "border-transparent text-(--color-muted-foreground) hover:border-(--color-border) hover:text-(--color-foreground)",
                )}
              >
                {label}
                <span className="rounded-full bg-(--color-secondary) px-2 text-xs/5 font-medium tabular-nums text-(--color-secondary-foreground)">
                  {count}
                </span>
              </Link>
            );
          })}
        </nav>
        <UserList data={data} />
      </section>
    </main>
  );
}
