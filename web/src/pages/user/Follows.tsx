import { useSuspenseQuery } from "@tanstack/react-query";

import { UserList, type UserListPage } from "@/components/UserListItem";
import { usePageTitle } from "@/lib/page-title";
import { userProfileQuery } from "@/lib/queries/user";
import { ProfileSidebar, ProfileTabs } from "@/pages/user/Profile";

// Follows lists the followers of a user, or the users they follow, next to
// their profile, under the profile tabs.
export function Follows({
  username,
  list,
  data,
}: {
  username: string;
  list: "followers" | "following";
  data: UserListPage;
}) {
  const { data: profile } = useSuspenseQuery(userProfileQuery(username));
  usePageTitle(profile.fullName || profile.name);
  // The list endpoints turn organizations away, so the loader never gets here
  // for one.
  if (profile.kind !== "user") return null;

  return (
    <main className="mx-auto grid w-full max-w-6xl flex-1 content-start gap-6 px-4 glass-ui:gap-x-4 py-6 sm:px-6 md:grid-cols-[256px_minmax(0,1fr)] lg:grid-cols-[296px_minmax(0,1fr)]">
      <ProfileSidebar profile={profile} />
      <section className="glass-section min-w-0 glass-ui:self-start">
        <ProfileTabs profile={profile} active={list} />
        <div className="glass-ui:px-4 glass-ui:pb-4">
          <UserList data={data} />
        </div>
      </section>
    </main>
  );
}
