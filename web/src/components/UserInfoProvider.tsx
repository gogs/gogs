import type { ReactNode } from "react";

import type { UserInfo } from "@/lib/user-info";
import { UserInfoContext } from "@/lib/user-info-context";

export function UserInfoProvider({ value, children }: { value: UserInfo | null; children: ReactNode }) {
  return <UserInfoContext.Provider value={value}>{children}</UserInfoContext.Provider>;
}
