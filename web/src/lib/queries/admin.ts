import { fetchJSON, send, submit } from "@/lib/queries/request";
import { subUrl } from "@/lib/url";

function adminURL(path: string, search: { q?: string; page?: number } = {}) {
  const query = new URLSearchParams();
  if (search.q) query.set("q", search.q);
  if (search.page) query.set("page", String(search.page));
  const qs = query.toString();
  return subUrl(`/api/web/admin${path}`) + (qs ? `?${qs}` : "");
}

// AdminList is one page of a list in the admin panel.
export interface AdminList<T> {
  items: T[];
  page: number;
  totalPages: number;
  // The number of all matches, on every page.
  total: number;
}

export interface AdminUser {
  id: number;
  name: string;
  avatarURL: string;
  email: string;
  isActive: boolean;
  isAdmin: boolean;
  repoCount: number;
  createdAt: string;
}

export function fetchAdminUsers(search: { q?: string; page?: number }) {
  return fetchJSON<AdminList<AdminUser>>(adminURL("/users", search));
}

export interface AdminOrg {
  id: number;
  name: string;
  avatarURL: string;
  teamCount: number;
  memberCount: number;
  repoCount: number;
  createdAt: string;
}

export function fetchAdminOrgs(search: { q?: string; page?: number }) {
  return fetchJSON<AdminList<AdminOrg>>(adminURL("/orgs", search));
}

export interface AdminRepo {
  id: number;
  owner: string;
  name: string;
  isPrivate: boolean;
  watchCount: number;
  starCount: number;
  issueCount: number;
  // Formatted, e.g., "1.2 MB".
  size: string;
  createdAt: string;
}

export function fetchAdminRepos(search: { q?: string; page?: number }) {
  return fetchJSON<AdminList<AdminRepo>>(adminURL("/repos", search));
}

export function deleteAdminRepo(id: number) {
  return send("DELETE", adminURL(`/repos/${id}`));
}

export interface AdminNotice {
  id: number;
  // Translated, e.g., "Repository".
  type: string;
  description: string;
  createdAt: string;
}

export function fetchAdminNotices(search: { page?: number }) {
  return fetchJSON<AdminList<AdminNotice>>(adminURL("/notices", search));
}

export function deleteAdminNotices(ids: number[]) {
  return submit<void>("DELETE", adminURL("/notices"), { ids });
}

export function deleteAllAdminNotices() {
  return send("DELETE", adminURL("/notices/all"));
}

export interface AdminUserOptions {
  // Authentication sources besides local accounts.
  loginSources: { id: number; name: string }[];
  // True when the site can email a new user about their account.
  canSendEmail: boolean;
  // True when users may be allowed to import repositories from the local disk.
  localPathMigrationEnabled: boolean;
}

export function fetchAdminUserOptions() {
  return fetchJSON<AdminUserOptions>(adminURL("/user-options"));
}

export interface AdminUserCreateForm {
  // 0 for a local account.
  loginSource: number;
  loginName: string;
  name: string;
  email: string;
  password: string;
  sendNotify: boolean;
}

export function createAdminUser(form: AdminUserCreateForm) {
  return submit<{ id: number; name: string }>("POST", adminURL("/users"), form);
}

export interface AdminUserForm {
  // 0 for a local account.
  loginSource: number;
  loginName: string;
  fullName: string;
  email: string;
  // Left unchanged when empty.
  password: string;
  website: string;
  location: string;
  maxRepoCreation: number;
  isActive: boolean;
  isAdmin: boolean;
  allowGitHook: boolean;
  allowImportLocal: boolean;
  prohibitLogin: boolean;
}

export interface AdminUserDetail extends Omit<AdminUserForm, "password"> {
  id: number;
  name: string;
  avatarURL: string;
}

export function fetchAdminUser(id: number) {
  return fetchJSON<AdminUserDetail>(adminURL(`/users/${id}`));
}

export function updateAdminUser(id: number, form: AdminUserForm) {
  return submit<void>("PATCH", adminURL(`/users/${id}`), form);
}

export function deleteAdminUser(id: number) {
  return send("DELETE", adminURL(`/users/${id}`));
}

// AdminReportRow is one setting or measurement of a report, with its label
// translated by the server.
export interface AdminReportRow {
  label: string;
  // "block" is multi-line code, and "unset" a setting with no value.
  kind: "text" | "code" | "block" | "bool" | "unset";
  value?: string;
  on?: boolean;
}

export interface AdminReportSection {
  id: string;
  title: string;
  // Groups are shown with a divider between them.
  groups: AdminReportRow[][];
}

export interface AdminDashboard {
  appVersion: string;
  gitVersion: string;
  goVersion: string;
  buildTime: string;
  buildCommit: string;
  // A translated sentence that marks each count with <b>.
  statistics: string;
  systemStatus: AdminReportSection;
}

export function fetchAdminDashboard() {
  return fetchJSON<AdminDashboard>(adminURL("/dashboard"));
}

export const ADMIN_OPERATIONS = [
  "delete-inactive-users",
  "delete-repo-archives",
  "delete-missing-repos",
  "git-gc-repos",
  "resync-ssh-keys",
  "resync-repo-hooks",
  "reinit-missing-repos",
] as const;

export type AdminOperation = (typeof ADMIN_OPERATIONS)[number];

// runAdminOperation runs a maintenance task and returns the message telling
// it succeeded.
export async function runAdminOperation(op: AdminOperation) {
  const { message } = await submit<{ message: string }>("POST", adminURL(`/operations/${op}`), {});
  return message;
}

export interface AdminConfig {
  sections: AdminReportSection[];
  // True when the site can send a test email.
  emailEnabled: boolean;
}

export function fetchAdminConfig() {
  return fetchJSON<AdminConfig>(adminURL("/config"));
}

export async function sendAdminTestMail(email: string) {
  const { message } = await submit<{ message: string }>("POST", adminURL("/config/test-mail"), { email });
  return message;
}

export interface AdminCronTask {
  description: string;
  schedule: string;
  // Null when the task never runs.
  next: string | null;
  // Null when the task has not run yet.
  prev: string | null;
  execTimes: number;
}

export interface AdminProcess {
  pid: number;
  description: string;
  start: string;
}

export interface AdminMonitor {
  cronTasks: AdminCronTask[];
  processes: AdminProcess[];
}

export function fetchAdminMonitor() {
  return fetchJSON<AdminMonitor>(adminURL("/monitor"));
}
