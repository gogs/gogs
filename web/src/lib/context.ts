// WebContext mirrors the Go struct the server injects via
// <script>window.__webContext = {...}</script> at the top of index.html.
// Read it once at module load so callers get a stable snapshot.
export interface WebContext {
  lang: string;
  subURL: string;
  // localeURL is where the translations of the active locale are served.
  localeURL: string;
  // The page the server chose to show in place of the one at the path.
  page?: "serverError" | "prohibitLogin";
  // The reason of a server error, for viewers allowed to see it.
  error?: string;
  // The version of Gogs, for site admins on a server error.
  version?: string;
  // Where unauthenticated visitors land instead of the home page, if anywhere
  // else, e.g., "/explore/repos".
  landingURL?: string;
  // Whether viewers must be authenticated to view any content.
  requireAuthenticatedViewer?: boolean;
  // The HTML of the notice banner that every page shows.
  notice?: string;
  // The URL of the script that customizes parts of the site, if any.
  customScript?: string;
  externalURL: string;
}

declare global {
  interface Window {
    __webContext?: Partial<WebContext>;
  }
}

function read(): WebContext {
  if (typeof window === "undefined") {
    return { lang: "en-US", subURL: "", localeURL: "", externalURL: "" };
  }
  const ctx = window.__webContext ?? {};
  return {
    lang: ctx.lang || "en-US",
    subURL: ctx.subURL ?? "",
    localeURL: ctx.localeURL ?? "",
    page: ctx.page,
    error: ctx.error,
    version: ctx.version,
    landingURL: ctx.landingURL,
    requireAuthenticatedViewer: ctx.requireAuthenticatedViewer,
    notice: ctx.notice,
    customScript: ctx.customScript,
    externalURL: ctx.externalURL ?? "",
  };
}

export const webContext: WebContext = read();
