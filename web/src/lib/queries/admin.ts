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

// AUTH_SOURCE_TYPES lists the types of authentication sources, with the
// names the server gives them.
export const AUTH_SOURCE_TYPES = [
  { type: 2, name: "LDAP (via BindDN)" },
  { type: 5, name: "LDAP (simple auth)" },
  { type: 3, name: "SMTP" },
  { type: 4, name: "PAM" },
  { type: 6, name: "GitHub" },
] as const;

export const LDAP_SECURITY_PROTOCOLS = [
  { value: 0, name: "Unencrypted" },
  { value: 1, name: "LDAPS" },
  { value: 2, name: "StartTLS" },
] as const;

export const SMTP_AUTH_TYPES = ["PLAIN", "LOGIN"] as const;

export interface AdminAuthSource {
  id: number;
  name: string;
  type: number;
  typeName: string;
  isActive: boolean;
  isDefault: boolean;
  // True for sources loaded from a file, which can't be deleted here.
  fromFile: boolean;
  createdAt: string | null;
  updatedAt: string | null;
}

export async function fetchAdminAuthSources() {
  const { items } = await fetchJSON<{ items: AdminAuthSource[] }>(adminURL("/auths"));
  return items;
}

// AdminAuthSourceConfig holds the settings of every type of source. Only the
// ones of the source's type are used.
export interface AdminAuthSourceConfig {
  securityProtocol: number;
  host: string;
  port: number;
  bindDN: string;
  userBase: string;
  userDN: string;
  filter: string;
  adminFilter: string;
  attributeUsername: string;
  attributeName: string;
  attributeSurname: string;
  attributeMail: string;
  attributesInBind: boolean;
  groupEnabled: boolean;
  groupDN: string;
  groupFilter: string;
  groupMemberUID: string;
  userUID: string;
  smtpAuth: string;
  smtpHost: string;
  smtpPort: number;
  allowedDomains: string;
  tls: boolean;
  pamServiceName: string;
  githubAPIEndpoint: string;
  skipVerify: boolean;
}

export interface AdminAuthSourceDetail extends AdminAuthSource, AdminAuthSourceConfig {
  // The bind password itself is never sent back.
  hasBindPassword: boolean;
}

export function fetchAdminAuthSource(id: number) {
  return fetchJSON<AdminAuthSourceDetail>(adminURL(`/auths/${id}`));
}

export interface AdminAuthSourceForm extends AdminAuthSourceConfig {
  // Only read when creating a source.
  type: number;
  name: string;
  isActive: boolean;
  isDefault: boolean;
  // Left unchanged when empty on update.
  bindPassword: string;
}

export function createAdminAuthSource(form: AdminAuthSourceForm) {
  return submit<{ id: number; name: string }>("POST", adminURL("/auths"), form);
}

export function updateAdminAuthSource(id: number, form: AdminAuthSourceForm) {
  return submit<void>("PATCH", adminURL(`/auths/${id}`), form);
}

export function deleteAdminAuthSource(id: number) {
  return send("DELETE", adminURL(`/auths/${id}`));
}
