import { useMutation } from "@tanstack/react-query";
import { Check, ExternalLink, LoaderCircle, Minus, Play } from "lucide-react";
import { type FormEvent, type ReactNode, useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";

import { AdminLayout } from "@/components/AdminLayout";
import { FormAlert, FormField } from "@/components/FormField";
import { RichText } from "@/components/RichText";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  ADMIN_OPERATIONS,
  type AdminConfig as AdminConfigData,
  type AdminDashboard as AdminDashboardData,
  type AdminMonitor as AdminMonitorData,
  type AdminOperation,
  type AdminReportRow,
  type AdminReportSection,
  runAdminOperation,
  sendAdminTestMail,
} from "@/lib/queries/admin";
import { formErrors } from "@/lib/queries/request";
import { formatAbsoluteTime, formatRelativeTime } from "@/lib/relative-time";
import { cn } from "@/lib/utils";

const ROW_CLASS = "border-b border-(--color-border) py-3";

// Section heads a part of an admin page.
function Section({ id, title, children }: { id?: string; title: string; children: ReactNode }) {
  return (
    <section aria-labelledby={id ? `${id}-title` : undefined} className="mb-8">
      <h2 id={id ? `${id}-title` : undefined} className="mb-2 text-lg font-semibold">
        {title}
      </h2>
      {children}
    </section>
  );
}

// Facts lists labeled values, with the label beside the value on wide screens
// and above it on narrow ones.
function Facts({ ruled = true, children }: { ruled?: boolean; children: ReactNode }) {
  return <dl className={cn("text-sm", ruled && "border-t border-(--color-border)")}>{children}</dl>;
}

function Fact({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="grid gap-x-4 gap-y-0.5 border-b border-(--color-border) py-2 sm:grid-cols-[minmax(0,15rem)_minmax(0,1fr)]">
      <dt className="font-medium">{label}</dt>
      <dd className="min-w-0 wrap-anywhere text-(--color-muted-foreground)">{children}</dd>
    </div>
  );
}

function InlineCode({ children }: { children: ReactNode }) {
  return (
    // Cloned decoration keeps the chip's padding and corners on every line a
    // long value wraps to.
    <code className="rounded border border-(--color-border) box-decoration-clone bg-(--color-surface) px-1.5 font-mono text-[0.8125rem] text-(--color-foreground)">
      {children}
    </code>
  );
}

function ReportValue({ row }: { row: AdminReportRow }) {
  switch (row.kind) {
    case "code":
      return row.value ? (
        <>
          <InlineCode>{row.value}</InlineCode>
          {row.unit && ` ${row.unit}`}
        </>
      ) : null;
    case "block":
      return (
        // Focusable so keyboard users can scroll a long line into view.
        <pre
          tabIndex={0}
          className="overflow-x-auto rounded-md bg-(--color-surface) p-2 font-mono text-[0.8125rem] whitespace-pre outline-none focus-visible:ring-1 focus-visible:ring-(--color-ring)"
        >
          {row.value}
        </pre>
      );
    case "bool":
      return (
        <span className="inline-flex h-5 items-center gap-1.5 align-top">
          {row.on ? (
            <Check className="size-4 text-(--color-success)" aria-hidden />
          ) : (
            <Minus className="size-4" aria-hidden />
          )}
          {row.value}
        </span>
      );
    case "unset":
      return <i>{row.value}</i>;
    default:
      return row.value;
  }
}

// Report shows a section of settings or measurements the server labeled.
function Report({ section, children }: { section: AdminReportSection; children?: ReactNode }) {
  return (
    <Section id={section.id} title={section.title}>
      {section.groups.map((group, i) => (
        // The gap between groups replaces the dividers of the server-rendered
        // page.
        <div key={i} className={i > 0 ? "mt-6" : undefined}>
          <Facts ruled={i === 0}>
            {group.map((row) => (
              <Fact key={row.label} label={row.label}>
                <ReportValue row={row} />
                {row.hint && <span className="ms-2 text-xs">{row.hint}</span>}
              </Fact>
            ))}
          </Facts>
        </div>
      ))}
      {children}
    </Section>
  );
}

const OPERATION_LABELS: Record<AdminOperation, string> = {
  "delete-inactive-users": "admin.dashboard.delete_inactivate_accounts",
  "delete-repo-archives": "admin.dashboard.delete_repo_archives",
  "delete-missing-repos": "admin.dashboard.delete_missing_repos",
  "git-gc-repos": "admin.dashboard.git_gc_repos",
  "resync-ssh-keys": "admin.dashboard.resync_all_sshkeys",
  "resync-repo-hooks": "admin.dashboard.resync_all_hooks",
  "reinit-missing-repos": "admin.dashboard.reinit_missing_repos",
};

// AdminDashboard shows how the site was built, what its database holds and how
// the server is doing.
export function AdminDashboard({ dashboard }: { dashboard: AdminDashboardData }) {
  const { t } = useTranslation();
  return (
    <AdminLayout active="dashboard">
      <h1 className="mb-4 text-xl font-semibold">{t("admin.overview")}</h1>
      <Section id="build-info" title={t("admin.dashboard.build_info")}>
        <Facts>
          <Fact label={t("admin.dashboard.app_ver")}>
            <InlineCode>{dashboard.appVersion}</InlineCode>
          </Fact>
          <Fact label={t("admin.dashboard.git_version")}>
            <InlineCode>{dashboard.gitVersion}</InlineCode>
          </Fact>
          <Fact label={t("admin.dashboard.go_version")}>
            <InlineCode>{dashboard.goVersion}</InlineCode>
          </Fact>
          <Fact label={t("admin.dashboard.build_time")}>
            <InlineCode>{dashboard.buildTime || "N/A"}</InlineCode>
          </Fact>
          <Fact label={t("admin.dashboard.build_commit")}>
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
          </Fact>
        </Facts>
      </Section>
      <Section id="statistics" title={t("admin.dashboard.statistic")}>
        <p className="text-sm leading-6">
          <RichText text={dashboard.statistics} />
        </p>
      </Section>
      <Report section={dashboard.systemStatus} />
    </AdminLayout>
  );
}

// TestMailForm sends a test email, to check the email settings work.
function TestMailForm() {
  const { t } = useTranslation();
  const [email, setEmail] = useState("");
  const send = useMutation({
    mutationFn: () => sendAdminTestMail(email),
    onSuccess: (message) => toast.success(message),
  });
  const { fields, formError } = formErrors(send.error);

  const onSubmit = (e: FormEvent) => {
    e.preventDefault();
    send.mutate();
  };

  return (
    <form onSubmit={onSubmit} noValidate className="mt-4">
      <fieldset disabled={send.isPending} className="contents">
        <FormAlert message={formError} />
        <div className="flex flex-wrap items-start gap-2">
          <div className="min-w-0 flex-1 basis-64">
            <FormField
              id="testMailEmail"
              label={t("admin.config.email.send_test_mail")}
              error={fields.email}
              invalid={"email" in fields}
            >
              <Input
                id="testMailEmail"
                name="email"
                type="email"
                autoComplete="email"
                required
                value={email}
                onChange={(e) => setEmail(e.target.value)}
              />
            </FormField>
          </div>
          {/* Lines the button up with the input below the label. */}
          <Button type="submit" className="sm:mt-[1.625rem]">
            {t("admin.config.email.send_test_mail")}
          </Button>
        </div>
      </fieldset>
    </form>
  );
}

// AdminOperations lists the maintenance tasks of the site, each run with its
// own button. One task runs at a time, since each works through every
// repository or user.
export function AdminOperations() {
  const { t } = useTranslation();
  const run = useMutation({
    mutationFn: (op: AdminOperation) => runAdminOperation(op),
    onSuccess: (message) => toast.success(message),
    onError: (err) => toast.error(err.message),
  });
  return (
    <AdminLayout active="operations">
      <h1 className="mb-4 text-xl font-semibold">{t("admin.dashboard.operations")}</h1>
      <ul className="glass-list border-t border-(--color-border)">
        {ADMIN_OPERATIONS.map((op) => {
          const label = t(OPERATION_LABELS[op]);
          const running = run.isPending && run.variables === op;
          return (
            <li key={op} className="flex flex-wrap items-center gap-x-3 gap-y-2 border-b border-(--color-border) py-3">
              <span className="min-w-0 flex-1 basis-56 text-sm wrap-anywhere">{label}</span>
              <Button
                type="button"
                variant="outline"
                size="sm"
                className="shrink-0"
                aria-label={`${t("admin.dashboard.operation_run")} ${label}`}
                disabled={run.isPending}
                onClick={() => run.mutate(op)}
              >
                {running ? (
                  <LoaderCircle className="size-3.5 animate-spin" aria-hidden />
                ) : (
                  <Play className="size-3.5" aria-hidden />
                )}
                {t("admin.dashboard.operation_run")}
              </Button>
            </li>
          );
        })}
      </ul>
    </AdminLayout>
  );
}

// AdminConfig shows the settings the site runs with.
export function AdminConfig({ config }: { config: AdminConfigData }) {
  const { t } = useTranslation();
  return (
    <AdminLayout active="config">
      <h1 className="mb-4 text-xl font-semibold">{t("admin.config")}</h1>
      {config.sections.map((section) => (
        <Report key={section.id} section={section}>
          {section.id === "email" && config.emailEnabled ? <TestMailForm /> : null}
        </Report>
      ))}
    </AdminLayout>
  );
}

// Details lays out the facts of a monitor row. Each is a line of a label and
// its value on narrow screens, and a column with the label above its value
// from the sm breakpoint.
function Details({ children }: { children: ReactNode }) {
  return <dl className="mt-2 grid gap-x-6 gap-y-1 text-sm tabular-nums sm:grid-cols-3 sm:gap-y-2">{children}</dl>;
}

function Detail({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="flex min-w-0 items-baseline justify-between gap-3 sm:block">
      <dt className="shrink-0 text-xs text-(--color-muted-foreground)">{label}</dt>
      <dd className="min-w-0 text-right wrap-anywhere sm:text-left">{children}</dd>
    </div>
  );
}

function NotAvailable() {
  return <span className="text-(--color-muted-foreground)">N/A</span>;
}

function Empty() {
  const { t } = useTranslation();
  return (
    <p className="border-t border-(--color-border) py-6 text-sm text-(--color-muted-foreground)">
      {t("repo.pulls.no_results")}
    </p>
  );
}

// AdminCron lists the scheduled tasks of the server.
export function AdminCron({ monitor }: { monitor: AdminMonitorData }) {
  const { t } = useTranslation();
  return (
    <AdminLayout active="cron">
      <h1 className="mb-4 text-xl font-semibold">{t("admin.monitor.cron")}</h1>
      {monitor.cronTasks.length > 0 ? (
        <ul className="glass-list border-t border-(--color-border)">
          {monitor.cronTasks.map((task) => (
            <li key={task.description} className={ROW_CLASS}>
              <p className="flex flex-wrap items-baseline gap-x-3 gap-y-1">
                <span className="font-semibold wrap-anywhere">{task.description}</span>
                <code className="rounded border border-(--color-border) bg-(--color-surface) px-1.5 font-mono text-xs">
                  {task.schedule}
                </code>
              </p>
              <Details>
                <Detail label={t("admin.monitor.next")}>
                  {task.next ? formatAbsoluteTime(task.next) : <NotAvailable />}
                </Detail>
                <Detail label={t("admin.monitor.previous")}>
                  {task.prev ? formatAbsoluteTime(task.prev) : <NotAvailable />}
                </Detail>
                <Detail label={t("admin.monitor.execute_times")}>{task.execTimes}</Detail>
              </Details>
            </li>
          ))}
        </ul>
      ) : (
        <Empty />
      )}
    </AdminLayout>
  );
}

// AdminProcesses lists the Git processes running now.
export function AdminProcesses({ monitor }: { monitor: AdminMonitorData }) {
  const { t } = useTranslation();
  return (
    <AdminLayout active="processes">
      <h1 className="mb-4 text-xl font-semibold">{t("admin.processes")}</h1>
      {monitor.processes.length > 0 ? (
        <ul className="glass-list border-t border-(--color-border)">
          {monitor.processes.map((proc) => (
            <li key={proc.pid} className={ROW_CLASS}>
              <p className="font-mono text-sm wrap-anywhere">{proc.description}</p>
              <Details>
                <Detail label="PID">{proc.pid}</Detail>
                <Detail label={t("admin.monitor.start")}>{formatAbsoluteTime(proc.start)}</Detail>
                <Detail label={t("admin.monitor.execute_time")}>{formatRelativeTime(t, proc.start)}</Detail>
              </Details>
            </li>
          ))}
        </ul>
      ) : (
        <Empty />
      )}
    </AdminLayout>
  );
}
