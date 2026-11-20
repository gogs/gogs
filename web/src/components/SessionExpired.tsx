import { Trans } from "react-i18next";

import { subUrl } from "@/lib/url";

// SessionExpired tells a viewer whose session expired to sign in again, in a
// form or in a toast. The link opens on another tab, since leaving the page
// would lose what they typed.
export function SessionExpired() {
  return (
    <Trans
      i18nKey="session_expired"
      components={{
        // Trans puts the words of the translation in the link.
        signIn: <a href={subUrl("/user/sign-in")} target="_blank" rel="noreferrer" className="font-medium underline" />,
      }}
    />
  );
}
