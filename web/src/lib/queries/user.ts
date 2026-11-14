import { infiniteQueryOptions, queryOptions } from "@tanstack/react-query";

import type { RepoListItemData } from "@/components/RepoListItem";
import { loaderResponseError } from "@/lib/loader-error";
import { subUrl } from "@/lib/url";

export interface UserProfileData {
  name: string;
  fullName?: string;
  avatarURL: string;
  location?: string;
  email?: string;
  website?: string;
  createdAt?: string;
  followerCount: number;
  followingCount: number;
  organizations: { name: string; avatarURL: string }[];
  viewerIsSelf: boolean;
  viewerIsFollowing: boolean;
}

async function fetchJSON<T>(url: string, init?: RequestInit): Promise<T> {
  const res = await fetch(url, { credentials: "same-origin", ...init });
  if (!res.ok) throw await loaderResponseError(res);
  return (await res.json()) as T;
}

export function userProfileQuery(username: string) {
  return queryOptions({
    queryKey: ["user", username, "profile"] as const,
    queryFn: ({ signal }) =>
      fetchJSON<UserProfileData>(subUrl(`/api/web/users/${encodeURIComponent(username)}`), { signal }),
  });
}

export interface ActivityCommit {
  sha: string;
  message: string;
  avatarURL: string;
}

export interface ActivityItem {
  id: number;
  // The `ActionType` of the action, e.g., 5 for pushing commits.
  opType: number;
  actorName: string;
  actorAvatarURL: string;
  // Sanitized by the server, safe to render as HTML.
  headlineHTML: string;
  title?: string;
  detail?: string;
  repoURL: string;
  commits?: ActivityCommit[];
  commitCount?: number;
  compareURL?: string;
  createdAt: string;
}

export interface ActivityFeedPage {
  items: ActivityItem[];
  nextAfterID?: number;
}

export function userActivityQuery(username: string) {
  return infiniteQueryOptions({
    queryKey: ["user", username, "activity"] as const,
    queryFn: ({ pageParam, signal }) => {
      const url = subUrl(`/api/web/users/${encodeURIComponent(username)}/activity`);
      return fetchJSON<ActivityFeedPage>(pageParam ? `${url}?afterID=${pageParam}` : url, { signal });
    },
    initialPageParam: 0,
    getNextPageParam: (lastPage) => lastPage.nextAfterID,
  });
}

export interface UserFollowResult {
  followerCount: number;
}

export function followUser(username: string) {
  return fetchJSON<UserFollowResult>(subUrl(`/api/web/users/${encodeURIComponent(username)}/follow`), {
    method: "POST",
  });
}

export function unfollowUser(username: string) {
  return fetchJSON<UserFollowResult>(subUrl(`/api/web/users/${encodeURIComponent(username)}/follow`), {
    method: "DELETE",
  });
}

export interface UserReposPage {
  items: RepoListItemData[];
  page: number;
  totalPages: number;
}

export function fetchUserRepos(username: string, page?: number) {
  const url = subUrl(`/api/web/users/${encodeURIComponent(username)}/repos`);
  return fetchJSON<UserReposPage>(page ? `${url}?page=${page}` : url);
}
