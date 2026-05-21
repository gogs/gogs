import type { UserInfo } from "@/lib/user-info";

import { AppRouter } from "./router";

export function App({ user }: { user: UserInfo | null }) {
  return <AppRouter user={user} />;
}
