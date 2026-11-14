import { loaderResponseError } from "@/lib/loader-error";

// A signal cancels the request, e.g., when the answer is no longer needed.
export async function fetchJSON<T>(url: string, signal?: AbortSignal): Promise<T> {
  const res = await fetch(url, { credentials: "same-origin", signal });
  if (!res.ok) throw await loaderResponseError(res);
  return (await res.json()) as T;
}

// send makes a change that answers with no content.
export async function send(method: "POST" | "PUT" | "DELETE", url: string) {
  const res = await fetch(url, { method, credentials: "same-origin" });
  if (!res.ok) throw await loaderResponseError(res);
}

// FormError is a failed form submission, with the message for the whole form
// or the messages for its fields.
export class FormError extends Error {
  fields: Record<string, string | null>;
  // body is the whole response, for errors that tell more, e.g., a conflict.
  body: Record<string, unknown>;

  constructor(message: string, fields: Record<string, string | null>, body: Record<string, unknown> = {}) {
    super(message);
    this.name = "FormError";
    this.fields = fields;
    this.body = body;
  }
}

export async function submit<T>(method: "POST" | "PUT" | "PATCH" | "DELETE", url: string, body: unknown): Promise<T> {
  const res = await fetch(url, {
    method,
    credentials: "same-origin",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  if (!res.ok) {
    const data = (await res.json().catch(() => ({}))) as { error?: string; fields?: Record<string, string | null> };
    throw new FormError(data.error ?? `HTTP ${res.status}`, data.fields ?? {}, data);
  }
  // Some changes answer without a body, e.g., 201 Created for a new SSH key.
  const text = await res.text();
  return (text ? JSON.parse(text) : undefined) as T;
}

// uploadAvatar uploads an avatar image as a form.
export async function uploadAvatar(url: string, file: File) {
  const body = new FormData();
  body.append("avatar", file);
  const res = await fetch(url, { method: "PUT", credentials: "same-origin", body });
  if (!res.ok) throw await loaderResponseError(res);
}

// formErrors returns the messages for the fields of a failed form, and the
// message for the whole form when no field is to blame.
export function formErrors(error: Error | null) {
  const fields = error instanceof FormError ? error.fields : {};
  const formError = error && Object.keys(fields).length === 0 ? error.message : null;
  return { fields, formError };
}
