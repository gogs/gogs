import { useMutation, useQueryClient, useSuspenseInfiniteQuery, useSuspenseQuery } from "@tanstack/react-query";
import { Link, getRouteApi } from "@tanstack/react-router";
import {
  Activity,
  BookMarked,
  Camera,
  Clock,
  Link as LinkIcon,
  Mail,
  MapPin,
  UserMinus,
  UserPlus,
  Users,
} from "lucide-react";
import type { ComponentType, ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";

import { ActivityFeed } from "@/components/ActivityFeed";
import { PageTabs } from "@/components/PageTabs";
import { Pagination } from "@/components/Pagination";
import { RepoListItem } from "@/components/RepoListItem";
import { Button } from "@/components/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { webContext } from "@/lib/context";
import { usePageTitle } from "@/lib/page-title";
import {
  type UserProfileData,
  type UserReposPage,
  followUser,
  unfollowUser,
  userActivityQuery,
  userProfileQuery,
} from "@/lib/queries/user";
import { subUrl } from "@/lib/url";
import { useUserInfo } from "@/lib/use-user-info";

export type ProfilePage = { tab: "repos"; repos: UserReposPage } | { tab: "activity" };

const route = getRouteApi("/$username");

const JOINED_DATE_FMT = new Intl.DateTimeFormat(webContext.lang, { year: "numeric", month: "short", day: "2-digit" });

export function Profile() {
  const { t } = useTranslation();
  const { username } = route.useParams();
  const data = route.useLoaderData();
  const { data: profile } = useSuspenseQuery(userProfileQuery(username));
  usePageTitle(profile.fullName || profile.name);

  const tabs = [
    { key: "repos", label: t("user.repositories"), icon: BookMarked },
    { key: "activity", label: t("user.activity"), icon: Activity },
  ] as const;

  return (
    <main className="mx-auto grid w-full max-w-6xl flex-1 content-start gap-6 px-4 py-6 sm:px-6 md:grid-cols-[256px_minmax(0,1fr)] lg:grid-cols-[296px_minmax(0,1fr)]">
      <ProfileSidebar profile={profile} />
      <section className="min-w-0">
        <PageTabs
          label={profile.fullName || profile.name}
          tabs={tabs}
          active={data.tab}
          renderLink={(tab, props) => (
            <Link
              to="/$username"
              params={{ username }}
              search={tab.key === "activity" ? { tab: "activity" } : {}}
              {...props}
            />
          )}
          className="border-b border-(--color-border)"
        />
        {data.tab === "repos" ? (
          <>
            <ul>
              {data.repos.items.map((repo) => (
                <RepoListItem key={repo.name} repo={repo} />
              ))}
            </ul>
            {data.repos.totalPages > 1 ? (
              <div className="mt-4">
                <Pagination page={data.repos.page} totalPages={data.repos.totalPages} />
              </div>
            ) : null}
          </>
        ) : (
          <ProfileActivity username={username} />
        )}
      </section>
    </main>
  );
}

function ProfileActivity({ username }: { username: string }) {
  const { data, hasNextPage, isFetchingNextPage, fetchNextPage } = useSuspenseInfiniteQuery(
    userActivityQuery(username),
  );
  return (
    <ActivityFeed
      items={data.pages.flatMap((page) => page.items)}
      hasMore={hasNextPage}
      loadingMore={isFetchingNextPage}
      onLoadMore={() => void fetchNextPage()}
    />
  );
}

function ProfileSidebar({ profile }: { profile: UserProfileData }) {
  const { t } = useTranslation();
  const isWebsiteLink = profile.website ? /^https?:\/\//i.test(profile.website) : false;
  const homeLink = subUrl(`/${profile.name}`);
  const authenticated = useUserInfo() !== null;

  return (
    <aside className="min-w-0">
      <div className="flex items-center gap-4 md:block">
        {profile.viewerIsSelf ? (
          <a
            href={subUrl("/user/settings/avatar")}
            aria-label={t("user.change_avatar")}
            className="group relative block size-18 shrink-0 overflow-hidden rounded-full border border-(--color-border) outline-none focus-visible:ring-2 focus-visible:ring-(--color-ring) focus-visible:ring-offset-2 focus-visible:ring-offset-(--color-background) md:size-auto md:w-full"
          >
            <img src={profile.avatarURL} alt="" className="aspect-square w-full object-cover" />
            <span className="absolute inset-0 hidden flex-col items-center justify-center gap-1 bg-black/60 text-xs text-white md:group-hover:flex md:group-focus-visible:flex">
              <Camera className="size-3.5" aria-hidden />
              {t("user.change_avatar")}
            </span>
          </a>
        ) : (
          <img
            src={profile.avatarURL}
            alt=""
            className="aspect-square size-18 shrink-0 rounded-full border border-(--color-border) object-cover md:size-auto md:w-full"
          />
        )}
        <h1 className="min-w-0 md:mt-4">
          {profile.fullName ? (
            <span className="block text-2xl/[30px] font-semibold break-words">{profile.fullName}</span>
          ) : null}
          <span className="block text-xl/6 font-light break-all text-(--color-muted-foreground)">{profile.name}</span>
        </h1>
      </div>

      {authenticated && !profile.viewerIsSelf ? <FollowButton profile={profile} /> : null}

      <ul className="mt-4 grid gap-1.5 text-sm">
        {profile.location ? (
          <Fact icon={MapPin} label={t("settings.location")}>
            {profile.location}
          </Fact>
        ) : null}
        {profile.email ? (
          <Fact icon={Mail} label={t("email")}>
            <a href={`mailto:${profile.email}`} rel="nofollow" className="text-(--color-primary) hover:underline">
              {profile.email}
            </a>
          </Fact>
        ) : null}
        {profile.website ? (
          <Fact icon={LinkIcon} label={t("website")}>
            {isWebsiteLink ? (
              <a
                href={profile.website}
                target="_blank"
                rel="noopener noreferrer me nofollow"
                className="text-(--color-primary) hover:underline"
              >
                {profile.website}
              </a>
            ) : (
              profile.website
            )}
          </Fact>
        ) : null}
        {profile.createdAt ? (
          <Fact icon={Clock}>
            {t("user.join_on")}{" "}
            <time dateTime={profile.createdAt}>{JOINED_DATE_FMT.format(new Date(profile.createdAt))}</time>
          </Fact>
        ) : null}
        <Fact icon={Users}>
          <a href={`${homeLink}/followers`} className="group hover:text-(--color-primary)">
            <span className="font-semibold tabular-nums">{profile.followerCount}</span>{" "}
            <span className="text-(--color-muted-foreground) group-hover:text-(--color-primary)">
              {t("user.followers")}
            </span>
          </a>
          <span className="text-(--color-muted-foreground)" aria-hidden>
            {" · "}
          </span>
          <a href={`${homeLink}/following`} className="group hover:text-(--color-primary)">
            <span className="font-semibold tabular-nums">{profile.followingCount}</span>{" "}
            <span className="text-(--color-muted-foreground) group-hover:text-(--color-primary)">
              {t("user.following")}
            </span>
          </a>
        </Fact>
      </ul>

      {profile.organizations.length > 0 ? (
        <div className="mt-4 border-t border-(--color-border) pt-4">
          <h2 className="mb-2 text-sm font-semibold">{t("explore.organizations")}</h2>
          <ul className="flex flex-wrap gap-1.5">
            {profile.organizations.map((org) => (
              <li key={org.name}>
                <Tooltip>
                  <TooltipTrigger asChild>
                    <a href={subUrl(`/${org.name}`)} aria-label={org.name} className="block rounded-md">
                      <img
                        src={org.avatarURL}
                        alt=""
                        className="size-8 rounded-md border border-(--color-border) object-cover"
                      />
                    </a>
                  </TooltipTrigger>
                  <TooltipContent>{org.name}</TooltipContent>
                </Tooltip>
              </li>
            ))}
          </ul>
        </div>
      ) : null}
    </aside>
  );
}

function FollowButton({ profile }: { profile: UserProfileData }) {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const mutation = useMutation({
    mutationFn: async () => {
      const next = !profile.viewerIsFollowing;
      const result = next ? await followUser(profile.name) : await unfollowUser(profile.name);
      return { result, next };
    },
    // The outcome is unambiguous (POST always follows, DELETE always unfollows),
    // so merge it into the cached profile instead of refetching.
    onSuccess: ({ result, next }) => {
      queryClient.setQueryData(userProfileQuery(profile.name).queryKey, (prev) =>
        prev ? { ...prev, viewerIsFollowing: next, followerCount: result.followerCount } : prev,
      );
    },
    onError: (err) => toast.error(t("status.internal_server_error"), { description: err.message }),
  });

  const Icon = profile.viewerIsFollowing ? UserMinus : UserPlus;
  return (
    <Button
      variant="outline"
      size="sm"
      className="mt-3 w-full"
      aria-pressed={profile.viewerIsFollowing}
      disabled={mutation.isPending}
      onClick={() => mutation.mutate()}
    >
      <Icon className="size-4" aria-hidden />
      {profile.viewerIsFollowing ? t("user.unfollow") : t("user.follow")}
    </Button>
  );
}

function Fact({
  icon: Icon,
  label,
  children,
}: {
  icon: ComponentType<{ className?: string }>;
  label?: string;
  children: ReactNode;
}) {
  return (
    <li className="flex min-w-0 items-start gap-2">
      <Icon className="mt-0.5 size-4 shrink-0 text-(--color-muted-foreground)" aria-hidden />
      {label ? <span className="sr-only">{label}</span> : null}
      <span className="min-w-0 break-all">{children}</span>
    </li>
  );
}
