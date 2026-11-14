import { queryOptions } from "@tanstack/react-query";

import type { UserListPage } from "@/components/UserListItem";
import { loaderResponseError } from "@/lib/loader-error";
import { fetchJSON, send, submit, uploadAvatar } from "@/lib/queries/request";
import type { SSHKey } from "@/lib/queries/settings";
import type { DashboardIssue, IssueLabel } from "@/lib/queries/user";
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

export interface RepoIssue extends DashboardIssue {
  isUnread: boolean;
  milestone?: { id: number; name: string };
}

export interface RepoIssuesData {
  items: RepoIssue[];
  page: number;
  totalPages: number;
  openCount: number;
  closedCount: number;
  // labels, milestones and assignees are what the list can be filtered by.
  labels: IssueLabel[];
  milestones: { id: number; name: string; isClosed: boolean }[];
  assignees: { id: number; name: string; avatarURL: string }[];
  // newPullRequestURL is where the viewer proposes a pull request, and is left
  // out for viewers who cannot propose one.
  newPullRequestURL?: string;
}

// fetchRepoIssues lists the issues or pull requests of a repository, with the
// query parameters of the server-rendered page, e.g., `state=closed`.
export function fetchRepoIssues(
  owner: string,
  name: string,
  kind: "issues" | "pulls",
  query: Record<string, string | number | undefined>,
) {
  const params = new URLSearchParams();
  for (const [key, value] of Object.entries(query)) {
    if (value !== undefined) params.set(key, String(value));
  }
  const search = params.toString();
  return fetchJSON<RepoIssuesData>(repoURL(owner, name, `/${kind}${search ? `?${search}` : ""}`));
}

export interface RepoFork {
  owner: string;
  ownerAvatarURL: string;
  name: string;
  isPrivate: boolean;
}

export async function fetchRepoForks(owner: string, name: string) {
  return (await fetchJSON<{ items: RepoFork[] }>(repoURL(owner, name, "/forks"))).items;
}

export interface IssueUser {
  name: string;
  avatarURL: string;
  // isGhost marks a deleted user, who has no profile.
  isGhost?: boolean;
}

export interface IssueAttachment {
  name: string;
  url: string;
  isImage: boolean;
}

export interface IssueComment {
  id: number;
  type: "comment" | "reopen" | "close" | "commit_ref";
  poster: IssueUser;
  createdAt: string;
  // contentHTML is the rendered content of a comment, or the link to the
  // commit of a commit reference.
  contentHTML?: string;
  // content is the Markdown of a comment, only sent to viewers who can edit
  // it.
  content?: string;
  tag?: "owner" | "writer" | "poster";
  attachments?: IssueAttachment[];
  viewerCanEdit?: boolean;
}

export interface IssueAttachmentSettings {
  allowedTypes: string[];
  // maxSize is the largest file in bytes.
  maxSize: number;
  maxFiles: number;
}

// IssueMetaOptions are the labels, milestones and assignees that writers can
// set on issues.
export interface IssueMetaOptions {
  labelOptions?: IssueLabel[];
  milestoneOptions?: { id: number; name: string; isClosed: boolean }[];
  assigneeOptions?: { id: number; name: string; avatarURL: string }[];
}

export interface RepoIssueData extends IssueMetaOptions {
  index: number;
  title: string;
  isClosed: boolean;
  poster: IssueUser;
  createdAt: string;
  contentHTML: string;
  content: string;
  attachments: IssueAttachment[];
  comments: IssueComment[];
  labels: IssueLabel[];
  milestone?: { id: number; name: string };
  assignee?: { id: number; name: string; avatarURL: string };
  participants: IssueUser[];
  // attachment is how files can be attached to comments, only sent to
  // signed-in viewers when attachments are enabled.
  attachment?: IssueAttachmentSettings;
  viewerCanEdit: boolean;
  viewerCanWrite: boolean;
}

export function fetchRepoIssue(owner: string, name: string, index: number) {
  return fetchJSON<RepoIssueData>(repoURL(owner, name, `/issues/${index}`));
}

export interface RepoNewIssueOptions extends IssueMetaOptions {
  // defaultBranch is where the issue template is read from, only sent to
  // viewers who can read the repository.
  defaultBranch?: string;
  attachment?: IssueAttachmentSettings;
  viewerCanWrite: boolean;
}

// The files the issue template is read from, in the order the server-rendered
// page looked for them.
const ISSUE_TEMPLATE_FILES = ["ISSUE_TEMPLATE.md", ".gogs/ISSUE_TEMPLATE.md", ".github/ISSUE_TEMPLATE.md"];

// fetchRepoNewIssue loads what the form to open an issue offers, along with
// the issue template of the repository, if any.
export async function fetchRepoNewIssue(owner: string, name: string) {
  const options = await fetchJSON<RepoNewIssueOptions>(repoURL(owner, name, "/issues/new"));
  let template = "";
  if (options.defaultBranch) {
    const branch = encodeURIComponent(options.defaultBranch);
    const found = await Promise.all(
      ISSUE_TEMPLATE_FILES.map(async (file) => {
        const res = await fetch(subUrl(`/${owner}/${name}/raw/${branch}/${file}`), { credentials: "same-origin" });
        return res.ok ? res.text() : null;
      }),
    );
    template = found.find((text) => text !== null) ?? "";
  }
  return { ...options, template };
}

export interface RepoNewIssueForm {
  title: string;
  content: string;
  labelIDs: number[];
  milestoneID: number;
  assigneeID: number;
  attachments: string[];
}

export function createRepoIssue(owner: string, name: string, form: RepoNewIssueForm) {
  return submit<{ index: number }>("POST", repoURL(owner, name, "/issues"), form);
}

export function updateRepoIssue(
  owner: string,
  name: string,
  index: number,
  form: { title?: string; content?: string },
) {
  return submit<void>("PATCH", repoURL(owner, name, `/issues/${index}`), form);
}

export function setRepoIssueClosed(owner: string, name: string, index: number, closed: boolean) {
  return send(closed ? "PUT" : "DELETE", repoURL(owner, name, `/issues/${index}/closed`));
}

export function createRepoIssueComment(
  owner: string,
  name: string,
  index: number,
  form: { content: string; attachments: string[] },
) {
  return submit<{ id: number }>("POST", repoURL(owner, name, `/issues/${index}/comments`), form);
}

export function updateRepoIssueComment(owner: string, name: string, index: number, id: number, content: string) {
  return submit<void>("PATCH", repoURL(owner, name, `/issues/${index}/comments/${id}`), { content });
}

export function deleteRepoIssueComment(owner: string, name: string, index: number, id: number) {
  return send("DELETE", repoURL(owner, name, `/issues/${index}/comments/${id}`));
}

export function setRepoIssueLabels(owner: string, name: string, index: number, ids: number[]) {
  return submit<void>("PUT", repoURL(owner, name, `/issues/${index}/labels`), { ids });
}

// setRepoIssueMilestone sets the milestone of the issue, or clears it with 0.
export function setRepoIssueMilestone(owner: string, name: string, index: number, id: number) {
  return submit<void>("PUT", repoURL(owner, name, `/issues/${index}/milestone`), { id });
}

// setRepoIssueAssignee sets the assignee of the issue, or clears it with 0.
export function setRepoIssueAssignee(owner: string, name: string, index: number, id: number) {
  return submit<void>("PUT", repoURL(owner, name, `/issues/${index}/assignee`), { id });
}

// renderRepoMarkdown renders Markdown the way the issues of the repository
// are.
export async function renderRepoMarkdown(owner: string, name: string, text: string) {
  return (await submit<{ html: string }>("POST", repoURL(owner, name, "/markdown"), { text })).html;
}

// uploadRepoAttachment uploads a file to attach to an issue, a comment or a
// release, and returns its UUID.
export async function uploadRepoAttachment(owner: string, name: string, kind: "issues" | "releases", file: File) {
  const body = new FormData();
  body.append("file", file);
  const res = await fetch(repoURL(owner, name, `/${kind}/attachments`), {
    method: "POST",
    credentials: "same-origin",
    body,
  });
  if (!res.ok) throw await loaderResponseError(res);
  return ((await res.json()) as { uuid: string }).uuid;
}

export interface RepoReleaseAttachment {
  uuid: string;
  name: string;
  url: string;
}

export interface RepoRelease {
  tagName: string;
  sha: string;
  // The fields below are left out for a tag that has no release.
  title?: string;
  target?: string;
  isDraft?: boolean;
  isPrerelease?: boolean;
  publisher?: { name: string; avatarURL: string; isGhost?: boolean };
  createdAt?: string;
  noteHTML?: string;
  attachments?: RepoReleaseAttachment[];
  // numCommitsBehind is how many commits the target branch is ahead of the
  // release.
  numCommitsBehind?: number;
}

export interface RepoReleasesData {
  releases: RepoRelease[];
  // previousAfter is the "after" of the previous page when hasPrevious is set.
  hasPrevious: boolean;
  previousAfter: string;
  // nextAfter is left out on the last page.
  nextAfter?: string;
  viewerCanWrite: boolean;
  isMirror: boolean;
}

// releaseTagPath encodes a tag name, which may contain slashes, for a path.
export function releaseTagPath(tag: string) {
  return tag.split("/").map(encodeURIComponent).join("/");
}

export function fetchRepoReleases(owner: string, name: string, after?: string) {
  const query = after ? `?${new URLSearchParams({ after }).toString()}` : "";
  return fetchJSON<RepoReleasesData>(repoURL(owner, name, `/releases${query}`));
}

export interface RepoReleaseFormOptions {
  branches: string[];
  defaultBranch: string;
  attachment?: IssueAttachmentSettings;
}

export function fetchRepoNewRelease(owner: string, name: string) {
  return fetchJSON<RepoReleaseFormOptions>(repoURL(owner, name, "/releases/new"));
}

export interface RepoReleaseEdit {
  tagName: string;
  target: string;
  title: string;
  content: string;
  isDraft: boolean;
  isPrerelease: boolean;
  attachments: RepoReleaseAttachment[];
  attachment?: IssueAttachmentSettings;
}

export function fetchRepoRelease(owner: string, name: string, tag: string) {
  return fetchJSON<RepoReleaseEdit>(repoURL(owner, name, `/releases/edit/${releaseTagPath(tag)}`));
}

export interface RepoReleaseForm {
  // tagName and target are only read when a release is created.
  tagName?: string;
  target?: string;
  title: string;
  content: string;
  isDraft: boolean;
  isPrerelease: boolean;
  attachments: string[];
}

export function createRepoRelease(owner: string, name: string, form: RepoReleaseForm) {
  return submit<void>("POST", repoURL(owner, name, "/releases"), form);
}

export function updateRepoRelease(owner: string, name: string, tag: string, form: RepoReleaseForm) {
  return submit<void>("PATCH", repoURL(owner, name, `/releases/edit/${releaseTagPath(tag)}`), form);
}

export function deleteRepoRelease(owner: string, name: string, tag: string) {
  return send("DELETE", repoURL(owner, name, `/releases/edit/${releaseTagPath(tag)}`));
}
