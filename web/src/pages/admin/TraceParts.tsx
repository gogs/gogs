import { AlertCircle } from "lucide-react";
import { useTranslation } from "react-i18next";

import type { AdminSpanKind, AdminTraceKept, AdminTraceRoute } from "@/lib/queries/admin";

const KIND_CHIPS: Record<AdminSpanKind, string> = { request: "http", git: "git", db: "db", other: "go" };

export function KindChip({ kind }: { kind: AdminSpanKind }) {
  return (
    <span className="shrink-0 rounded border border-(--color-border) px-1 font-mono text-[0.6875rem]/4 text-(--color-muted-foreground)">
      {KIND_CHIPS[kind]}
    </span>
  );
}

export function RouteName({ route }: { route: AdminTraceRoute }) {
  const { t } = useTranslation();
  switch (route.kind) {
    case "other":
      return <span>{t("admin.traces.other_pages", { method: route.name })}</span>;
    case "background":
      return (
        <span className="inline-flex flex-wrap items-baseline gap-x-1.5">
          <span className="font-mono wrap-anywhere">{route.name}</span>
          <span className="text-xs text-(--color-muted-foreground)">{t("admin.traces.background")}</span>
        </span>
      );
    default:
      return <span className="font-mono wrap-anywhere">{route.name}</span>;
  }
}

export function KeptLabel({ kept }: { kept: AdminTraceKept }) {
  const { t } = useTranslation();
  if (kept === "failed") {
    return (
      <span className="inline-flex items-center gap-1 text-(--color-destructive)">
        <AlertCircle className="size-3.5 shrink-0" aria-hidden />
        {t("admin.traces.kept_failed")}
      </span>
    );
  }
  return <span>{t(kept === "slow" ? "admin.traces.kept_slow" : "admin.traces.kept_sampled")}</span>;
}
