import { queryOptions } from "@tanstack/react-query";

import { LoaderResponseError } from "@/lib/loader-error";
import { fetchJSON, send, submit } from "@/lib/queries/request";
import { subUrl } from "@/lib/url";

function adminURL(path: string, search: { q?: string; page?: number } = {}) {
  const query = new URLSearchParams();
  if (search.q) query.set("q", search.q);
  if (search.page) query.set("page", String(search.page));
  const qs = query.toString();
  return subUrl(`/api/web/admin${path}`) + (qs ? `?${qs}` : "");
}

// Cards of the admin console fetch what they show with these queries, so a
// slow or failed request holds back only the cards that read it. A failed
// query is tried once more, unless the server turned the request away, which
// trying again does not change.
function retryOnce(failures: number, err: Error) {
  return failures < 1 && !(err instanceof LoaderResponseError && err.status < 500);
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

export function adminNoticesQuery(search: { page?: number }) {
  return queryOptions({
    queryKey: ["admin", "notices", search.page ?? 1] as const,
    queryFn: () => fetchAdminNotices(search),
    retry: retryOnce,
  });
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
  // Text that follows a "code" value, e.g., seconds.
  unit?: string;
  // A note shown after the value, e.g., which database types use it.
  hint?: string;
}

export interface AdminReportSection {
  id: string;
  title: string;
  // Groups are shown with a gap between them.
  groups: AdminReportRow[][];
}

export interface AdminDashboard {
  appVersion: string;
  gitVersion: string;
  goVersion: string;
  buildTime: string;
  buildCommit: string;
  // How many records of each kind the database holds, with translated labels.
  statistics: { label: string; count: number }[];
  systemStatus: AdminReportSection;
}

export function adminDashboardQuery() {
  return queryOptions({
    queryKey: ["admin", "dashboard"] as const,
    queryFn: ({ signal }) => fetchJSON<AdminDashboard>(adminURL("/dashboard"), signal),
    retry: retryOnce,
  });
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

export interface AdminMonitor {
  cronTasks: AdminCronTask[];
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

// AdminSpanKind is what a span of a trace measures: the request itself, a Git
// operation, a database query, or other Go code.
export type AdminSpanKind = "request" | "git" | "db" | "other";

// AdminTraceRoute is what the traces of a route are grouped under. A "route" is
// a matched route pattern, e.g., "GET /api/web/{owner}/{repo}/header". "other"
// groups the requests whose route is not known, e.g., those that matched no
// route pattern, under their method, which is the name. "background" is work
// that no request started, e.g., a mirror sync, named after what it ran.
export interface AdminTraceRoute {
  name: string;
  kind: "route" | "other" | "background";
  method: string;
}

export interface AdminTraceRouteStat extends AdminTraceRoute {
  requests: number;
  errors: number;
  p50Ms: number;
  p95Ms: number;
  p99Ms: number;
  totalMs: number;
}

export interface AdminTraceOperationStat {
  name: string;
  kind: AdminSpanKind;
  calls: number;
  p50Ms: number;
  p95Ms: number;
  // The time spent in the operation itself, without the operations it ran.
  selfMs: number;
}

export type AdminTraceRange = "15m" | "1h";

// AdminTraceOverview aggregates every request of the range, not only the kept
// traces.
export interface AdminTraceOverview {
  // False when tracing is turned off in the configuration, and then everything
  // else is empty.
  enabled: boolean;
  // The time on the server.
  now: string;
  maxAgeMinutes: number;
  range: AdminTraceRange;
  requests: number;
  errors: number;
  p50Ms: number;
  p95Ms: number;
  p99Ms: number;
  // The time all requests of the range took together.
  totalMs: number;
  // The time spent in every span of the range, each without the spans it
  // started, which is what the shares of operations are out of.
  spanSelfMs: number;
  // Five-minute buckets, oldest first.
  buckets: { start: string; requests: number; p95Ms: number }[];
  routes: AdminTraceRouteStat[];
  operations: AdminTraceOperationStat[];
}

export function fetchAdminTraceOverview(range: AdminTraceRange) {
  return fetchJSON<AdminTraceOverview>(subUrl(`/api/web/admin/traces/overview?range=${range}`));
}

export function adminTraceOverviewQuery(range: AdminTraceRange) {
  return queryOptions({
    queryKey: ["admin", "traces", "overview", range] as const,
    queryFn: () => fetchAdminTraceOverview(range),
    retry: retryOnce,
  });
}

// AdminTraceKept tells why a trace was kept: it failed, it was slow, or it was
// sampled from the rest.
export type AdminTraceKept = "failed" | "slow" | "sampled";

export interface AdminTraceSummary {
  id: string;
  route: AdminTraceRoute;
  method: string;
  path: string;
  // Zero for background work.
  status: number;
  start: string;
  durationMs: number;
  spanCount: number;
  error: boolean;
  kept: AdminTraceKept;
}

export type AdminTraceStatusFilter = "all" | "5xx" | "4xx";
export type AdminTraceSort = "recent" | "slowest";

export interface AdminTraceSearchParams {
  // A trace ID, or its first 8 or more characters. Other filters are ignored
  // when it is set.
  q?: string;
  route?: string;
  minMs?: number;
  status?: AdminTraceStatusFilter;
  sort?: AdminTraceSort;
  // A window of start times, as Unix milliseconds, e.g., a moment picked on the
  // server health charts.
  from?: number;
  to?: number;
}

export interface AdminTraceSearch {
  enabled: boolean;
  // The time on the server.
  now: string;
  // The first 50 matches.
  traces: AdminTraceSummary[];
  // The number of all matches.
  total: number;
  // Up to 2,000 matches for the scatter plot.
  points: { id: string; start: string; durationMs: number; status: number; error: boolean }[];
  // The routes of the kept traces, for the route filter.
  routes: AdminTraceRoute[];
  // When the oldest kept trace started, or null when none is kept.
  oldest: string | null;
  maxAgeMinutes: number;
  // The duration from which a request is always kept, or 0 when only failed
  // requests and the slowest of each route are.
  slowThresholdMs: number;
}

export function fetchAdminTraceSearch(params: AdminTraceSearchParams) {
  const query = new URLSearchParams();
  if (params.q) query.set("q", params.q);
  if (params.route) query.set("route", params.route);
  if (params.minMs) query.set("minMs", String(params.minMs));
  if (params.status && params.status !== "all") query.set("status", params.status);
  if (params.sort && params.sort !== "recent") query.set("sort", params.sort);
  if (params.from) query.set("from", String(params.from));
  if (params.to) query.set("to", String(params.to));
  const qs = query.toString();
  return fetchJSON<AdminTraceSearch>(subUrl("/api/web/admin/traces") + (qs ? `?${qs}` : ""));
}

export function adminTraceSearchQuery(params: AdminTraceSearchParams) {
  return queryOptions({
    queryKey: ["admin", "traces", "search", params] as const,
    queryFn: () => fetchAdminTraceSearch(params),
    retry: retryOnce,
  });
}

export interface AdminSpan {
  id: string;
  // Empty for the root span.
  parentId: string;
  name: string;
  kind: AdminSpanKind;
  // From the start of the trace.
  startMs: number;
  durationMs: number;
  // Empty when the span did not fail.
  error: string;
  attributes: { key: string; value: string }[];
}

export interface AdminTrace extends AdminTraceSummary {
  // Ordered by start, the root first.
  spans: AdminSpan[];
  // Spans left out past the limit of spans per trace.
  droppedSpans: number;
  // How this trace compares to the other requests of its route in the last
  // hour, or null when there are none.
  routeStats: { requests: number; p50Ms: number; p95Ms: number; fasterPercent: number } | null;
  // The last hour of every span name in this trace.
  operations: Record<string, { calls: number; p50Ms: number; p95Ms: number }>;
}

export function fetchAdminTrace(id: string) {
  return fetchJSON<AdminTrace>(subUrl(`/api/web/admin/traces/${encodeURIComponent(id)}`));
}

export type AdminHealthRange = "1h" | "24h";

// AdminHealthKey names a measurement of the server. Units: requests per second,
// milliseconds for p95 and GC pauses, bytes for the heap, and counts for the
// rest.
export type AdminHealthKey =
  | "requests"
  | "p95"
  | "heap"
  | "goroutines"
  | "gcPause"
  | "dbConnections"
  | "gitProcesses"
  | "openFiles"
  | "mirrorQueue";

export interface AdminHealthSeries {
  key: AdminHealthKey;
  // [Unix milliseconds, value] pairs, oldest first. Times without a sample,
  // e.g., before the server started, have no point.
  points: [number, number][];
  // The most the value can be, e.g., the size of the connection pool, or 0
  // when there is no limit.
  limit: number;
}

export interface AdminHealth {
  range: AdminHealthRange;
  // How far apart the points are.
  stepSeconds: number;
  start: string;
  end: string;
  // Measurements the server cannot take, e.g., open files outside Linux, or
  // requests while tracing is off, are left out.
  series: AdminHealthSeries[];
}

export function fetchAdminHealth(range: AdminHealthRange) {
  return fetchJSON<AdminHealth>(subUrl(`/api/web/admin/health?range=${range}`));
}

export function adminHealthQuery(range: AdminHealthRange) {
  return queryOptions({
    queryKey: ["admin", "health", range] as const,
    queryFn: () => fetchAdminHealth(range),
    retry: retryOnce,
  });
}
