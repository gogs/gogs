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
  // viewerCanRead is false for visitors who only see the public issues or wiki.
  viewerCanRead: boolean;
  viewerCanAdminister: boolean;
  viewerCanWrite: boolean;
  issuesEnabled: boolean;
  pullRequestsEnabled: boolean;
  wikiEnabled: boolean;
  // externalTrackerURL and externalWikiURL are set when the issues or the
  // wiki are kept elsewhere, for viewers who can read the code.
  externalTrackerURL?: string;
  externalWikiURL?: string;
  watchCount: number;
  starCount: number;
  forkCount: number;
  openIssueCount: number;
  openPullRequestCount: number;
  viewerIsWatching: boolean;
  viewerIsStarring: boolean;
  mirrorOf?: string;
  // defaultBranch and cloneLinks are set for viewers who can read the code.
  defaultBranch?: string;
  cloneLinks?: RepoCloneLinks;
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
    headers: { "Content-Type": "application/json" },
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
    headers: { "Content-Type": "application/json" },
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
  // The site makes every new repository, including forks, private.
  forcePrivate: boolean;
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
  pullsAllowMerge: boolean;
  pullsAllowRebase: boolean;
  pullsAllowSquash: boolean;
  pullsDeleteBranch: boolean;
  // The presets of the messages merge commits and squashing start with,
  // empty for the default message.
  pullsMergeMessage: "" | "title" | "title_description";
  pullsSquashMessage: "" | "title" | "title_description" | "title_commits";
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
  isFork: boolean;
  // Forks keep the visibility of their base repository, unless the site forces private repositories.
  visibilityFollowsBase: boolean;
  // The site forces private repositories and the viewer cannot make this one public.
  forcePrivate: boolean;
  // False for forks whose base repository is private or unlisted.
  canBePublic: boolean;
  canBeListed: boolean;
  // False for forks whose base repository does not let them allow public access.
  canAllowPublicWiki: boolean;
  canAllowPublicIssues: boolean;
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

export interface RepoGitHook {
  name: string;
  // isActive is false when the hook only has its sample content.
  isActive: boolean;
}

export async function fetchRepoGitHooks(owner: string, name: string) {
  return (await fetchJSON<{ items: RepoGitHook[] }>(repoSettingsURL(owner, name, "/githooks"))).items;
}

export interface RepoGitHookContent extends RepoGitHook {
  content: string;
}

export function fetchRepoGitHook(owner: string, name: string, hook: string) {
  return fetchJSON<RepoGitHookContent>(repoSettingsURL(owner, name, `/githooks/${encodeURIComponent(hook)}`));
}

export function updateRepoGitHook(owner: string, name: string, hook: string, content: string) {
  return submit<void>("PUT", repoSettingsURL(owner, name, `/githooks/${encodeURIComponent(hook)}`), { content });
}

export function deleteRepoGitHook(owner: string, name: string, hook: string) {
  return send("DELETE", repoSettingsURL(owner, name, `/githooks/${encodeURIComponent(hook)}`));
}

export interface RepoBranchSettingsData {
  // isEmpty is set for a repository without commits, which has no branches
  // to manage yet.
  isEmpty: boolean;
  defaultBranch: string;
  branches: string[];
  protectedBranches: string[];
}

export function fetchRepoBranchSettings(owner: string, name: string) {
  return fetchJSON<RepoBranchSettingsData>(repoSettingsURL(owner, name, "/branches"));
}

export function updateRepoDefaultBranch(owner: string, name: string, branch: string) {
  return submit<void>("PUT", repoSettingsURL(owner, name, "/default-branch"), { branch });
}

export interface RepoProtectedBranchForm {
  protected: boolean;
  requirePullRequest: boolean;
  enableAllowlist: boolean;
  allowlistUserIDs: number[];
  allowlistTeamIDs: number[];
}

export interface RepoProtectedBranch extends RepoProtectedBranchForm {
  name: string;
  // The options are only sent for repositories of organizations, which alone
  // can let people and teams push past the protection.
  userOptions?: { id: number; name: string; avatarURL: string }[];
  teamOptions?: { id: number; name: string }[];
}

export function fetchRepoProtectedBranch(owner: string, name: string, branch: string) {
  return fetchJSON<RepoProtectedBranch>(repoSettingsURL(owner, name, `/branches/${refPath(branch)}`));
}

export function updateRepoProtectedBranch(owner: string, name: string, branch: string, form: RepoProtectedBranchForm) {
  return submit<void>("PUT", repoSettingsURL(owner, name, `/branches/${refPath(branch)}`), form);
}

// refPath encodes the name of a branch or a tag, which may contain slashes,
// for a path.
export function refPath(ref: string) {
  return ref.split("/").map(encodeURIComponent).join("/");
}

function repoURL(owner: string, name: string, path: string) {
  return subUrl(`/api/web/${owner}/${name}${path}`);
}

export interface RepoBranch {
  name: string;
  isProtected: boolean;
  commitID: string;
  committedAt: string;
  committerName: string;
}

// RepoBranchesView is a view of the branches page. Every view but the overview
// is a paged list.
export type RepoBranchesView = "overview" | "yours" | "active" | "stale" | "all";

export interface RepoBranchSection {
  kind: "default" | "yours" | "active" | "stale";
  branches: RepoBranch[];
  // hasMore is true when the section shows only the first of its branches.
  hasMore: boolean;
}

export interface RepoBranchesData {
  defaultBranch: string;
  view: RepoBranchesView;
  // sections are the groups of the overview, and empty for the other views.
  sections: RepoBranchSection[];
  // branches are the page of the other views, and empty for the overview.
  branches: RepoBranch[];
  page: number;
  hasNext: boolean;
  viewerCanWrite: boolean;
  allowsPulls: boolean;
  viewerCanChangeDefault: boolean;
}

export function fetchRepoBranches(
  owner: string,
  name: string,
  { view, query, page }: { view: RepoBranchesView; query?: string; page?: number },
) {
  const params = new URLSearchParams({ view });
  if (query) params.set("query", query);
  if (page) params.set("page", String(page));
  return fetchJSON<RepoBranchesData>(repoURL(owner, name, `/branches?${params.toString()}`));
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
// query parameters of the list page, e.g., `state=closed`.
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
  // completeness is the percentage of the issues of the milestone that are
  // closed.
  milestone?: { id: number; name: string; completeness: number };
  assignee?: { id: number; name: string; avatarURL: string };
  participants: IssueUser[];
  // attachment is how files can be attached to comments, only sent to
  // authenticated viewers when attachments are enabled.
  attachment?: IssueAttachmentSettings;
  viewerCanEdit: boolean;
  viewerCanWrite: boolean;
}

export function fetchRepoIssue(owner: string, name: string, index: number) {
  return fetchJSON<RepoIssueData>(repoURL(owner, name, `/issues/${index}`));
}

export interface IssueTemplateSummary {
  // file is the name of the template in the ISSUE_TEMPLATE directory.
  file: string;
  name: string;
  about: string;
}

export interface IssueTemplateField {
  type: "markdown" | "textarea" | "input" | "dropdown" | "checkboxes" | "upload";
  label?: string;
  // descriptionHTML and html are rendered Markdown.
  descriptionHTML?: string;
  html?: string;
  placeholder?: string;
  value?: string;
  render?: string;
  multiple?: boolean;
  // labelHTML is the rendered Markdown of the label of a checkbox.
  options?: { label: string; labelHTML?: string; required?: boolean }[];
  // default is the index of the option a dropdown starts with.
  default?: number;
  required?: boolean;
  // accept is the comma-separated file extensions an upload takes.
  accept?: string;
}

export interface IssueTemplate extends IssueTemplateSummary {
  title: string;
  labels: string[];
  assignees: string[];
  // body is the content of a Markdown template, and form the fields of an
  // issue form.
  body: string;
  form?: IssueTemplateField[];
}

export interface RepoNewIssueOptions extends IssueMetaOptions {
  // The templates are only sent to viewers who can read the repository.
  templates: IssueTemplateSummary[];
  contactLinks: { name: string; url: string; about: string }[];
  blankIssuesEnabled: boolean;
  // template is the one asked for, if it exists.
  template?: IssueTemplate;
  // content is what a blank issue starts with.
  content: string;
  attachment?: IssueAttachmentSettings;
  viewerCanWrite: boolean;
}

// fetchRepoNewIssue loads what the form to open an issue offers, along with
// the issue templates of the repository and the chosen one, if any.
export function fetchRepoNewIssue(owner: string, name: string, template?: string) {
  const query = template ? `?template=${encodeURIComponent(template)}` : "";
  return fetchJSON<RepoNewIssueOptions>(repoURL(owner, name, `/issues/new${query}`));
}

export interface RepoNewIssueForm {
  title: string;
  content: string;
  labelIDs: number[];
  milestoneID: number;
  assigneeID: number;
  attachments: string[];
  // template is the file of the chosen template. fields are the values of
  // the elements of an issue form, in order.
  template?: string;
  fields?: string[][];
}

export function createRepoIssue(owner: string, name: string, form: RepoNewIssueForm) {
  return submit<{ index: number }>("POST", repoURL(owner, name, "/issues"), form);
}

// IssueRef names an issue or a pull request, which take the same changes
// under their own path.
export interface IssueRef {
  owner: string;
  name: string;
  kind: "issues" | "pulls";
  index: number;
}

function issueURL(issue: IssueRef, path = "") {
  return repoURL(issue.owner, issue.name, `/${issue.kind}/${issue.index}${path}`);
}

// attachments are the UUIDs of the files uploaded while editing the content.
export function updateRepoIssue(issue: IssueRef, form: { title?: string; content?: string; attachments?: string[] }) {
  return submit<void>("PATCH", issueURL(issue), form);
}

export function setRepoIssueClosed(issue: IssueRef, closed: boolean) {
  return send(closed ? "PUT" : "DELETE", issueURL(issue, "/closed"));
}

export function createRepoIssueComment(issue: IssueRef, form: { content: string; attachments: string[] }) {
  return submit<{ id: number }>("POST", issueURL(issue, "/comments"), form);
}

export function updateRepoIssueComment(issue: IssueRef, id: number, content: string, attachments: string[] = []) {
  return submit<void>("PATCH", issueURL(issue, `/comments/${id}`), { content, attachments });
}

export function deleteRepoIssueComment(issue: IssueRef, id: number) {
  return send("DELETE", issueURL(issue, `/comments/${id}`));
}

export function setRepoIssueLabels(issue: IssueRef, ids: number[]) {
  return submit<void>("PUT", issueURL(issue, "/labels"), { ids });
}

// setRepoIssueMilestone sets the milestone of the issue, or clears it with 0.
export function setRepoIssueMilestone(issue: IssueRef, id: number) {
  return submit<void>("PUT", issueURL(issue, "/milestone"), { id });
}

// setRepoIssueAssignee sets the assignee of the issue, or clears it with 0.
export function setRepoIssueAssignee(issue: IssueRef, id: number) {
  return submit<void>("PUT", issueURL(issue, "/assignee"), { id });
}

export interface RepoPullInfo {
  // state is "broken" when a branch or the fork is gone, and "checking"
  // while conflicts are checked.
  state: "merged" | "closed" | "broken" | "checking" | "mergeable" | "conflict";
  headBranch: string;
  baseBranch: string;
  // headLabel and baseLabel name the branches after their owners, or are
  // "deleted" for a branch that is gone.
  headLabel: string;
  baseLabel: string;
  headRepoLink?: string;
  commitCount: number;
  fileCount: number;
  merger?: IssueUser;
  mergedAt?: string;
  allowMerge: boolean;
  allowRebase: boolean;
  allowSquash: boolean;
  viewerCanMerge: boolean;
  // viewerNotInAllowlist is whether the viewer could merge if they were in
  // the push allowlist of the protected base branch.
  viewerNotInAllowlist: boolean;
  // viewerID is the viewer who can merge, whose browser remembers the merge
  // style they last used in the repository.
  viewerID?: number;
  // mergeMessages are the messages the commits of the merge styles that
  // create one start with, for viewers who can merge.
  mergeMessages?: Partial<Record<MergeStyle, MergeMessage>>;
  viewerCanDeleteBranch: boolean;
  // viewerCanRestoreBranch is whether the viewer can create the deleted head
  // branch again at the commit that was merged.
  viewerCanRestoreBranch: boolean;
  // diff is the commits the changes are between, in the repository that has
  // both, unless the pull request is broken.
  // markdownExtensions are the extensions of files that show a rich diff.
  diff?: { base: string; head: string; owner: string; name: string; markdownExtensions: string[] };
}

export interface RepoPullData extends RepoIssueData {
  pull: RepoPullInfo;
}

export function fetchRepoPull(owner: string, name: string, index: number) {
  return fetchJSON<RepoPullData>(repoURL(owner, name, `/pulls/${index}`));
}

export function fetchRepoPullCommits(owner: string, name: string, index: number) {
  return fetchJSON<{ commits: RepoCommitListItem[] }>(repoURL(owner, name, `/pulls/${index}/commits`));
}

export type MergeStyle = "create_merge_commit" | "rebase_before_merging" | "squash_and_merge";

export interface MergeMessage {
  title: string;
  body: string;
}

export function mergeRepoPull(issue: IssueRef, form: { style: MergeStyle } & MergeMessage) {
  return submit<void>("POST", issueURL(issue, "/merge"), form);
}

// deleteRepoPullBranch deletes the head branch of the merged pull request.
export function deleteRepoPullBranch(issue: IssueRef) {
  return send("DELETE", issueURL(issue, "/branch"));
}

// restoreRepoPullBranch creates the deleted head branch of the merged pull
// request again at the commit that was merged.
export function restoreRepoPullBranch(issue: IssueRef) {
  return send("POST", issueURL(issue, "/branch"));
}

export interface RepoNewPullData extends IssueMetaOptions {
  baseBranch: string;
  // head names the head branch as the URL of the comparison does, with its
  // owner when it is in a fork.
  head: string;
  headOwner: string;
  headName: string;
  headBranch: string;
  // baseRepos are the repositories the head branch can be merged into: the
  // repository of the head branch and its base repository when it is a fork,
  // each when it takes pull requests.
  baseRepos: { owner: string; ownerAvatarURL: string; name: string; defaultBranch: string }[];
  // branches are those of the repository to merge into, and headBranches
  // those of the repository of the head branch.
  branches: string[];
  headBranches: string[];
  // state is "unrelated" when the branches have no common history, "even"
  // when the head branch has nothing to merge, and "exists" when an open
  // pull request has the same branches.
  state: "unrelated" | "even" | "exists" | "ready";
  pullIndex?: number;
  commits: RepoCommitListItem[];
  // diff is what the pull request would change, unless there is nothing to
  // merge, in the repository of the head branch.
  diff?: { base: string; head: string; owner: string; name: string; markdownExtensions: string[] };
  // title and template prefill the form from the templates of the default
  // branch.
  title: string;
  template: string;
  attachment?: IssueAttachmentSettings;
  viewerCanWrite: boolean;
}

// fetchRepoNewPull compares the branches of the range, written as
// "<base>...[<owner>:]<head>", to open a pull request from them.
export function fetchRepoNewPull(owner: string, name: string, range: string) {
  return fetchJSON<RepoNewPullData>(repoURL(owner, name, `/pulls/new/${refPath(range)}`));
}

export function createRepoPull(owner: string, name: string, form: RepoNewIssueForm & { base: string; head: string }) {
  return submit<{ index: number }>("POST", repoURL(owner, name, "/pulls"), form);
}

// renderRepoMarkdown renders Markdown the way the issues of the repository
// are. With wiki set, it renders the way its wiki pages are, and with branch
// and path set, the way that file is.
export async function renderRepoMarkdown(
  owner: string,
  name: string,
  text: string,
  as: { wiki?: boolean; branch?: string; path?: string } = {},
) {
  return (await submit<{ html: string }>("POST", repoURL(owner, name, "/markdown"), { text, ...as })).html;
}

// uploadRepoAttachment uploads a file to attach to an issue, a comment or a
// release, or to commit with the editor, and returns its UUID and, except for
// the editor, the URL content links to it with.
export async function uploadRepoAttachment(
  owner: string,
  name: string,
  kind: "issues" | "releases" | "editor",
  file: File,
) {
  const body = new FormData();
  body.append("file", file);
  const res = await fetch(repoURL(owner, name, kind === "editor" ? "/editor/uploads" : `/${kind}/attachments`), {
    method: "POST",
    credentials: "same-origin",
    body,
  });
  if (!res.ok) throw await loaderResponseError(res);
  return (await res.json()) as { uuid: string; url?: string };
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
  return fetchJSON<RepoReleaseEdit>(repoURL(owner, name, `/releases/edit/${refPath(tag)}`));
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
  // removedAttachments are the downloads removed in the editor, which are
  // deleted.
  removedAttachments?: string[];
}

export function createRepoRelease(owner: string, name: string, form: RepoReleaseForm) {
  return submit<void>("POST", repoURL(owner, name, "/releases"), form);
}

export function updateRepoRelease(owner: string, name: string, tag: string, form: RepoReleaseForm) {
  return submit<void>("PATCH", repoURL(owner, name, `/releases/edit/${refPath(tag)}`), form);
}

export function deleteRepoRelease(owner: string, name: string, tag: string) {
  return send("DELETE", repoURL(owner, name, `/releases/edit/${refPath(tag)}`));
}

export interface RepoWikiPage {
  name: string;
  html: string;
  author: { name: string; when: string };
}

export interface RepoWikiData {
  // hasWiki is false until the first page is created, and the other fields
  // are then left out.
  hasWiki: boolean;
  page?: RepoWikiPage;
  pages?: string[];
  cloneLinks?: { https?: string; ssh?: string };
  viewerCanWrite: boolean;
}

function wikiPageQuery(page: string) {
  return `?${new URLSearchParams({ page }).toString()}`;
}

export function fetchRepoWiki(owner: string, name: string, page: string) {
  return fetchJSON<RepoWikiData>(repoURL(owner, name, `/wiki${wikiPageQuery(page)}`));
}

export interface RepoWikiPagesData {
  pages: { name: string; updatedAt: string }[];
  viewerCanWrite: boolean;
}

export function fetchRepoWikiPages(owner: string, name: string) {
  return fetchJSON<RepoWikiPagesData>(repoURL(owner, name, "/wiki/pages"));
}

export function fetchRepoWikiPageSource(owner: string, name: string, page: string) {
  return fetchJSON<{ name: string; content: string }>(repoURL(owner, name, `/wiki/source${wikiPageQuery(page)}`));
}

export interface RepoWikiPageForm {
  // oldTitle is the page being changed, and is left out for a new page.
  oldTitle?: string;
  title: string;
  content: string;
  message: string;
}

// saveRepoWikiPage creates or changes a page, and returns the name it was
// saved under.
export async function saveRepoWikiPage(owner: string, name: string, form: RepoWikiPageForm) {
  const { name: saved } = await submit<{ name: string }>(
    form.oldTitle === undefined ? "POST" : "PATCH",
    repoURL(owner, name, "/wiki"),
    form,
  );
  return saved;
}

export function deleteRepoWikiPage(owner: string, name: string, page: string) {
  return send("DELETE", repoURL(owner, name, `/wiki${wikiPageQuery(page)}`));
}

export interface RepoEditorTarget {
  branch: string;
  // path is the path in the branch, without the branch.
  path: string;
  lastCommit: string;
  kind: "file" | "dir" | "symlink" | "none";
  // content is only sent for a text file.
  content?: string;
  markdownExtensions: string[];
  // previewMarkdown tells whether Markdown files can be previewed.
  previewMarkdown: boolean;
  lineWrapExtensions: string[];
  // pullRequestsAllowed tells whether committing to a new branch starts a
  // pull request.
  pullRequestsAllowed: boolean;
  upload?: IssueAttachmentSettings;
  // directCommitRefused is why the viewer can only commit to a new branch,
  // with <code>{branch}</code> where the branch name goes.
  directCommitRefused?: string;
}

// fetchRepoEditorTarget describes the branch and the path in the editor
// pages' path, which is the branch name followed by the path.
export function fetchRepoEditorTarget(owner: string, name: string, splat: string) {
  return fetchJSON<RepoEditorTarget>(repoURL(owner, name, `/editor/${refPath(splat)}`));
}

export interface RepoEditorIndent {
  // style is left out when .editorconfig does not say.
  style?: "tab" | "space";
  size?: number;
  tabWidth?: number;
}

// fetchRepoEditorIndent returns how the .editorconfig of the branch indents
// the file at the path.
export function fetchRepoEditorIndent(owner: string, name: string, branch: string, path: string) {
  const query = new URLSearchParams({ branch, path }).toString();
  return fetchJSON<RepoEditorIndent>(repoURL(owner, name, `/editor-indent?${query}`));
}

export interface RepoEditorCommit {
  branch: string;
  // newBranch is the branch to commit to instead, created from branch.
  newBranch: string;
  summary: string;
  message: string;
}

// Each change answers with where to go next: the changed path, or the form to
// open a pull request from the new branch.
type RepoEditorResult = { location: string };

export function commitRepoFile(
  owner: string,
  name: string,
  form: RepoEditorCommit & { oldPath: string; path: string; content: string; lastCommit: string },
) {
  return submit<RepoEditorResult>("POST", repoURL(owner, name, "/editor/files"), form);
}

export function deleteRepoFile(owner: string, name: string, form: RepoEditorCommit & { path: string }) {
  return submit<RepoEditorResult>("POST", repoURL(owner, name, "/editor/delete"), form);
}

export function uploadRepoFiles(
  owner: string,
  name: string,
  form: RepoEditorCommit & { path: string; files: string[] },
) {
  return submit<RepoEditorResult>("POST", repoURL(owner, name, "/editor/upload"), form);
}

export function deleteRepoEditorUpload(owner: string, name: string, uuid: string) {
  return send("DELETE", repoURL(owner, name, `/editor/uploads/${encodeURIComponent(uuid)}`));
}

export interface RepoCommitAuthor {
  name: string;
  email: string;
  when: string;
  avatarURL: string;
  // profileURL is set when the email belongs to a user.
  profileURL?: string;
}

export interface RepoCommitListItem {
  sha: string;
  // subjectHTML is the first line of the message, with links to the users
  // and issues it mentions.
  subjectHTML: string;
  isMerge: boolean;
  author: RepoCommitAuthor;
  // coAuthors are the people the "Co-authored-by" trailers name.
  coAuthors?: RepoCommitAuthor[];
}

export interface RepoCommitsData {
  ref: string;
  refType: "branch" | "tag" | "commit";
  // path is the file or directory whose history is listed, empty for the
  // whole repository.
  path: string;
  // search is set for a search of the commit messages, which lists every
  // match instead of a page.
  search: boolean;
  keyword: string;
  commits: RepoCommitListItem[];
  page: number;
  hasNext: boolean;
  branches: string[];
  tags: string[];
}

export interface RepoCloneLinks {
  https?: string;
  ssh?: string;
}

export interface RepoEntryCommit {
  sha: string;
  subjectHTML: string;
  when: string;
}

export interface RepoTreeEntry {
  name: string;
  type: "dir" | "file" | "symlink" | "submodule";
  // submoduleURL is where the repository of a submodule is, when it can be
  // inferred.
  submoduleURL?: string;
  submoduleCommit?: string;
  commit?: RepoEntryCommit;
}

// RepoNotebook is a Jupyter notebook with its Markdown rendered and its HTML
// sanitized by the server.
export interface RepoNotebook {
  // fileExtension is of the source files of the notebook's language, e.g.,
  // ".py", or empty when not known.
  fileExtension: string;
  cells: RepoNotebookCell[];
}

export interface RepoNotebookCell {
  type: "markdown" | "code" | "raw";
  // html is set for a Markdown cell, and source otherwise.
  html?: string;
  source?: string;
  executionCount?: number;
  outputs?: RepoNotebookOutput[];
}

export interface RepoNotebookOutput {
  // The text of "text" and "error" may contain ANSI escape sequences.
  type: "text" | "error" | "image" | "html";
  // executionCount is set for the result of a cell.
  executionCount?: number;
  text?: string;
  // src is the data URI of an image.
  src?: string;
  html?: string;
}

export interface RepoFile {
  name: string;
  size: number;
  // too_large is a text file too large to show.
  kind: "text" | "markup" | "notebook" | "image" | "video" | "pdf" | "binary" | "too_large";
  // content is set for a text file, including a notebook or a file in a markup
  // language for their code view, html for a file in a markup language, and
  // notebook for a Jupyter notebook.
  content?: string;
  html?: string;
  notebook?: RepoNotebook;
  // isSymlink is set for a symbolic link, whose content is its target.
  isSymlink: boolean;
  // tabWidth is the width of a tab that .editorconfig sets for the file.
  tabWidth?: number;
}

export type RepoOverviewKind = "readme" | "code_of_conduct" | "contributing" | "license" | "security";

// RepoOverviewFile is a file shown in a tab below the file list, e.g., the
// README or the license. It has html in a markup language, and text otherwise.
export interface RepoOverviewFile {
  kind: RepoOverviewKind;
  // path is relative to the directory being viewed, e.g., ".github/CONTRIBUTING.md".
  path: string;
  html?: string;
  text?: string;
}

export interface RepoSrcData {
  // empty is set for a repository without commits, which only has the fields
  // of its quick guide.
  empty: boolean;
  defaultBranch: string;
  isMirror: boolean;
  viewerCanWrite: boolean;
  cloneLinks?: RepoCloneLinks;
  ref: string;
  refType: "branch" | "tag" | "commit";
  commitID: string;
  path: string;
  kind: "dir" | "file";
  latestCommit?: RepoCommitListItem;
  entries?: RepoTreeEntry[];
  // overview lists the files shown in tabs below the file list, in the order
  // of the tabs.
  overview?: RepoOverviewFile[];
  file?: RepoFile;
  // summary is set for the root directory.
  summary?: {
    description: string;
    website: string;
    commitCount: number;
    cloneLinks: RepoCloneLinks;
    // latestRelease is the newest published release that is not a
    // pre-release, or the newest tag without one. It is left out without tags.
    latestRelease?: { tagName: string; title?: string; createdAt?: string };
  };
  // editor tells whether files can be changed in the browser, or why not. It
  // is left out where files are never changed in the browser.
  editor?: "ok" | "not_branch" | "no_write";
  canUploadFile: boolean;
  // pullRequest is where the changes of the branch can be proposed.
  pullRequest?: { repoLink: string; baseBranch: string; head: string };
  branches: string[];
  tags: string[];
}

export interface RepoPathsData {
  ref: string;
  commitID: string;
  paths: string[];
  // truncated is set when the tree has more files than paths lists.
  truncated: boolean;
}

// repoPathsQuery lists the paths of every file of the commit, for jumping to
// a file.
export function repoPathsQuery(owner: string, name: string, commitID: string) {
  return queryOptions({
    queryKey: ["repo", owner, name, "paths", commitID] as const,
    queryFn: () => fetchJSON<RepoPathsData>(repoURL(owner, name, `/paths/${commitID}`)),
    staleTime: Infinity,
  });
}

// fetchRepoSrc describes the directory or the file at the path after the ref
// at the start of the splat, or the default branch for an empty splat.
export function fetchRepoSrc(owner: string, name: string, splat: string) {
  return fetchJSON<RepoSrcData>(repoURL(owner, name, `/src/${refPath(splat)}`));
}

export interface RepoSearchLine {
  line: number;
  text: string;
  // isMatch is set for lines that contain the keyword, and not for the lines
  // around them.
  isMatch: boolean;
}

export interface RepoSearchFile {
  // repo is the "owner/name" of the repository, and ref the ref searched, in
  // a search of all repositories.
  repo?: string;
  ref?: string;
  path: string;
  lines: RepoSearchLine[];
}

export interface RepoSearchData {
  ref: string;
  refType: "branch" | "tag" | "commit";
  // query is what was searched for, and keyword its part without qualifiers.
  query: string;
  keyword: string;
  // exhaustive is set for a search that lists every matching line of a file,
  // with higher limits.
  exhaustive: boolean;
  // matchCase is set when letter case must match, rather than being ignored.
  matchCase: boolean;
  // files are ordered by path.
  files: RepoSearchFile[];
  // truncated is set when more lines matched than files lists.
  truncated: boolean;
  // timedOut is set when the search took too long and listed nothing.
  timedOut: boolean;
  branches: string[];
  tags: string[];
}

export interface CodeSearchData {
  query: string;
  keyword: string;
  // matchCase is set when letter case must match, rather than being ignored.
  matchCase: boolean;
  // files are grouped by repository, the most recently updated first.
  files: RepoSearchFile[];
  reposSearched: number;
  truncated: boolean;
  // timedOut is set when the search took too long, and some repositories
  // were not searched.
  timedOut: boolean;
}

// fetchRepoSearch searches the files of the ref in the splat, or of the
// default branch for an empty splat, for lines that contain the keyword.
export function fetchRepoSearch(
  owner: string,
  name: string,
  splat: string,
  { q, exhaustive, matchCase }: { q?: string; exhaustive?: boolean; matchCase?: boolean },
) {
  const params = new URLSearchParams();
  if (q) params.set("q", q);
  if (exhaustive) params.set("exhaustive", "true");
  if (matchCase) params.set("matchCase", "true");
  const qs = params.size ? `?${params.toString()}` : "";
  return fetchJSON<RepoSearchData>(repoURL(owner, name, `/search/${refPath(splat)}${qs}`));
}

// fetchRepoSearchPreview lists the first few lines of the default branch
// that contain the keyword, e.g., as the viewer types.
export function fetchRepoSearchPreview(owner: string, name: string, q: string, limit: number, signal?: AbortSignal) {
  const query = new URLSearchParams({ q, limit: String(limit) });
  return fetchJSON<RepoSearchData>(repoURL(owner, name, `/search?${query.toString()}`), signal);
}

// fetchRepoPaths lists the paths of every file of the default branch.
export function fetchRepoPaths(owner: string, name: string, signal?: AbortSignal) {
  return fetchJSON<RepoPathsData>(repoURL(owner, name, "/paths"), signal);
}

export interface IssueSearchItem {
  // repo is the "owner/name" of the repository.
  repo: string;
  index: number;
  title: string;
  isPull: boolean;
  isClosed: boolean;
}

// fetchIssueSearch finds the issues and pull requests whose titles contain
// the keyword, or whose number it is, in every repository the viewer can see
// them in, or only in the "owner/name" of repo.
export function fetchIssueSearch(q: string, repo?: string, signal?: AbortSignal) {
  const query = new URLSearchParams({ q });
  if (repo) query.set("repo", repo);
  return fetchJSON<{ items: IssueSearchItem[] }>(subUrl(`/api/web/search/issues?${query.toString()}`), signal);
}

export interface RepoCompareData {
  before: string;
  after: string;
  // base is the commit the changes are compared against: before, or where
  // after forked from the default branch when before is the empty ID.
  base: string;
  // baseIsEmptyTree is true when base is the empty tree, so the comparison
  // shows everything after has.
  baseIsEmptyTree: boolean;
  // commits are the newest commits after base up to after, at most 50 of
  // them.
  commits: RepoCommitListItem[];
  // totalCommits is the number of commits after base up to after, which is
  // more than commits.length when the list is cut off.
  totalCommits: number;
  // markdownExtensions are the extensions of files that show a rich diff.
  markdownExtensions: string[];
}

// repoMarkdownFileQuery renders the Markdown file at the path in the commit
// the way the file view does. Without a commit, the file is empty, e.g., the
// old side of an added file.
export function repoMarkdownFileQuery(owner: string, name: string, sha: string | undefined, path: string) {
  return queryOptions({
    queryKey: ["repo", owner, name, "markdown", sha, path] as const,
    queryFn: async () =>
      sha ? (await fetchJSON<{ html: string }>(repoURL(owner, name, `/markdown/${sha}/${refPath(path)}`))).html : "",
    staleTime: Infinity,
  });
}

// fetchRepoCommits lists the commits of the ref at the start of the path,
// which is followed by the path whose history to list, or by "search" to
// search the messages for the keyword.
export function fetchRepoCommits(owner: string, name: string, splat: string, search: { page?: number; q?: string }) {
  const query = new URLSearchParams();
  if (search.page) query.set("page", String(search.page));
  if (search.q) query.set("q", search.q);
  const qs = query.size ? `?${query.toString()}` : "";
  return fetchJSON<RepoCommitsData>(repoURL(owner, name, `/commits/${refPath(splat)}${qs}`));
}
