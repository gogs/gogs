import { useSuspenseQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";

import { PageTabs } from "@/components/PageTabs";
import { UserList, type UserListPage } from "@/components/UserListItem";
import { usePageTitle } from "@/lib/page-title";
import { userProfileQuery } from "@/lib/queries/user";
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
        <PageTabs
          label={profile.fullName || profile.name}
          tabs={tabs}
          active={list}
          renderLink={(tab, props) => (
            <Link
              to={tab.key === "followers" ? "/$username/followers" : "/$username/following"}
              params={{ username }}
              {...props}
            />
          )}
          className="border-b border-(--color-border)"
        />
        <UserList data={data} />
      </section>
    </main>
  );
}
