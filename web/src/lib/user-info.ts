import { subUrl } from "@/lib/url";

export interface UserInfo {
  username: string;
  avatarURL: string;
  isAdmin: boolean;
  canCreateOrganization: boolean;
}

export async function fetchUserInfo(): Promise<UserInfo | null> {
  try {
    const res = await fetch(subUrl("/api/web/user-info"), { credentials: "same-origin" });
    if (res.status === 204) return null;
    if (!res.ok) {
      console.error(`fetchUserInfo: unexpected status ${res.status}`);
      return null;
    }
    return (await res.json()) as UserInfo;
  } catch (err) {
    console.error("fetchUserInfo: request failed", err);
    return null;
  }
}
