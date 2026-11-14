import { loaderResponseError } from "@/lib/loader-error";
import { subUrl } from "@/lib/url";

export interface OrgMember {
  name: string;
  fullName?: string;
  avatarURL: string;
  isPublic: boolean;
  isOwner: boolean;
}

export interface OrgTeamMember {
  name: string;
  fullName?: string;
  avatarURL: string;
}

export interface OrgTeamSummary {
  name: string;
  lowerName: string;
  memberCount: number;
  repoCount: number;
  members: OrgTeamMember[];
  viewerIsMember: boolean;
}

export interface OrgTeamDetail {
  name: string;
  lowerName: string;
  description?: string;
  // Sanitized on the server.
  permissionHTML: string;
  memberCount: number;
  repoCount: number;
  permission: TeamPermission | "owner";
  isOwnerTeam: boolean;
  viewerIsMember: boolean;
}

export type TeamPermission = "read" | "write" | "admin";

export interface OrgTeamRepo {
  name: string;
  isPrivate: boolean;
  isFork: boolean;
  isMirror: boolean;
}

function orgURL(org: string, path: string) {
  return subUrl(`/api/web/orgs/${encodeURIComponent(org)}${path}`);
}

function teamURL(org: string, team: string, path = "") {
  return orgURL(org, `/teams/${encodeURIComponent(team)}${path}`);
}

async function fetchJSON<T>(url: string): Promise<T> {
  const res = await fetch(url, { credentials: "same-origin" });
  if (!res.ok) throw await loaderResponseError(res);
  return (await res.json()) as T;
}

// send makes a change that answers with no content.
async function send(method: "PUT" | "DELETE", url: string) {
  const res = await fetch(url, { method, credentials: "same-origin" });
  if (!res.ok) throw await loaderResponseError(res);
}

export async function fetchOrgMembers(org: string) {
  return (await fetchJSON<{ items: OrgMember[] }>(orgURL(org, "/members"))).items;
}

export async function fetchOrgTeams(org: string) {
  return (await fetchJSON<{ items: OrgTeamSummary[] }>(orgURL(org, "/teams"))).items;
}

export function fetchOrgTeam(org: string, team: string) {
  return fetchJSON<OrgTeamDetail>(teamURL(org, team));
}

export async function fetchOrgTeamMembers(org: string, team: string) {
  return (await fetchJSON<{ items: OrgTeamMember[] }>(teamURL(org, team, "/members"))).items;
}

export async function fetchOrgTeamRepos(org: string, team: string) {
  return (await fetchJSON<{ items: OrgTeamRepo[] }>(teamURL(org, team, "/repos"))).items;
}

export function setOrgMemberPublic(org: string, username: string, isPublic: boolean) {
  return send(isPublic ? "PUT" : "DELETE", orgURL(org, `/members/${encodeURIComponent(username)}/public`));
}

// removeOrgMember removes a user from the organization, or makes the viewer
// leave it.
export function removeOrgMember(org: string, username: string) {
  return send("DELETE", orgURL(org, `/members/${encodeURIComponent(username)}`));
}

// inviteOrgMember adds a user to the organization.
export function inviteOrgMember(org: string, username: string) {
  return send("PUT", orgURL(org, `/members/${encodeURIComponent(username)}`));
}

export interface OrgTeamForm {
  teamName: string;
  description: string;
  permission: TeamPermission;
}

// FormError is a failed form submission, with the message for the whole form
// or the messages for its fields.
export class FormError extends Error {
  fields: Record<string, string | null>;

  constructor(message: string, fields: Record<string, string | null>) {
    super(message);
    this.name = "FormError";
    this.fields = fields;
  }
}

async function submit<T>(method: "POST" | "PATCH", url: string, body: unknown): Promise<T> {
  const res = await fetch(url, {
    method,
    credentials: "same-origin",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  if (!res.ok) {
    const data = (await res.json().catch(() => ({}))) as { error?: string; fields?: Record<string, string | null> };
    throw new FormError(data.error ?? `HTTP ${res.status}`, data.fields ?? {});
  }
  return (await res.json()) as T;
}

// createOrgTeam creates a team and returns the name of its page.
export async function createOrgTeam(org: string, form: OrgTeamForm) {
  return (await submit<{ lowerName: string }>("POST", orgURL(org, "/teams"), form)).lowerName;
}

// updateOrgTeam changes a team and returns the name of its page, which
// changes with the team name.
export async function updateOrgTeam(org: string, team: string, form: OrgTeamForm) {
  return (await submit<{ lowerName: string }>("PATCH", teamURL(org, team), form)).lowerName;
}

export function deleteOrgTeam(org: string, team: string) {
  return send("DELETE", teamURL(org, team));
}

export function addOrgTeamMember(org: string, team: string, username: string) {
  return send("PUT", teamURL(org, team, `/members/${encodeURIComponent(username)}`));
}

// removeOrgTeamMember removes a user from the team, or makes the viewer leave
// it.
export function removeOrgTeamMember(org: string, team: string, username: string) {
  return send("DELETE", teamURL(org, team, `/members/${encodeURIComponent(username)}`));
}

export function addOrgTeamRepo(org: string, team: string, repo: string) {
  return send("PUT", teamURL(org, team, `/repos/${encodeURIComponent(repo)}`));
}

export function removeOrgTeamRepo(org: string, team: string, repo: string) {
  return send("DELETE", teamURL(org, team, `/repos/${encodeURIComponent(repo)}`));
}
