import { infiniteQueryOptions, queryOptions } from "@tanstack/react-query";

import type { RepoListItemData } from "@/components/RepoListItem";
import { loaderResponseError } from "@/lib/loader-error";
import { subUrl } from "@/lib/url";

export interface UserProfileData {
  kind: "user";
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

export interface OrgProfileData {
  kind: "organization";
  id: number;
  name: string;
  fullName?: string;
  avatarURL: string;
  description?: string;
  location?: string;
  website?: string;
  members: { name: string; fullName?: string; avatarURL: string }[];
  // The counts and the teams are only sent to members.
  memberCount?: number;
  teamCount?: number;
  teams?: { name: string; lowerName: string; memberCount: number; repoCount: number }[];
  viewerIsMember: boolean;
  viewerIsOwner: boolean;
}

// The profile route serves both individual users and organizations.
export type OwnerProfileData = UserProfileData | OrgProfileData;

async function fetchJSON<T>(url: string, init?: RequestInit): Promise<T> {
  const res = await fetch(url, { credentials: "same-origin", ...init });
  if (!res.ok) throw await loaderResponseError(res);
  return (await res.json()) as T;
}

export function userProfileQuery(username: string) {
  return queryOptions({
    queryKey: ["user", username, "profile"] as const,
    queryFn: ({ signal }) =>
      fetchJSON<OwnerProfileData>(subUrl(`/api/web/users/${encodeURIComponent(username)}`), { signal }),
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

export interface OwnerReposPage {
  items: RepoListItemData[];
  page: number;
  totalPages: number;
}

export function fetchUserRepos(username: string, page?: number) {
  const url = subUrl(`/api/web/users/${encodeURIComponent(username)}/repos`);
  return fetchJSON<OwnerReposPage>(page ? `${url}?page=${page}` : url);
}

export interface DashboardRepo {
  owner: string;
  name: string;
  isPrivate: boolean;
  isFork: boolean;
  isMirror: boolean;
  starCount: number;
}

// The owner of a dashboard page, and the organizations of the signed-in user to
// switch to, whichever dashboard this is.
export interface DashboardContextData {
  kind: "user" | "organization";
  id: number;
  name: string;
  avatarURL: string;
  organizations: { name: string; avatarURL: string; repoCount: number }[];
}

export interface DashboardData extends DashboardContextData {
  repos: DashboardRepo[];
  repoCount: number;
  hasMoreRepos: boolean;
  // Only on the dashboard of the signed-in user.
  collaborativeRepos?: DashboardRepo[];
  mirrors: { name: string; isPrivate: boolean; intervalHours: number }[];
}

// The dashboard of the signed-in user, or of one of their organizations.
function dashboardURL(org?: string) {
  return subUrl(org ? `/api/web/orgs/${encodeURIComponent(org)}/dashboard` : "/api/web/dashboard");
}

export function dashboardQuery(org?: string) {
  return queryOptions({
    queryKey: ["dashboard", org ?? ""] as const,
    queryFn: ({ signal }) => fetchJSON<DashboardData>(dashboardURL(org), { signal }),
  });
}

export function dashboardActivityQuery(org?: string) {
  return infiniteQueryOptions({
    queryKey: ["dashboard", org ?? "", "activity"] as const,
    queryFn: ({ pageParam, signal }) => {
      const url = `${dashboardURL(org)}/activity`;
      return fetchJSON<ActivityFeedPage>(pageParam ? `${url}?afterID=${pageParam}` : url, { signal });
    },
    initialPageParam: 0,
    getNextPageParam: (lastPage) => lastPage.nextAfterID,
  });
}

export interface DashboardIssue {
  // The full name of the repository, e.g., "owner/name".
  repo: string;
  index: number;
  title: string;
  isClosed: boolean;
  commentCount: number;
  labels: { name: string; color: string; textColor: string }[];
  poster: { name: string; avatarURL: string };
  assignee?: { name: string; avatarURL: string };
  createdAt: string;
}

export interface DashboardIssuesData extends DashboardContextData {
  items: DashboardIssue[];
  page: number;
  totalPages: number;
  // The filter in effect. Organizations only have "your_repositories".
  type: "your_repositories" | "assigned" | "created_by";
  counts: { yourRepos: number; assigned: number; created: number; open: number; closed: number };
  // Only listed for the "your_repositories" filter.
  repos: { id: number; fullName: string; count: number }[];
}

// fetchDashboardIssues lists the issues or pull requests of a dashboard, with
// the query parameters of the server-rendered page, e.g., `state=closed`.
export function fetchDashboardIssues(
  org: string | undefined,
  kind: "issues" | "pulls",
  query: Record<string, string | number | undefined>,
) {
  const params = new URLSearchParams();
  for (const [key, value] of Object.entries(query)) {
    if (value !== undefined) params.set(key, String(value));
  }
  const qs = params.toString();
  return fetchJSON<DashboardIssuesData>(`${dashboardURL(org)}/${kind}${qs ? `?${qs}` : ""}`);
}
