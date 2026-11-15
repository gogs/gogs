import { useRouter } from "@tanstack/react-router";
import { KeyRound } from "lucide-react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";

import { ConfirmButton } from "@/components/ConfirmButton";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { webContext } from "@/lib/context";
import type { Credential } from "@/lib/queries/settings";

const DATE_FMT = new Intl.DateTimeFormat(webContext.lang, { year: "numeric", month: "short", day: "2-digit" });

export const ROW_CLASS = "flex flex-wrap items-center gap-x-3 gap-y-2 border-b border-(--color-border) py-3";

// CredentialRow shows an SSH key, a deploy key or an access token with when it
// was added and last used, and deletes it.
export function CredentialRow({
  item,
  detail,
  recentLabel,
  deletion,
}: {
  item: Credential;
  detail?: string;
  recentLabel: string;
  deletion: { label: string; title: string; description: string; success: string; action: () => Promise<void> };
}) {
  const { t } = useTranslation();
  const router = useRouter();
  return (
    <li className={ROW_CLASS}>
      <span className="flex min-w-0 flex-1 basis-56 items-start gap-3">
        <KeyRound className="mt-0.5 size-5 shrink-0 text-(--color-muted-foreground)" aria-hidden />
        <span className="min-w-0">
          <span className="flex items-center gap-2">
            <span className="min-w-0 font-semibold wrap-anywhere">{item.name}</span>
            {item.isRecentlyUsed ? (
              <Tooltip>
                <TooltipTrigger asChild>
                  <span
                    role="img"
                    aria-label={recentLabel}
                    tabIndex={0}
                    className="size-2.5 shrink-0 rounded-full bg-(--color-success)"
                  />
                </TooltipTrigger>
                <TooltipContent>{recentLabel}</TooltipContent>
              </Tooltip>
            ) : null}
          </span>
          {detail ? (
            <span className="block font-mono text-xs wrap-anywhere text-(--color-muted-foreground)">{detail}</span>
          ) : null}
          <span className="mt-1 block text-xs text-(--color-muted-foreground)">
            {t("settings.add_on")} {DATE_FMT.format(new Date(item.createdAt))}
            {" · "}
            {item.lastUsedAt
              ? `${t("settings.last_used")} ${DATE_FMT.format(new Date(item.lastUsedAt))}`
              : t("settings.no_activity")}
          </span>
        </span>
      </span>
      <ConfirmButton
        label={deletion.label}
        ariaLabel={`${deletion.label} ${item.name}`}
        title={deletion.title}
        description={deletion.description}
        size="sm"
        action={deletion.action}
        onDone={async () => {
          toast.success(deletion.success);
          await router.invalidate();
        }}
      />
    </li>
  );
}
