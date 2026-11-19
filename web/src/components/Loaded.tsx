import type { UseQueryResult } from "@tanstack/react-query";
import { CircleAlert, LoaderCircle, RefreshCw } from "lucide-react";
import { type ReactNode, useState } from "react";
import { useTranslation } from "react-i18next";

import { Button } from "@/components/ui/button";

// Loaded shows what a query answered. Until then it shows the skeleton, which
// takes the room of the content so the page does not jump, and a failed query
// shows its error with a way to try again. A failed refetch keeps the content
// of the last answer.
export function Loaded<T>({
  query,
  what,
  skeleton,
  compact,
  children,
}: {
  query: UseQueryResult<T>;
  // what names the content in the error, e.g., "Server health".
  what: string;
  skeleton: ReactNode;
  compact?: boolean;
  children: (data: T) => ReactNode;
}) {
  if (query.data !== undefined) return children(query.data);
  return <NotLoaded query={query} what={what} skeleton={skeleton} compact={compact} />;
}

// NotLoaded stands in for what a query has not answered yet: the skeleton, or
// the error of a failed query with a way to try again.
export function NotLoaded({
  query,
  what,
  skeleton,
  compact,
}: {
  query: UseQueryResult<unknown>;
  what: string;
  skeleton: ReactNode;
  compact?: boolean;
}) {
  // A failed query drops its error while it is tried again. The last error
  // stays up until then, so the Retry button keeps its place and the focus.
  const [lastError, setLastError] = useState(query.error);
  if (query.error && query.error !== lastError) setLastError(query.error);
  if (lastError && query.errorUpdateCount > 0) {
    return (
      <LoadError
        what={what}
        error={lastError}
        retrying={query.isFetching}
        onRetry={() => void query.refetch()}
        compact={compact}
      />
    );
  }
  return <div aria-busy="true">{skeleton}</div>;
}

// LoadError tells that a part of the page failed to load, without taking the
// rest of the page down. Without `compact`, it also shows the reason.
function LoadError({
  what,
  error,
  retrying,
  onRetry,
  compact = false,
}: {
  what: string;
  error: Error;
  retrying: boolean;
  onRetry: () => void;
  compact?: boolean;
}) {
  const { t } = useTranslation();
  return (
    <div role="alert" className="flex flex-wrap items-center gap-x-3 gap-y-2 text-sm">
      <CircleAlert className="size-4 shrink-0 text-(--color-destructive)" aria-hidden />
      <div className="min-w-0 flex-1 basis-32">
        <p>{t("admin.load_failed", { what })}</p>
        {compact ? null : (
          <p className="font-mono text-xs wrap-anywhere text-(--color-muted-foreground)">{error.message}</p>
        )}
      </div>
      {/* Not `disabled`, which takes the focus off the button. */}
      <Button
        type="button"
        variant="outline"
        size="sm"
        aria-disabled={retrying || undefined}
        className={retrying ? "opacity-60" : undefined}
        onClick={retrying ? undefined : onRetry}
      >
        {retrying ? (
          <LoaderCircle className="size-3.5 animate-spin" aria-hidden />
        ) : (
          <RefreshCw className="size-3.5" aria-hidden />
        )}
        {t("admin.retry")}
      </Button>
    </div>
  );
}
