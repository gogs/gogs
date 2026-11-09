import { loaderResponseError } from "@/lib/loader-error";

export async function fetchJSON<T>(url: string): Promise<T> {
  const res = await fetch(url, { credentials: "same-origin" });
  if (!res.ok) throw await loaderResponseError(res);
  return (await res.json()) as T;
}

// send makes a change that answers with no content.
export async function send(method: "PUT" | "DELETE", url: string) {
  const res = await fetch(url, { method, credentials: "same-origin" });
  if (!res.ok) throw await loaderResponseError(res);
}

// FormError is a failed form submission, with the message for the whole form
// or the messages for its fields.
export class FormError extends Error {
  fields: Record<string, string | null>;

  constructor(message: string, fields: Record<string, string | null>) {
    super(message);
    this.name = "FormError";
    this.fields = fields;
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
    throw new FormError(data.error ?? `HTTP ${res.status}`, data.fields ?? {});
  }
  if (res.status === 204) return undefined as T;
  return (await res.json()) as T;
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
