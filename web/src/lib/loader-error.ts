import { notFound, redirect } from "@tanstack/react-router";

// LoaderResponseError carries enough of a failed loader fetch for the route
// error component to render a useful message: the HTTP status, the parsed
// `error` field when the body is JSON-shaped like the webapi 4xx/5xx
// responses, and the raw body as a fallback for non-JSON responses (e.g. a
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
      // Body is not JSON; fall back to raw body in ServerError.
    }
  }
  return new LoaderResponseError(res.status, body, errorField);
}

// loadSignedInPage maps the errors of loading a page that needs a signed-in
// user, e.g., a settings page, to router signals.
export async function loadSignedInPage<T>(load: () => Promise<T>): Promise<T> {
  try {
    return await load();
  } catch (err) {
    if (err instanceof LoaderResponseError && (err.status === 401 || err.status === 403)) {
      // The viewer is signed out, has not confirmed their email, or is
      // prohibited from signing in. Reload the page so the server sends them to
      // sign in, account activation, or the login prohibited page.
      // eslint-disable-next-line @typescript-eslint/only-throw-error -- TanStack's redirect() returns a sentinel that must be thrown.
      throw redirect({ href: window.location.href, reloadDocument: true });
    }
    if (err instanceof LoaderResponseError && err.status === 404) {
      // eslint-disable-next-line @typescript-eslint/only-throw-error -- `notFound()` is the documented TanStack Router signal for 404, not an Error subclass.
      throw notFound();
    }
    throw err;
  }
}
