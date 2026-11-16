import { type ReactNode, useCallback, useState } from "react";

import { type UserInfo, fetchUserInfo } from "@/lib/user-info";
import { UserInfoContext, UserInfoRefreshContext } from "@/lib/user-info-context";

export function UserInfoProvider({ value, children }: { value: UserInfo | null; children: ReactNode }) {
  const [info, setInfo] = useState(value);
  const refresh = useCallback(async () => {
    const next = await fetchUserInfo();
    // An uploaded avatar keeps its URL, so make the browser load it again.
    const bust = (url: string) => `${url}${url.includes("?") ? "&" : "?"}v=${Date.now()}`;
    setInfo(next && { ...next, avatarURL: bust(next.avatarURL) });
  }, []);
  return (
    <UserInfoRefreshContext.Provider value={refresh}>
      <UserInfoContext.Provider value={info}>{children}</UserInfoContext.Provider>
    </UserInfoRefreshContext.Provider>
  );
}
