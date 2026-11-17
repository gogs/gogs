import { useMutation, useSuspenseQuery } from "@tanstack/react-query";
import { Link, useRouter } from "@tanstack/react-router";
import {
  Check,
  ChevronDown,
  CircleAlert,
  CircleCheck,
  CircleX,
  FileDiff,
  GitCommitHorizontal,
  GitMerge,
  LoaderCircle,
  type LucideIcon,
  MessagesSquare,
  RotateCcw,
  Trash2,
} from "lucide-react";
import { type FormEvent, type ReactNode, useState } from "react";
import { useTranslation } from "react-i18next";

import { type DiffReview, DiffWorkspace } from "@/components/DiffWorkspace";
import { PageTabs } from "@/components/PageTabs";
import { RepoHeader } from "@/components/RepoHeader";
import { RichText } from "@/components/RichText";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Popover, PopoverAnchor, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { usePageTitle } from "@/lib/page-title";
import {
  type IssueRef,
  type MergeMessage,
  type MergeStyle,
  type RepoCommitListItem,
  type RepoPullData,
  deleteRepoPullBranch,
  mergeRepoPull,
  repoHeaderQuery,
  restoreRepoPullBranch,
} from "@/lib/queries/repo";
import { formErrors } from "@/lib/queries/request";
import { formatAbsoluteTime, formatRelativeTime } from "@/lib/relative-time";
import { toastError } from "@/lib/toast-error";
import { useUserInfo } from "@/lib/use-user-info";
import { cn } from "@/lib/utils";

import type { RepoCommitSearch } from "./Commit.search";
import { CommitList } from "./Commits";
import { IssueConversation, IssueHeader, UserLink } from "./Issue";
import {
  CodeCommentForm,
  PullTimelineItem,
  ReviewChanges,
  ReviewStatusRows,
  ReviewThread,
  ReviewersSection,
} from "./PullReview";

const TEXTAREA_CLASS =
  "block w-full rounded-md border border-(--color-input) bg-(--color-field) px-3 py-2 text-sm text-(--color-foreground) outline-none transition-colors focus-visible:border-(--color-ring) focus-visible:ring-1 focus-visible:ring-(--color-ring) disabled:cursor-not-allowed disabled:opacity-60";

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
  const review = usePullDiffReview(ref, data);

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
        <main className="mx-auto w-full max-w-7xl flex-1 px-4 py-6 sm:px-6">
          <IssueConversation
            issueRef={ref}
            data={data}
            mergeBox={<MergeBox issueRef={ref} data={data} />}
            canChangeState={data.viewerCanEdit && !merged}
            renderItem={(comment) => <PullTimelineItem issueRef={ref} data={data} comment={comment} />}
            sidebar={<ReviewersSection issueRef={ref} data={data} />}
          />
        </main>
      ) : null}
      {tab === "commits" ? (
        <main className="mx-auto w-full max-w-7xl flex-1 px-4 py-6 sm:px-6">
          {commits?.length && pull.diff ? (
            <CommitList owner={pull.diff.owner} name={pull.diff.name} commits={commits} />
          ) : (
            <p className="glass rounded-md border border-(--color-border) px-4 py-6 text-center text-sm text-(--color-muted-foreground)">
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
              markdownExtensions={pull.diff.markdownExtensions}
              search={search}
              review={review}
            />
          </div>
        ) : (
          <main className="mx-auto w-full max-w-7xl flex-1 px-4 py-6 sm:px-6">
            <p className="glass rounded-md border border-(--color-border) px-4 py-6 text-center text-sm text-(--color-muted-foreground)">
              {pull.diff ? t("repo.pulls.nothing_to_compare") : t("repo.pulls.data_broken")}
            </p>
          </main>
        )
      ) : null}
    </div>
  );
}

// PullSummary says who wants to merge the commits from which branch into
// which, or who merged them and when.
function PullSummary({ data }: { data: RepoPullData }) {
  const { t } = useTranslation();
  const { pull } = data;
  const user = pull.merger ?? data.poster;
  const text = t(pull.mergedAt ? "repo.pulls.merged_title_desc" : "repo.pulls.title_desc", {
    count: pull.commitCount,
  });
  const [before, after = ""] = text.split("{time}");
  const branches = { head: pull.headLabel, base: pull.baseLabel };
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
          <RichText text={after} code={branches} />
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

// The colored borders need "!" to outrank the edge of the glass look, while
// the gray ones fall back to it.
const MERGE_BOX_STATES: Record<RepoPullData["pull"]["state"], { icon: LucideIcon; color: string; border: string }> = {
  merged: { icon: GitMerge, color: "text-(--color-merged)", border: "!border-(--color-merged)" },
  closed: { icon: CircleX, color: "text-(--color-muted-foreground)", border: "border-(--color-border)" },
  broken: { icon: CircleAlert, color: "text-(--color-destructive)", border: "!border-(--color-destructive)" },
  checking: { icon: LoaderCircle, color: "text-(--color-muted-foreground)", border: "border-(--color-border)" },
  mergeable: { icon: CircleCheck, color: "text-(--color-success)", border: "!border-(--color-success)" },
  conflict: { icon: CircleAlert, color: "text-(--color-destructive)", border: "!border-(--color-destructive)" },
};
const MERGE_BOX_BLOCKED = { icon: CircleAlert, color: "text-(--color-muted-foreground)", border: "border-(--color-border)" };

// MergeBox says whether the pull request can be merged, lets writers merge it
// when it can, and lets them delete or restore its branch once it is merged.
function MergeBox({ issueRef, data }: { issueRef: IssueRef; data: RepoPullData }) {
  const { t } = useTranslation();
  const { pull } = data;
  const blocked = pull.state === "mergeable" && data.mergeBlockers.length > 0;
  const { icon: Icon, color, border } = blocked ? MERGE_BOX_BLOCKED : MERGE_BOX_STATES[pull.state];
  const message = {
    merged: t("repo.pulls.has_merged"),
    closed: t("repo.pulls.reopen_to_merge"),
    broken: t("repo.pulls.data_broken"),
    checking: t("repo.pulls.is_checking"),
    mergeable: t("repo.pulls.can_auto_merge_desc"),
    conflict: t("repo.pulls.cannot_auto_merge_desc"),
  }[pull.state];

  let body: ReactNode = null;
  if (pull.state === "merged" && (pull.viewerCanDeleteBranch || pull.viewerCanRestoreBranch)) {
    body = <BranchActions issueRef={issueRef} branch={pull.headBranch} deleted={pull.viewerCanRestoreBranch} />;
  } else if (pull.state === "mergeable" && pull.viewerCanMerge) {
    const styles: MergeStyle[] = [];
    if (pull.allowMerge) styles.push("create_merge_commit");
    if (pull.allowSquash) styles.push("squash_and_merge");
    if (pull.allowRebase) styles.push("rebase_before_merging");
    body =
      styles.length > 0 ? (
        <MergeForm
          issueRef={issueRef}
          viewerID={pull.viewerID ?? 0}
          styles={styles}
          defaultMessages={pull.mergeMessages ?? {}}
        />
      ) : null;
  }

  return (
    <section aria-label={t("repo.pulls.merge_pull_request")} className="flex gap-3">
      {/* Sized and placed like the avatars of the comments and the icons of the
          timeline events above it. */}
      <span className="flex size-10 shrink-0 items-center justify-center rounded-full bg-(--color-surface)">
        {/* The merge glyph has no enclosing circle and looks heavier than the
            circle icons at the same size. */}
        <Icon
          className={cn(
            pull.state === "merged" ? "size-5" : "size-6",
            color,
            pull.state === "checking" && "animate-spin",
          )}
          aria-hidden
        />
      </span>
      <div className={cn("min-w-0 flex-1 glass rounded-md border", border)}>
        {pull.state === "merged" || pull.state === "closed" ? null : <ReviewStatusRows data={data} />}
        <div className="px-4 py-3 text-sm">
          <p className="font-medium">{message}</p>
          {pull.state === "conflict" ? (
            <p className="mt-1 text-(--color-muted-foreground)">{t("repo.pulls.cannot_auto_merge_helper")}</p>
          ) : null}
          {pull.state === "mergeable" && pull.viewerNotInAllowlist ? (
            <p className="mt-1 text-(--color-muted-foreground)">{t("repo.pulls.merge_not_in_allowlist")}</p>
          ) : null}
          {blocked ? <p className="mt-1 text-(--color-muted-foreground)">{t("repo.pulls.review.blocked")}</p> : null}
        </div>
        {body}
      </div>
    </section>
  );
}

// BranchActions deletes the head branch of a merged pull request right away,
// and restores it once deleted, like GitHub.
function BranchActions({ issueRef, branch, deleted }: { issueRef: IssueRef; branch: string; deleted: boolean }) {
  const { t } = useTranslation();
  const router = useRouter();
  const mutation = useMutation({
    mutationFn: () => (deleted ? restoreRepoPullBranch(issueRef) : deleteRepoPullBranch(issueRef)),
    onSuccess: () => router.invalidate(),
    onError: (err) => toastError(err),
  });
  return (
    <div className="flex flex-wrap items-center gap-x-3 gap-y-2 border-t border-(--color-border) px-4 py-3 text-sm">
      <Button
        type="button"
        variant="outline"
        size="sm"
        disabled={mutation.isPending}
        onClick={() => mutation.mutate()}
        className={cn(!deleted && "text-(--color-destructive)")}
      >
        {deleted ? (
          <>
            <RotateCcw className="size-4" aria-hidden />
            {t("repo.pulls.restore_branch")}
          </>
        ) : (
          <>
            <Trash2 className="size-4" aria-hidden />
            {t("repo.pulls.delete_branch")}
          </>
        )}
      </Button>
      {deleted ? (
        <p className="min-w-0 text-(--color-muted-foreground)">
          <RichText text={t("repo.pulls.branch_deleted")} code={[branch]} />
        </p>
      ) : null}
    </div>
  );
}

// The drafts of merge messages are kept in this browser for a day, and the
// merge style each viewer last used in a repository until it changes.
const MERGE_DRAFT_TTL_MS = 24 * 60 * 60 * 1000;

interface MergeDraft {
  savedAt: number;
  messages: Partial<Record<MergeStyle, MergeMessage>>;
}

function mergeDraftKey(viewerID: number, issue: IssueRef) {
  return `gogs-merge-draft:${viewerID}:${issue.owner}/${issue.name}#${issue.index}`;
}

function mergeStyleKey(viewerID: number, issue: IssueRef) {
  return `gogs-merge-style:${viewerID}:${issue.owner}/${issue.name}`;
}

function readMergeDraft(key: string): MergeDraft | null {
  try {
    const draft = JSON.parse(window.localStorage.getItem(key) ?? "null") as MergeDraft | null;
    if (draft && Date.now() - draft.savedAt < MERGE_DRAFT_TTL_MS) return draft;
    window.localStorage.removeItem(key);
  } catch {
    // A draft that cannot be read is dropped.
  }
  return null;
}

// MergeForm merges the pull request in one of the styles the repository
// allows. Like GitHub, it starts as the merge button, which names the chosen
// style with a menu beside it to switch, and opens the commit message to edit
// before confirming. The style defaults to the one the viewer last used in the
// repository.
function MergeForm({
  issueRef,
  viewerID,
  styles,
  defaultMessages,
}: {
  issueRef: IssueRef;
  viewerID: number;
  styles: MergeStyle[];
  defaultMessages: Partial<Record<MergeStyle, MergeMessage>>;
}) {
  const { t } = useTranslation();
  const router = useRouter();
  const draftKey = mergeDraftKey(viewerID, issueRef);
  const styleKey = mergeStyleKey(viewerID, issueRef);
  const [style, setStyle] = useState<MergeStyle>(() => {
    const last = window.localStorage.getItem(styleKey) as MergeStyle | null;
    return last && styles.includes(last) ? last : styles[0];
  });
  const [menuOpen, setMenuOpen] = useState(false);
  const [editing, setEditing] = useState(false);
  // Each style keeps its own edits, so switching back to one finds them.
  const [messages, setMessages] = useState(() => ({
    ...defaultMessages,
    ...readMergeDraft(draftKey)?.messages,
  }));
  // Rebasing keeps the commits as they are, so there is no commit message.
  const message = messages[style];
  const setMessage = (change: Partial<MergeMessage>) => {
    const next = { ...messages, [style]: { title: "", body: "", ...message, ...change } };
    setMessages(next);
    window.localStorage.setItem(draftKey, JSON.stringify({ savedAt: Date.now(), messages: next } satisfies MergeDraft));
  };
  const mutation = useMutation({
    mutationFn: () => mergeRepoPull(issueRef, { style, title: message?.title ?? "", body: message?.body ?? "" }),
    onSuccess: () => {
      window.localStorage.removeItem(draftKey);
      window.localStorage.setItem(styleKey, style);
      return router.invalidate();
    },
  });
  const { formError } = formErrors(mutation.error);
  const onSubmit = (e: FormEvent) => {
    e.preventDefault();
    if (editing) mutation.mutate();
    else setEditing(true);
  };

  const buttonLabel = style === "create_merge_commit" ? t("repo.pulls.merge_pull_request") : t(`repo.pulls.${style}`);
  return (
    <form onSubmit={onSubmit} className="border-t border-(--color-border) px-4 py-3">
      <fieldset disabled={mutation.isPending} className="flex flex-col gap-3">
        {editing ? (
          <>
            {message ? (
              <>
                <div className="flex flex-col gap-1.5">
                  <label htmlFor="merge-title" className="text-sm font-medium">
                    {t("repo.pulls.commit_message")}
                  </label>
                  <Input
                    id="merge-title"
                    autoFocus
                    value={message.title}
                    onChange={(e) => setMessage({ title: e.target.value })}
                  />
                </div>
                <div className="flex flex-col gap-1.5">
                  <label htmlFor="merge-body" className="text-sm font-medium">
                    {t("repo.pulls.commit_description")}
                  </label>
                  <textarea
                    id="merge-body"
                    rows={6}
                    className={cn(TEXTAREA_CLASS, "font-mono")}
                    value={message.body}
                    onChange={(e) => setMessage({ body: e.target.value })}
                  />
                </div>
              </>
            ) : null}
            {formError ? <p className="text-sm text-(--color-destructive)">{formError}</p> : null}
            <div className="flex flex-wrap gap-2">
              <Button type="submit" size="sm" autoFocus={!message}>
                {t(`repo.pulls.confirm_${style}`)}
              </Button>
              <Button type="button" size="sm" variant="outline" onClick={() => setEditing(false)}>
                {t("cancel")}
              </Button>
            </div>
          </>
        ) : (
          <Popover open={menuOpen} onOpenChange={setMenuOpen}>
            {/* The menu opens under the whole button group, as on GitHub. */}
            <PopoverAnchor asChild>
              <div className="flex w-fit">
                <Button type="submit" size="sm" className={cn(styles.length > 1 && "rounded-r-none")}>
                  <GitMerge className="size-4" aria-hidden />
                  {buttonLabel}
                </Button>
                {styles.length > 1 ? (
                  <PopoverTrigger asChild>
                    <Button
                      type="button"
                      size="sm"
                      aria-label={t("repo.pulls.select_merge_method")}
                      className="rounded-l-none border-l border-(--color-primary-foreground)/30 px-2"
                    >
                      <ChevronDown className="size-4" aria-hidden />
                    </Button>
                  </PopoverTrigger>
                ) : null}
              </div>
            </PopoverAnchor>
            <PopoverContent align="start" className="w-80" role="menu" aria-label={t("repo.pulls.select_merge_method")}>
              {styles.map((value) => (
                <button
                  key={value}
                  type="button"
                  role="menuitemradio"
                  aria-checked={style === value}
                  onClick={() => {
                    setStyle(value);
                    setMenuOpen(false);
                  }}
                  className="flex w-full cursor-pointer items-start gap-2 rounded px-2 py-1.5 text-left hover:bg-(--color-surface)"
                >
                  <span className="grid size-5 shrink-0 place-items-center">
                    {style === value ? <Check className="size-4 text-(--color-primary)" aria-hidden /> : null}
                  </span>
                  <span className="flex flex-col gap-0.5">
                    <span className="text-sm font-medium">{t(`repo.pulls.${value}`)}</span>
                    <span className="text-xs text-(--color-muted-foreground)">{t(`repo.pulls.${value}_desc`)}</span>
                  </span>
                </button>
              ))}
            </PopoverContent>
          </Popover>
        )}
      </fieldset>
    </form>
  );
}

// usePullDiffReview shows the threads of the pull request below their lines
// in the diff, and lets signed-in viewers start threads and review it while
// it is open.
function usePullDiffReview(issueRef: IssueRef, data: RepoPullData): DiffReview {
  const user = useUserInfo();
  const open = !data.isClosed && data.pull.state !== "merged";
  const byID = new Map(data.threads.map((th) => [th.id, th]));
  return {
    threads: data.threads
      .filter((th) => th.thread && !th.thread.outdated)
      .map((th) => ({ id: th.id, path: th.thread!.path, side: th.thread!.side, line: th.thread!.line })),
    renderThread: (id) => {
      const thread = byID.get(id);
      return thread ? <ReviewThread issueRef={issueRef} data={data} thread={thread} inDiff /> : null;
    },
    renderComposer:
      user && open
        ? (target, onClose) => <CodeCommentForm issueRef={issueRef} data={data} target={target} onClose={onClose} />
        : undefined,
    actions: user && open ? <ReviewChanges issueRef={issueRef} data={data} /> : null,
  };
}
