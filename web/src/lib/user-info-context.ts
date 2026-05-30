import { createContext } from "react";

import type { UserInfo } from "./user-info";

export const UserInfoContext = createContext<UserInfo | null | undefined>(undefined);
