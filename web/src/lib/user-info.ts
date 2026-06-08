import { loaderResponseError } from "@/lib/loader-error";
import { subUrl } from "@/lib/url";

export interface UserInfo {
  username: string;
  avatarURL: string;
  isAdmin: boolean;
  canCreateOrganization: boolean;
}

export async function fetchUserInfo(): Promise<UserInfo | null> {
  const res = await fetch(subUrl("/api/web/user/info"), { credentials: "same-origin" });
  if (res.status === 204) return null;
  if (!res.ok) {
    throw await loaderResponseError(res);
  }
  return (await res.json()) as UserInfo;
}
