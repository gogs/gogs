import { useMutation } from "@tanstack/react-query";
import { Link, useNavigate, useRouter } from "@tanstack/react-router";
import { Calendar, CalendarX, CircleCheck, CircleDot, Clock, Plus } from "lucide-react";
import { type FormEvent, useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";

import { ConfirmButton } from "@/components/ConfirmButton";
import { FormAlert, FormField } from "@/components/FormField";
import { Pagination } from "@/components/Pagination";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  type RepoMilestone,
  type RepoMilestoneForm,
  type RepoMilestonesData,
  createRepoMilestone,
  deleteRepoMilestone,
  setRepoMilestoneClosed,
  updateRepoMilestone,
} from "@/lib/queries/repo";
import { formErrors } from "@/lib/queries/request";
import { formatRelativeTime } from "@/lib/relative-time";
import { subUrl } from "@/lib/url";
import { cn } from "@/lib/utils";
import { IssueTrackerLayout } from "@/pages/repo/Labels";

export interface RepoMilestonesSearch {
  state?: "closed";
  page?: number;
}

const STATE_LINK_CLASS =
  "inline-flex items-center gap-1.5 rounded-md px-2 py-1.5 text-sm tabular-nums hover:bg-(--color-surface)";

// RepoMilestones lists the open or the closed milestones of a repository.
// Writers can change, close, reopen and delete them.
export function RepoMilestones({
  owner,
  name,
  data,
  search,
}: {
  owner: string;
  name: string;
  data: RepoMilestonesData;
  search: RepoMilestonesSearch;
}) {
  const { t } = useTranslation();
  const states = [
    { state: undefined, label: t("repo.milestones.open_tab"), count: data.openCount, icon: CircleDot },
    { state: "closed" as const, label: t("repo.milestones.close_tab"), count: data.closedCount, icon: CircleCheck },
  ];

  return (
    <IssueTrackerLayout
      owner={owner}
      name={name}
      active="milestones"
      title={t("repo.milestones")}
      action={
        data.viewerCanWrite ? (
          <Button asChild variant="outline" size="sm">
            <Link to="/$owner/$repo/milestones/new" params={{ owner, repo: name }}>
              <Plus className="size-4" aria-hidden />
              {t("repo.milestones.new")}
            </Link>
          </Button>
        ) : null
      }
    >
      <div className="rounded-md border border-(--color-border)">
        <ul className="flex gap-1 bg-(--color-surface) px-2 py-1.5">
          {states.map(({ state, label, count, icon: Icon }) => {
            const active = state === search.state;
            return (
              <li key={state ?? "open"}>
                <Link
                  to="."
                  search={{ state }}
                  aria-current={active ? "true" : undefined}
                  className={cn(
                    STATE_LINK_CLASS,
                    active ? "font-semibold text-(--color-foreground)" : "text-(--color-muted-foreground)",
                  )}
                >
                  <Icon className="size-4" aria-hidden />
                  {label.replace(/%(?:\[1\])?d/, String(count))}
                </Link>
              </li>
            );
          })}
        </ul>
        {data.items.length ? (
          <ul>
            {data.items.map((milestone) => (
              <MilestoneRow
                key={milestone.id}
                owner={owner}
                name={name}
                milestone={milestone}
                canWrite={data.viewerCanWrite}
                // Once the only milestone on a page goes, move to the previous page.
                previousPage={data.items.length === 1 && data.page > 1 ? data.page - 1 : undefined}
              />
            ))}
          </ul>
        ) : (
          <p className="border-t border-(--color-border) py-8 text-center text-sm text-(--color-muted-foreground)">
            {t("repo.pulls.no_results")}
          </p>
        )}
      </div>
      {data.totalPages > 1 ? (
        <div className="mt-4">
          <Pagination page={data.page} totalPages={data.totalPages} />
        </div>
      ) : null}
    </IssueTrackerLayout>
  );
}

function MilestoneRow({
  owner,
  name,
  milestone,
  canWrite,
  previousPage,
}: {
  owner: string;
  name: string;
  milestone: RepoMilestone;
  canWrite: boolean;
  // previousPage is the page to show once the milestone leaves the list.
  previousPage?: number;
}) {
  const { t } = useTranslation();
  const router = useRouter();
  const navigate = useNavigate();
  const afterRemoval = async () => {
    if (previousPage) {
      await navigate({
        to: ".",
        search: (prev: RepoMilestonesSearch) => ({ ...prev, page: previousPage > 1 ? previousPage : undefined }),
      });
    }
    await router.invalidate();
  };
  const toggle = useMutation({
    mutationFn: () => setRepoMilestoneClosed(owner, name, milestone.id, !milestone.isClosed),
    onSuccess: afterRemoval,
    onError: (err) => toast.error(err.message),
  });
  const state = milestone.isClosed ? "closed" : "open";

  return (
    <li className="flex flex-col gap-3 border-t border-(--color-border) px-4 py-4 md:flex-row md:gap-8">
      <div className="min-w-0 md:flex-1">
        {/* The issue list is still served by the server-rendered pages. */}
        <a
          href={subUrl(`/${owner}/${name}/issues?state=${state}&milestone=${milestone.id}`)}
          className="text-lg font-semibold wrap-anywhere hover:underline"
        >
          {milestone.name}
        </a>
        <p className="mt-1 flex flex-wrap items-center gap-x-4 gap-y-1 text-sm text-(--color-muted-foreground)">
          {milestone.isClosed && milestone.closedAt ? (
            <span className="inline-flex items-center gap-1.5">
              <Clock className="size-4" aria-hidden />
              {t("repo.milestones.closed").replace(/%(?:\[1\])?s/, formatRelativeTime(t, milestone.closedAt))}
            </span>
          ) : milestone.deadline && milestone.isOverdue ? (
            <span className="inline-flex items-center gap-1.5 text-(--color-destructive)">
              <CalendarX className="size-4" aria-hidden />
              <span>
                {milestone.deadline} ({t("repo.milestones.overdue")})
              </span>
            </span>
          ) : (
            <span className="inline-flex items-center gap-1.5">
              <Calendar className="size-4" aria-hidden />
              {milestone.deadline ?? t("repo.milestones.no_due_date")}
            </span>
          )}
        </p>
        {milestone.descriptionHTML ? (
          <div
            className="mt-2 text-sm wrap-anywhere [&_a]:text-(--color-primary) [&_a]:hover:underline [&_code]:rounded [&_code]:bg-(--color-surface) [&_code]:px-1 [&_code]:font-mono [&_li]:ml-5 [&_ol]:list-decimal [&_p]:my-1 [&_ul]:list-disc"
            // The server renders the Markdown description and sanitizes the result.
            dangerouslySetInnerHTML={{ __html: milestone.descriptionHTML }}
          />
        ) : null}
      </div>
      <div className="flex flex-col gap-2 md:w-80 md:shrink-0">
        <div
          role="progressbar"
          aria-label={milestone.name}
          aria-valuenow={milestone.completeness}
          aria-valuemin={0}
          aria-valuemax={100}
          className="h-2 overflow-hidden rounded-full bg-(--color-secondary)"
        >
          <div className="h-full bg-(--color-success)" style={{ width: `${milestone.completeness}%` }} />
        </div>
        <p className="flex flex-wrap gap-x-4 gap-y-1 text-sm text-(--color-muted-foreground) tabular-nums">
          <span className="font-semibold text-(--color-foreground)">{milestone.completeness}%</span>
          <span>{t("repo.issues.open_tab").replace(/%(?:\[1\])?d/, String(milestone.openIssueCount))}</span>
          <span>{t("repo.issues.close_tab").replace(/%(?:\[1\])?d/, String(milestone.closedIssueCount))}</span>
        </p>
        {canWrite ? (
          <div className="flex flex-wrap gap-2">
            <Button asChild variant="outline" size="sm">
              <Link
                to="/$owner/$repo/milestones/$id/edit"
                params={{ owner, repo: name, id: String(milestone.id) }}
                aria-label={`${t("repo.issues.label_edit")} ${milestone.name}`}
              >
                {t("repo.issues.label_edit")}
              </Link>
            </Button>
            <Button
              variant="outline"
              size="sm"
              aria-label={`${milestone.isClosed ? t("repo.milestones.open") : t("repo.milestones.close")} ${milestone.name}`}
              disabled={toggle.isPending}
              onClick={() => toggle.mutate()}
            >
              {milestone.isClosed ? t("repo.milestones.open") : t("repo.milestones.close")}
            </Button>
            <ConfirmButton
              size="sm"
              label={t("repo.issues.label_delete")}
              ariaLabel={`${t("repo.issues.label_delete")} ${milestone.name}`}
              title={t("repo.milestones.deletion")}
              description={t("repo.milestones.deletion_desc")}
              action={() => deleteRepoMilestone(owner, name, milestone.id)}
              onDone={async () => {
                toast.success(t("repo.milestones.deletion_success"));
                await afterRemoval();
              }}
            />
          </div>
        ) : null}
      </div>
    </li>
  );
}

const TEXTAREA_CLASS =
  "block w-full rounded-md border border-(--color-input) bg-(--color-background) px-3 py-2 text-sm text-(--color-foreground) outline-none transition-colors focus-visible:border-(--color-ring) focus-visible:ring-1 focus-visible:ring-(--color-ring) disabled:cursor-not-allowed disabled:opacity-60 aria-invalid:border-(--color-destructive)";
const MILESTONE_FIELDS = ["title", "description", "deadline"] as const;

// RepoMilestoneEditor creates a milestone, or changes the one with the given
// ID. Only writers get here.
export function RepoMilestoneEditor({
  owner,
  name,
  id,
  initial,
}: {
  owner: string;
  name: string;
  id?: number;
  initial?: RepoMilestoneForm;
}) {
  const { t } = useTranslation();
  const router = useRouter();
  const navigate = useNavigate();
  const [form, setForm] = useState<RepoMilestoneForm>(initial ?? { title: "", description: "", deadline: "" });
  const save = useMutation({
    mutationFn: () => (id ? updateRepoMilestone(owner, name, id, form) : createRepoMilestone(owner, name, form)),
    onSuccess: async () => {
      const message = id ? t("repo.milestones.edit_success") : t("repo.milestones.create_success");
      toast.success(message.replace(/%(?:\[1\])?s/, form.title));
      await router.invalidate();
      await navigate({ to: "/$owner/$repo/milestones", params: { owner, repo: name } });
    },
    onError: (err) => {
      const { fields } = formErrors(err);
      const first = MILESTONE_FIELDS.find((f) => f in fields);
      if (first) requestAnimationFrame(() => document.getElementById(first)?.focus());
    },
  });
  const { fields, formError } = formErrors(save.error);
  const title = id ? t("repo.milestones.edit") : t("repo.milestones.new");

  const onSubmit = (e: FormEvent) => {
    e.preventDefault();
    save.mutate();
  };

  return (
    <IssueTrackerLayout owner={owner} name={name} active="milestones" title={title}>
      <form onSubmit={onSubmit} noValidate className="max-w-2xl">
        <fieldset disabled={save.isPending} className="contents">
          <h2 className="text-xl font-semibold">{title}</h2>
          <p className="mt-1 mb-6 text-sm text-(--color-muted-foreground)">
            {id ? t("repo.milestones.edit_subheader") : t("repo.milestones.new_subheader")}
          </p>
          <FormAlert message={formError} />
          <div className="flex flex-col gap-5">
            <FormField id="title" label={t("repo.milestones.title")} error={fields.title} invalid={"title" in fields}>
              <Input
                id="title"
                name="title"
                autoComplete="off"
                autoFocus
                required
                maxLength={50}
                value={form.title}
                onChange={(e) => setForm({ ...form, title: e.target.value })}
              />
            </FormField>
            <FormField
              id="description"
              label={t("repo.milestones.desc")}
              error={fields.description}
              invalid={"description" in fields}
            >
              <textarea
                id="description"
                name="description"
                rows={6}
                className={TEXTAREA_CLASS}
                value={form.description}
                onChange={(e) => setForm({ ...form, description: e.target.value })}
              />
            </FormField>
            <FormField
              id="deadline"
              label={t("repo.milestones.due_date")}
              error={fields.deadline}
              invalid={"deadline" in fields}
            >
              <Input
                id="deadline"
                name="deadline"
                type="date"
                value={form.deadline}
                onChange={(e) => setForm({ ...form, deadline: e.target.value })}
                className="w-full sm:w-56"
              />
            </FormField>
            {form.deadline ? (
              <div className="-mt-3">
                <Button type="button" variant="outline" size="sm" onClick={() => setForm({ ...form, deadline: "" })}>
                  {t("repo.milestones.clear")}
                </Button>
              </div>
            ) : null}
            <div className="flex flex-wrap gap-2">
              <Button type="submit" variant="outline">
                {id ? t("repo.milestones.modify") : t("repo.milestones.create")}
              </Button>
              <Button asChild variant="ghost">
                <Link
                  to="/$owner/$repo/milestones"
                  params={{ owner, repo: name }}
                  tabIndex={save.isPending ? -1 : undefined}
                  aria-disabled={save.isPending || undefined}
                  className={save.isPending ? "pointer-events-none opacity-50" : undefined}
                  onClick={(e) => {
                    if (save.isPending) e.preventDefault();
                  }}
                >
                  {t("repo.milestones.cancel")}
                </Link>
              </Button>
            </div>
          </div>
        </fieldset>
      </form>
    </IssueTrackerLayout>
  );
}
