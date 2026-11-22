import { toast } from "sonner";

import { signInURL } from "@/lib/loader-error";
import { isSessionExpired } from "@/lib/queries/request";

// toastError tells why a change made with a button failed, under `title` when
// the message alone does not say what went wrong. A viewer whose session
// expired signs in and comes back to the page instead: unlike in a form,
// nothing they typed went into the change.
export function toastError(err: Error, title?: string) {
  if (isSessionExpired(err)) {
    const { pathname, search, hash } = window.location;
    window.location.assign(signInURL(pathname + search + hash));
    return;
  }
  if (title) toast.error(title, { description: err.message });
  else toast.error(err.message);
}
