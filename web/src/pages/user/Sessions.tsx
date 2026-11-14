import { useRouter } from "@tanstack/react-router";
import { Monitor } from "lucide-react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";

import { ConfirmButton } from "@/components/ConfirmButton";
import { ROW_CLASS } from "@/components/CredentialRow";
import { UserSettingsLayout } from "@/components/UserSettingsLayout";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { type LoginSession, deleteLoginSession, deleteOtherLoginSessions } from "@/lib/queries/settings";
import { formatAbsoluteTime, formatRelativeTime } from "@/lib/relative-time";
import { describeUserAgent } from "@/lib/user-agent";

// UserSessionSettings lists the browsers the viewer is signed in on, and signs
// them out.
export function UserSessionSettings({ sessions }: { sessions: LoginSession[] }) {
  const { t } = useTranslation();
  const router = useRouter();
  const hasOthers = sessions.some((s) => !s.isCurrent);
  return (
    <UserSettingsLayout active="sessions">
      <div className="mb-4 flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0 flex-1 basis-64">
          <h1 className="mb-2 text-xl font-semibold">{t("settings.sessions")}</h1>
          <p className="text-sm text-(--color-muted-foreground)">{t("settings.sessions_desc")}</p>
        </div>
        {hasOthers ? (
          <ConfirmButton
            label={t("settings.sessions_sign_out_others")}
            title={t("settings.sessions_sign_out_others")}
            description={t("settings.sessions_sign_out_others_desc")}
            size="sm"
            action={deleteOtherLoginSessions}
            onDone={async () => {
              toast.success(t("settings.sessions_sign_out_others_success"));
              await router.invalidate();
            }}
          />
        ) : null}
      </div>
      <ul>
        {sessions.map((session) => (
          <SessionRow key={session.id} session={session} />
        ))}
      </ul>
    </UserSettingsLayout>
  );
}

function SessionRow({ session }: { session: LoginSession }) {
  const { t } = useTranslation();
  const router = useRouter();
  const { browser, os } = describeUserAgent(session.userAgent);
  const name =
    browser && os
      ? t("settings.session_browser_on_os", { browser, os })
      : (browser ?? os ?? t("settings.session_unknown"));
  return (
    <li className={ROW_CLASS}>
      <span className="flex min-w-0 flex-1 basis-56 items-start gap-3">
        <Monitor className="mt-0.5 size-5 shrink-0 text-(--color-muted-foreground)" aria-hidden />
        <span className="min-w-0">
          <span className="flex flex-wrap items-center gap-2">
            <span className="min-w-0 font-semibold wrap-anywhere">{name}</span>
            {session.isCurrent ? (
              <span className="rounded-full border border-(--color-success) px-2 py-0.5 text-xs text-(--color-success)">
                {t("settings.session_current")}
              </span>
            ) : null}
          </span>
          {session.ip ? (
            <span className="block font-mono text-xs wrap-anywhere text-(--color-muted-foreground)">{session.ip}</span>
          ) : null}
          <span className="mt-1 block text-xs text-(--color-muted-foreground)">
            {t("settings.session_signed_in")} <SessionTime iso={session.createdAt} />
            {" · "}
            {t("settings.session_last_active")} <SessionTime iso={session.lastSeenAt} />
          </span>
        </span>
      </span>
      {session.isCurrent ? null : (
        <ConfirmButton
          label={t("settings.session_sign_out")}
          ariaLabel={`${t("settings.session_sign_out")} ${name}`}
          title={t("settings.session_sign_out_title")}
          description={t("settings.session_sign_out_desc")}
          size="sm"
          action={() => deleteLoginSession(session.id)}
          onDone={async () => {
            toast.success(t("settings.session_sign_out_success"));
            await router.invalidate();
          }}
        />
      )}
    </li>
  );
}

function SessionTime({ iso }: { iso: string }) {
  const { t } = useTranslation();
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <time dateTime={iso} tabIndex={0}>
          {formatRelativeTime(t, iso)}
        </time>
      </TooltipTrigger>
      <TooltipContent>{formatAbsoluteTime(iso)}</TooltipContent>
    </Tooltip>
  );
}
