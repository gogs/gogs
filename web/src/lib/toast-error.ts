import { toast } from "sonner";

import { isSessionExpired, toastSessionExpired } from "@/lib/queries/request";

// toastError tells why a change made with a button failed, under `title` when
// the message alone does not say what went wrong. A viewer whose session
// expired reads how to sign in again, and stays on the page, which may hold
// what they typed for another change, e.g., the draft of a comment.
export function toastError(err: Error, title?: string) {
  if (isSessionExpired(err)) {
    toastSessionExpired();
    return;
  }
  if (title) toast.error(title, { description: err.message });
  else toast.error(err.message);
}
