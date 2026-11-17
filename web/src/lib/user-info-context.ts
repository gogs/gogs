import { createContext } from "react";

import type { UserInfo } from "./user-info";

export const UserInfoContext = createContext<UserInfo | null | undefined>(undefined);

// UserInfoRefreshContext loads the user info of the viewer again, e.g., after
// they change their avatar, so the navigation bar shows it without a reload.
export const UserInfoRefreshContext = createContext<() => Promise<void>>(() => Promise.resolve());
