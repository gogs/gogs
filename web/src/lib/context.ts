// WebContext mirrors the Go struct the server injects via
// <script>window.__webContext = {...}</script> at the top of index.html.
// Read it once at module load so callers get a stable snapshot.
export interface WebContext {
  lang: string;
  subURL: string;
}

declare global {
  interface Window {
    __webContext?: Partial<WebContext>;
  }
}

function read(): WebContext {
  if (typeof window === "undefined") {
    return { lang: "en-US", subURL: "" };
  }
  const ctx = window.__webContext ?? {};
  return {
    lang: ctx.lang || "en-US",
    subURL: ctx.subURL ?? "",
  };
}

export const webContext: WebContext = read();
