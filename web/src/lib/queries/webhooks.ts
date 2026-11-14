import { fetchJSON, send, submit } from "@/lib/queries/request";
import { subUrl } from "@/lib/url";

// WebhookScope is the repository or the organization whose webhooks are
// managed.
export type WebhookScope = { owner: string; repo: string } | { org: string };

export type WebhookType = "gogs" | "slack" | "discord" | "dingtalk";

export interface WebhookListItem {
  id: number;
  type: WebhookType;
  url: string;
  lastStatus: "succeeded" | "failed" | "none";
  active: boolean;
}

export interface WebhookList {
  items: WebhookListItem[];
  // types are the kinds of webhooks the site allows to add.
  types: WebhookType[];
}

export interface WebhookEvents {
  create: boolean;
  delete: boolean;
  fork: boolean;
  push: boolean;
  issues: boolean;
  issueComment: boolean;
  pullRequest: boolean;
  release: boolean;
}

export interface WebhookSettings {
  type: WebhookType;
  url: string;
  contentType: "json" | "form" | "";
  secret: string;
  channel: string;
  username: string;
  iconURL: string;
  color: string;
  events: "push_only" | "send_everything" | "choose_events";
  chosenEvents: WebhookEvents;
  active: boolean;
}

export interface WebhookDelivery {
  uuid: string;
  // deliveredAt is missing while the delivery waits in the queue.
  deliveredAt?: string;
  succeeded: boolean;
  url: string;
  requestHeaders?: Record<string, string>;
  payload: string;
  // response is missing before the delivery is attempted.
  response?: { status: number; headers: Record<string, string>; body: string };
}

export interface WebhookDetail {
  id: number;
  settings: WebhookSettings;
  deliveries: WebhookDelivery[];
}

// webhooksPath returns the path of the webhook settings pages.
export function webhooksPath(scope: WebhookScope, path = "") {
  return "org" in scope
    ? `/org/${scope.org}/settings/hooks${path}`
    : `/${scope.owner}/${scope.repo}/settings/hooks${path}`;
}

function webhooksURL(scope: WebhookScope, path = "") {
  const base =
    "org" in scope
      ? `/api/web/orgs/${encodeURIComponent(scope.org)}`
      : `/api/web/${encodeURIComponent(scope.owner)}/${encodeURIComponent(scope.repo)}`;
  return subUrl(`${base}/settings/hooks${path}`);
}

export function fetchWebhooks(scope: WebhookScope) {
  return fetchJSON<WebhookList>(webhooksURL(scope));
}

export function fetchWebhook(scope: WebhookScope, id: number) {
  return fetchJSON<WebhookDetail>(webhooksURL(scope, `/${id}`));
}

export function createWebhook(scope: WebhookScope, settings: WebhookSettings) {
  return submit<{ id: number }>("POST", webhooksURL(scope), settings);
}

export function updateWebhook(scope: WebhookScope, id: number, settings: WebhookSettings) {
  return submit<void>("PATCH", webhooksURL(scope, `/${id}`), settings);
}

export function deleteWebhook(scope: WebhookScope, id: number) {
  return send("DELETE", webhooksURL(scope, `/${id}`));
}

export function testWebhook(scope: WebhookScope, id: number) {
  return send("POST", webhooksURL(scope, `/${id}/test`));
}

export function redeliverWebhook(scope: WebhookScope, id: number, uuid: string) {
  return send("POST", webhooksURL(scope, `/${id}/deliveries/${encodeURIComponent(uuid)}/redeliver`));
}
