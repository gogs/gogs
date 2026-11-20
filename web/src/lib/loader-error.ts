import { notFound, redirect } from "@tanstack/react-router";

import { webContext } from "@/lib/context";

// displayMessage starts a message of the web API with a capital letter for
// showing it to people. The API words its errors like Go errors, e.g.,
// "authentication required", and callers compare those words as codes.
export function displayMessage(message: string) {
  return message.charAt(0).toUpperCase() + message.slice(1);
}

// LoaderResponseError carries enough of a failed loader fetch for the route
// error component to render a useful message: the HTTP status, the parsed
// `error` field when the body is JSON-shaped like the webapi 4xx/5xx
// responses, and the raw body as a fallback for non-JSON responses (e.g., a
// reverse proxy error page).
export class LoaderResponseError extends Error {
  status: number;
  body: string;
  errorField: string | null;

  constructor(status: number, body: string, errorField: string | null) {
    super(errorField ? displayMessage(errorField) : `HTTP ${status}`);
    this.name = "LoaderResponseError";
    this.status = status;
    this.body = body;
    this.errorField = errorField;
  }
}

export async function loaderResponseError(res: Response): Promise<LoaderResponseError> {
  const body = await res.text().catch(() => "");
  let errorField: string | null = null;
  if (body) {
    try {
      const parsed = JSON.parse(body) as { error?: unknown };
      if (typeof parsed.error === "string" && parsed.error) {
        errorField = parsed.error;
      }
    } catch {
      // Body is not JSON. ServerError falls back to the raw body.
    }
  }
  return new LoaderResponseError(res.status, body, errorField);
}

// authenticationRedirect sends the viewer to sign in, and back to the page at href,
// which is relative to the subpath, afterwards. The app only asks who the
// viewer is when a page loads, so it takes a viewer whose session ended since
// then, i.e., `sessionEnded`, there with a full page load. Without one, the
// sign-in page still counts them as authenticated and sends them home.
export function authenticationRedirect(href: string, sessionEnded = false) {
  const redirectTo = webContext.subURL + href;
  if (sessionEnded) return redirect({ href: signInURL(redirectTo), replace: true, reloadDocument: true });
  return redirect({ to: "/user/sign-in", search: { redirect_to: redirectTo }, replace: true });
}

// signInURL is the sign-in page that leads on to the URL `redirectTo`, which
// includes the subpath.
export function signInURL(redirectTo: string) {
  return `${webContext.subURL}/user/sign-in?${new URLSearchParams({ redirect_to: redirectTo }).toString()}`;
}

// loadAuthenticatedPage maps the errors of loading a page that needs an
// authenticated user, e.g., a settings page, to router signals. The viewers the
// web API turns away reach the route error component, see `RouteError`.
export async function loadAuthenticatedPage<T>(load: () => Promise<T>): Promise<T> {
  try {
    return await load();
  } catch (err) {
    if (err instanceof LoaderResponseError && err.status === 404) {
      // eslint-disable-next-line @typescript-eslint/only-throw-error -- `notFound()` is the documented TanStack Router signal for 404, not an Error subclass.
      throw notFound();
    }
    throw err;
  }
}
