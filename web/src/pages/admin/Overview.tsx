import { type UseQueryResult, useQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { Bell, CircleAlert, CircleCheck, ExternalLink, type LucideIcon, TriangleAlert } from "lucide-react";
import { type ReactNode, useId } from "react";
import { useTranslation } from "react-i18next";
import { Area, AreaChart, XAxis, YAxis } from "recharts";

import { Row, Rows, STAT_VALUE, StatCard, TrendChip } from "@/components/AdminPage";
import { InlineCode } from "@/components/InlineCode";
import { Loaded, NotLoaded } from "@/components/Loaded";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { type ChartConfig, ChartContainer } from "@/components/ui/chart";
import { Skeleton } from "@/components/ui/skeleton";
import { METRICS, flag, flagText } from "@/lib/admin-health";
import { formatNumber } from "@/lib/format";
import {
  type AdminDashboard,
  type AdminHealth,
  type AdminList,
  type AdminNotice,
  type AdminTraceOverview,
  adminDashboardQuery,
  adminHealthQuery,
  adminNoticesQuery,
  adminTraceOverviewQuery,
} from "@/lib/queries/admin";
import { formatClockTime } from "@/lib/relative-time";
import { cn } from "@/lib/utils";
import { ReportValue } from "@/pages/admin/System";
import { TraceTrafficChart } from "@/pages/admin/TraceCharts";
import { formatShare } from "@/pages/admin/TraceFormat";
import { TracingDisabledNote } from "@/pages/admin/Traces";

const LINK = "text-sm text-(--color-primary) hover:underline";

const HEADLINE_METRICS = METRICS.filter((metric) => ["requests", "p95", "heap"].includes(metric.key));

// Sparkline draws the shape of a measurement over time, without axes. Flagged
// ones take the primary color, next to the text that tells why.
function Sparkline({ points, flagged, label }: { points: [number, number][]; flagged: boolean; label: string }) {
  const gradient = `spark-${useId().replace(/\W/g, "")}`;
  const config = {
    value: { color: flagged ? "var(--color-primary)" : "var(--color-muted-foreground)" },
  } satisfies ChartConfig;
  return (
    <ChartContainer config={config} role="img" aria-label={label} className="aspect-auto h-12 w-full">
      <AreaChart
        data={points.map(([time, value]) => ({ time, value }))}
        margin={{ top: 4, right: 0, bottom: 0, left: 0 }}
        accessibilityLayer={false}
      >
        <defs>
          <linearGradient id={gradient} x1="0" y1="0" x2="0" y2="1">
            <stop offset="0%" stopColor="var(--color-value)" stopOpacity={0.25} />
            <stop offset="100%" stopColor="var(--color-value)" stopOpacity={0} />
          </linearGradient>
        </defs>
        <XAxis dataKey="time" type="number" domain={["dataMin", "dataMax"]} hide />
        <YAxis domain={[0, "dataMax"]} hide />
        <Area
          dataKey="value"
          type="monotone"
          stroke="var(--color-value)"
          strokeWidth={1.5}
          fill={`url(#${gradient})`}
          isAnimationActive={false}
        />
      </AreaChart>
    </ChartContainer>
  );
}

const STAT_SKELETON = (
  <div className="flex flex-col gap-2">
    <Skeleton className="h-8 w-24" />
    <Skeleton className="h-12 w-full" />
  </div>
);

function HealthStat({ metric, health }: { metric: (typeof METRICS)[number]; health: UseQueryResult<AdminHealth> }) {
  const { t } = useTranslation();
  const label = t(metric.label);
  const points = health.data?.series.find((series) => series.key === metric.key)?.points ?? [];
  const now = points.at(-1)?.[1];
  const value = now === undefined ? "N/A" : metric.format(now);
  const flagged = flagText(t, flag(points, metric), formatClockTime);
  return (
    <StatCard
      label={label}
      chip={flagged ? <TrendChip>{flagged}</TrendChip> : null}
      footer={
        <Link to="/admin/health" className={LINK}>
          {t("admin.health")}
        </Link>
      }
    >
      <Loaded query={health} what={t("admin.health")} skeleton={STAT_SKELETON} compact>
        {() => (
          <>
            <p className={cn(STAT_VALUE, now === undefined && "text-(--color-muted-foreground)")}>{value}</p>
            {points.length > 1 ? (
              <Sparkline
                points={points}
                flagged={flagged !== ""}
                label={
                  t("admin.health.chart", { label, range: t("admin.traces.last_hour"), value }) +
                  (flagged ? `, ${flagged}` : "")
                }
              />
            ) : (
              <div className="h-12" />
            )}
          </>
        )}
      </Loaded>
    </StatCard>
  );
}

function ErrorsStat({ traces }: { traces: UseQueryResult<AdminTraceOverview> }) {
  const { t } = useTranslation();
  return (
    <StatCard
      label={t("admin.overview.errors")}
      footer={
        <Link to="/admin/traces" search={{ view: "search", status: "5xx" }} className={LINK}>
          {t("admin.traces")}
        </Link>
      }
    >
      <Loaded query={traces} what={t("admin.traces")} skeleton={STAT_SKELETON} compact>
        {(overview) =>
          overview.enabled ? (
            <>
              <p className={cn(STAT_VALUE, overview.errors > 0 && "text-(--color-destructive)")}>
                {formatShare(overview.requests > 0 ? overview.errors / overview.requests : 0)}
              </p>
              <p className="min-h-12 text-sm text-(--color-muted-foreground)">
                {t("admin.overview.failed_requests", {
                  errors: formatNumber(overview.errors),
                  requests: formatNumber(overview.requests),
                })}
              </p>
            </>
          ) : (
            <p className="min-h-22 text-sm text-(--color-muted-foreground)">
              <TracingDisabledNote />
            </p>
          )
        }
      </Loaded>
    </StatCard>
  );
}

function AttentionRow({
  icon: Icon,
  tone,
  children,
}: {
  icon: LucideIcon;
  tone: "warning" | "destructive" | "success";
  children: ReactNode;
}) {
  return (
    <li className="flex items-start gap-3 py-3 text-sm first:pt-0 last:pb-0">
      <Icon
        className={cn(
          "mt-0.5 size-4 shrink-0",
          tone === "warning" && "text-(--color-warning)",
          tone === "destructive" && "text-(--color-destructive)",
          tone === "success" && "text-(--color-success)",
        )}
        aria-hidden
      />
      <div className="min-w-0 flex-1">{children}</div>
    </li>
  );
}

// AttentionSource lists what one request found. While it loads, or after it
// failed, a row stands in for it, so the other sources still show.
const ATTENTION_SKELETON = (
  <div className="flex gap-3">
    <Skeleton className="size-4 rounded-full" />
    <div className="flex flex-1 flex-col gap-2">
      <Skeleton className="h-4 w-3/4" />
      <Skeleton className="h-4 w-1/3" />
    </div>
  </div>
);

function AttentionSource<T>({
  query,
  what,
  children,
}: {
  query: UseQueryResult<T>;
  what: string;
  children: (data: T) => ReactNode;
}) {
  if (query.data !== undefined) return children(query.data);
  return (
    <li className="py-3 first:pt-0 last:pb-0">
      <NotLoaded query={query} what={what} skeleton={ATTENTION_SKELETON} compact />
    </li>
  );
}

// AttentionCard gathers what an admin may want to look at, from the requests
// the other cards make, with a link to each page that tells more.
function AttentionCard({
  health,
  traces,
  notices,
}: {
  health: UseQueryResult<AdminHealth>;
  traces: UseQueryResult<AdminTraceOverview>;
  notices: UseQueryResult<AdminList<AdminNotice>>;
}) {
  const { t } = useTranslation();
  const flagged = (data: AdminHealth) =>
    METRICS.flatMap((metric) => {
      const points = data.series.find((series) => series.key === metric.key)?.points ?? [];
      const text = flagText(t, flag(points, metric), formatClockTime);
      return text ? [{ key: metric.key, text: `${t(metric.label)}: ${text}` }] : [];
    });
  const clear =
    health.data !== undefined &&
    traces.data !== undefined &&
    notices.data !== undefined &&
    flagged(health.data).length === 0 &&
    traces.data.errors === 0 &&
    notices.data.total === 0;
  return (
    <Card className="min-w-0">
      <CardHeader>
        <CardTitle className="text-base">{t("admin.overview.needs_attention")}</CardTitle>
        <CardDescription>{t("admin.overview.needs_attention_desc")}</CardDescription>
      </CardHeader>
      <CardContent className="pt-4 sm:pt-4">
        <ul className="divide-y divide-(--color-border)">
          <AttentionSource query={health} what={t("admin.health")}>
            {(data) =>
              flagged(data).map((row) => (
                <AttentionRow key={row.key} icon={TriangleAlert} tone="warning">
                  <p className="wrap-anywhere">{row.text}</p>
                  <Link to="/admin/health" className={LINK}>
                    {t("admin.health")}
                  </Link>
                </AttentionRow>
              ))
            }
          </AttentionSource>
          <AttentionSource query={traces} what={t("admin.traces")}>
            {(overview) =>
              overview.errors > 0 ? (
                <AttentionRow icon={CircleAlert} tone="destructive">
                  <p>
                    {t("admin.overview.failed_requests", {
                      errors: formatNumber(overview.errors),
                      requests: formatNumber(overview.requests),
                    })}
                  </p>
                  <Link to="/admin/traces" search={{ view: "search", status: "5xx" }} className={LINK}>
                    {t("admin.traces")}
                  </Link>
                </AttentionRow>
              ) : null
            }
          </AttentionSource>
          <AttentionSource query={notices} what={t("admin.notices")}>
            {(list) =>
              list.total > 0 ? (
                <AttentionRow icon={Bell} tone="warning">
                  <p>{t("admin.overview.notices_count", { count: formatNumber(list.total) })}</p>
                  <Link to="/admin/notices" className={LINK}>
                    {t("admin.notices")}
                  </Link>
                </AttentionRow>
              ) : null
            }
          </AttentionSource>
          {clear ? (
            <AttentionRow icon={CircleCheck} tone="success">
              {t("admin.overview.all_clear")}
            </AttentionRow>
          ) : null}
        </ul>
      </CardContent>
    </Card>
  );
}

function ReportSkeleton({ rows }: { rows: number }) {
  return (
    <div className="flex flex-col gap-3">
      {Array.from({ length: rows }, (_, i) => (
        <Skeleton key={i} className="h-5 w-full" />
      ))}
    </div>
  );
}

function BuildFacts({ dashboard }: { dashboard: AdminDashboard }) {
  const { t } = useTranslation();
  return (
    <Rows>
      <Row label={t("admin.dashboard.app_ver")}>
        <InlineCode>{dashboard.appVersion}</InlineCode>
      </Row>
      <Row label={t("admin.dashboard.git_version")}>
        <InlineCode>{dashboard.gitVersion}</InlineCode>
      </Row>
      <Row label={t("admin.dashboard.go_version")}>
        <InlineCode>{dashboard.goVersion}</InlineCode>
      </Row>
      <Row label={t("admin.dashboard.build_time")}>
        <InlineCode>{dashboard.buildTime || "N/A"}</InlineCode>
      </Row>
      <Row label={t("admin.dashboard.build_commit")}>
        {dashboard.buildCommit ? (
          <a
            href={`https://github.com/gogs/gogs/commit/${dashboard.buildCommit}`}
            target="_blank"
            rel="noreferrer"
            className="inline-flex items-center gap-1 text-(--color-primary) hover:underline"
          >
            <InlineCode>{dashboard.buildCommit}</InlineCode>
            <ExternalLink className="size-3.5 shrink-0" aria-hidden />
          </a>
        ) : (
          <InlineCode>N/A</InlineCode>
        )}
      </Row>
    </Rows>
  );
}

// AdminOverview is the first page of the admin console: how the server is
// doing right now and what needs attention, then how it was built, what its
// database holds and what it uses. Each card shows when its own request
// answers.
export function AdminOverview() {
  const { t } = useTranslation();
  const dashboard = useQuery(adminDashboardQuery());
  const health = useQuery(adminHealthQuery("1h"));
  const traces = useQuery(adminTraceOverviewQuery("1h"));
  const notices = useQuery(adminNoticesQuery({}));
  return (
    <div className="grid gap-4">
      <h1 className="text-xl font-semibold">{t("admin.overview")}</h1>
      <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
        {HEADLINE_METRICS.map((metric) => (
          <HealthStat key={metric.key} metric={metric} health={health} />
        ))}
        <ErrorsStat traces={traces} />
      </div>
      <div className="grid gap-4 xl:grid-cols-3">
        <Card className="min-w-0 xl:col-span-2">
          <CardHeader>
            <CardTitle className="text-base">{t("admin.overview.traffic")}</CardTitle>
            <CardDescription>{t("admin.overview.traffic_desc")}</CardDescription>
          </CardHeader>
          <CardContent className="pt-4 sm:pt-4">
            <Loaded query={traces} what={t("admin.traces")} skeleton={<Skeleton className="h-64 w-full" />}>
              {(overview) =>
                overview.enabled && overview.requests > 0 ? (
                  <TraceTrafficChart overview={overview} />
                ) : (
                  <div className="flex min-h-64 items-center justify-center">
                    <p className="max-w-prose text-center text-sm text-(--color-muted-foreground)">
                      {overview.enabled ? t("admin.traces.no_requests") : <TracingDisabledNote />}
                    </p>
                  </div>
                )
              }
            </Loaded>
          </CardContent>
        </Card>
        <AttentionCard health={health} traces={traces} notices={notices} />
      </div>
      <div className="grid gap-4 xl:grid-cols-3">
        <Card className="min-w-0 xl:col-span-2">
          <CardHeader>
            <CardTitle className="text-base">{t("admin.dashboard.statistic")}</CardTitle>
          </CardHeader>
          <CardContent className="pt-4 sm:pt-4">
            <Loaded query={dashboard} what={t("admin.dashboard.statistic")} skeleton={<ReportSkeleton rows={6} />}>
              {(data) => (
                <dl className="grid grid-cols-2 gap-x-4 gap-y-4 sm:grid-cols-3 lg:grid-cols-4 2xl:grid-cols-6">
                  {data.statistics.map((statistic) => (
                    <div key={statistic.label} className="flex min-w-0 flex-col-reverse">
                      <dt className="text-sm wrap-anywhere text-(--color-muted-foreground)">{statistic.label}</dt>
                      <dd className="text-lg font-semibold tabular-nums">{formatNumber(statistic.count)}</dd>
                    </div>
                  ))}
                </dl>
              )}
            </Loaded>
          </CardContent>
        </Card>
        <Card className="min-w-0">
          <CardHeader>
            <CardTitle className="text-base">{t("admin.dashboard.build_info")}</CardTitle>
          </CardHeader>
          <CardContent className="pt-4 sm:pt-4">
            <Loaded query={dashboard} what={t("admin.dashboard.build_info")} skeleton={<ReportSkeleton rows={5} />}>
              {(data) => <BuildFacts dashboard={data} />}
            </Loaded>
          </CardContent>
        </Card>
      </div>
      <Card className="min-w-0">
        <CardHeader>
          <CardTitle className="text-base">{t("admin.dashboard.system_status")}</CardTitle>
        </CardHeader>
        <CardContent className="pt-4 sm:pt-4">
          <Loaded query={dashboard} what={t("admin.dashboard.system_status")} skeleton={<ReportSkeleton rows={8} />}>
            {(data) => (
              <div className="-mb-6 gap-x-8 lg:columns-2 2xl:columns-3 [&>dl]:mb-6 [&>dl]:break-inside-avoid">
                {data.systemStatus.groups.map((group, i) => (
                  <Rows key={i}>
                    {group.map((row) => (
                      <Row key={row.label} label={row.label}>
                        <ReportValue row={row} />
                      </Row>
                    ))}
                  </Rows>
                ))}
              </div>
            )}
          </Loaded>
        </CardContent>
      </Card>
    </div>
  );
}
