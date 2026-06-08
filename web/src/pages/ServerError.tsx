import { useTranslation } from "react-i18next";

import { LoaderResponseError } from "@/lib/loader-error";
import { usePageTitle } from "@/lib/page-title";

export function ServerError({ error }: { error: unknown }) {
  const { t } = useTranslation();
  usePageTitle(t("status.internal_server_error"));
  const path = typeof window === "undefined" ? "/" : window.location.pathname;

  // Prefer the structured `error` field from the webapi JSON response; fall
  // back to the raw body when the upstream returned non-JSON (e.g. a proxy
  // error page); fall back again to the generic message when nothing useful
  // was carried over.
  let detail = t("status.internal_server_error");
  if (error instanceof LoaderResponseError) {
    if (error.errorField) {
      detail = error.errorField;
    } else if (error.body) {
      detail = error.body;
    }
  } else if (error instanceof Error && error.message) {
    detail = error.message;
  }

  return (
    <main className="flex flex-1 items-center justify-center px-4 py-10 sm:px-6 sm:py-16">
      <div className="w-full max-w-2xl">
        <div className="rounded-lg border border-(--color-foreground)/80 bg-(--color-surface)/40 font-mono shadow-xs dark:border-(--color-border)">
          <div className="flex items-center gap-1.5 border-b border-(--color-foreground)/80 px-3 py-2 sm:px-4 sm:py-2.5 dark:border-(--color-border)">
            <span className="size-2.5 rounded-full bg-(--color-destructive)/70" />
            <span className="size-2.5 rounded-full bg-(--color-warning,oklch(0.795_0.184_86.047))/70" />
            <span className="size-2.5 rounded-full bg-(--color-foreground)/20" />
            <span className="ml-2 text-xs text-(--color-muted-foreground) sm:ml-3">gogs — zsh</span>
          </div>
          <pre className="px-4 py-4 font-pixel text-sm leading-relaxed break-all whitespace-pre-wrap text-(--color-foreground) sm:px-5 sm:py-5 sm:text-base">
            <span className="text-(--color-muted-foreground)">$ </span>
            <span>gogs show {path}</span>
            {"\n"}
            <span className="text-(--color-destructive)">fatal:</span> {detail}
            {"\n"}
            {"\n"}
            <span className="text-(--color-muted-foreground)">$ </span>
            <span className="inline-block w-2 animate-pulse bg-(--color-foreground) align-baseline"> </span>
          </pre>
        </div>
      </div>
    </main>
  );
}
