import { useMutation } from "@tanstack/react-query";
import { Check, LoaderCircle, Minus, Play, Search } from "lucide-react";
import { type FormEvent, useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";

import { AdminPageHeader } from "@/components/AdminPage";
import { FormAlert, FormField } from "@/components/FormField";
import { InlineCode } from "@/components/InlineCode";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { formatNumber } from "@/lib/format";
import {
  ADMIN_OPERATIONS,
  type AdminConfig as AdminConfigData,
  type AdminMonitor as AdminMonitorData,
  type AdminOperation,
  type AdminReportRow,
  type AdminReportSection,
  runAdminOperation,
  sendAdminTestMail,
} from "@/lib/queries/admin";
import { formErrors } from "@/lib/queries/request";
import { formatAbsoluteTime } from "@/lib/relative-time";
import { useFocusFirstError } from "@/lib/use-focus-error";

export function ReportValue({ row }: { row: AdminReportRow }) {
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
  }
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

const TEST_MAIL_FIELDS = ["email"] as const;
// The "email" ID belongs to the section of the email settings.
const TEST_MAIL_IDS = { email: "testMailEmail" };

// TestMailForm sends a test email, to check the email settings work.
function TestMailForm() {
  const { t } = useTranslation();
  const [email, setEmail] = useState("");
  const send = useMutation({
    mutationFn: () => sendAdminTestMail(email),
    onSuccess: (message) => toast.success(message),
  });
  useFocusFirstError(send.error, TEST_MAIL_FIELDS, TEST_MAIL_IDS);
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
            {send.isPending
              ? t("admin.config.email.send_test_mail_submitting")
              : t("admin.config.email.send_test_mail")}
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
    <div className="grid max-w-4xl gap-4">
      <AdminPageHeader title={t("admin.dashboard.operations")} description={t("admin.operations_desc")} />
      <Card>
        <ul className="divide-y divide-(--color-border)">
          {ADMIN_OPERATIONS.map((op) => {
            const label = t(OPERATION_LABELS[op]);
            const running = run.isPending && run.variables === op;
            return (
              <li key={op} className="flex flex-wrap items-center gap-x-3 gap-y-2 px-5 py-3 sm:px-6">
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
      </Card>
    </div>
  );
}

function matches(row: AdminReportRow, needle: string) {
  // A value reads with its unit, e.g., "600 seconds".
  const value = [row.value, row.unit].filter(Boolean).join(" ");
  return [row.label, value, row.hint].some((text) => text?.toLowerCase().includes(needle));
}

// filterSection keeps the settings of a section that match the filter, or all
// of them when the title of the section matches.
function filterSection(section: AdminReportSection, needle: string): AdminReportSection {
  if (section.title.toLowerCase().includes(needle)) return section;
  const groups = section.groups.map((group) => group.filter((row) => matches(row, needle)));
  return { ...section, groups: groups.filter((group) => group.length > 0) };
}

function countRows(sections: AdminReportSection[]) {
  return sections.reduce((n, section) => n + section.groups.reduce((m, group) => m + group.length, 0), 0);
}

// ConfigSection shows the settings of one section of the configuration, with
// the label beside the value where the card is wide enough for both.
function ConfigSection({ section, emailEnabled }: { section: AdminReportSection; emailEnabled: boolean }) {
  return (
    <section id={section.id} aria-labelledby={`${section.id}-title`} className="mb-4 scroll-mt-16 break-inside-avoid">
      <Card className="@container">
        <CardHeader>
          <CardTitle id={`${section.id}-title`} className="text-base">
            {section.title}
          </CardTitle>
        </CardHeader>
        <CardContent className="grid gap-5 pt-4 sm:pt-4">
          {section.groups.map((group, i) => (
            <dl key={i} className="divide-y divide-(--color-border) text-sm">
              {group.map((row) => (
                <div
                  key={row.label}
                  className="grid gap-x-4 gap-y-0.5 py-2 first:pt-0 last:pb-0 @md:grid-cols-[minmax(0,14rem)_minmax(0,1fr)]"
                >
                  <dt className="font-medium">{row.label}</dt>
                  <dd className="min-w-0 wrap-anywhere text-(--color-muted-foreground)">
                    <ReportValue row={row} />
                    {row.hint && <span className="ms-2 text-xs">{row.hint}</span>}
                  </dd>
                </div>
              ))}
            </dl>
          ))}
          {section.id === "email" && emailEnabled ? <TestMailForm /> : null}
        </CardContent>
      </Card>
    </section>
  );
}

// AdminConfig shows the settings the site runs with, a card per section. The
// filter narrows them down by label, value or section, and the list of
// sections jumps to one of them.
export function AdminConfig({ config }: { config: AdminConfigData }) {
  const { t } = useTranslation();
  const [query, setQuery] = useState("");
  const needle = query.trim().toLowerCase();
  const sections =
    needle === ""
      ? config.sections
      : config.sections.map((section) => filterSection(section, needle)).filter((section) => section.groups.length > 0);
  const total = countRows(config.sections);
  return (
    <div className="grid gap-4">
      <AdminPageHeader
        title={t("admin.config")}
        description={
          <span aria-live="polite">
            {needle === ""
              ? t("admin.config.summary", { count: formatNumber(total), sections: config.sections.length })
              : t("admin.config.filtered", { shown: formatNumber(countRows(sections)), count: formatNumber(total) })}
          </span>
        }
      >
        <div className="relative w-full sm:w-64">
          <Search
            className="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-(--color-muted-foreground)"
            aria-hidden
          />
          <Input
            type="search"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder={t("admin.config.filter")}
            aria-label={t("admin.config.filter")}
            className="pl-9"
          />
        </div>
      </AdminPageHeader>
      {sections.length > 0 ? (
        <div className="grid items-start gap-4 lg:grid-cols-[13rem_minmax(0,1fr)]">
          <nav
            aria-label={t("admin.config.sections")}
            className="glass sticky top-16 hidden max-h-[calc(100dvh-5rem)] overflow-y-auto lg:block glass-ui:border glass-ui:p-2"
          >
            <ul className="flex flex-col gap-0.5">
              {sections.map((section) => (
                <li key={section.id}>
                  <a
                    href={`#${section.id}`}
                    className="block rounded-md px-2 py-1.5 text-sm text-(--color-muted-foreground) outline-none hover:bg-(--color-surface) hover:text-(--color-foreground) focus-visible:ring-1 focus-visible:ring-(--color-ring) focus-visible:ring-inset"
                  >
                    {section.title}
                  </a>
                </li>
              ))}
            </ul>
          </nav>
          <div className="-mb-4 gap-4 2xl:columns-2">
            {sections.map((section) => (
              <ConfigSection key={section.id} section={section} emailEnabled={config.emailEnabled} />
            ))}
          </div>
        </div>
      ) : (
        <Card>
          <CardContent className="text-sm text-(--color-muted-foreground)">
            {t("admin.config.no_match", { query: query.trim() })}
          </CardContent>
        </Card>
      )}
    </div>
  );
}

function NotAvailable() {
  return <span className="text-(--color-muted-foreground)">N/A</span>;
}

// AdminJobs lists the background jobs of the server and their schedules. In
// a narrow card the schedule and the times of a job move below its name.
export function AdminJobs({ monitor }: { monitor: AdminMonitorData }) {
  const { t } = useTranslation();
  return (
    <div className="grid gap-4">
      <AdminPageHeader title={t("admin.monitor.cron")} />
      <Card className="@container overflow-hidden">
        {monitor.cronTasks.length > 0 ? (
          <Table>
            <TableHeader className="bg-(--color-surface)">
              <TableRow className="hover:bg-transparent">
                <TableHead>{t("admin.monitor.name")}</TableHead>
                <TableHead className="hidden @xl:table-cell">{t("admin.monitor.schedule")}</TableHead>
                <TableHead className="hidden @4xl:table-cell">{t("admin.monitor.next")}</TableHead>
                <TableHead className="hidden @4xl:table-cell">{t("admin.monitor.previous")}</TableHead>
                <TableHead className="text-right">{t("admin.monitor.execute_times")}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {monitor.cronTasks.map((task) => {
                const next = task.next ? formatAbsoluteTime(task.next) : <NotAvailable />;
                const previous = task.prev ? formatAbsoluteTime(task.prev) : <NotAvailable />;
                const schedule = <InlineCode>{task.schedule}</InlineCode>;
                return (
                  <TableRow key={task.description}>
                    <TableCell>
                      <span className="font-medium wrap-anywhere">{task.description}</span>
                      <dl className="mt-1 grid gap-0.5 text-xs text-(--color-muted-foreground) @4xl:hidden">
                        <div className="flex flex-wrap gap-x-1.5 @xl:hidden">
                          <dt>{t("admin.monitor.schedule")}</dt>
                          <dd>{schedule}</dd>
                        </div>
                        <div className="flex flex-wrap gap-x-1.5">
                          <dt>{t("admin.monitor.next")}</dt>
                          <dd className="text-(--color-foreground)">{next}</dd>
                        </div>
                        <div className="flex flex-wrap gap-x-1.5">
                          <dt>{t("admin.monitor.previous")}</dt>
                          <dd className="text-(--color-foreground)">{previous}</dd>
                        </div>
                      </dl>
                    </TableCell>
                    <TableCell className="hidden @xl:table-cell">{schedule}</TableCell>
                    <TableCell className="hidden tabular-nums @4xl:table-cell">{next}</TableCell>
                    <TableCell className="hidden tabular-nums @4xl:table-cell">{previous}</TableCell>
                    <TableCell className="text-right tabular-nums">{formatNumber(task.execTimes)}</TableCell>
                  </TableRow>
                );
              })}
            </TableBody>
          </Table>
        ) : (
          <CardContent className="text-sm text-(--color-muted-foreground)">{t("repo.pulls.no_results")}</CardContent>
        )}
      </Card>
    </div>
  );
}
