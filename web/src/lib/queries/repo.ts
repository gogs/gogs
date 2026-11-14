import { queryOptions } from "@tanstack/react-query";

import type { UserListPage } from "@/components/UserListItem";
import { loaderResponseError } from "@/lib/loader-error";
import { fetchJSON, send, submit, uploadAvatar } from "@/lib/queries/request";
import type { SSHKey } from "@/lib/queries/settings";
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

export async function fetchRepoPeople(owner: string, name: string, list: "stargazers" | "watchers", page?: number) {
  const url = subUrl(`/api/web/${owner}/${name}/${list}`);
  const res = await fetch(page ? `${url}?page=${page}` : url, { credentials: "same-origin" });
  if (!res.ok) throw await loaderResponseError(res);
  return (await res.json()) as UserListPage;
}

// removeRepoWatcher makes another user stop watching the repository. Only site
// admins may do so.
export async function removeRepoWatcher(owner: string, name: string, username: string) {
  const res = await fetch(subUrl(`/api/web/${owner}/${name}/watchers/${encodeURIComponent(username)}`), {
    method: "DELETE",
    credentials: "same-origin",
  });
  if (!res.ok) throw await loaderResponseError(res);
  return (await res.json()) as RepoWatchResult;
}

export interface RepoNewOwner {
  id: number;
  name: string;
  avatarURL: string;
}

export interface RepoNewOptions {
  owners: RepoNewOwner[];
  ownerID: number;
  // The visibility of the viewer's last repository.
  private: boolean;
  forcePrivate: boolean;
  canImportLocal: boolean;
  gitignores: string[];
  licenses: string[];
  readmes: string[];
}

export function fetchRepoNewOptions(org?: number) {
  return fetchJSON<RepoNewOptions>(subUrl(`/api/web/repo/new${org ? `?org=${org}` : ""}`));
}

interface RepoNewForm {
  ownerID: number;
  name: string;
  description: string;
  private: boolean;
  unlisted: boolean;
}

export interface RepoCreateForm extends RepoNewForm {
  autoInit: boolean;
  gitignores: string[];
  license: string;
  readme: string;
  objectFormat: "sha1" | "sha256";
}

export interface RepoMigrateForm extends RepoNewForm {
  cloneAddr: string;
  authUsername: string;
  authPassword: string;
  mirror: boolean;
}

export function createRepo(form: RepoCreateForm) {
  return submit<{ owner: string; name: string }>("POST", subUrl("/api/web/repo/create"), form);
}

export function migrateRepo(form: RepoMigrateForm) {
  return submit<{ owner: string; name: string }>("POST", subUrl("/api/web/repo/migrate"), form);
}

export interface RepoForkOptions {
  base: {
    owner: string;
    name: string;
    description: string;
    isPrivate: boolean;
    isUnlisted: boolean;
  };
  owners: RepoNewOwner[];
}

export function fetchRepoForkOptions(id: number) {
  return fetchJSON<RepoForkOptions>(subUrl(`/api/web/repo/fork/${id}`));
}

export interface RepoForkForm {
  ownerID: number;
  name: string;
  description: string;
}

// forkRepo forks a repository, or finds the owner's existing fork of it.
export function forkRepo(id: number, form: RepoForkForm) {
  return submit<{ owner: string; name: string }>("POST", subUrl(`/api/web/repo/fork/${id}`), form);
}

export interface RepoAdvancedSettings {
  enableWiki: boolean;
  allowPublicWiki: boolean;
  enableExternalWiki: boolean;
  externalWikiURL: string;
  enableIssues: boolean;
  allowPublicIssues: boolean;
  enableExternalTracker: boolean;
  externalTrackerURL: string;
  externalTrackerFormat: string;
  externalTrackerStyle: string;
  enablePulls: boolean;
  pullsIgnoreWhitespace: boolean;
  pullsAllowRebase: boolean;
}

export interface RepoMirrorSettings {
  address: string;
  interval: number;
  enablePrune: boolean;
  // Only sent for mirrors that synced since they were created.
  lastSyncedAt?: string;
}

export interface RepoSettingsData {
  name: string;
  description: string;
  website: string;
  private: boolean;
  unlisted: boolean;
  // Forks keep the visibility of their base repository.
  isFork: boolean;
  forkCount: number;
  avatarURL: string;
  hasCustomAvatar: boolean;
  advanced: RepoAdvancedSettings;
  canEnablePulls: boolean;
  // Only sent for mirrors.
  mirror?: RepoMirrorSettings;
  ownerIsOrganization: boolean;
  // The owners of the repository see its danger zone.
  viewerIsOwner: boolean;
  viewerCanEditGitHooks: boolean;
}

function repoSettingsURL(owner: string, name: string, path = "") {
  return subUrl(`/api/web/${owner}/${name}/settings${path}`);
}

// repoSettingsQuery fetches the settings of a repository, which only its
// admins can see.
export function repoSettingsQuery(owner: string, name: string) {
  return queryOptions({
    queryKey: ["repo", owner, name, "settings"] as const,
    queryFn: () => fetchJSON<RepoSettingsData>(repoSettingsURL(owner, name)),
  });
}

export interface RepoSettingsForm {
  name: string;
  description: string;
  website: string;
  private: boolean;
  unlisted: boolean;
}

export function updateRepoSettings(owner: string, name: string, form: RepoSettingsForm) {
  return submit<{ owner: string; name: string }>("PATCH", repoSettingsURL(owner, name), form);
}

export function uploadRepoAvatar(owner: string, name: string, file: File) {
  return uploadAvatar(repoSettingsURL(owner, name, "/avatar"), file);
}

export function deleteRepoAvatar(owner: string, name: string) {
  return send("DELETE", repoSettingsURL(owner, name, "/avatar"));
}

export function updateRepoMirror(
  owner: string,
  name: string,
  form: { address: string; interval: number; enablePrune: boolean },
) {
  return submit<void>("PUT", repoSettingsURL(owner, name, "/mirror"), form);
}

export function syncRepoMirror(owner: string, name: string) {
  return submit<void>("POST", repoSettingsURL(owner, name, "/mirror/sync"), {});
}

export function updateRepoAdvanced(owner: string, name: string, form: RepoAdvancedSettings) {
  return submit<void>("PUT", repoSettingsURL(owner, name, "/advanced"), form);
}

export function convertRepo(owner: string, name: string, repoName: string) {
  return submit<void>("POST", repoSettingsURL(owner, name, "/convert"), { repoName });
}

export function transferRepo(owner: string, name: string, form: { repoName: string; newOwner: string }) {
  return submit<{ owner: string; name: string }>("POST", repoSettingsURL(owner, name, "/transfer"), form);
}

export function deleteRepoWiki(owner: string, name: string, repoName: string) {
  return submit<void>("DELETE", repoSettingsURL(owner, name, "/wiki"), { repoName });
}

export function deleteRepo(owner: string, name: string, repoName: string) {
  return submit<void>("DELETE", subUrl(`/api/web/${owner}/${name}`), { repoName });
}

export type RepoCollaboratorMode = "read" | "write" | "admin";

export interface RepoCollaborator {
  name: string;
  fullName: string;
  avatarURL: string;
  mode: RepoCollaboratorMode;
}

export async function fetchRepoCollaborators(owner: string, name: string) {
  return (await fetchJSON<{ items: RepoCollaborator[] }>(repoSettingsURL(owner, name, "/collaborators"))).items;
}

export function addRepoCollaborator(owner: string, name: string, username: string) {
  return submit<void>("POST", repoSettingsURL(owner, name, "/collaborators"), { username });
}

export function changeRepoCollaborator(owner: string, name: string, username: string, mode: RepoCollaboratorMode) {
  return submit<void>("PATCH", repoSettingsURL(owner, name, `/collaborators/${encodeURIComponent(username)}`), {
    mode,
  });
}

export function removeRepoCollaborator(owner: string, name: string, username: string) {
  return send("DELETE", repoSettingsURL(owner, name, `/collaborators/${encodeURIComponent(username)}`));
}

export async function fetchRepoDeployKeys(owner: string, name: string) {
  return (await fetchJSON<{ items: SSHKey[] }>(repoSettingsURL(owner, name, "/keys"))).items;
}

export function addRepoDeployKey(owner: string, name: string, form: { title: string; content: string }) {
  return submit<void>("POST", repoSettingsURL(owner, name, "/keys"), form);
}

export function deleteRepoDeployKey(owner: string, name: string, id: number) {
  return send("DELETE", repoSettingsURL(owner, name, `/keys/${id}`));
}

function repoURL(owner: string, name: string, path: string) {
  return subUrl(`/api/web/${owner}/${name}${path}`);
}

export interface RepoLabel {
  id: number;
  name: string;
  // color is a hex color, e.g., "#70c24a".
  color: string;
  openIssueCount: number;
}

export interface RepoLabelsData {
  items: RepoLabel[];
  // templates are the predefined sets of labels, only sent to viewers who can
  // add them.
  templates?: string[];
  viewerCanWrite: boolean;
}

export interface RepoLabelForm {
  name: string;
  color: string;
}

export function fetchRepoLabels(owner: string, name: string) {
  return fetchJSON<RepoLabelsData>(repoURL(owner, name, "/labels"));
}

export function createRepoLabel(owner: string, name: string, form: RepoLabelForm) {
  return submit<void>("POST", repoURL(owner, name, "/labels"), form);
}

export function updateRepoLabel(owner: string, name: string, id: number, form: RepoLabelForm) {
  return submit<void>("PATCH", repoURL(owner, name, `/labels/${id}`), form);
}

export function deleteRepoLabel(owner: string, name: string, id: number) {
  return send("DELETE", repoURL(owner, name, `/labels/${id}`));
}

export function initializeRepoLabels(owner: string, name: string, template: string) {
  return submit<void>("POST", repoURL(owner, name, "/labels/initialize"), { template });
}

export interface RepoMilestone {
  id: number;
  name: string;
  descriptionHTML?: string;
  // deadline is a date, e.g., "2026-12-31", and is left out when there is
  // none.
  deadline?: string;
  isOverdue: boolean;
  isClosed: boolean;
  closedAt?: string;
  openIssueCount: number;
  closedIssueCount: number;
  completeness: number;
}

export interface RepoMilestonesData {
  items: RepoMilestone[];
  page: number;
  totalPages: number;
  openCount: number;
  closedCount: number;
  viewerCanWrite: boolean;
}

export interface RepoMilestoneForm {
  title: string;
  description: string;
  // deadline is a date, e.g., "2026-12-31", or empty for none.
  deadline: string;
}

export function fetchRepoMilestones(owner: string, name: string, closed: boolean, page?: number) {
  const params = new URLSearchParams();
  if (closed) params.set("state", "closed");
  if (page) params.set("page", String(page));
  const query = params.toString();
  return fetchJSON<RepoMilestonesData>(repoURL(owner, name, `/milestones${query ? `?${query}` : ""}`));
}

export function fetchRepoMilestone(owner: string, name: string, id: number) {
  return fetchJSON<RepoMilestoneForm>(repoURL(owner, name, `/milestones/${id}`));
}

export function createRepoMilestone(owner: string, name: string, form: RepoMilestoneForm) {
  return submit<void>("POST", repoURL(owner, name, "/milestones"), form);
}

export function updateRepoMilestone(owner: string, name: string, id: number, form: RepoMilestoneForm) {
  return submit<void>("PATCH", repoURL(owner, name, `/milestones/${id}`), form);
}

export function setRepoMilestoneClosed(owner: string, name: string, id: number, closed: boolean) {
  return send(closed ? "PUT" : "DELETE", repoURL(owner, name, `/milestones/${id}/closed`));
}

export function deleteRepoMilestone(owner: string, name: string, id: number) {
  return send("DELETE", repoURL(owner, name, `/milestones/${id}`));
}
