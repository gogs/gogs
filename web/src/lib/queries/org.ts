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
  isOwnerTeam: boolean;
  viewerIsMember: boolean;
}

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
