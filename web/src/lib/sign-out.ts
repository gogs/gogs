import { subUrl } from "@/lib/url";

// signOut ends the session and goes where the server says, or home.
export async function signOut() {
  let redirectTo = subUrl("/");
  try {
    const res = await fetch(subUrl("/api/web/user/sign-out"), {
      method: "POST",
      credentials: "same-origin",
    });
    if (res.ok && res.status !== 204) {
      const body = (await res.json().catch(() => null)) as {
        redirectTo?: string;
      } | null;
      if (body?.redirectTo) {
        redirectTo = body.redirectTo;
      }
    }
  } catch (err) {
    console.error("signOut: request failed", err);
  }
  window.location.assign(redirectTo);
}
