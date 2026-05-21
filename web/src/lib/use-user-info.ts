import { useContext } from "react";

import type { UserInfo } from "./user-info";
import { UserInfoContext } from "./user-info-context";

export function useUserInfo(): UserInfo | null {
  const ctx = useContext(UserInfoContext);
  if (ctx === undefined) {
    throw new Error("useUserInfo must be used within UserInfoProvider");
  }
  return ctx;
}
