import type { TFunction } from "i18next";

import { webContext } from "@/lib/context";

// passwordHint tells what a new password needs, e.g., "Use at least 8
// characters.", so people learn the rule before the server turns them away.
export function passwordHint(t: TFunction) {
  return t("form.password_length_hint").replace(/%(?:\[1\])?d/, String(webContext.minPasswordLength));
}
