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
    if (!res.ok) return null;
    return (await res.json()) as UserInfo | null;
  } catch {
    return null;
  }
}
