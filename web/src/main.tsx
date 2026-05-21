import { createRoot } from "react-dom/client";

import { App } from "./App";
import { UserInfoProvider } from "./components/UserInfoProvider";
import "./index.css";
import "./lib/i18n";
import { fetchUserInfo } from "./lib/user-info";

const userInfo = await fetchUserInfo();

const root = document.getElementById("root");
if (root) {
  createRoot(root).render(
    <UserInfoProvider value={userInfo}>
      <App />
    </UserInfoProvider>,
  );
}
