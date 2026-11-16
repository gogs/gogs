import { use } from "react";

import type { UserInfo } from "./user-info";
import { UserInfoContext, UserInfoRefreshContext } from "./user-info-context";

export function useUserInfo(): UserInfo | null {
  const ctx = use(UserInfoContext);
  if (ctx === undefined) {
    throw new Error("useUserInfo must be used within UserInfoProvider");
  }
  return ctx;
}

export function useRefreshUserInfo(): () => Promise<void> {
  return use(UserInfoRefreshContext);
}
