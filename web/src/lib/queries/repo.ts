import { queryOptions } from "@tanstack/react-query";

import { loaderResponseError } from "@/lib/loader-error";
import { subUrl } from "@/lib/url";

export interface RepoHeaderData {
  id: number;
  owner: string;
  name: string;
  avatarURL: string;
  visibility: "public" | "private";
  viewerCanAdminister: boolean;
  issuesEnabled: boolean;
  pullRequestsEnabled: boolean;
  wikiEnabled: boolean;
  watchCount: number;
  starCount: number;
  forkCount: number;
  openIssueCount: number;
  openPullRequestCount: number;
  viewerIsWatching: boolean;
  viewerIsStarring: boolean;
  mirrorOf?: string;
}

export function repoHeaderQuery(owner: string, name: string) {
  return queryOptions({
    queryKey: ["repo", owner, name, "header"] as const,
    queryFn: async ({ signal }) => {
      const res = await fetch(subUrl(`/api/web/${owner}/${name}/header`), {
        credentials: "same-origin",
        signal,
      });
      if (!res.ok) throw await loaderResponseError(res);
      return (await res.json()) as RepoHeaderData;
    },
  });
}

export interface RepoWatchResult {
  watchCount: number;
}

export interface RepoStarResult {
  starCount: number;
}

async function repoAction<T>(method: "POST" | "DELETE", owner: string, name: string, action: "watch" | "star") {
  const res = await fetch(subUrl(`/api/web/${owner}/${name}/${action}`), {
    method,
    credentials: "same-origin",
  });
  if (!res.ok) throw await loaderResponseError(res);
  return (await res.json()) as T;
}

export function watchRepo(owner: string, name: string) {
  return repoAction<RepoWatchResult>("POST", owner, name, "watch");
}

export function unwatchRepo(owner: string, name: string) {
  return repoAction<RepoWatchResult>("DELETE", owner, name, "watch");
}

export function starRepo(owner: string, name: string) {
  return repoAction<RepoStarResult>("POST", owner, name, "star");
}

export function unstarRepo(owner: string, name: string) {
  return repoAction<RepoStarResult>("DELETE", owner, name, "star");
}
