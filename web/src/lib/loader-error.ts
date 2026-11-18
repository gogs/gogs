import { notFound, redirect } from "@tanstack/react-router";

import { webContext } from "@/lib/context";

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
    super(errorField ?? `HTTP ${status}`);
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
// which is relative to the subpath, afterwards.
export function authenticationRedirect(href: string) {
  return redirect({ to: "/user/sign-in", search: { redirect_to: webContext.subURL + href }, replace: true });
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
