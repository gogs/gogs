import { createRoot } from "react-dom/client";

import { App } from "./App";
import { ThemeProvider } from "./components/ThemeProvider";
import { UserInfoProvider } from "./components/UserInfoProvider";
import "./index.css";
import { loadCustomization } from "./lib/customization";
import "./lib/i18n";
import { fetchUserInfo } from "./lib/user-info";
import { ServerError } from "./pages/ServerError";

const root = document.getElementById("root");
if (root) {
  try {
    const [userInfo] = await Promise.all([fetchUserInfo(), loadCustomization()]);
    createRoot(root).render(
      <ThemeProvider>
        <UserInfoProvider value={userInfo}>
          <App user={userInfo} />
        </UserInfoProvider>
      </ThemeProvider>,
    );
  } catch (err) {
    createRoot(root).render(
      <ThemeProvider>
        <ServerError error={err} />
      </ThemeProvider>,
    );
  }
}
