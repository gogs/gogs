import { queryOptions } from "@tanstack/react-query";

import { loaderResponseError } from "@/lib/loader-error";
import { subUrl } from "@/lib/url";

export interface RepoInfo {
  owner: string;
  name: string;
  avatarURL: string;
  visibility: "public" | "private";
  isAdmin: boolean;
  enableIssues: boolean;
  allowsPulls: boolean;
  enableWiki: boolean;
  counts: {
    watchers: number;
    stars: number;
    forks: number;
    openIssues: number;
    openPulls: number;
  };
  viewerWatching: boolean;
  viewerStarred: boolean;
  mirrorOf?: string;
}

export function repoInfoQuery(owner: string, name: string) {
  return queryOptions({
    queryKey: ["repo", owner, name, "info"] as const,
    queryFn: async ({ signal }) => {
      const res = await fetch(subUrl(`/api/web/${owner}/${name}/info`), {
        credentials: "same-origin",
        signal,
      });
      if (!res.ok) throw await loaderResponseError(res);
      return (await res.json()) as RepoInfo;
    },
  });
}

export interface RepoActionResult {
  viewerWatching?: boolean;
  viewerStarred?: boolean;
  watchers?: number;
  stars?: number;
}

async function repoAction(method: "POST" | "DELETE", owner: string, name: string, action: "watch" | "star") {
  const res = await fetch(subUrl(`/api/web/${owner}/${name}/${action}`), {
    method,
    credentials: "same-origin",
  });
  if (!res.ok) throw await loaderResponseError(res);
  return (await res.json()) as RepoActionResult;
}

export function watchRepo(owner: string, name: string) {
  return repoAction("POST", owner, name, "watch");
}

export function unwatchRepo(owner: string, name: string) {
  return repoAction("DELETE", owner, name, "watch");
}

export function starRepo(owner: string, name: string) {
  return repoAction("POST", owner, name, "star");
}

export function unstarRepo(owner: string, name: string) {
  return repoAction("DELETE", owner, name, "star");
}
