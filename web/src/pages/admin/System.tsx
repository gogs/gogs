import { useMutation } from "@tanstack/react-query";
import { useRouter } from "@tanstack/react-router";
import { Check, ExternalLink, Minus } from "lucide-react";
import { type FormEvent, type ReactNode, useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";

import { AdminLayout } from "@/components/AdminLayout";
import { FormAlert, FormField } from "@/components/FormField";
import { RichText } from "@/components/RichText";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
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

function ReportValue({ row }: { row: AdminReportRow }) {
  switch (row.kind) {
    case "code":
      return row.value ? <code className="align-top font-mono text-[0.8125rem]">{row.value}</code> : null;
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

// Operations runs a maintenance task picked from the list.
function Operations() {
  const { t } = useTranslation();
  const router = useRouter();
  const [op, setOp] = useState<AdminOperation | "">("");
  const run = useMutation({
    mutationFn: (picked: AdminOperation) => runAdminOperation(picked),
    onSuccess: async (message) => {
      toast.success(message);
      // A task may change the statistics, e.g., by deleting users.
      await router.invalidate();
    },
    onError: (err) => toast.error(err.message),
  });

  const onSubmit = (e: FormEvent) => {
    e.preventDefault();
    if (op) run.mutate(op);
  };

  return (
    <Section id="operations" title={t("admin.dashboard.operations")}>
      <form onSubmit={onSubmit} className="flex flex-wrap items-center gap-2">
        <div className="min-w-0 flex-1 basis-64">
          <Select value={op} onValueChange={(v) => setOp(v as AdminOperation)} disabled={run.isPending}>
            <SelectTrigger aria-label={t("admin.dashboard.operations")}>
              <SelectValue placeholder={t("admin.dashboard.select_operation_to_run")} />
            </SelectTrigger>
            <SelectContent>
              {ADMIN_OPERATIONS.map((o) => (
                <SelectItem key={o} value={o}>
                  {t(OPERATION_LABELS[o])}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        <Button type="submit" variant="outline" disabled={!op || run.isPending}>
          {t("admin.dashboard.operation_run")}
        </Button>
      </form>
    </Section>
  );
}

// AdminDashboard shows how the site was built, what its database holds and how
// the server is doing, and runs maintenance tasks.
export function AdminDashboard({ dashboard }: { dashboard: AdminDashboardData }) {
  const { t } = useTranslation();
  return (
    <AdminLayout active="dashboard" wide>
      <h1 className="mb-4 text-xl font-semibold">{t("admin.dashboard")}</h1>
      <Section id="build-info" title={t("admin.dashboard.build_info")}>
        <Facts>
          <Fact label={t("admin.dashboard.app_ver")}>{dashboard.appVersion}</Fact>
          <Fact label={t("admin.dashboard.git_version")}>{dashboard.gitVersion}</Fact>
          <Fact label={t("admin.dashboard.go_version")}>{dashboard.goVersion}</Fact>
          <Fact label={t("admin.dashboard.build_time")}>{dashboard.buildTime || "N/A"}</Fact>
          <Fact label={t("admin.dashboard.build_commit")}>
            {dashboard.buildCommit ? (
              <a
                href={`https://github.com/gogs/gogs/commit/${dashboard.buildCommit}`}
                target="_blank"
                rel="noreferrer"
                className="inline-flex items-center gap-1 font-mono text-(--color-primary) hover:underline"
              >
                {dashboard.buildCommit}
                <ExternalLink className="size-3.5 shrink-0" aria-hidden />
              </a>
            ) : (
              "N/A"
            )}
          </Fact>
        </Facts>
      </Section>
      <Section id="statistics" title={t("admin.dashboard.statistic")}>
        <p className="text-sm leading-6">
          <RichText text={dashboard.statistics} />
        </p>
      </Section>
      <Operations />
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
          <Button type="submit" variant="outline" className="sm:mt-[1.625rem]">
            {t("admin.config.email.send_test_mail")}
          </Button>
        </div>
      </fieldset>
    </form>
  );
}

// AdminConfig shows the settings the site runs with.
export function AdminConfig({ config }: { config: AdminConfigData }) {
  const { t } = useTranslation();
  return (
    <AdminLayout active="config" wide>
      <h1 className="mb-4 text-xl font-semibold">{t("admin.config")}</h1>
      {config.sections.map((section) => (
        <Report key={section.id} section={section}>
          {section.id === "email" && config.emailEnabled ? <TestMailForm /> : null}
        </Report>
      ))}
    </AdminLayout>
  );
}

function Meta({ children }: { children: ReactNode }) {
  return (
    <p className="mt-1 flex flex-wrap gap-x-3 gap-y-1 text-xs text-(--color-muted-foreground) tabular-nums">
      {children}
    </p>
  );
}

function Empty() {
  const { t } = useTranslation();
  return (
    <p className="border-t border-(--color-border) py-6 text-sm text-(--color-muted-foreground)">
      {t("repo.pulls.no_results")}
    </p>
  );
}

// AdminMonitor lists the scheduled tasks and the Git processes running now.
export function AdminMonitor({ monitor }: { monitor: AdminMonitorData }) {
  const { t } = useTranslation();
  return (
    <AdminLayout active="monitor" wide>
      <h1 className="mb-4 text-xl font-semibold">{t("admin.monitor")}</h1>
      <Section id="cron" title={t("admin.monitor.cron")}>
        {monitor.cronTasks.length > 0 ? (
          <ul className="border-t border-(--color-border)">
            {monitor.cronTasks.map((task) => (
              <li key={task.description} className={ROW_CLASS}>
                <p className="flex flex-wrap items-baseline gap-x-3 gap-y-1">
                  <span className="font-semibold wrap-anywhere">{task.description}</span>
                  <code className="font-mono text-xs text-(--color-muted-foreground)">{task.schedule}</code>
                </p>
                <Meta>
                  <span>
                    {t("admin.monitor.next")} {task.next ? formatAbsoluteTime(task.next) : "N/A"}
                  </span>
                  <span>
                    {t("admin.monitor.previous")} {task.prev ? formatAbsoluteTime(task.prev) : "N/A"}
                  </span>
                  <span>
                    {t("admin.monitor.execute_times")} {task.execTimes}
                  </span>
                </Meta>
              </li>
            ))}
          </ul>
        ) : (
          <Empty />
        )}
      </Section>
      <Section id="processes" title={t("admin.monitor.process")}>
        {monitor.processes.length > 0 ? (
          <ul className="border-t border-(--color-border)">
            {monitor.processes.map((proc) => (
              <li key={proc.pid} className={ROW_CLASS}>
                <p className="font-mono text-sm wrap-anywhere">{proc.description}</p>
                <Meta>
                  <span>PID {proc.pid}</span>
                  <span>
                    {t("admin.monitor.start")} {formatAbsoluteTime(proc.start)}
                  </span>
                  <span>
                    {t("admin.monitor.execute_time")} {formatRelativeTime(t, proc.start)}
                  </span>
                </Meta>
              </li>
            ))}
          </ul>
        ) : (
          <Empty />
        )}
      </Section>
    </AdminLayout>
  );
}
