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
