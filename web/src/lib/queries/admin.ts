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
