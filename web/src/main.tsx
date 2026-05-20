import { createRoot } from "react-dom/client";

import { App } from "./App";
import "./index.css";
import "./lib/i18n";

const root = document.getElementById("root");
if (root) {
  createRoot(root).render(<App />);
}
