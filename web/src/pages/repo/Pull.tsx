import { useMutation, useSuspenseQuery } from "@tanstack/react-query";
import { Link, useRouter } from "@tanstack/react-router";
import {
  CircleAlert,
  CircleCheck,
  CircleX,
  Clock,
  FileDiff,
  GitCommitHorizontal,
  GitMerge,
  GitPullRequest,
  GitPullRequestClosed,
  LoaderCircle,
  type LucideIcon,
  MessagesSquare,
  RefreshCw,
} from "lucide-react";
import { type FormEvent, type ReactNode, useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";

import { ConfirmButton } from "@/components/ConfirmButton";
import { DiffWorkspace } from "@/components/DiffWorkspace";
import { Picker, SidebarSection } from "@/components/IssueForm";
import { PageTabs } from "@/components/PageTabs";
import { PullStack } from "@/components/PullStack";
import { RepoHeader } from "@/components/RepoHeader";
import { RichText } from "@/components/RichText";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { usePageTitle } from "@/lib/page-title";
import {
  type IssueRef,
  type RepoCommitListItem,
  type RepoPullData,
  deleteRepoPullBranch,
  mergeRepoPull,
  mergeRepoPullStack,
  repoHeaderQuery,
  setRepoPullParents,
  updateRepoPullBranch,
} from "@/lib/queries/repo";
import { formErrors } from "@/lib/queries/request";
import { formatAbsoluteTime, formatRelativeTime } from "@/lib/relative-time";
import { cn } from "@/lib/utils";

import type { RepoCommitSearch } from "./Commit.search";
import { CommitList } from "./Commits";
import { IssueConversation, IssueHeader, UserLink } from "./Issue";

const TEXTAREA_CLASS =
  "block w-full rounded-md border border-(--color-input) bg-(--color-background) px-3 py-2 text-sm text-(--color-foreground) outline-none transition-colors focus-visible:border-(--color-ring) focus-visible:ring-1 focus-visible:ring-(--color-ring) disabled:cursor-not-allowed disabled:opacity-60";

// RepoPull shows a pull request on one of its tabs: the conversation, the
// commits it merges, or the files it changes. The files tab has no patch when
// the pull request is broken.
export function RepoPull({
  owner,
  name,
  data,
  tab,
  commits,
  patch,
  search,
}: {
  owner: string;
  name: string;
  data: RepoPullData;
  tab: "conversation" | "commits" | "files";
  commits?: RepoCommitListItem[];
  patch?: string | null;
  search?: RepoCommitSearch;
}) {
  const { t } = useTranslation();
  const { data: repo } = useSuspenseQuery(repoHeaderQuery(owner, name));
  usePageTitle(`${data.title} - ${t("pull_requests")} - ${repo.owner}/${repo.name}`);
  const ref: IssueRef = { owner: repo.owner, name: repo.name, kind: "pulls", index: data.index };
  const { pull } = data;
  const merged = pull.state === "merged";

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <RepoHeader repo={repo} activeTab="pulls" />
      <section className="mx-auto w-full max-w-7xl px-4 pt-6 sm:px-6">
        <IssueHeader
          issueRef={ref}
          data={data}
          state={merged ? "merged" : data.isClosed ? "closed" : "open"}
          summary={<PullSummary data={data} />}
        />
        <PullTabs issueRef={ref} data={data} tab={tab} />
      </section>
      {tab === "conversation" ? (
        <main className="mx-auto flex w-full max-w-7xl flex-1 flex-col gap-4 px-4 py-6 sm:px-6">
          {/* Below md the sidebar comes after the conversation, so the stack
              moves above it to stay at hand. */}
          {pull.stack ? (
            <PullStack
              owner={repo.owner}
              repo={repo.name}
              stack={pull.stack}
              current={data.index}
              className="rounded-md border border-(--color-border) px-3 py-2 md:hidden"
            />
          ) : null}
          <IssueConversation
            issueRef={ref}
            data={data}
            mergeBox={<MergeBox issueRef={ref} data={data} />}
            sidebar={<PullParents issueRef={ref} pull={pull} />}
            aside={
              pull.stack ? (
                <PullStack
                  owner={repo.owner}
                  repo={repo.name}
                  stack={pull.stack}
                  current={data.index}
                  className="hidden border-b border-(--color-border) pb-4 mb-4 md:block"
                />
              ) : null
            }
            canChangeState={data.viewerCanEdit && !merged}
          />
        </main>
      ) : null}
      {tab === "commits" ? (
        <main className="mx-auto w-full max-w-7xl flex-1 px-4 py-6 sm:px-6">
          {commits?.length && pull.diff ? (
            <CommitList owner={pull.diff.owner} name={pull.diff.name} commits={commits} />
          ) : (
            <p className="rounded-md border border-(--color-border) px-4 py-6 text-center text-sm text-(--color-muted-foreground)">
              {t("repo.commits.no_commits")}
            </p>
          )}
        </main>
      ) : null}
      {tab === "files" ? (
        pull.diff && patch && search ? (
          <div className="mt-4 flex min-h-0 flex-1 flex-col">
            <DiffWorkspace
              owner={pull.diff.owner}
              repo={pull.diff.name}
              patch={patch}
              oldRef={pull.diff.base}
              newRef={pull.diff.head}
              search={search}
            />
          </div>
        ) : (
          <main className="mx-auto w-full max-w-7xl flex-1 px-4 py-6 sm:px-6">
            <p className="rounded-md border border-(--color-border) px-4 py-6 text-center text-sm text-(--color-muted-foreground)">
              {pull.diff ? t("repo.pulls.nothing_to_compare") : t("repo.pulls.data_broken")}
            </p>
          </main>
        )
      ) : null}
    </div>
  );
}

const PARENT_STATE_ICONS = { open: GitPullRequest, closed: GitPullRequestClosed, merged: GitMerge } as const;

const PARENT_STATE_CLASSES = {
  open: "text-(--color-success)",
  closed: "text-(--color-destructive)",
  merged: "text-(--color-merged)",
} as const;

// PullParents lists the pull requests the pull request depends on, which its
// changes are reviewed on top of, and lets writers change them.
function PullParents({ issueRef, pull }: { issueRef: IssueRef; pull: RepoPullData["pull"] }) {
  const { t } = useTranslation();
  const { parents, parentOptions } = pull;
  const router = useRouter();
  const change = useMutation({
    mutationFn: (indexes: number[]) => setRepoPullParents(issueRef, indexes),
    onSuccess: () => router.invalidate(),
    onError: (err) => toast.error(err.message),
  });
  const checked = new Set(parents.map((p) => p.index));
  const toggle = (index: number) =>
    change.mutate(checked.has(index) ? [...checked].filter((i) => i !== index) : [...checked, index]);
  return (
    <SidebarSection
      title={t("repo.pulls.depends_on")}
      editor={
        pull.viewerCanEditParents && parentOptions
          ? () => (
              <Picker
                heading={t("repo.pulls.set_parents")}
                filterLabel={t("repo.pulls.filter_parents")}
                groups={[{ items: parentOptions.map((p) => ({ ...p, id: p.index, name: `#${p.index} ${p.title}` })) }]}
                checked={checked}
                disabled={change.isPending}
                render={(p) => {
                  const Icon = PARENT_STATE_ICONS[p.state];
                  return (
                    <>
                      <Icon className={cn("size-4 shrink-0", PARENT_STATE_CLASSES[p.state])} aria-hidden />
                      <span className="min-w-0 truncate">
                        <span className="text-(--color-muted-foreground)">#{p.index}</span> {p.title}
                      </span>
                    </>
                  );
                }}
                onToggle={toggle}
              />
            )
          : undefined
      }
    >
      {parents.length ? (
        <ul className="flex flex-col gap-1">
          {parents.map((parent) => {
            const Icon = PARENT_STATE_ICONS[parent.state];
            return (
              <li key={parent.index}>
                <Link
                  to="/$owner/$repo/pulls/$index"
                  params={{ owner: issueRef.owner, repo: issueRef.name, index: String(parent.index) }}
                  className="flex min-h-6 items-start gap-1.5 text-sm hover:text-(--color-primary)"
                >
                  <Icon className={cn("mt-0.5 size-4 shrink-0", PARENT_STATE_CLASSES[parent.state])} aria-hidden />
                  <span className="min-w-0 wrap-anywhere">
                    <span className="text-(--color-muted-foreground)">#{parent.index}</span> {parent.title}
                    <span className="sr-only"> ({t(`repo.pulls.parent_state_${parent.state}`)})</span>
                  </span>
                </Link>
              </li>
            );
          })}
        </ul>
      ) : (
        <p className="text-sm text-(--color-muted-foreground)">{t("repo.pulls.no_parents")}</p>
      )}
    </SidebarSection>
  );
}

// PullSummary says who wants to merge the commits from which branch into
// which, or who merged them and when.
function PullSummary({ data }: { data: RepoPullData }) {
  const { t } = useTranslation();
  const { pull } = data;
  const user = pull.merger ?? data.poster;
  const text = t(pull.mergedAt ? "repo.pulls.merged_title_desc" : "repo.pulls.title_desc").replace(
    "%[1]d",
    String(pull.commitCount),
  );
  const [before, after = ""] = text.split("%[4]s");
  const branches = [pull.headLabel, pull.baseLabel];
  return (
    <>
      <UserLink user={user} className="font-semibold text-(--color-foreground) hover:underline">
        {user.name}
      </UserLink>{" "}
      <RichText text={before} code={branches} />
      {pull.mergedAt ? (
        <>
          <Tooltip>
            <TooltipTrigger asChild>
              <time dateTime={pull.mergedAt} tabIndex={0}>
                {formatRelativeTime(t, pull.mergedAt)}
              </time>
            </TooltipTrigger>
            <TooltipContent>{formatAbsoluteTime(pull.mergedAt)}</TooltipContent>
          </Tooltip>
          <RichText text={after} />
        </>
      ) : null}
    </>
  );
}

// PullTabs links the tabs of the pull request, with the number of comments,
// commits and changed files. A broken pull request has no commits or files to
// show.
const PULL_TAB_PATHS = {
  conversation: "/$owner/$repo/pulls/$index",
  commits: "/$owner/$repo/pulls/$index/commits",
  files: "/$owner/$repo/pulls/$index/files",
} as const;

function PullTabs({
  issueRef,
  data,
  tab,
}: {
  issueRef: IssueRef;
  data: RepoPullData;
  tab: "conversation" | "commits" | "files";
}) {
  const { t } = useTranslation();
  const params = { owner: issueRef.owner, repo: issueRef.name, index: String(issueRef.index) };
  const commentCount = data.comments.filter((c) => c.type === "comment").length;
  const tabs = [
    {
      key: "conversation",
      icon: MessagesSquare,
      label: t("repo.pulls.tab_conversation"),
      count: commentCount,
    },
    {
      key: "commits",
      icon: GitCommitHorizontal,
      label: t("repo.pulls.tab_commits"),
      count: data.pull.commitCount,
      disabled: !data.pull.diff,
    },
    {
      key: "files",
      icon: FileDiff,
      label: t("repo.pulls.tab_files"),
      count: data.pull.fileCount,
      disabled: !data.pull.diff,
    },
  ] as const;

  return (
    <PageTabs
      label={t("pull_requests")}
      tabs={tabs}
      active={tab}
      // Only the active tab with its count fits next to the menu at 320px.
      inline={1}
      renderLink={(item, props) => (
        <Link
          to={PULL_TAB_PATHS[item.key]}
          params={params}
          // The conversation path is a prefix of the other tabs, so only an
          // exact match is the current page.
          activeOptions={{ exact: true }}
          {...props}
        />
      )}
      className="mt-4 border-b border-(--color-border)"
    />
  );
}

const MERGE_BOX_STATES: Record<RepoPullData["pull"]["state"], { icon: LucideIcon; color: string; tint: string }> = {
  merged: { icon: GitMerge, color: "text-(--color-merged)", tint: "bg-(--color-merged)/15" },
  closed: { icon: CircleX, color: "text-(--color-muted-foreground)", tint: "bg-(--color-surface)" },
  broken: { icon: CircleAlert, color: "text-(--color-destructive)", tint: "bg-(--color-destructive)/15" },
  blocked: { icon: Clock, color: "text-(--color-muted-foreground)", tint: "bg-(--color-surface)" },
  checking: { icon: LoaderCircle, color: "text-(--color-muted-foreground)", tint: "bg-(--color-surface)" },
  mergeable: { icon: CircleCheck, color: "text-(--color-success)", tint: "bg-(--color-success)/15" },
  conflict: { icon: CircleAlert, color: "text-(--color-destructive)", tint: "bg-(--color-destructive)/15" },
};

// MergeBox says whether the pull request can be merged, lets writers merge it
// when it can, and lets them delete its branch once it is merged.
function MergeBox({ issueRef, data }: { issueRef: IssueRef; data: RepoPullData }) {
  const { t } = useTranslation();
  const router = useRouter();
  const { pull } = data;
  const { icon: Icon, color, tint } = MERGE_BOX_STATES[pull.state];
  const iconClassName = cn(color, pull.state === "checking" && "animate-spin");
  const message = {
    merged: t("repo.pulls.has_merged"),
    closed: t("repo.pulls.reopen_to_merge"),
    broken: t("repo.pulls.data_broken"),
    blocked: t("repo.pulls.blocked_desc").replace("%s", listIndexes(pull.waiting)),
    checking: t("repo.pulls.is_checking"),
    mergeable: t("repo.pulls.can_auto_merge_desc"),
    conflict: t("repo.pulls.cannot_auto_merge_desc"),
  }[pull.state];

  let body: ReactNode = null;
  if (pull.state === "merged" && pull.viewerCanDeleteBranch) {
    body = (
      <div className="border-t border-(--color-border) px-4 py-3">
        <ConfirmButton
          label={t("repo.pulls.delete_branch")}
          title={t("repo.pulls.delete_branch")}
          description={t("repo.pulls.delete_branch_desc").replace("%s", pull.headBranch)}
          size="sm"
          action={() => deleteRepoPullBranch(issueRef)}
          onDone={() => router.invalidate()}
        />
      </div>
    );
  } else if (pull.state === "mergeable" && pull.viewerCanMerge) {
    body = <MergeForm issueRef={issueRef} allowRebase={pull.allowRebase} />;
  } else if (pull.state === "blocked" && (pull.viewerCanUpdate || pull.viewerCanMergeStack)) {
    body = <StackActions issueRef={issueRef} pull={pull} />;
  }

  return (
    <section aria-label={t("repo.pulls.merge_pull_request")} className="flex gap-3">
      <span className={cn("hidden size-10 shrink-0 items-center justify-center rounded-full sm:flex", tint)}>
        <Icon className={cn("size-5", iconClassName)} aria-hidden />
      </span>
      <div className="min-w-0 flex-1 rounded-md border border-(--color-border)">
        <div className="flex items-start gap-2 px-4 py-3 text-sm">
          <Icon className={cn("mt-0.5 size-4 shrink-0 sm:hidden", iconClassName)} aria-hidden />
          <div className="min-w-0">
            <p className="font-medium">{message}</p>
            {pull.state === "conflict" ? (
              <p className="mt-1 text-(--color-muted-foreground)">{t("repo.pulls.cannot_auto_merge_helper")}</p>
            ) : null}
            {pull.state === "blocked" ? (
              <p className="mt-1 text-(--color-muted-foreground)">{t("repo.pulls.blocked_helper")}</p>
            ) : null}
            {pull.state === "blocked" && pull.parents.some((p) => p.state === "closed") ? (
              <p className="mt-1 text-(--color-destructive)">
                {t("repo.pulls.blocked_closed").replace(
                  "%s",
                  listIndexes(pull.parents.filter((p) => p.state === "closed").map((p) => p.index)),
                )}
              </p>
            ) : null}
          </div>
        </div>
        {body}
      </div>
    </section>
  );
}

// listIndexes names pull requests by number, e.g., "#1, #2".
function listIndexes(indexes: number[]) {
  return indexes.map((i) => `#${i}`).join(", ");
}

// StackActions lets writers bring the changes of the parents into the branch
// of a pull request that waits for them, and merge it together with them.
function StackActions({ issueRef, pull }: { issueRef: IssueRef; pull: RepoPullData["pull"] }) {
  const { t } = useTranslation();
  const router = useRouter();
  const [open, setOpen] = useState(false);
  const update = useMutation({
    mutationFn: () => updateRepoPullBranch(issueRef),
    onSuccess: () => router.invalidate(),
    onError: (err) => toast.error(err.message),
  });
  const merge = useMutation({
    mutationFn: () => mergeRepoPullStack(issueRef),
    onSuccess: async () => {
      setOpen(false);
      await router.invalidate();
    },
  });
  const { formError } = formErrors(merge.error);
  const ancestors = listIndexes(stackAncestors(pull));

  return (
    <div className="flex flex-col divide-y divide-(--color-border) border-t border-(--color-border)">
      {pull.viewerCanUpdate ? (
        <div className="flex flex-wrap items-center justify-between gap-3 px-4 py-3 text-sm">
          <p className="flex min-w-0 items-start gap-2">
            <RefreshCw className="mt-0.5 size-4 shrink-0 text-(--color-primary)" aria-hidden />
            <span>{t("repo.pulls.behind_desc").replace("%s", listIndexes(pull.behind))}</span>
          </p>
          <Button type="button" size="sm" variant="outline" disabled={update.isPending} onClick={() => update.mutate()}>
            <RefreshCw className={cn("size-4", update.isPending && "motion-safe:animate-spin")} aria-hidden />
            {update.isPending ? t("repo.pulls.updating_branch") : t("repo.pulls.update_branch")}
          </Button>
        </div>
      ) : null}
      {pull.viewerCanMergeStack ? (
        <div className="flex flex-wrap items-center justify-between gap-3 px-4 py-3 text-sm">
          <p className="min-w-0 text-(--color-muted-foreground)">
            {t("repo.pulls.merge_stack_hint").replace("%s", ancestors)}
          </p>
          <Dialog
            open={open}
            onOpenChange={(next) => {
              setOpen(next);
              if (!next) merge.reset();
            }}
          >
            <DialogTrigger asChild>
              <Button type="button" size="sm">
                <GitMerge className="size-4" aria-hidden />
                {t("repo.pulls.merge_stack")}
              </Button>
            </DialogTrigger>
            <DialogContent>
              <DialogHeader className="pb-3">
                <DialogTitle>{t("repo.pulls.merge_stack")}</DialogTitle>
                <DialogDescription>
                  {t("repo.pulls.merge_stack_desc")
                    .replace("%[1]s", ancestors)
                    .replace("%[2]s", `#${issueRef.index}`)
                    .replace("%[3]s", pull.baseBranch)}
                </DialogDescription>
              </DialogHeader>
              {formError ? (
                <p role="alert" className="px-6 pb-3 text-sm text-(--color-destructive)">
                  {formError}
                </p>
              ) : null}
              <DialogFooter>
                <Button type="button" variant="outline" onClick={() => setOpen(false)}>
                  {t("cancel")}
                </Button>
                <Button type="button" disabled={merge.isPending} onClick={() => merge.mutate()}>
                  <GitMerge className="size-4" aria-hidden />
                  {merge.isPending ? t("repo.pulls.merging_stack") : t("repo.pulls.merge_stack")}
                </Button>
              </DialogFooter>
            </DialogContent>
          </Dialog>
        </div>
      ) : null}
    </div>
  );
}

// stackAncestors returns the numbers of the pull requests the pull request
// waits for, directly or through others, parents first, as merging the stack
// merges them.
function stackAncestors(pull: RepoPullData["pull"]) {
  const nodes = pull.stack?.nodes ?? [];
  const byIndex = new Map(nodes.map((n) => [n.index, n]));
  const seen = new Set<number>();
  const pending = [...pull.waiting];
  while (pending.length) {
    const index = pending.pop() as number;
    const node = byIndex.get(index);
    if (seen.has(index) || !node || node.status === "merged") continue;
    seen.add(index);
    pending.push(...node.parents);
  }
  // The nodes are parents first, which is the order they merge in.
  return nodes.filter((n) => seen.has(n.index)).map((n) => n.index);
}

// MergeForm merges the pull request with a merge commit, or by rebasing it
// when the repository allows it.
function MergeForm({ issueRef, allowRebase }: { issueRef: IssueRef; allowRebase: boolean }) {
  const { t } = useTranslation();
  const router = useRouter();
  const [style, setStyle] = useState<"create_merge_commit" | "rebase_before_merging">("create_merge_commit");
  const [description, setDescription] = useState("");
  const mutation = useMutation({
    mutationFn: () =>
      mergeRepoPull(issueRef, { style, description: style === "create_merge_commit" ? description : "" }),
    onSuccess: () => router.invalidate(),
  });
  const { formError } = formErrors(mutation.error);
  const onSubmit = (e: FormEvent) => {
    e.preventDefault();
    mutation.mutate();
  };

  return (
    <form onSubmit={onSubmit} className="border-t border-(--color-border) px-4 py-3">
      <fieldset disabled={mutation.isPending} className="flex flex-col gap-3">
        {allowRebase ? (
          <div role="radiogroup" aria-label={t("repo.pulls.merge_pull_request")} className="flex flex-col gap-2">
            {(["create_merge_commit", "rebase_before_merging"] as const).map((value) => (
              <label key={value} className="flex cursor-pointer items-center gap-2 text-sm">
                <input
                  type="radio"
                  name="merge-style"
                  value={value}
                  checked={style === value}
                  onChange={() => setStyle(value)}
                  className="size-4 shrink-0 cursor-pointer accent-(--color-primary)"
                />
                {t(`repo.pulls.${value}`)}
              </label>
            ))}
          </div>
        ) : null}
        {style === "create_merge_commit" ? (
          <div className="flex flex-col gap-1.5">
            <label htmlFor="merge-description" className="text-sm font-medium">
              {t("repo.pulls.commit_description")}
            </label>
            <textarea
              id="merge-description"
              rows={3}
              className={TEXTAREA_CLASS}
              value={description}
              onChange={(e) => setDescription(e.target.value)}
            />
          </div>
        ) : null}
        {formError ? <p className="text-sm text-(--color-destructive)">{formError}</p> : null}
        <div>
          <Button type="submit" size="sm">
            <GitMerge className="size-4" aria-hidden />
            {t("repo.pulls.merge_pull_request")}
          </Button>
        </div>
      </fieldset>
    </form>
  );
}
