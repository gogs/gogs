import { useMutation } from "@tanstack/react-query";
import { Link, useNavigate, useRouter } from "@tanstack/react-router";
import { CircleCheck, CircleDashed, Pencil, RefreshCw, Send, TriangleAlert } from "lucide-react";
import { type FormEvent, type ReactNode, useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";

import { ConfirmButton } from "@/components/ConfirmButton";
import { CheckboxField, FormAlert, FormField, RadioField } from "@/components/FormField";
import { RichText } from "@/components/RichText";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { webContext } from "@/lib/context";
import { formErrors } from "@/lib/queries/request";
import {
  type WebhookDelivery,
  type WebhookDetail,
  type WebhookEvents,
  type WebhookList,
  type WebhookListItem,
  type WebhookScope,
  type WebhookSettings,
  type WebhookType,
  createWebhook,
  deleteWebhook,
  redeliverWebhook,
  testWebhook,
  updateWebhook,
  webhooksPath,
} from "@/lib/queries/webhooks";
import { subUrl } from "@/lib/url";
import { OrgSettingsLayout } from "@/pages/org/Settings";
import { RepoSettingsLayout } from "@/pages/repo/Settings";

const DATE_TIME_FMT = new Intl.DateTimeFormat(webContext.lang, { dateStyle: "medium", timeStyle: "medium" });
const PRE_CLASS =
  "overflow-x-auto rounded-md bg-(--color-surface) p-3 font-mono text-xs whitespace-pre outline-none focus-visible:ring-1 focus-visible:ring-(--color-ring)";
const FIELD_ORDER = ["url", "channel"] as const;

// WEBHOOK_TYPES are the kinds of webhooks, with their names and logos.
const WEBHOOK_TYPES: Record<WebhookType, { name: string; logo: string; site: string }> = {
  gogs: { name: "Gogs", logo: "/img/favicon.png", site: "https://gogs.io/advancing/webhooks" },
  slack: { name: "Slack", logo: "/img/slack.png", site: "https://slack.com" },
  discord: { name: "Discord", logo: "/img/discord.png", site: "https://discordapp.com/" },
  dingtalk: { name: "Dingtalk", logo: "/img/dingtalk.png", site: "https://open-doc.dingtalk.com/" },
};

const EVENTS: { key: keyof WebhookEvents; label: string }[] = [
  { key: "create", label: "create" },
  { key: "delete", label: "delete" },
  { key: "fork", label: "fork" },
  { key: "push", label: "push" },
  { key: "issues", label: "issues" },
  { key: "pullRequest", label: "pull_request" },
  { key: "issueComment", label: "issue_comment" },
  { key: "release", label: "release" },
];

function TypeLogo({ type, className = "size-4" }: { type: WebhookType; className?: string }) {
  return <img src={subUrl(WEBHOOK_TYPES[type].logo)} alt="" className={`${className} shrink-0`} />;
}

// WebhooksLayout heads the webhook pages with the settings of the repository
// or the organization.
function WebhooksLayout({ scope, children }: { scope: WebhookScope; children: ReactNode }) {
  return "org" in scope ? (
    <OrgSettingsLayout org={scope.org} active="hooks">
      {children}
    </OrgSettingsLayout>
  ) : (
    <RepoSettingsLayout owner={scope.owner} name={scope.repo} active="hooks">
      {children}
    </RepoSettingsLayout>
  );
}

// Webhooks lists the webhooks of a repository or an organization, and links
// to adding one of each kind the site allows.
export function Webhooks({ scope, list }: { scope: WebhookScope; list: WebhookList }) {
  const { t } = useTranslation();
  return (
    <WebhooksLayout scope={scope}>
      <h1 className="mb-2 text-xl font-semibold">{t("repo.settings.hooks")}</h1>
      <p className="mb-4 text-sm text-(--color-muted-foreground)">
        {"org" in scope ? (
          <RichText text={t("org.settings.hooks_desc")} />
        ) : (
          <RichText text={t("repo.settings.hooks_desc")} links={[WEBHOOK_TYPES.gogs.site]} />
        )}
      </p>
      {list.items.length ? (
        <ul>
          {list.items.map((hook) => (
            <WebhookRow key={hook.id} scope={scope} hook={hook} />
          ))}
        </ul>
      ) : (
        <p className="border-y border-(--color-border) py-3 text-sm text-(--color-muted-foreground)">
          {t("repo.settings.no_webhooks")}
        </p>
      )}
      {list.types.length ? (
        <div className="mt-8">
          <h2 className="mb-3 text-base font-semibold">{t("repo.settings.webhooks.add_new")}</h2>
          <div className="flex flex-wrap gap-2">
            {list.types.map((type) => (
              <Button key={type} asChild variant="outline" size="sm">
                <Link to={webhooksPath(scope, `/${type}/new`)}>
                  <TypeLogo type={type} />
                  {WEBHOOK_TYPES[type].name}
                </Link>
              </Button>
            ))}
          </div>
        </div>
      ) : null}
    </WebhooksLayout>
  );
}

function LastStatus({ status }: { status: WebhookListItem["lastStatus"] }) {
  const { t } = useTranslation();
  const [label, icon] =
    status === "succeeded"
      ? [t("repo.settings.webhook.last_delivery_succeeded"), <CircleCheck className="size-4 text-(--color-success)" />]
      : status === "failed"
        ? [
            t("repo.settings.webhook.last_delivery_failed"),
            <TriangleAlert className="size-4 text-(--color-destructive)" />,
          ]
        : [
            t("repo.settings.webhook.no_deliveries"),
            <CircleDashed className="size-4 text-(--color-muted-foreground)" />,
          ];
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span
          role="img"
          aria-label={label}
          tabIndex={0}
          className="mt-0.5 shrink-0 rounded-sm outline-none focus-visible:ring-2 focus-visible:ring-(--color-ring)"
        >
          {icon}
        </span>
      </TooltipTrigger>
      <TooltipContent>{label}</TooltipContent>
    </Tooltip>
  );
}

function WebhookRow({ scope, hook }: { scope: WebhookScope; hook: WebhookListItem }) {
  const { t } = useTranslation();
  const router = useRouter();
  const editPath = webhooksPath(scope, `/${hook.id}`);
  return (
    <li className="flex flex-wrap items-center gap-x-3 gap-y-2 border-b border-(--color-border) py-3">
      <span className="flex min-w-0 flex-1 basis-56 items-start gap-2.5">
        <LastStatus status={hook.lastStatus} />
        <TypeLogo type={hook.type} className="mt-0.5 size-4" />
        <span className="min-w-0">
          <Link to={editPath} className="font-semibold wrap-anywhere text-(--color-primary) hover:underline">
            {hook.url}
          </Link>
          {hook.active ? null : (
            <span className="ml-2 rounded-full border border-(--color-border) px-2 py-0.5 text-xs text-(--color-muted-foreground)">
              {t("repo.settings.webhook.inactive")}
            </span>
          )}
        </span>
      </span>
      <span className="flex shrink-0 gap-2">
        <Button asChild variant="outline" size="sm">
          <Link to={editPath} aria-label={`${t("repo.settings.webhook.edit")} ${hook.url}`}>
            <Pencil className="size-3.5" aria-hidden />
            {t("repo.settings.webhook.edit")}
          </Link>
        </Button>
        <ConfirmButton
          label={t("repo.settings.delete_webhook")}
          ariaLabel={`${t("repo.settings.delete_webhook")} ${hook.url}`}
          title={t("repo.settings.webhook_deletion")}
          description={t("repo.settings.webhook_deletion_desc")}
          size="sm"
          action={() => deleteWebhook(scope, hook.id)}
          onDone={async () => {
            toast.success(t("repo.settings.webhook_deletion_success"));
            await router.invalidate();
          }}
        />
      </span>
    </li>
  );
}

const NEW_SETTINGS: Omit<WebhookSettings, "type"> = {
  url: "",
  contentType: "json",
  secret: "",
  channel: "",
  username: "",
  iconURL: "",
  color: "",
  events: "push_only",
  chosenEvents: {
    create: false,
    delete: false,
    fork: false,
    push: false,
    issues: false,
    issueComment: false,
    pullRequest: false,
    release: false,
  },
  active: true,
};

// WebhookForm adds a webhook of a kind, or changes one when `hook` is given.
// The fields shown depend on the kind of the webhook.
function WebhookForm({ scope, type, hook }: { scope: WebhookScope; type: WebhookType; hook?: WebhookDetail }) {
  const { t } = useTranslation();
  const router = useRouter();
  const navigate = useNavigate();
  const [form, setForm] = useState<WebhookSettings>(() => hook?.settings ?? { ...NEW_SETTINGS, type });
  const save = useMutation({
    mutationFn: async () => {
      if (hook) await updateWebhook(scope, hook.id, form);
      else await createWebhook(scope, form);
    },
    onSuccess: async () => {
      if (hook) {
        toast.success(t("repo.settings.update_hook_success"));
        await router.invalidate();
        return;
      }
      toast.success(t("repo.settings.add_hook_success"));
      await navigate({ to: webhooksPath(scope) });
    },
    onError: (err) => {
      const { fields } = formErrors(err);
      const first = FIELD_ORDER.find((f) => f in fields);
      if (first) requestAnimationFrame(() => document.getElementById(first)?.focus());
    },
  });
  const { fields, formError } = formErrors(save.error);
  const set = (change: Partial<WebhookSettings>) => setForm({ ...form, ...change });
  const text = (key: "username" | "iconURL" | "color", label: string, placeholder: string) => (
    <FormField id={key} label={label} invalid={false}>
      <Input
        id={key}
        name={key}
        autoComplete="off"
        placeholder={placeholder}
        value={form[key]}
        onChange={(e) => set({ [key]: e.target.value })}
      />
    </FormField>
  );
  const descriptions: Record<WebhookType, string> = {
    gogs: t("repo.settings.add_webhook_desc"),
    slack: t("repo.settings.add_slack_hook_desc"),
    discord: t("repo.settings.add_discord_hook_desc"),
    dingtalk: t("repo.settings.add_dingtalk_hook_desc"),
  };

  const onSubmit = (e: FormEvent) => {
    e.preventDefault();
    save.mutate();
  };

  return (
    <form onSubmit={onSubmit} noValidate>
      <fieldset disabled={save.isPending} className="contents">
        <p className="mb-5 text-sm text-(--color-muted-foreground)">
          <RichText text={descriptions[type]} links={[WEBHOOK_TYPES[type].site]} />
        </p>
        <FormAlert message={formError} />
        <div className="flex flex-col gap-5">
          <FormField id="url" label={t("repo.settings.payload_url")} error={fields.url} invalid={"url" in fields}>
            <Input
              id="url"
              name="url"
              type="url"
              autoComplete="off"
              required
              placeholder={
                type === "dingtalk" ? "https://oapi.dingtalk.com/robot/send?access_token=xxxxxxxx" : undefined
              }
              value={form.url}
              onChange={(e) => set({ url: e.target.value })}
            />
          </FormField>
          {type === "gogs" ? (
            <>
              <FormField id="contentType" label={t("repo.settings.content_type")} invalid={false}>
                <Select
                  value={form.contentType || "json"}
                  onValueChange={(v) => set({ contentType: v as WebhookSettings["contentType"] })}
                >
                  <SelectTrigger id="contentType">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="json">application/json</SelectItem>
                    <SelectItem value="form">application/x-www-form-urlencoded</SelectItem>
                  </SelectContent>
                </Select>
              </FormField>
              <FormField
                id="secret"
                label={t("repo.settings.secret")}
                helper={<RichText text={t("repo.settings.secret_desc")} />}
                invalid={false}
              >
                <Input
                  id="secret"
                  name="secret"
                  type="password"
                  autoComplete="new-password"
                  value={form.secret}
                  onChange={(e) => set({ secret: e.target.value })}
                />
              </FormField>
            </>
          ) : null}
          {type === "slack" ? (
            <FormField
              id="channel"
              label={t("repo.settings.slack_channel")}
              error={fields.channel}
              invalid={"channel" in fields}
            >
              <Input
                id="channel"
                name="channel"
                autoComplete="off"
                required
                placeholder="e.g., #general"
                value={form.channel}
                onChange={(e) => set({ channel: e.target.value })}
              />
            </FormField>
          ) : null}
          {type === "slack" || type === "discord" ? (
            <>
              {text("username", t("repo.settings.slack_username"), "e.g., Gogs")}
              {text("iconURL", t("repo.settings.slack_icon_url"), "e.g., https://example.com/img/favicon.png")}
              {text(
                "color",
                t("repo.settings.slack_color"),
                type === "slack" ? "e.g., #dd4b39, good, warning, danger" : "e.g., #dd4b39",
              )}
            </>
          ) : null}

          <div
            role="radiogroup"
            aria-labelledby="events-label"
            className="flex flex-col gap-3 border-t border-(--color-border) pt-5"
          >
            <p id="events-label" className="text-sm font-semibold">
              {t("repo.settings.event_desc")}
            </p>
            {(["push_only", "send_everything", "choose_events"] as const).map((value) => (
              <RadioField
                key={value}
                id={`events-${value}`}
                name="events"
                checked={form.events === value}
                onChange={() => set({ events: value })}
                label={
                  <RichText
                    text={t(value === "choose_events" ? "repo.settings.event_choose" : `repo.settings.event_${value}`)}
                  />
                }
              />
            ))}
            {form.events === "choose_events" ? (
              <div className="grid gap-3 pl-6 sm:grid-cols-2">
                {EVENTS.map(({ key, label }) => (
                  <CheckboxField
                    key={key}
                    id={`event-${key}`}
                    checked={form.chosenEvents[key]}
                    onChange={(checked) => set({ chosenEvents: { ...form.chosenEvents, [key]: checked } })}
                    label={
                      <>
                        <span className="font-medium">{t(`repo.settings.event_${label}`)}</span>
                        <span className="block text-(--color-muted-foreground)">
                          {t(`repo.settings.event_${label}_desc`)}
                        </span>
                      </>
                    }
                  />
                ))}
              </div>
            ) : null}
          </div>

          <div className="border-t border-(--color-border) pt-5">
            <CheckboxField
              id="active"
              checked={form.active}
              onChange={(active) => set({ active })}
              label={
                <>
                  <span className="font-medium">{t("repo.settings.active")}</span>
                  <span className="block text-(--color-muted-foreground)">{t("repo.settings.active_helper")}</span>
                </>
              }
            />
          </div>

          <div className="flex flex-wrap items-center gap-2 border-t border-(--color-border) pt-5">
            <Button type="submit" variant="outline">
              {hook ? t("repo.settings.update_webhook") : t("repo.settings.add_webhook")}
            </Button>
            {hook ? (
              <ConfirmButton
                label={t("repo.settings.delete_webhook")}
                title={t("repo.settings.webhook_deletion")}
                description={t("repo.settings.webhook_deletion_desc")}
                align="start"
                action={() => deleteWebhook(scope, hook.id)}
                onDone={async () => {
                  toast.success(t("repo.settings.webhook_deletion_success"));
                  await navigate({ to: webhooksPath(scope) });
                }}
              />
            ) : null}
          </div>
        </div>
      </fieldset>
    </form>
  );
}

function FormHeading({ type, title }: { type: WebhookType; title: string }) {
  return (
    <h1 className="mb-2 flex items-center gap-2 text-xl font-semibold">
      <TypeLogo type={type} className="size-5" />
      {title}
    </h1>
  );
}

// WebhookNew adds a webhook of a kind.
export function WebhookNew({ scope, type }: { scope: WebhookScope; type: WebhookType }) {
  const { t } = useTranslation();
  return (
    <WebhooksLayout scope={scope}>
      <FormHeading type={type} title={`${t("repo.settings.add_webhook")} (${WEBHOOK_TYPES[type].name})`} />
      <WebhookForm scope={scope} type={type} />
    </WebhooksLayout>
  );
}

// WebhookEdit changes or deletes a webhook, and shows its recent deliveries.
// Only webhooks of repositories can be tested or redelivered, since an
// organization has no repository to send a push from.
export function WebhookEdit({ scope, hook }: { scope: WebhookScope; hook: WebhookDetail }) {
  const { t } = useTranslation();
  const router = useRouter();
  const canDeliver = !("org" in scope);
  const queued = hook.deliveries.some((d) => !d.deliveredAt);

  // Queued deliveries are sent within seconds, so check back until they are.
  useEffect(() => {
    if (!queued) return;
    const timer = setTimeout(() => void router.invalidate(), 3000);
    return () => clearTimeout(timer);
  }, [queued, hook, router]);

  const test = useMutation({
    mutationFn: () => testWebhook(scope, hook.id),
    onSuccess: async () => {
      toast.success(t("repo.settings.webhook.test_delivery_success"));
      await router.invalidate();
    },
    onError: (err) => toast.error(err.message),
  });

  return (
    <WebhooksLayout scope={scope}>
      <FormHeading type={hook.settings.type} title={t("repo.settings.update_webhook")} />
      <WebhookForm scope={scope} type={hook.settings.type} hook={hook} />

      <section className="mt-10 border-t border-(--color-border) pt-6">
        <div className="mb-3 flex flex-wrap items-center justify-between gap-2">
          <h2 className="text-base font-semibold">{t("repo.settings.recent_deliveries")}</h2>
          {canDeliver ? (
            <Button
              type="button"
              variant="outline"
              size="sm"
              disabled={test.isPending}
              aria-describedby="test-delivery-desc"
              onClick={() => test.mutate()}
            >
              <Send className="size-3.5" aria-hidden />
              {t("repo.settings.webhook.test_delivery")}
            </Button>
          ) : null}
        </div>
        {canDeliver ? (
          <p id="test-delivery-desc" className="mb-3 text-sm text-(--color-muted-foreground)">
            {t("repo.settings.webhook.test_delivery_desc")}
          </p>
        ) : null}
        {hook.deliveries.length ? (
          <ul className="border-t border-(--color-border)">
            {hook.deliveries.map((delivery) => (
              <DeliveryRow
                key={delivery.uuid}
                scope={scope}
                hookID={hook.id}
                delivery={delivery}
                canRedeliver={canDeliver}
              />
            ))}
          </ul>
        ) : (
          <p className="border-y border-(--color-border) py-3 text-sm text-(--color-muted-foreground)">
            {t("repo.settings.webhook.no_deliveries")}
          </p>
        )}
      </section>
    </WebhooksLayout>
  );
}

function Headers({ lines }: { lines: [string, string][] }) {
  return (
    <pre tabIndex={0} className={PRE_CLASS}>
      {lines.map(([key, value]) => (
        <span key={key} className="block">
          <strong>{key}:</strong> {value}
        </span>
      ))}
    </pre>
  );
}

function DeliveryRow({
  scope,
  hookID,
  delivery,
  canRedeliver,
}: {
  scope: WebhookScope;
  hookID: number;
  delivery: WebhookDelivery;
  canRedeliver: boolean;
}) {
  const { t } = useTranslation();
  const router = useRouter();
  const redeliver = useMutation({
    mutationFn: () => redeliverWebhook(scope, hookID, delivery.uuid),
    onSuccess: async () => {
      toast.success(t("repo.settings.webhook.redelivery_success").replace(/%(?:\[1\])?s/, delivery.uuid));
      await router.invalidate();
    },
    onError: (err) => toast.error(err.message),
  });
  const queued = !delivery.deliveredAt;
  const status = queued ? "none" : delivery.succeeded ? "succeeded" : "failed";
  const response = delivery.response;

  return (
    <li className="border-b border-(--color-border)">
      <details className="group">
        <summary className="flex cursor-pointer list-none flex-wrap items-center gap-x-3 gap-y-1 rounded-sm py-3 outline-none focus-visible:ring-2 focus-visible:ring-(--color-ring) [&::-webkit-details-marker]:hidden">
          <span className="flex min-w-0 flex-1 basis-56 items-center gap-2.5">
            <DeliveryStatus status={status} />
            <span className="min-w-0 font-mono text-xs wrap-anywhere text-(--color-primary) group-open:font-semibold">
              {delivery.uuid}
            </span>
          </span>
          <span className="text-xs text-(--color-muted-foreground)">
            {delivery.deliveredAt
              ? DATE_TIME_FMT.format(new Date(delivery.deliveredAt))
              : t("repo.settings.webhook.queued")}
          </span>
        </summary>
        <div className="flex flex-col gap-4 pb-4">
          {canRedeliver ? (
            <div>
              <Button
                type="button"
                variant="outline"
                size="sm"
                disabled={redeliver.isPending || queued}
                onClick={() => redeliver.mutate()}
              >
                <RefreshCw className="size-3.5" aria-hidden />
                {t("repo.settings.webhook.redelivery")}
              </Button>
            </div>
          ) : null}
          <div className="flex flex-col gap-2">
            <h3 className="text-sm font-semibold">{t("repo.settings.webhook.request")}</h3>
            {delivery.requestHeaders ? (
              <>
                <h4 className="text-xs font-semibold text-(--color-muted-foreground)">
                  {t("repo.settings.webhook.headers")}
                </h4>
                <Headers
                  lines={[
                    ["Request URL", delivery.url],
                    ["Request method", "POST"],
                    ...Object.entries(delivery.requestHeaders).sort(([a], [b]) => a.localeCompare(b)),
                  ]}
                />
                <h4 className="text-xs font-semibold text-(--color-muted-foreground)">
                  {t("repo.settings.webhook.payload")}
                </h4>
                <pre tabIndex={0} className={PRE_CLASS}>
                  {delivery.payload}
                </pre>
              </>
            ) : (
              <p className="text-sm text-(--color-muted-foreground)">N/A</p>
            )}
          </div>
          <div className="flex flex-col gap-2">
            <h3 className="flex items-center gap-2 text-sm font-semibold">
              {t("repo.settings.webhook.response")}
              {response ? (
                <span
                  className={`rounded-full border px-2 py-0.5 font-mono text-xs ${
                    delivery.succeeded
                      ? "border-(--color-success) text-(--color-success)"
                      : "border-(--color-destructive) text-(--color-destructive)"
                  }`}
                >
                  {response.status}
                </span>
              ) : null}
            </h3>
            {response ? (
              <>
                <h4 className="text-xs font-semibold text-(--color-muted-foreground)">
                  {t("repo.settings.webhook.headers")}
                </h4>
                <Headers lines={Object.entries(response.headers).sort(([a], [b]) => a.localeCompare(b))} />
                <h4 className="text-xs font-semibold text-(--color-muted-foreground)">
                  {t("repo.settings.webhook.body")}
                </h4>
                <pre tabIndex={0} className={`${PRE_CLASS} whitespace-pre-wrap wrap-anywhere`}>
                  {response.body}
                </pre>
              </>
            ) : (
              <p className="text-sm text-(--color-muted-foreground)">N/A</p>
            )}
          </div>
        </div>
      </details>
    </li>
  );
}

function DeliveryStatus({ status }: { status: "succeeded" | "failed" | "none" }) {
  const { t } = useTranslation();
  if (status === "succeeded") {
    return (
      <CircleCheck
        role="img"
        aria-label={t("repo.settings.webhook.delivery_succeeded")}
        className="size-4 shrink-0 text-(--color-success)"
      />
    );
  }
  if (status === "failed") {
    return (
      <TriangleAlert
        role="img"
        aria-label={t("repo.settings.webhook.delivery_failed")}
        className="size-4 shrink-0 text-(--color-destructive)"
      />
    );
  }
  return (
    <CircleDashed
      role="img"
      aria-label={t("repo.settings.webhook.queued")}
      className="size-4 shrink-0 text-(--color-muted-foreground)"
    />
  );
}
