import { useMutation } from "@tanstack/react-query";
import { Link, useRouter } from "@tanstack/react-router";
import type { TFunction } from "i18next";
import {
  Check,
  ChevronDown,
  CircleAlert,
  CircleCheck,
  Clock,
  type LucideIcon,
  MessageSquare,
  UserMinus,
  UserPlus,
} from "lucide-react";
import { type FormEvent, type ReactNode, useState } from "react";
import { Trans, useTranslation } from "react-i18next";
import { toast } from "sonner";

import type { DiffLineTarget } from "@/components/DiffWorkspace";
import { AttachFilesButton, MarkdownEditor, SidebarSection } from "@/components/IssueForm";
import { Button } from "@/components/ui/button";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import {
  type IssueComment,
  type IssueRef,
  type RepoPullData,
  type RepoPullReviewer,
  type ReviewState,
  createRepoPullCodeComment,
  discardRepoPullReview,
  setRepoPullReviewer,
  setRepoPullThreadResolved,
  submitRepoPullReview,
} from "@/lib/queries/repo";
import { formErrors } from "@/lib/queries/request";
import { linkedUploads, useAttachmentUploads } from "@/lib/use-attachment-uploads";
import { useIsLinked } from "@/lib/use-is-linked";
import { useUserInfo } from "@/lib/use-user-info";
import { cn } from "@/lib/utils";

import { Chip, CommentCard, EventSentence, ThreadReplies, UserLink } from "./Issue";

const REVIEW_EVENTS: Record<ReviewState, { icon: LucideIcon; text: string; className: string; label: string }> = {
  approve: {
    icon: CircleCheck,
    text: "repo.pulls.review.event_approve",
    className: "text-(--color-success)",
    label: "repo.pulls.review.approved",
  },
  request_changes: {
    icon: CircleAlert,
    text: "repo.pulls.review.event_request_changes",
    className: "text-(--color-destructive)",
    label: "repo.pulls.review.changes_requested",
  },
  comment: {
    icon: MessageSquare,
    text: "repo.pulls.review.event_comment",
    className: "text-(--color-muted-foreground)",
    label: "repo.pulls.review.commented",
  },
};

// PullTimelineItem shows the items of the timeline only a pull request has:
// reviews with their threads, and requests for reviews.
export function PullTimelineItem({
  issueRef,
  data,
  comment,
}: {
  issueRef: IssueRef;
  data: RepoPullData;
  comment: IssueComment;
}) {
  const { t } = useTranslation();
  if (comment.type === "review_request" || comment.type === "review_request_removed") {
    const removed = comment.type === "review_request_removed";
    const text = t(removed ? "repo.pulls.review.event_request_removed" : "repo.pulls.review.event_requested", {
      name: comment.target?.name ?? "",
    });
    return (
      <EventRow
        anchor={`event-${comment.id}`}
        icon={removed ? UserMinus : UserPlus}
        iconClassName="text-(--color-muted-foreground)"
        comment={comment}
        text={text}
      />
    );
  }
  if (comment.type !== "review" || !comment.review) return null;

  const event = REVIEW_EVENTS[comment.review.state];
  const threads = data.threads.filter((th) => th.thread?.reviewID === comment.review?.id);
  const anchor = `pullrequestreview-${comment.review.id}`;
  const Icon = event.icon;
  return (
    <div className="flex flex-col gap-3">
      {/* A review with a summary shows like a comment, with its outcome in the
          header. One without shows as a line of the timeline. */}
      {comment.contentHTML ? (
        <CommentCard
          issueRef={issueRef}
          anchor={anchor}
          poster={comment.poster}
          createdAt={comment.createdAt}
          html={comment.contentHTML}
          tag={comment.tag}
          attachments={comment.attachments ?? []}
          sentence={t(event.text)}
          badge={
            <span
              className={cn(
                "mr-1 inline-flex items-center gap-1 rounded-full border border-current px-2 text-xs/5",
                event.className,
              )}
            >
              <Icon className="size-3.5" aria-hidden />
              {t(event.label)}
            </span>
          }
        />
      ) : (
        <EventRow
          anchor={anchor}
          icon={event.icon}
          iconClassName={event.className}
          comment={comment}
          text={t(event.text)}
        />
      )}
      {threads.length ? (
        <div className="flex flex-col gap-3 sm:pl-13">
          {/* The padding is the avatar column of comments, size-10 and gap-3,
              so the threads line up with the comment cards around them. */}
          {threads.map((thread) => (
            <ReviewThread key={thread.id} issueRef={issueRef} data={data} thread={thread} />
          ))}
        </div>
      ) : null}
    </div>
  );
}

// EventRow is one line in the timeline about something a user did.
function EventRow({
  anchor,
  icon: Icon,
  iconClassName,
  comment,
  text,
}: {
  anchor: string;
  icon: LucideIcon;
  iconClassName: string;
  comment: IssueComment;
  text: string;
}) {
  const linked = useIsLinked(anchor);
  return (
    <div
      id={anchor}
      className={cn(
        "-my-1 -ml-1 flex scroll-mt-4 gap-3 rounded-md py-1 pr-2 pl-1 text-sm sm:ml-0 sm:pl-3",
        linked && "outline outline-(--color-primary) ring-2 ring-(--color-primary)/30",
      )}
    >
      <span className="flex size-8 shrink-0 items-center justify-center rounded-full bg-(--color-surface)">
        <Icon className={cn("size-4", iconClassName)} aria-hidden />
      </span>
      <p className="min-w-0 flex-1 py-1.5 text-(--color-muted-foreground) wrap-anywhere">
        <UserLink user={comment.poster} className="font-semibold text-(--color-foreground) hover:underline">
          <img
            src={comment.poster.avatarURL}
            alt=""
            className="mr-1.5 inline-block size-5 rounded-full border border-(--color-border) align-text-bottom"
          />
          {comment.poster.name}
        </UserLink>{" "}
        <EventSentence text={text} anchor={anchor} time={comment.createdAt} />
      </p>
    </div>
  );
}

function linesLabel(t: TFunction, thread: NonNullable<IssueComment["thread"]>) {
  return thread.startLine === thread.line && thread.startSide === thread.side
    ? t("repo.pulls.review.on_line", { line: thread.line })
    : t("repo.pulls.review.on_lines", { start: thread.startLine, end: thread.line });
}

// ReviewThread shows a thread of comments on lines of the diff: in the
// conversation with the path and the lines it started on, or below the lines
// in the diff. A resolved thread is collapsed until opened.
export function ReviewThread({
  issueRef,
  data,
  thread: root,
  inDiff = false,
}: {
  issueRef: IssueRef;
  data: RepoPullData;
  thread: IssueComment;
  inDiff?: boolean;
}) {
  const { t } = useTranslation();
  const router = useRouter();
  const thread = root.thread!;
  const anchor = `thread-${root.id}`;
  const linked = useIsLinked(anchor);
  // A resolved thread starts collapsed, unless it is linked, until the viewer
  // opens or closes it.
  const [toggled, setOpen] = useState<boolean>();
  const open = toggled ?? (!thread.resolvedBy || linked);
  const resolve = useMutation({
    mutationFn: (resolved: boolean) => setRepoPullThreadResolved(issueRef, root.id, resolved),
    onSuccess: async (_, resolved) => {
      await router.invalidate();
      setOpen(!resolved);
    },
    onError: (err) => toast.error(err.message),
  });
  const params = { owner: issueRef.owner, repo: issueRef.name, index: String(issueRef.index) };

  return (
    <div
      id={anchor}
      className={cn(
        "glass min-w-0 scroll-mt-4 rounded-md border bg-(--color-background)",
        // The diff clips what is drawn outside the thread.
        linked
          ? cn(
              "!border-(--color-primary)",
              inDiff ? "ring-1 ring-(--color-primary) ring-inset" : "ring-2 ring-(--color-primary)/30",
            )
          : "border-(--color-border)",
      )}
    >
      {inDiff ? null : (
        <div className="flex flex-wrap items-center justify-between gap-x-2 gap-y-1 rounded-t-[inherit] border-b border-(--color-border) bg-(--color-surface) px-3 py-1.5 text-sm">
          <span className="min-w-0 font-mono text-xs wrap-anywhere">{thread.path}</span>
          <span className="flex items-center gap-2">
            {thread.outdated ? <Chip>{t("repo.pulls.review.outdated")}</Chip> : null}
            {thread.outdated ? (
              <span className="text-(--color-muted-foreground)">{linesLabel(t, thread)}</span>
            ) : (
              <Link
                to="/$owner/$repo/pulls/$index/files"
                params={params}
                hash={anchor}
                className="text-(--color-muted-foreground) hover:text-(--color-primary) hover:underline"
              >
                {linesLabel(t, thread)}
              </Link>
            )}
          </span>
        </div>
      )}
      {thread.resolvedBy ? (
        <div className="flex flex-wrap items-center gap-x-2 gap-y-1 px-3 py-2 text-sm">
          <Check className="size-4 text-(--color-muted-foreground)" aria-hidden />
          <span className="min-w-0 flex-1 text-(--color-muted-foreground)">
            <Trans
              i18nKey="repo.pulls.review.resolved_by"
              values={{ name: thread.resolvedBy.name }}
              components={{ name: <span className="font-semibold text-(--color-foreground)" /> }}
            />
          </span>
          <Button
            type="button"
            variant="ghost"
            size="sm"
            aria-expanded={open}
            onClick={() => setOpen(!open)}
            className="text-(--color-muted-foreground)"
          >
            {open ? t("repo.pulls.review.hide_conversation") : t("repo.pulls.review.show_conversation")}
            <ChevronDown className={cn("size-4 transition-transform", open && "rotate-180")} aria-hidden />
          </Button>
        </div>
      ) : null}
      {open ? (
        <>
          {inDiff || !thread.diffHunk ? null : <DiffExcerpt thread={thread} />}
          <div className={cn("rounded-b-[inherit]", thread.resolvedBy && "border-t border-(--color-border)")}>
            <ThreadReplies
              issueRef={issueRef}
              root={root}
              attachment={data.attachment}
              pending={inDiff && !!data.viewerPendingReview}
              includeRoot
              actions={
                thread.viewerCanResolve ? (
                  <Button
                    type="button"
                    variant="outline"
                    size="sm"
                    disabled={resolve.isPending}
                    onClick={() => resolve.mutate(!thread.resolvedBy)}
                  >
                    <Check className="size-4" aria-hidden />
                    {thread.resolvedBy ? t("repo.pulls.review.unresolve") : t("repo.pulls.review.resolve")}
                  </Button>
                ) : null
              }
            />
          </div>
        </>
      ) : null}
    </div>
  );
}

// DiffExcerpt shows the last lines of the diff up to the lines a thread is on,
// as they were when it started.
function DiffExcerpt({ thread }: { thread: NonNullable<IssueComment["thread"]> }) {
  const lines = thread.diffHunk.split("\n");
  // The range is at the end of the excerpt.
  const span = thread.startSide === thread.side ? thread.line - thread.startLine + 1 : lines.length;
  return (
    <div className="overflow-x-auto border-b border-(--color-border) py-1 font-mono text-xs/5">
      {lines.map((line, i) => (
        <div
          key={i}
          className={cn(
            "px-3 whitespace-pre-wrap wrap-anywhere",
            line.startsWith("+") && "bg-(--color-diff-added)/10",
            line.startsWith("-") && "bg-(--color-diff-removed)/10",
            i >= lines.length - span && "shadow-[inset_3px_0_0_var(--color-primary)]",
          )}
        >
          {line || " "}
        </div>
      ))}
    </div>
  );
}

// CodeCommentForm starts a thread on lines of the diff, as a single comment
// or as part of the viewer's pending review.
export function CodeCommentForm({
  issueRef,
  data,
  target,
  onClose,
}: {
  issueRef: IssueRef;
  data: RepoPullData;
  target: DiffLineTarget;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const router = useRouter();
  const [content, setContent] = useState("");
  const uploads = useAttachmentUploads(issueRef.owner, issueRef.name, data.attachment);
  const inReview = !!data.viewerPendingReview;
  const mutation = useMutation({
    mutationFn: (pending: boolean) =>
      createRepoPullCodeComment(issueRef, {
        ...target,
        headSHA: data.pull.diff?.head ?? "",
        content,
        attachments: linkedUploads(uploads.uploaded, content),
        pending,
      }),
    onSuccess: async () => {
      await router.invalidate();
      onClose();
    },
  });
  const { fields, formError } = formErrors(mutation.error);
  const sign = (side: "old" | "new") => (side === "old" ? "-" : "+");
  const title =
    target.startLine === target.line && target.startSide === target.side
      ? t("repo.pulls.review.comment_on_line", { line: `${sign(target.side)}${target.line}` })
      : t("repo.pulls.review.comment_on_lines", {
          start: `${sign(target.startSide)}${target.startLine}`,
          end: `${sign(target.side)}${target.line}`,
        });
  const onSubmit = (e: FormEvent) => {
    e.preventDefault();
    mutation.mutate(true);
  };
  const disabled = !content.trim() || uploads.uploading;

  return (
    <form onSubmit={onSubmit} className="glass rounded-md border border-(--color-border) bg-(--color-background)">
      <p className="rounded-t-[inherit] border-b border-(--color-border) bg-(--color-surface) px-3 py-1.5 text-sm text-(--color-muted-foreground)">
        {title}
      </p>
      <fieldset disabled={mutation.isPending} className="block p-3">
        <MarkdownEditor
          repo={issueRef}
          label={title}
          value={content}
          onChange={setContent}
          error={fields.content ?? formError}
          autoFocus
          disabled={mutation.isPending}
          uploads={uploads}
          linkFiles
        />
        <div className="mt-3 flex flex-wrap items-center justify-between gap-2">
          <AttachFilesButton uploads={uploads} />
          <div className="flex flex-wrap justify-end gap-2">
            <Button type="button" variant="outline" size="sm" onClick={onClose}>
              {t("repo.issues.cancel")}
            </Button>
            {inReview ? null : (
              <Button
                type="button"
                variant="outline"
                size="sm"
                disabled={disabled}
                onClick={() => mutation.mutate(false)}
              >
                {t("repo.pulls.review.add_single")}
              </Button>
            )}
            <Button type="submit" size="sm" disabled={disabled}>
              {inReview ? t("repo.pulls.review.add_to_review") : t("repo.pulls.review.start")}
            </Button>
          </div>
        </div>
      </fieldset>
    </form>
  );
}

// ReviewChanges is the button in the toolbar of the files that opens the form
// to submit the viewer's review.
export function ReviewChanges({ issueRef, data }: { issueRef: IssueRef; data: RepoPullData }) {
  const { t } = useTranslation();
  const router = useRouter();
  const user = useUserInfo();
  const [open, setOpen] = useState(false);
  const [state, setState] = useState<ReviewState>("comment");
  const [content, setContent] = useState("");
  const isPoster = user?.username === data.poster.name;
  const pendingCount = data.viewerPendingReview?.commentCount ?? 0;
  const submit = useMutation({
    mutationFn: () => submitRepoPullReview(issueRef, { state, content, headSHA: data.pull.diff?.head ?? "" }),
    onSuccess: async () => {
      await router.invalidate();
      setContent("");
      setState("comment");
      setOpen(false);
      toast.success(t("repo.pulls.review.submitted"));
    },
  });
  const discard = useMutation({
    mutationFn: () => discardRepoPullReview(issueRef),
    onSuccess: async () => {
      await router.invalidate();
      setOpen(false);
    },
    onError: (err) => toast.error(err.message),
  });
  const { fields, formError } = formErrors(submit.error);
  const onSubmit = (e: FormEvent) => {
    e.preventDefault();
    submit.mutate();
  };
  const busy = submit.isPending || discard.isPending;

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button type="button" size="sm" variant={pendingCount ? "default" : "outline"} className="h-7">
          {pendingCount ? t("repo.pulls.review.finish") : t("repo.pulls.review.review_changes")}
          {pendingCount ? (
            <span className="rounded-full bg-(--color-primary-foreground) px-1.5 text-xs/4.5 text-(--color-primary) tabular-nums">
              {pendingCount}
            </span>
          ) : null}
          <ChevronDown className="size-4" aria-hidden />
        </Button>
      </PopoverTrigger>
      <PopoverContent align="end" collisionPadding={16} className="w-104 max-w-[calc(100vw-2rem)] p-3">
        <form onSubmit={onSubmit}>
          <fieldset disabled={busy} className="flex flex-col gap-3">
            <p className="text-sm font-semibold">{t("repo.pulls.review.finish")}</p>
            <MarkdownEditor
              repo={issueRef}
              label={t("repo.pulls.review.summary")}
              value={content}
              onChange={setContent}
              error={fields.content ?? formError}
              disabled={busy}
              minLines={3}
            />
            <div role="radiogroup" aria-label={t("repo.pulls.review.finish")} className="flex flex-col gap-2">
              {(["comment", "approve", "request_changes"] as const).map((value) => {
                const disabled = isPoster && value !== "comment";
                return (
                  <label
                    key={value}
                    className={cn(
                      "flex items-start gap-2 text-sm",
                      disabled ? "cursor-not-allowed opacity-60" : "cursor-pointer",
                    )}
                  >
                    <input
                      type="radio"
                      name="review-state"
                      value={value}
                      checked={state === value}
                      disabled={disabled}
                      onChange={() => setState(value)}
                      className="mt-0.5 size-4 shrink-0 cursor-pointer accent-(--color-primary) disabled:cursor-not-allowed"
                    />
                    <span>
                      <span className="font-medium">{t(`repo.pulls.review.${value}`)}</span>
                      <span className="block text-(--color-muted-foreground)">
                        {t(`repo.pulls.review.${value}_desc`)}
                      </span>
                    </span>
                  </label>
                );
              })}
            </div>
            {isPoster ? (
              <p className="text-sm text-(--color-muted-foreground)">{t("repo.pulls.review.own_pull")}</p>
            ) : null}
            <div className="flex flex-wrap items-center justify-end gap-2">
              {pendingCount ? (
                <Button type="button" variant="ghost" size="sm" onClick={() => discard.mutate()}>
                  {t("repo.pulls.review.discard")}
                </Button>
              ) : null}
              <Button type="submit" size="sm" disabled={state === "comment" && !pendingCount && !content.trim()}>
                {submit.isPending ? t("repo.pulls.review.submitting") : t("repo.pulls.review.submit")}
              </Button>
            </div>
          </fieldset>
        </form>
      </PopoverContent>
    </Popover>
  );
}

const REVIEWER_STATES: Record<RepoPullReviewer["state"], { icon: LucideIcon; className: string; label: string }> = {
  requested: { icon: Clock, className: "text-(--color-muted-foreground)", label: "repo.pulls.review.awaiting" },
  approved: { icon: CircleCheck, className: "text-(--color-success)", label: "repo.pulls.review.approved" },
  changes_requested: {
    icon: CircleAlert,
    className: "text-(--color-destructive)",
    label: "repo.pulls.review.changes_requested",
  },
  commented: {
    icon: MessageSquare,
    className: "text-(--color-muted-foreground)",
    label: "repo.pulls.review.commented",
  },
};

// ReviewersSection shows who reviewed the pull request or was asked to, and
// lets writers and whoever opened it ask others.
export function ReviewersSection({ issueRef, data }: { issueRef: IssueRef; data: RepoPullData }) {
  const { t } = useTranslation();
  const router = useRouter();
  const request = useMutation({
    mutationFn: ({ name, requested }: { name: string; requested: boolean }) =>
      setRepoPullReviewer(issueRef, name, requested),
    onSuccess: () => router.invalidate(),
    onError: (err) => toast.error(err.message),
  });
  const requested = new Set(data.reviewers.filter((r) => r.state === "requested").map((r) => r.user.name));
  const options = data.reviewerOptions;

  return (
    <SidebarSection
      title={t("repo.pulls.review.reviewers")}
      editor={
        options
          ? () => (
              <ul className="max-h-72 overflow-y-auto p-1">
                {options.map((u) => {
                  const on = requested.has(u.name);
                  return (
                    <li key={u.name}>
                      <button
                        type="button"
                        role="menuitemcheckbox"
                        aria-checked={on}
                        disabled={request.isPending}
                        onClick={() => request.mutate({ name: u.name, requested: !on })}
                        className="flex w-full cursor-pointer items-center gap-2 rounded px-2 py-1.5 text-left text-sm hover:bg-(--color-surface) disabled:cursor-wait"
                      >
                        <Check className={cn("size-4 shrink-0", !on && "invisible")} aria-hidden />
                        <img src={u.avatarURL} alt="" className="size-5 rounded-full" />
                        <span className="min-w-0 truncate">{u.name}</span>
                      </button>
                    </li>
                  );
                })}
              </ul>
            )
          : undefined
      }
    >
      {data.reviewers.length ? (
        <ul className="flex flex-col gap-2">
          {data.reviewers.map((r) => {
            const state = REVIEWER_STATES[r.state];
            const Icon = state.icon;
            const note =
              !r.counts && r.state !== "requested" && r.state !== "commented"
                ? t("repo.pulls.review.not_counted")
                : r.stale
                  ? t("repo.pulls.review.stale")
                  : null;
            return (
              <li key={r.user.name} className="flex items-start gap-2 text-sm">
                <UserLink user={r.user} className="shrink-0">
                  <img src={r.user.avatarURL} alt="" className="size-5 rounded-full border border-(--color-border)" />
                </UserLink>
                <span className="min-w-0 flex-1">
                  <UserLink user={r.user} className="font-medium wrap-anywhere hover:underline">
                    {r.user.name}
                  </UserLink>
                  {note ? <span className="block text-xs text-(--color-muted-foreground)">{note}</span> : null}
                </span>
                <span className={cn("inline-flex shrink-0 items-center gap-1 text-xs", state.className)}>
                  <Icon className="size-3.5" aria-hidden />
                  {t(state.label)}
                </span>
              </li>
            );
          })}
        </ul>
      ) : (
        <p className="text-sm text-(--color-muted-foreground)">{t("repo.pulls.review.no_reviews")}</p>
      )}
    </SidebarSection>
  );
}

// ReviewStatusRows say what the reviews decide in the merge box, and why
// they block the merge.
export function ReviewStatusRows({ data }: { data: RepoPullData }): ReactNode {
  const { t } = useTranslation();
  const rows: { icon: LucideIcon; className: string; title: string; desc?: string }[] = [];
  const approvals = data.reviewers.filter((r) => r.counts && r.state === "approved" && !r.stale).length;
  if (data.reviewDecision === "changes_requested") {
    rows.push({
      icon: CircleAlert,
      className: "text-(--color-destructive)",
      title: t("repo.pulls.review.decision_changes_requested"),
      desc: t("repo.pulls.review.blocker_changes_requested"),
    });
  } else if (data.reviewDecision === "review_required") {
    rows.push({
      icon: Clock,
      className: "text-(--color-muted-foreground)",
      title: t("repo.pulls.review.decision_review_required"),
      desc: t("repo.pulls.review.blocker_approvals", { approvals, required: data.requiredApprovals ?? 0 }),
    });
  } else if (data.reviewDecision === "approved") {
    rows.push({
      icon: CircleCheck,
      className: "text-(--color-success)",
      title: t("repo.pulls.review.decision_approved"),
    });
  }
  if (data.mergeBlockers.includes("unresolved_conversations")) {
    rows.push({
      icon: MessageSquare,
      className: "text-(--color-muted-foreground)",
      title: t("repo.pulls.review.blocker_unresolved"),
    });
  }
  if (!rows.length) return null;
  return rows.map(({ icon: Icon, className, title, desc }) => (
    <div key={title} className="flex items-start gap-2 border-b border-(--color-border) px-4 py-3 text-sm">
      <Icon className={cn("mt-0.5 size-4 shrink-0", className)} aria-hidden />
      <div className="min-w-0">
        <p className="font-medium">{title}</p>
        {desc ? <p className="text-(--color-muted-foreground)">{desc}</p> : null}
      </div>
    </div>
  ));
}
