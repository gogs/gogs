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
}

declare global {
  interface Window {
    __webContext?: Partial<WebContext>;
  }
}

function read(): WebContext {
  if (typeof window === "undefined") {
    return { lang: "en-US", subURL: "", localeURL: "" };
  }
  const ctx = window.__webContext ?? {};
  return {
    lang: ctx.lang || "en-US",
    subURL: ctx.subURL ?? "",
    localeURL: ctx.localeURL ?? "",
    page: ctx.page,
    error: ctx.error,
    version: ctx.version,
  };
}

export const webContext: WebContext = read();
