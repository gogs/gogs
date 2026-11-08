import { Link } from "@tanstack/react-router";
import { ChevronLeft, ChevronRight } from "lucide-react";
import type { ReactNode } from "react";
import { useTranslation } from "react-i18next";

import { cn } from "@/lib/utils";

// Number of page links shown around the current page, matching the legacy
// server-rendered pagination (`paginater.New(..., 5)`).
const WINDOW_SIZE = 5;

// pageWindow returns the page numbers to link, with `null` marking a gap
// before or after the window.
function pageWindow(current: number, total: number): (number | null)[] {
  if (total <= WINDOW_SIZE) {
    return Array.from({ length: total }, (_, i) => i + 1);
  }
  const end = Math.min(total, Math.max(1, current - Math.floor(WINDOW_SIZE / 2)) + WINDOW_SIZE - 1);
  const start = end - WINDOW_SIZE + 1;
  const pages: (number | null)[] = Array.from({ length: WINDOW_SIZE }, (_, i) => start + i);
  if (start > 1) pages.unshift(null);
  if (end < total) pages.push(null);
  return pages;
}

const itemClassName =
  "inline-flex h-9 min-w-9 items-center justify-center gap-1 rounded-md px-2.5 text-sm tabular-nums text-(--color-foreground)";

function PageLink({ page, label, children }: { page: number; label?: string; children: ReactNode }) {
  return (
    <Link
      to="."
      search={(prev: Record<string, unknown>) => ({ ...prev, page: page > 1 ? page : undefined })}
      aria-label={label}
      className={cn(itemClassName, "hover:bg-(--color-surface)")}
    >
      {children}
    </Link>
  );
}

// Pagination links each page through the `page` search param of the current
// route, keeping the other search params. The first page leaves `page` out of
// the URL. Narrow viewports show the position instead of the page links.
export function Pagination({ page, totalPages }: { page: number; totalPages: number }) {
  const { t } = useTranslation();
  if (totalPages <= 1) return null;

  const previous = (
    <>
      <ChevronLeft className="size-4" aria-hidden />
      {t("repo.issues.previous")}
    </>
  );
  const next = (
    <>
      {t("repo.issues.next")}
      <ChevronRight className="size-4" aria-hidden />
    </>
  );
  const disabledClassName = cn(itemClassName, "text-(--color-muted-foreground)");

  return (
    <nav aria-label={t("page")} className="flex items-center justify-center gap-1">
      {page > 1 ? (
        <PageLink page={page - 1}>{previous}</PageLink>
      ) : (
        <span aria-disabled="true" className={disabledClassName}>
          {previous}
        </span>
      )}
      <span className={cn(disabledClassName, "sm:hidden")}>
        {page} / {totalPages}
      </span>
      <span className="hidden items-center gap-1 sm:flex">
        {pageWindow(page, totalPages).map((n, i) =>
          n === null ? (
            <span key={`gap-${i}`} className={disabledClassName} aria-hidden>
              …
            </span>
          ) : n === page ? (
            <span key={n} aria-current="page" className={cn(itemClassName, "bg-(--color-surface) font-semibold")}>
              {n}
            </span>
          ) : (
            <PageLink key={n} page={n} label={`${t("page")} ${n}`}>
              {n}
            </PageLink>
          ),
        )}
      </span>
      {page < totalPages ? (
        <PageLink page={page + 1}>{next}</PageLink>
      ) : (
        <span aria-disabled="true" className={disabledClassName}>
          {next}
        </span>
      )}
    </nav>
  );
}
