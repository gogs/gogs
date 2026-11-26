import type { ComponentType } from "react";

import { webContext } from "@/lib/context";
import type { UserInfo } from "@/lib/user-info";

// SLOT_NAMES are the parts of the site that admins can replace, each with a
// script of its own.
export const SLOT_NAMES = ["landing", "navbar", "footer"] as const;
export type SlotName = (typeof SLOT_NAMES)[number];

// The path of the page that previews a draft, which the customization editor
// frames.
export const PREVIEW_PATH = "/-/customize/preview";

export interface SlotProps {
  // Default is the built-in component, to wrap or to fall back to.
  Default: ComponentType;
  user: UserInfo | null;
}

// Customization is what the scripts of the customized parts change.
export interface Customization {
  slots: Partial<Record<SlotName, ComponentType<SlotProps>>>;
  // css is the stylesheets the scripts export, joined.
  css: string;
}

// PreviewRequest is what the editor sends the preview page to show: the
// scripts of every part, with the draft of the part being edited.
export interface PreviewRequest {
  type: "gogs-preview-render";
  sources: Partial<Record<SlotName, string>>;
  part: SlotName;
  viewer: "visitor" | "user";
}

// PreviewMessage is what the preview page tells the editor.
export type PreviewMessage =
  | { type: "gogs-preview-ready" }
  | { type: "gogs-preview-result"; errors: Partial<Record<SlotName, string>> }
  | { type: "gogs-preview-render-error"; part: SlotName; message: string };

// LOAD_TIMEOUT_MS is how long the first render waits for the customization at
// most. Each part has a shorter limit of its own (see customization-api.ts),
// and this one only covers the bundle and the code that runs it.
const LOAD_TIMEOUT_MS = 5000;

let active: Customization | null = null;

// activeCustomization returns the published customization, or null when the
// site is not customized or its scripts failed to load.
export function activeCustomization() {
  return active;
}

// loadCustomization loads the published customization. The code that runs the
// scripts only loads when there are some, so a site without any pays nothing
// for it. The admin console and the preview of the editor never load it, so a
// broken script cannot lock admins out of fixing it.
export async function loadCustomization() {
  const url = webContext.customScript;
  const path = window.location.pathname.slice(webContext.subURL.length);
  if (!url || path === "/admin" || path.startsWith("/admin/") || path === PREVIEW_PATH) return;
  let timer: ReturnType<typeof setTimeout> | undefined;
  const timeout = new Promise<null>((resolve) => {
    timer = setTimeout(() => {
      console.error(`The customization did not load within ${LOAD_TIMEOUT_MS} ms and is left out.`);
      resolve(null);
    }, LOAD_TIMEOUT_MS);
  });
  try {
    const load = import("@/lib/customization-api").then(({ importCustomization }) => importCustomization(url));
    const result = await Promise.race([load, timeout]);
    if (!result) return;
    for (const [name, err] of Object.entries(result.errors)) {
      console.error(`Failed to load the customized ${name}`, err);
    }
    active = result.customization;
    if (active.css) {
      const style = document.createElement("style");
      style.textContent = active.css;
      document.head.append(style);
    }
  } catch (err) {
    console.error("Failed to load the customization", err);
  } finally {
    clearTimeout(timer);
  }
}
