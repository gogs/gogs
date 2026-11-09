import { fetchJSON, send, submit, uploadAvatar } from "@/lib/queries/request";
import { subUrl } from "@/lib/url";

function settingsURL(path = "") {
  return subUrl(`/api/web/user/settings${path}`);
}

export interface UserSettingsData {
  name: string;
  fullName: string;
  website: string;
  location: string;
  // False for users who sign in through an external authentication source,
  // who cannot change their username or password.
  isLocal: boolean;
  avatarURL: string;
  hasCustomAvatar: boolean;
  // Only sent when the site uses Gravatar.
  avatarEmail?: string;
}

export interface UserProfileForm {
  name: string;
  fullName: string;
  website: string;
  location: string;
}

export interface UserPasswordForm {
  oldPassword: string;
  password: string;
  retype: string;
}

export interface SettingsRepo {
  id: number;
  owner: string;
  name: string;
  isPrivate: boolean;
  isFork: boolean;
  isMirror: boolean;
  size: string;
  // The viewer owns the repository, so they cannot leave it.
  isOwner: boolean;
}

export interface SettingsOrg {
  name: string;
  avatarURL: string;
}

export function fetchUserSettings() {
  return fetchJSON<UserSettingsData>(settingsURL());
}

// updateUserProfile changes the profile of the viewer and returns their
// username, which changes when they rename themselves.
export async function updateUserProfile(form: UserProfileForm) {
  return (await submit<{ name: string }>("PATCH", settingsURL(), form)).name;
}

export function uploadUserAvatar(file: File) {
  return uploadAvatar(settingsURL("/avatar"), file);
}

export function deleteUserAvatar() {
  return send("DELETE", settingsURL("/avatar"));
}

// setGravatarEmail shows the Gravatar of the email as the avatar of the
// viewer, in place of any uploaded avatar.
export function setGravatarEmail(email: string) {
  return submit<void>("PUT", settingsURL("/gravatar"), { email });
}

export function changePassword(form: UserPasswordForm) {
  return submit<void>("PUT", settingsURL("/password"), form);
}

export async function fetchSettingsRepos() {
  return (await fetchJSON<{ items: SettingsRepo[] }>(settingsURL("/repositories"))).items;
}

// leaveRepo makes the viewer leave a repository they collaborate on.
export function leaveRepo(id: number) {
  return send("DELETE", settingsURL(`/repositories/${id}`));
}

export async function fetchSettingsOrgs() {
  return (await fetchJSON<{ items: SettingsOrg[] }>(settingsURL("/organizations"))).items;
}

// deleteAccount deletes the account of the viewer once they confirm their
// password.
export function deleteAccount(password: string) {
  return submit<void>("DELETE", subUrl("/api/web/user"), { password });
}
