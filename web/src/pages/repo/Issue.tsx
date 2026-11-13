import { useMutation, useSuspenseQuery } from "@tanstack/react-query";
import { Link, useRouter } from "@tanstack/react-router";
import { CircleCheck, CircleDot, Download, GitCommitHorizontal, GitMerge, Pencil, Plus, Trash2 } from "lucide-react";
import { type FormEvent, type ReactNode, useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";

import { ConfirmButton } from "@/components/ConfirmButton";
import { AttachFilesButton, IssueMeta, MarkdownEditor, PendingFiles } from "@/components/IssueForm";
import { OpenedBy } from "@/components/IssueListItem";
import { Markdown } from "@/components/Markdown";
import { RepoHeader } from "@/components/RepoHeader";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { usePageTitle } from "@/lib/page-title";
import {
  type IssueAttachment,
  type IssueComment,
  type IssueRef,
  type IssueUser,
  type RepoIssueData,
  createRepoIssueComment,
  deleteRepoIssueComment,
  repoHeaderQuery,
  setRepoIssueAssignee,
  setRepoIssueClosed,
  setRepoIssueLabels,
  setRepoIssueMilestone,
  updateRepoIssue,
  updateRepoIssueComment,
} from "@/lib/queries/repo";
import { formErrors } from "@/lib/queries/request";
import { formatAbsoluteTime, formatRelativeTime } from "@/lib/relative-time";
import { subUrl } from "@/lib/url";
import { useAttachmentUploads } from "@/lib/use-attachment-uploads";
import { useUserInfo } from "@/lib/use-user-info";
import { cn } from "@/lib/utils";

const LINK_CLASS = "hover:text-(--color-primary) hover:underline";

// useIssueChange makes a change to the issue and loads the page again.
function useIssueChange<T>(change: (input: T) => Promise<unknown>) {
  const router = useRouter();
  return useMutation({
    mutationFn: change,
    onSuccess: () => router.invalidate(),
    onError: (err) => toast.error(err.message),
  });
}

// RepoIssue shows an issue with its comments, and lets those who can change it
// do so.
export function RepoIssue({ owner, name, data }: { owner: string; name: string; data: RepoIssueData }) {
  const { t } = useTranslation();
  const { data: repo } = useSuspenseQuery(repoHeaderQuery(owner, name));
  usePageTitle(`${data.title} - ${t("issues")} - ${repo.owner}/${repo.name}`);
  const ref: IssueRef = { owner: repo.owner, name: repo.name, kind: "issues", index: data.index };
  const commentCount = data.comments.filter((c) => c.type === "comment").length;

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <RepoHeader repo={repo} activeTab="issues" />
      <main className="mx-auto w-full max-w-7xl flex-1 px-4 py-6 sm:px-6">
        <div className="border-b border-(--color-border) pb-4">
          <IssueHeader
            issueRef={ref}
            data={data}
            state={data.isClosed ? "closed" : "open"}
            summary={
              <>
                <OpenedBy issue={data} />
                {" · "}
                {t("repo.issues.num_comments").replace(/%(?:\[1\])?d/, String(commentCount))}
              </>
            }
          />
        </div>
        <IssueConversation issueRef={ref} data={data} className="mt-6" />
      </main>
    </div>
  );
}

const STATE_PILLS = {
  open: {
    icon: CircleDot,
    label: "repo.issues.open_title",
    className: "bg-(--color-success)/15 text-(--color-success)",
  },
  closed: {
    icon: CircleCheck,
    label: "repo.issues.closed_title",
    className: "bg-(--color-secondary) text-(--color-foreground)",
  },
  merged: { icon: GitMerge, label: "repo.pulls.merged", className: "bg-(--color-merged)/15 text-(--color-merged)" },
};

// IssueHeader shows the title of an issue or a pull request, its state, and a
// summary of it.
export function IssueHeader({
  issueRef,
  data,
  state,
  summary,
}: {
  issueRef: IssueRef;
  data: RepoIssueData;
  state: keyof typeof STATE_PILLS;
  summary: ReactNode;
}) {
  const { t } = useTranslation();
  const pill = STATE_PILLS[state];
  const Icon = pill.icon;
  return (
    <>
      <IssueTitle issueRef={issueRef} data={data} />
      <div className="mt-3 flex flex-wrap items-center gap-x-3 gap-y-2 text-sm text-(--color-muted-foreground)">
        <span className={cn("inline-flex items-center gap-1.5 rounded-full px-3 py-1 font-medium", pill.className)}>
          <Icon className="size-4" aria-hidden />
          {t(pill.label)}
        </span>
        <span className="min-w-0 wrap-anywhere">{summary}</span>
      </div>
    </>
  );
}

// IssueConversation shows the content and the comments of an issue or a pull
// request with the form to comment, and its sidebar. A pull request shows its
// merge box before the form, and cannot be closed or reopened once merged.
export function IssueConversation({
  issueRef,
  data,
  mergeBox,
  canChangeState = data.viewerCanEdit,
  className,
}: {
  issueRef: IssueRef;
  data: RepoIssueData;
  mergeBox?: ReactNode;
  canChangeState?: boolean;
  className?: string;
}) {
  // The browser looks for the anchor of a link, e.g., to a comment, before
  // the page renders, so scroll to it once it does.
  useEffect(() => {
    if (window.location.hash) document.getElementById(window.location.hash.slice(1))?.scrollIntoView();
  }, []);
  const ref = issueRef;

  return (
    <div className={cn("grid content-start gap-8 md:grid-cols-[minmax(0,1fr)_16rem]", className)}>
      <div className="flex min-w-0 flex-col gap-4">
        <CommentCard
          issueRef={ref}
          anchor={`issue-${data.index}`}
          poster={data.poster}
          createdAt={data.createdAt}
          html={data.contentHTML}
          attachments={data.attachments}
          edit={
            data.viewerCanEdit
              ? {
                  content: data.content,
                  save: (content) => updateRepoIssue(ref, { content }),
                }
              : undefined
          }
        />
        {data.comments.map((comment) =>
          comment.type === "comment" ? (
            <CommentCard
              key={comment.id}
              issueRef={ref}
              anchor={`issuecomment-${comment.id}`}
              poster={comment.poster}
              createdAt={comment.createdAt}
              html={comment.contentHTML ?? ""}
              tag={comment.tag}
              attachments={comment.attachments ?? []}
              edit={
                comment.viewerCanEdit
                  ? {
                      content: comment.content ?? "",
                      save: (content) => updateRepoIssueComment(ref, comment.id, content),
                      remove: () => deleteRepoIssueComment(ref, comment.id),
                    }
                  : undefined
              }
            />
          ) : (
            <TimelineEvent key={comment.id} comment={comment} />
          ),
        )}
        {mergeBox}
        <NewComment issueRef={ref} data={data} canChangeState={canChangeState} />
      </div>
      <IssueSidebar issueRef={ref} data={data} />
    </div>
  );
}

// IssueTitle shows the title of the issue, and lets those who can change it
// edit it in place.
function IssueTitle({ issueRef, data }: { issueRef: IssueRef; data: RepoIssueData }) {
  const { t } = useTranslation();
  const router = useRouter();
  const [editing, setEditing] = useState(false);
  const [title, setTitle] = useState(data.title);
  const mutation = useMutation({
    mutationFn: () => updateRepoIssue(issueRef, { title }),
    onSuccess: async () => {
      await router.invalidate();
      setEditing(false);
    },
  });
  const { fields, formError } = formErrors(mutation.error);
  const onSubmit = (e: FormEvent) => {
    e.preventDefault();
    mutation.mutate();
  };

  if (editing) {
    return (
      <form onSubmit={onSubmit} className="flex flex-col gap-2 sm:flex-row sm:items-start">
        <div className="flex flex-1 flex-col gap-1">
          <Input
            aria-label={t("repo.milestones.title")}
            value={title}
            maxLength={255}
            autoFocus
            disabled={mutation.isPending}
            aria-invalid={Boolean(fields.title) || undefined}
            aria-describedby={fields.title || formError ? "issue-title-error" : undefined}
            onChange={(e) => setTitle(e.target.value)}
          />
          {fields.title || formError ? (
            <p id="issue-title-error" className="text-sm text-(--color-destructive)">
              {fields.title ?? formError}
            </p>
          ) : null}
        </div>
        <div className="flex gap-2">
          <Button type="submit" disabled={mutation.isPending}>
            {t("repo.issues.save")}
          </Button>
          <Button
            type="button"
            variant="outline"
            disabled={mutation.isPending}
            onClick={() => {
              setEditing(false);
              setTitle(data.title);
              mutation.reset();
            }}
          >
            {t("repo.issues.cancel")}
          </Button>
        </div>
      </form>
    );
  }

  return (
    <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
      <h1 className="text-2xl font-semibold wrap-anywhere">
        {data.title} <span className="font-normal text-(--color-muted-foreground)">#{data.index}</span>
      </h1>
      <div className="flex shrink-0 gap-2">
        {data.viewerCanEdit ? (
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={() => {
              setTitle(data.title);
              setEditing(true);
            }}
          >
            {t("repo.issues.edit")}
          </Button>
        ) : null}
        {issueRef.kind === "issues" ? (
          <Button asChild size="sm">
            <Link to="/$owner/$repo/issues/new" params={{ owner: issueRef.owner, repo: issueRef.name }}>
              <Plus className="size-4" aria-hidden />
              {t("repo.issues.new")}
            </Link>
          </Button>
        ) : null}
      </div>
    </div>
  );
}

// UserLink links to the profile of a user, except for deleted users.
export function UserLink({ user, className, children }: { user: IssueUser; className?: string; children: ReactNode }) {
  if (user.isGhost) return <span className={className}>{children}</span>;
  return (
    <Link to="/$username" params={{ username: user.name }} className={className}>
      {children}
    </Link>
  );
}

// EventSentence renders a translated sentence about something that happened,
// e.g., "commented <a>3 hours ago</a>", making its link the time, which links
// to the anchor of the event.
function EventSentence({ text, anchor, time }: { text: string; anchor: string; time: string }) {
  const { t } = useTranslation();
  return (
    <>
      {text.split(/(<a\b[^>]*>.*?<\/a>)/).map((part, i) =>
        part.startsWith("<a") ? (
          <Tooltip key={i}>
            <TooltipTrigger asChild>
              <a href={`#${anchor}`} className={LINK_CLASS}>
                <time dateTime={time}>{formatRelativeTime(t, time)}</time>
              </a>
            </TooltipTrigger>
            <TooltipContent>{formatAbsoluteTime(time)}</TooltipContent>
          </Tooltip>
        ) : (
          part
        ),
      )}
    </>
  );
}

// CommentCard shows the content of the issue or one of its comments, and lets
// those who can change it edit or delete it.
function CommentCard({
  issueRef,
  anchor,
  poster,
  createdAt,
  html,
  tag,
  attachments,
  edit,
}: {
  issueRef: IssueRef;
  anchor: string;
  poster: IssueUser;
  createdAt: string;
  html: string;
  tag?: IssueComment["tag"];
  attachments: IssueAttachment[];
  // edit saves the content, and remove deletes the comment, for viewers who
  // can change it.
  edit?: { content: string; save: (content: string) => Promise<void>; remove?: () => Promise<void> };
}) {
  const { t } = useTranslation();
  const router = useRouter();
  const [editing, setEditing] = useState(false);
  const tagLabel = {
    owner: t("repo.issues.owner"),
    writer: t("repo.issues.collaborator"),
    poster: t("repo.issues.poster"),
  };

  return (
    <div id={anchor} className="flex scroll-mt-4 gap-3">
      <UserLink user={poster} className="hidden shrink-0 sm:block">
        <img src={poster.avatarURL} alt="" className="size-10 rounded-full border border-(--color-border)" />
      </UserLink>
      <div className="min-w-0 flex-1 rounded-md border border-(--color-border)">
        <div className="flex flex-wrap items-center justify-between gap-x-2 gap-y-1 rounded-t-md border-b border-(--color-border) bg-(--color-surface) py-1 pr-1 pl-3 text-sm">
          <p className="min-w-0 py-1 text-(--color-muted-foreground) wrap-anywhere">
            <UserLink user={poster} className="font-semibold text-(--color-foreground) hover:underline">
              {poster.name}
            </UserLink>{" "}
            <EventSentence text={t("repo.issues.commented_at")} anchor={anchor} time={createdAt} />
          </p>
          <div className="flex items-center gap-1">
            {tag ? (
              <span className="mr-1 rounded-full border border-(--color-border) px-2 text-xs/5 text-(--color-muted-foreground)">
                {tagLabel[tag]}
              </span>
            ) : null}
            {edit && !editing ? (
              <Tooltip>
                <TooltipTrigger asChild>
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon"
                    aria-label={t("repo.issues.edit")}
                    className="size-8 text-(--color-muted-foreground)"
                    onClick={() => setEditing(true)}
                  >
                    <Pencil className="size-4" aria-hidden />
                  </Button>
                </TooltipTrigger>
                <TooltipContent>{t("repo.issues.edit")}</TooltipContent>
              </Tooltip>
            ) : null}
            {edit?.remove ? (
              <ConfirmButton
                label={t("repo.issues.delete_comment")}
                title={t("repo.issues.delete_comment")}
                description={t("repo.issues.delete_comment_confirm")}
                icon={Trash2}
                action={edit.remove}
                onDone={() => router.invalidate()}
              />
            ) : null}
          </div>
        </div>
        {editing && edit ? (
          <ContentEditor
            issueRef={issueRef}
            initial={edit.content}
            save={edit.save}
            onClose={() => setEditing(false)}
          />
        ) : html ? (
          <div className="px-4 py-3">
            <Markdown html={html} />
          </div>
        ) : attachments.length === 0 ? (
          <p className="px-4 py-3 text-sm text-(--color-muted-foreground) italic">{t("repo.issues.no_content")}</p>
        ) : null}
        {attachments.length ? (
          <AttachmentList attachments={attachments} className={cn((html || editing) && "border-t")} />
        ) : null}
      </div>
    </div>
  );
}

// AttachmentList shows the files attached to a comment: images as thumbnails
// and other files as download links.
function AttachmentList({ attachments, className }: { attachments: IssueAttachment[]; className?: string }) {
  const { t } = useTranslation();
  return (
    <ul className={cn("flex flex-wrap gap-2 border-(--color-border) px-4 py-3", className)}>
      {attachments.map((a) => (
        <li key={a.url} className="min-w-0">
          {a.isImage ? (
            <a
              href={a.url}
              target="_blank"
              rel="noopener noreferrer"
              aria-label={t("repo.issues.attachment.open_tab").replace("%s", a.name)}
              className="block rounded-md border border-(--color-border) hover:border-(--color-primary)"
            >
              <img src={a.url} alt={a.name} className="h-24 max-w-full rounded-md object-cover" />
            </a>
          ) : (
            <a
              href={a.url}
              target="_blank"
              rel="noopener noreferrer"
              aria-label={t("repo.issues.attachment.download").replace("%s", a.name)}
              className="inline-flex max-w-full items-center gap-1.5 rounded-md border border-(--color-border) px-2 py-1 text-sm hover:bg-(--color-surface)"
            >
              <Download className="size-4 shrink-0" aria-hidden />
              <span className="truncate">{a.name}</span>
            </a>
          )}
        </li>
      ))}
    </ul>
  );
}

// ContentEditor edits the content of the issue or a comment in place.
function ContentEditor({
  issueRef,
  initial,
  save,
  onClose,
}: {
  issueRef: IssueRef;
  initial: string;
  save: (content: string) => Promise<void>;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const router = useRouter();
  const [content, setContent] = useState(initial);
  const mutation = useMutation({
    mutationFn: () => save(content),
    onSuccess: async () => {
      await router.invalidate();
      onClose();
    },
  });
  const { fields, formError } = formErrors(mutation.error);
  const onSubmit = (e: FormEvent) => {
    e.preventDefault();
    mutation.mutate();
  };

  return (
    <form onSubmit={onSubmit} className="p-3">
      <fieldset disabled={mutation.isPending} className="contents">
        <MarkdownEditor
          repo={issueRef}
          label={t("repo.issues.edit")}
          value={content}
          onChange={setContent}
          error={fields.content ?? formError}
          autoFocus
          disabled={mutation.isPending}
        />
        <div className="mt-3 flex justify-end gap-2">
          <Button type="button" variant="outline" size="sm" onClick={onClose}>
            {t("repo.issues.cancel")}
          </Button>
          <Button type="submit" size="sm">
            {t("repo.issues.save")}
          </Button>
        </div>
      </fieldset>
    </form>
  );
}

// TimelineEvent shows a change of state of the issue, or a commit that
// referenced it.
function TimelineEvent({ comment }: { comment: IssueComment }) {
  const { t } = useTranslation();
  const anchor = `event-${comment.id}`;
  const event = {
    reopen: { icon: CircleDot, text: t("repo.issues.reopened_at"), className: "text-(--color-success)" },
    close: { icon: CircleCheck, text: t("repo.issues.closed_at"), className: "text-(--color-destructive)" },
    commit_ref: {
      icon: GitCommitHorizontal,
      text: t("repo.issues.commit_ref_at"),
      className: "text-(--color-muted-foreground)",
    },
    comment: null,
  }[comment.type];
  if (!event) return null;
  const Icon = event.icon;

  return (
    <div id={anchor} className="flex scroll-mt-4 gap-3 text-sm sm:pl-3">
      <span className="flex size-8 shrink-0 items-center justify-center rounded-full bg-(--color-surface)">
        <Icon className={cn("size-4", event.className)} aria-hidden />
      </span>
      <div className="min-w-0 flex-1 py-1.5">
        <p className="text-(--color-muted-foreground) wrap-anywhere">
          <UserLink user={comment.poster} className="font-semibold text-(--color-foreground) hover:underline">
            <img
              src={comment.poster.avatarURL}
              alt=""
              className="mr-1.5 inline-block size-5 rounded-full border border-(--color-border) align-text-bottom"
            />
            {comment.poster.name}
          </UserLink>{" "}
          <EventSentence text={event.text} anchor={anchor} time={comment.createdAt} />
        </p>
        {comment.type === "commit_ref" && comment.contentHTML ? (
          <p
            className="mt-1 font-mono text-xs wrap-anywhere [&_a]:text-(--color-foreground) [&_a]:hover:text-(--color-primary) [&_a]:hover:underline"
            // The server sanitizes the link to the commit.
            dangerouslySetInnerHTML={{ __html: comment.contentHTML }}
          />
        ) : null}
      </div>
    </div>
  );
}

// NewComment is the form to comment on the issue, and to close or reopen it,
// or asks signed-out visitors to sign in.
function NewComment({
  issueRef,
  data,
  canChangeState,
}: {
  issueRef: IssueRef;
  data: RepoIssueData;
  canChangeState: boolean;
}) {
  const { t } = useTranslation();
  const router = useRouter();
  const user = useUserInfo();
  const [content, setContent] = useState("");
  const uploads = useAttachmentUploads(issueRef.owner, issueRef.name, data.attachment);
  // round starts the editor over after each comment, e.g., back on its write
  // tab.
  const [round, setRound] = useState(0);
  // newAnchor is the anchor of the new comment to show once it renders.
  const newAnchor = useRef<string | null>(null);
  useEffect(() => {
    const comment = newAnchor.current && document.getElementById(newAnchor.current);
    if (comment) {
      comment.scrollIntoView();
      newAnchor.current = null;
    }
  }, [data]);
  const { uploaded, uploading } = uploads;
  const hasComment = content.trim() !== "" || uploaded.length > 0;

  const mutation = useMutation({
    mutationFn: async (changeState: boolean) => {
      let id: number | undefined;
      if (hasComment) {
        ({ id } = await createRepoIssueComment(issueRef, { content, attachments: uploaded }));
      }
      if (changeState) await setRepoIssueClosed(issueRef, !data.isClosed);
      return id;
    },
    onSuccess: async (id) => {
      setContent("");
      uploads.clear();
      setRound((n) => n + 1);
      await router.invalidate();
      // Show the new comment, like the server-rendered page did.
      if (id) {
        newAnchor.current = `issuecomment-${id}`;
        await router.navigate({ to: ".", hash: newAnchor.current, params: true, search: true, resetScroll: false });
      }
    },
  });
  const { fields, formError } = formErrors(mutation.error);

  if (!user) {
    const signIn = subUrl(
      `/user/sign-in?redirect_to=${encodeURIComponent(window.location.pathname + window.location.search)}`,
    );
    return (
      <p className="mt-2 border-t border-(--color-border) pt-4 text-center text-sm text-(--color-muted-foreground)">
        {t("repo.issues.sign_in_require_desc")
          .split(/(<a\b[^>]*>.*?<\/a>)/)
          .map((part, i) =>
            part.startsWith("<a") ? (
              <a key={i} href={signIn} className="text-(--color-primary) underline underline-offset-2">
                {part.replace(/^<[^>]+>|<\/a>$/g, "")}
              </a>
            ) : (
              part
            ),
          )}
      </p>
    );
  }

  const onSubmit = (e: FormEvent) => {
    e.preventDefault();
    mutation.mutate(false);
  };
  const stateLabel = data.isClosed
    ? hasComment
      ? t("repo.issues.reopen_comment_issue")
      : t("repo.issues.reopen_issue")
    : hasComment
      ? t("repo.issues.close_comment_issue")
      : t("repo.issues.close_issue");

  return (
    <div className="mt-2 flex gap-3 border-t border-(--color-border) pt-6">
      <img
        src={user.avatarURL}
        alt=""
        className="hidden size-10 shrink-0 rounded-full border border-(--color-border) sm:block"
      />
      <form onSubmit={onSubmit} className="min-w-0 flex-1 rounded-md border border-(--color-border) p-3">
        <fieldset disabled={mutation.isPending} className="contents">
          <MarkdownEditor
            key={round}
            repo={issueRef}
            label={t("repo.issues.create_comment")}
            value={content}
            onChange={setContent}
            error={fields.content ?? formError}
            disabled={mutation.isPending}
            uploads={uploads}
          />
          <PendingFiles uploads={uploads} />
          <div className="mt-3 flex flex-wrap items-center justify-between gap-2">
            <AttachFilesButton uploads={uploads} />
            <div className="flex flex-wrap justify-end gap-2">
              {canChangeState ? (
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  disabled={uploading}
                  onClick={() => mutation.mutate(true)}
                >
                  {data.isClosed ? (
                    <CircleDot className="size-4 text-(--color-success)" aria-hidden />
                  ) : (
                    <CircleCheck className="size-4 text-(--color-destructive)" aria-hidden />
                  )}
                  {stateLabel}
                </Button>
              ) : null}
              <Button type="submit" size="sm" disabled={!hasComment || uploading}>
                {t("repo.issues.create_comment")}
              </Button>
            </div>
          </div>
        </fieldset>
      </form>
    </div>
  );
}

// IssueSidebar shows the labels, the milestone, the assignee and the
// participants of the issue, and lets writers change the first three.
function IssueSidebar({ issueRef, data }: { issueRef: IssueRef; data: RepoIssueData }) {
  const { t } = useTranslation();
  const labels = useIssueChange((ids: number[]) => setRepoIssueLabels(issueRef, ids));
  const milestone = useIssueChange((id: number) => setRepoIssueMilestone(issueRef, id));
  const assignee = useIssueChange((id: number) => setRepoIssueAssignee(issueRef, id));

  return (
    <IssueMeta
      repo={issueRef}
      value={data}
      options={data}
      pending={labels.isPending || milestone.isPending || assignee.isPending}
      onLabels={(ids) => labels.mutate(ids)}
      onMilestone={milestone.mutateAsync}
      onAssignee={assignee.mutateAsync}
    >
      <section>
        <h2 className="mb-2 text-sm font-semibold text-(--color-muted-foreground)">
          {t("repo.issues.num_participants").replace(/%(?:\[1\])?d/, String(data.participants.length))}
        </h2>
        <ul className="flex flex-wrap gap-1">
          {data.participants.map((u) => (
            <li key={u.name}>
              <Tooltip>
                <TooltipTrigger asChild>
                  <span className="inline-block">
                    <UserLink user={u} className="block">
                      <img
                        src={u.avatarURL}
                        alt={u.name}
                        className="size-7 rounded-full border border-(--color-border)"
                      />
                    </UserLink>
                  </span>
                </TooltipTrigger>
                <TooltipContent>{u.name}</TooltipContent>
              </Tooltip>
            </li>
          ))}
        </ul>
      </section>
    </IssueMeta>
  );
}
