import { type CreationOptionsJSON, createPasskey } from "@/lib/passkey";
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

export interface UserEmail {
  email: string;
  isPrimary: boolean;
  isActivated: boolean;
}

export async function fetchUserEmails() {
  return (await fetchJSON<{ items: UserEmail[] }>(settingsURL("/emails"))).items;
}

export interface EmailAdded {
  // True when the site requires confirming the address through the link sent
  // to it.
  confirmationSent: boolean;
  codeLifetimeHours?: number;
}

export function addUserEmail(email: string) {
  return submit<EmailAdded>("POST", settingsURL("/emails"), { email });
}

export function setPrimaryEmail(email: string) {
  return send("PUT", settingsURL(`/emails/${encodeURIComponent(email)}/primary`));
}

export function deleteUserEmail(email: string) {
  return send("DELETE", settingsURL(`/emails/${encodeURIComponent(email)}`));
}

// Credential is an SSH key, an access token or a passkey.
export interface Credential {
  id: number;
  name: string;
  createdAt: string;
  // Only sent for credentials that were used.
  lastUsedAt?: string;
  isRecentlyUsed: boolean;
}

export interface SSHKey extends Credential {
  fingerprint: string;
}

export async function fetchSSHKeys() {
  return (await fetchJSON<{ items: SSHKey[] }>(settingsURL("/ssh-keys"))).items;
}

export function addSSHKey(form: { title: string; content: string }) {
  return submit<void>("POST", settingsURL("/ssh-keys"), form);
}

export function deleteSSHKey(id: number) {
  return send("DELETE", settingsURL(`/ssh-keys/${id}`));
}

export async function fetchTwoFactorEnabled() {
  return (await fetchJSON<{ enabled: boolean }>(settingsURL("/two-factor"))).enabled;
}

export interface LoginSession {
  id: number;
  userAgent: string;
  ip: string;
  createdAt: string;
  lastSeenAt: string;
  // The browser that asks.
  isCurrent: boolean;
}

export async function fetchLoginSessions() {
  return (await fetchJSON<{ items: LoginSession[] }>(settingsURL("/sessions"))).items;
}

export function deleteLoginSession(id: number) {
  return send("DELETE", settingsURL(`/sessions/${id}`));
}

// deleteOtherLoginSessions signs out every browser of the viewer but the one
// that asks.
export function deleteOtherLoginSessions() {
  return send("DELETE", settingsURL("/sessions"));
}

export async function fetchPasskeys() {
  return (await fetchJSON<{ items: Credential[] }>(settingsURL("/passkeys"))).items;
}

// addPasskey has the browser create a passkey and adds it to the viewer. The
// server checks the name before the browser asks for the passkey.
export async function addPasskey(name: string) {
  const options = await submit<CreationOptionsJSON>("POST", settingsURL("/passkeys/registration"), { name });
  const credential = await createPasskey(options);
  await submit<void>("POST", settingsURL("/passkeys"), { name, credential });
}

export function deletePasskey(id: number) {
  return send("DELETE", settingsURL(`/passkeys/${id}`));
}

export interface TwoFactorEnrollment {
  secret: string;
  // A data URL of the QR code image of the secret.
  qrCodeURL: string;
}

export function fetchTwoFactorEnrollment() {
  return fetchJSON<TwoFactorEnrollment>(settingsURL("/two-factor/enrollment"));
}

// enableTwoFactor confirms the secret being enrolled with a passcode from the
// authentication app.
export function enableTwoFactor(passcode: string) {
  return submit<void>("POST", settingsURL("/two-factor"), { passcode });
}

export function disableTwoFactor() {
  return send("DELETE", settingsURL("/two-factor"));
}

export interface RecoveryCode {
  code: string;
  isUsed: boolean;
}

export async function fetchRecoveryCodes() {
  return (await fetchJSON<{ items: RecoveryCode[] }>(settingsURL("/two-factor/recovery-codes"))).items;
}

// regenerateRecoveryCodes replaces the recovery codes with new ones and
// returns them.
export async function regenerateRecoveryCodes() {
  return (await submit<{ items: RecoveryCode[] }>("POST", settingsURL("/two-factor/recovery-codes"), {})).items;
}

export async function fetchAccessTokens() {
  return (await fetchJSON<{ items: Credential[] }>(settingsURL("/tokens"))).items;
}

// createAccessToken returns the new token, which is never shown again.
export async function createAccessToken(name: string) {
  return (await submit<{ token: string }>("POST", settingsURL("/tokens"), { name })).token;
}

export function deleteAccessToken(id: number) {
  return send("DELETE", settingsURL(`/tokens/${id}`));
}
