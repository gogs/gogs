import { type ReactNode, createElement } from "react";
import { toast } from "sonner";

import { SessionExpired } from "@/components/SessionExpired";
import { LoaderResponseError, displayMessage, loaderResponseError } from "@/lib/loader-error";

const SESSION_EXPIRED_TOAST = "session-expired";
let sessionExpiredAt = -Infinity;

// toastSessionExpired tells a viewer whose session expired to sign in again.
// The toast holds a link, so it stays until it is closed or a change goes
// through, once for however many changes fail. Its class hides it while a
// dialog covers the place of the toasts (see index.css).
export function toastSessionExpired() {
  sessionExpiredAt = performance.now();
  toast.error(createElement(SessionExpired), {
    id: SESSION_EXPIRED_TOAST,
    className: "toast-until-closed",
    duration: Infinity,
  });
}

// sessionWorks takes that toast away once a change went through, which tells
// that the viewer signed in again, e.g., on the tab its link opens. `sentAt`
// is when the change was sent, as of `performance.now()`: the answer to a
// change sent before the toast showed says nothing about the session now.
export function sessionWorks(sentAt: number) {
  if (sentAt <= sessionExpiredAt) return;
  if (toast.getToasts().some((shown) => shown.id === SESSION_EXPIRED_TOAST)) toast.dismiss(SESSION_EXPIRED_TOAST);
}

// A signal cancels the request, e.g., when the answer is no longer needed.
export async function fetchJSON<T>(url: string, signal?: AbortSignal): Promise<T> {
  const res = await fetch(url, { credentials: "same-origin", signal });
  if (!res.ok) throw await loaderResponseError(res);
  return (await res.json()) as T;
}

// send makes a change that answers with no content.
export async function send(method: "POST" | "PUT" | "DELETE", url: string) {
  const sentAt = performance.now();
  const res = await fetch(url, { method, credentials: "same-origin", headers: { "Content-Type": "application/json" } });
  if (!res.ok) throw await loaderResponseError(res);
  sessionWorks(sentAt);
}

// FormError is a failed form submission, with the message for the whole form
// or the messages for its fields.
export class FormError extends Error {
  status: number;
  fields: Record<string, string | null>;
  // body is the whole response, for errors that tell more, e.g., a conflict.
  body: Record<string, unknown>;

  constructor(status: number, message: string, fields: Record<string, string | null>, body: Record<string, unknown>) {
    super(message);
    this.name = "FormError";
    this.status = status;
    this.fields = fields;
    this.body = body;
  }
}

// isSessionExpired reports whether a change failed because the viewer is no
// longer authenticated. The web API also answers 401 to a wrong password, so
// the words of the error decide.
export function isSessionExpired(err: unknown) {
  if (err instanceof FormError) return err.status === 401 && err.body.error === "authentication required";
  if (err instanceof LoaderResponseError) return err.status === 401 && err.errorField === "authentication required";
  return false;
}

export async function submit<T>(method: "POST" | "PUT" | "PATCH" | "DELETE", url: string, body: unknown): Promise<T> {
  const sentAt = performance.now();
  const res = await fetch(url, {
    method,
    credentials: "same-origin",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  if (!res.ok) {
    const data = (await res.json().catch(() => ({}))) as { error?: string; fields?: Record<string, string | null> };
    throw new FormError(
      res.status,
      data.error ? displayMessage(data.error) : `HTTP ${res.status}`,
      data.fields ?? {},
      data,
    );
  }
  sessionWorks(sentAt);
  // Some changes answer without a body, e.g., 201 Created for a new SSH key.
  const text = await res.text();
  return (text ? JSON.parse(text) : undefined) as T;
}

// uploadAvatar uploads an avatar image as a form.
export async function uploadAvatar(url: string, file: File) {
  const body = new FormData();
  body.append("avatar", file);
  const sentAt = performance.now();
  const res = await fetch(url, { method: "PUT", credentials: "same-origin", body });
  if (!res.ok) throw await loaderResponseError(res);
  sessionWorks(sentAt);
}

// formErrors returns the messages for the fields of a failed form, and the
// message for the whole form when no field is to blame. A viewer whose session
// expired keeps what they typed and reads how to send it.
export function formErrors(error: Error | null): { fields: Record<string, string | null>; formError: ReactNode } {
  const fields = error instanceof FormError ? error.fields : {};
  if (!error || Object.keys(fields).length > 0) return { fields, formError: null };
  return { fields, formError: isSessionExpired(error) ? createElement(SessionExpired) : error.message };
}
