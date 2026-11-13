import { useMutation, useQuery, useSuspenseQuery } from "@tanstack/react-query";
import { Link, useRouter } from "@tanstack/react-router";
import {
  Check,
  CircleCheck,
  CircleDot,
  Download,
  GitCommitHorizontal,
  Paperclip,
  Pencil,
  Plus,
  Settings,
  Trash2,
  X,
} from "lucide-react";
import { type FormEvent, type ReactNode, useEffect, useId, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";

import { CodeEditor } from "@/components/CodeEditor";
import { ConfirmButton } from "@/components/ConfirmButton";
import { OpenedBy } from "@/components/IssueListItem";
import { Markdown } from "@/components/Markdown";
import { RepoHeader } from "@/components/RepoHeader";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { usePageTitle } from "@/lib/page-title";
import {
  type IssueAttachment,
  type IssueComment,
  type IssueUser,
  type RepoIssueData,
  createRepoIssueComment,
  deleteRepoIssueComment,
  renderRepoMarkdown,
  repoHeaderQuery,
  setRepoIssueAssignee,
  setRepoIssueClosed,
  setRepoIssueLabels,
  setRepoIssueMilestone,
  updateRepoIssue,
  updateRepoIssueComment,
  uploadRepoIssueAttachment,
} from "@/lib/queries/repo";
import { formErrors } from "@/lib/queries/request";
import { formatAbsoluteTime, formatRelativeTime } from "@/lib/relative-time";
import { subUrl } from "@/lib/url";
import { useUserInfo } from "@/lib/use-user-info";
import { cn } from "@/lib/utils";

const LINK_CLASS = "hover:text-(--color-primary) hover:underline";

interface IssueRef {
  owner: string;
  name: string;
  index: number;
}

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
  const ref = { owner: repo.owner, name: repo.name, index: data.index };
  const commentCount = data.comments.filter((c) => c.type === "comment").length;
  // The browser looks for the anchor of a link, e.g., to a comment, before
  // the page renders, so scroll to it once it does.
  useEffect(() => {
    if (window.location.hash) document.getElementById(window.location.hash.slice(1))?.scrollIntoView();
  }, []);

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <RepoHeader repo={repo} activeTab="issues" />
      <main className="mx-auto w-full max-w-7xl flex-1 px-4 py-6 sm:px-6">
        <IssueTitle issueRef={ref} data={data} />
        <div className="mt-3 flex flex-wrap items-center gap-x-3 gap-y-2 border-b border-(--color-border) pb-4 text-sm text-(--color-muted-foreground)">
          <span
            className={cn(
              "inline-flex items-center gap-1.5 rounded-full px-3 py-1 font-medium",
              data.isClosed
                ? "bg-(--color-secondary) text-(--color-foreground)"
                : "bg-(--color-success)/15 text-(--color-success)",
            )}
          >
            {data.isClosed ? (
              <CircleCheck className="size-4" aria-hidden />
            ) : (
              <CircleDot className="size-4" aria-hidden />
            )}
            {data.isClosed ? t("repo.issues.closed_title") : t("repo.issues.open_title")}
          </span>
          <span>
            <OpenedBy issue={data} />
            {" · "}
            {t("repo.issues.num_comments").replace(/%(?:\[1\])?d/, String(commentCount))}
          </span>
        </div>

        <div className="mt-6 grid content-start gap-8 md:grid-cols-[minmax(0,1fr)_16rem]">
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
                      save: (content) => updateRepoIssue(ref.owner, ref.name, ref.index, { content }),
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
                          save: (content) =>
                            updateRepoIssueComment(ref.owner, ref.name, ref.index, comment.id, content),
                          remove: () => deleteRepoIssueComment(ref.owner, ref.name, ref.index, comment.id),
                        }
                      : undefined
                  }
                />
              ) : (
                <TimelineEvent key={comment.id} comment={comment} />
              ),
            )}
            <NewComment issueRef={ref} data={data} />
          </div>
          <IssueSidebar issueRef={ref} data={data} />
        </div>
      </main>
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
    mutationFn: () => updateRepoIssue(issueRef.owner, issueRef.name, issueRef.index, { title }),
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
        {/* Opening an issue is still served by the server-rendered pages. */}
        <Button asChild size="sm">
          <a href={subUrl(`/${issueRef.owner}/${issueRef.name}/issues/new`)}>
            <Plus className="size-4" aria-hidden />
            {t("repo.issues.new")}
          </a>
        </Button>
      </div>
    </div>
  );
}

// UserLink links to the profile of a user, except for deleted users.
function UserLink({ user, className, children }: { user: IssueUser; className?: string; children: ReactNode }) {
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
          issueRef={issueRef}
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

// MarkdownEditor is a text area for Markdown with a tab that previews it the
// way the server renders it.
function MarkdownEditor({
  issueRef,
  label,
  value,
  onChange,
  error,
  autoFocus,
  disabled,
  onFiles,
}: {
  issueRef: IssueRef;
  label: string;
  value: string;
  onChange: (value: string) => void;
  error?: string | null;
  autoFocus?: boolean;
  disabled?: boolean;
  // onFiles attaches the files pasted or dropped into the editor.
  onFiles?: (files: FileList) => void;
}) {
  const { t } = useTranslation();
  const id = useId();
  const [tab, setTab] = useState<"write" | "preview">("write");
  const preview = useQuery({
    queryKey: ["markdown", issueRef.owner, issueRef.name, value],
    queryFn: () => renderRepoMarkdown(issueRef.owner, issueRef.name, value),
    enabled: tab === "preview" && value.trim() !== "",
    staleTime: Infinity,
  });
  const tabs = [
    { key: "write", label: t("repo.release.write") },
    { key: "preview", label: t("repo.release.preview") },
  ] as const;

  return (
    <div>
      <div role="tablist" aria-label={label} className="mb-2 flex gap-1">
        {tabs.map((item) => (
          <button
            key={item.key}
            type="button"
            role="tab"
            id={`${id}-${item.key}-tab`}
            aria-selected={tab === item.key}
            aria-controls={`${id}-${item.key}`}
            onClick={() => setTab(item.key)}
            className={cn(
              "cursor-pointer rounded-md px-3 py-1.5 text-sm",
              tab === item.key
                ? "bg-(--color-surface) font-semibold text-(--color-foreground)"
                : "text-(--color-muted-foreground) hover:text-(--color-foreground)",
            )}
          >
            {item.label}
          </button>
        ))}
      </div>
      {tab === "write" ? (
        <div role="tabpanel" id={`${id}-write`} aria-labelledby={`${id}-write-tab`}>
          <CodeEditor
            id={`${id}-input`}
            label={label}
            lang="markdown"
            value={value}
            onChange={onChange}
            autoFocus={autoFocus}
            disabled={disabled}
            onFiles={onFiles}
            aria-invalid={Boolean(error) || undefined}
            aria-describedby={error ? `${id}-error` : undefined}
          />
          {error ? (
            <p id={`${id}-error`} className="mt-1.5 text-sm text-(--color-destructive)">
              {error}
            </p>
          ) : null}
        </div>
      ) : (
        <div
          role="tabpanel"
          id={`${id}-preview`}
          aria-labelledby={`${id}-preview-tab`}
          aria-busy={preview.isFetching}
          className="min-h-32 rounded-md border border-(--color-border) px-4 py-3"
        >
          {value.trim() === "" ? (
            <p className="text-sm text-(--color-muted-foreground) italic">{t("repo.issues.no_content")}</p>
          ) : preview.data !== undefined ? (
            <Markdown html={preview.data} />
          ) : preview.isError ? (
            <p className="text-sm text-(--color-destructive)">{preview.error.message}</p>
          ) : (
            <p className="text-sm text-(--color-muted-foreground)">{t("repo.release.loading")}</p>
          )}
        </div>
      )}
    </div>
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

interface PendingFile {
  key: string;
  name: string;
  // uuid is set once the file is uploaded.
  uuid?: string;
}

// NewComment is the form to comment on the issue, and to close or reopen it,
// or asks signed-out visitors to sign in.
function NewComment({ issueRef, data }: { issueRef: IssueRef; data: RepoIssueData }) {
  const { t } = useTranslation();
  const router = useRouter();
  const user = useUserInfo();
  const [content, setContent] = useState("");
  const [files, setFiles] = useState<PendingFile[]>([]);
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
  const fileInput = useRef<HTMLInputElement>(null);
  const uploaded = files.flatMap((f) => (f.uuid ? [f.uuid] : []));
  const uploading = files.length !== uploaded.length;
  const hasComment = content.trim() !== "" || uploaded.length > 0;

  const mutation = useMutation({
    mutationFn: async (changeState: boolean) => {
      let id: number | undefined;
      if (hasComment) {
        ({ id } = await createRepoIssueComment(issueRef.owner, issueRef.name, issueRef.index, {
          content,
          attachments: uploaded,
        }));
      }
      if (changeState) await setRepoIssueClosed(issueRef.owner, issueRef.name, issueRef.index, !data.isClosed);
      return id;
    },
    onSuccess: async (id) => {
      setContent("");
      setFiles([]);
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

  const attachment = data.attachment;
  const addFiles = (list: FileList) => {
    if (!attachment) return;
    const room = attachment.maxFiles - files.length;
    for (const file of Array.from(list).slice(0, Math.max(room, 0))) {
      if (file.size > attachment.maxSize) {
        toast.error(
          t("dropzone.file_too_big", {
            filesize: (file.size / (1 << 20)).toFixed(1),
            maxFilesize: attachment.maxSize / (1 << 20),
          }),
        );
        continue;
      }
      const key = crypto.randomUUID();
      setFiles((prev) => [...prev, { key, name: file.name }]);
      uploadRepoIssueAttachment(issueRef.owner, issueRef.name, file).then(
        (uuid) => setFiles((prev) => prev.map((f) => (f.key === key ? { ...f, uuid } : f))),
        (err: Error) => {
          toast.error(`${file.name}: ${err.message}`);
          setFiles((prev) => prev.filter((f) => f.key !== key));
        },
      );
    }
  };
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
            issueRef={issueRef}
            label={t("repo.issues.create_comment")}
            value={content}
            onChange={setContent}
            error={fields.content ?? formError}
            disabled={mutation.isPending}
            onFiles={attachment ? addFiles : undefined}
          />
          {files.length ? (
            <ul className="mt-2 flex flex-col gap-1">
              {files.map((f) => (
                <li key={f.key} className="flex items-center gap-2 text-sm">
                  <Paperclip className="size-4 shrink-0 text-(--color-muted-foreground)" aria-hidden />
                  <span className={cn("min-w-0 flex-1 truncate", !f.uuid && "text-(--color-muted-foreground)")}>
                    {f.name}
                  </span>
                  {f.uuid ? (
                    <Button
                      type="button"
                      variant="ghost"
                      size="icon"
                      aria-label={`${t("dropzone.remove_file")}: ${f.name}`}
                      className="size-7 text-(--color-muted-foreground)"
                      onClick={() => setFiles((prev) => prev.filter((p) => p.key !== f.key))}
                    >
                      <X className="size-4" aria-hidden />
                    </Button>
                  ) : (
                    <span className="text-xs text-(--color-muted-foreground)">{t("repo.release.loading")}</span>
                  )}
                </li>
              ))}
            </ul>
          ) : null}
          <div className="mt-3 flex flex-wrap items-center justify-between gap-2">
            {attachment ? (
              <>
                <input
                  ref={fileInput}
                  type="file"
                  multiple
                  hidden
                  accept={attachment.allowedTypes.includes("*/*") ? undefined : attachment.allowedTypes.join(",")}
                  onChange={(e) => {
                    if (e.target.files) addFiles(e.target.files);
                    e.target.value = "";
                  }}
                />
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  className="text-(--color-muted-foreground)"
                  disabled={files.length >= attachment.maxFiles}
                  onClick={() => fileInput.current?.click()}
                >
                  <Paperclip className="size-4" aria-hidden />
                  {t("repo.issues.attach_files")}
                </Button>
              </>
            ) : (
              <span />
            )}
            <div className="flex flex-wrap justify-end gap-2">
              {data.viewerCanEdit ? (
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

// SidebarSection is a part of the sidebar with a heading, which writers can
// change through the popover of its button.
function SidebarSection({
  title,
  editor,
  children,
}: {
  title: string;
  editor?: (close: () => void) => ReactNode;
  children: ReactNode;
}) {
  const [open, setOpen] = useState(false);
  return (
    <section className="border-b border-(--color-border) pb-4">
      <div className="mb-2 flex min-h-8 items-center justify-between gap-2">
        <h2 className="text-sm font-semibold text-(--color-muted-foreground)">{title}</h2>
        {editor ? (
          <Popover open={open} onOpenChange={setOpen}>
            <Tooltip>
              <TooltipTrigger asChild>
                <PopoverTrigger asChild>
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon"
                    aria-label={title}
                    className="size-8 text-(--color-muted-foreground)"
                  >
                    <Settings className="size-4" aria-hidden />
                  </Button>
                </PopoverTrigger>
              </TooltipTrigger>
              <TooltipContent>{title}</TooltipContent>
            </Tooltip>
            <PopoverContent align="end" collisionPadding={16} className="w-64 max-w-[calc(100vw-2rem)] p-1">
              {editor(() => setOpen(false))}
            </PopoverContent>
          </Popover>
        ) : null}
      </div>
      {children}
    </section>
  );
}

const OPTION_CLASS =
  "flex w-full cursor-pointer items-center gap-2 rounded-sm px-2 py-1.5 text-left text-sm hover:bg-(--color-surface) disabled:cursor-not-allowed disabled:opacity-60";

// OptionList lists the choices in the popover of a sidebar section: a button
// that clears the choice, then the options, marking the current one.
function OptionList<T extends { id: number }>({
  clearLabel,
  groups,
  current,
  disabled,
  render,
  onPick,
}: {
  clearLabel: string;
  groups: { label?: string; items: T[] }[];
  current?: number;
  disabled: boolean;
  render: (item: T) => ReactNode;
  onPick: (id: number) => void;
}) {
  return (
    <div className="max-h-80 overflow-y-auto">
      <button type="button" className={OPTION_CLASS} disabled={disabled || !current} onClick={() => onPick(0)}>
        <X className="size-4 shrink-0" aria-hidden />
        {clearLabel}
      </button>
      {groups.map((group, i) =>
        group.items.length ? (
          <div key={i} role="group" aria-label={group.label}>
            {group.label ? (
              <p className="px-2 pt-2 pb-1 text-xs font-semibold text-(--color-muted-foreground)">{group.label}</p>
            ) : null}
            {group.items.map((item) => (
              <button
                key={item.id}
                type="button"
                aria-pressed={item.id === current}
                className={OPTION_CLASS}
                disabled={disabled}
                onClick={() => onPick(item.id)}
              >
                <Check className={cn("size-4 shrink-0", item.id !== current && "invisible")} aria-hidden />
                {render(item)}
              </button>
            ))}
          </div>
        ) : null,
      )}
    </div>
  );
}

// IssueSidebar shows the labels, the milestone, the assignee and the
// participants of the issue, and lets writers change the first three.
function IssueSidebar({ issueRef, data }: { issueRef: IssueRef; data: RepoIssueData }) {
  const { t } = useTranslation();
  const { owner, name, index } = issueRef;
  const params = { owner, repo: name };
  const labels = useIssueChange((ids: number[]) => setRepoIssueLabels(owner, name, index, ids));
  const milestone = useIssueChange((id: number) => setRepoIssueMilestone(owner, name, index, id));
  const assignee = useIssueChange((id: number) => setRepoIssueAssignee(owner, name, index, id));
  const checked = new Set(data.labels.map((l) => l.id));
  const toggleLabel = (id: number) =>
    labels.mutate(checked.has(id) ? [...checked].filter((l) => l !== id) : [...checked, id]);
  const labelChip = (label: RepoIssueData["labels"][number]) => (
    <span
      className="inline-block max-w-full truncate rounded-full px-2 text-xs/5 font-medium"
      style={{ backgroundColor: label.color, color: label.textColor }}
    >
      {label.name}
    </span>
  );
  const emptyText = "text-sm text-(--color-muted-foreground)";

  return (
    <aside className="flex min-w-0 flex-col gap-4">
      <SidebarSection
        title={t("repo.issues.new.labels")}
        editor={
          data.labelOptions
            ? () => (
                <div className="max-h-80 overflow-y-auto">
                  <button
                    type="button"
                    className={OPTION_CLASS}
                    disabled={labels.isPending || checked.size === 0}
                    onClick={() => labels.mutate([])}
                  >
                    <X className="size-4 shrink-0" aria-hidden />
                    {t("repo.issues.new.clear_labels")}
                  </button>
                  {data.labelOptions?.map((label) => (
                    <label key={label.id} className={cn(OPTION_CLASS, labels.isPending && "opacity-60")}>
                      <Checkbox
                        checked={checked.has(label.id)}
                        disabled={labels.isPending}
                        onCheckedChange={() => toggleLabel(label.id)}
                      />
                      <span
                        className="size-3 shrink-0 rounded-full"
                        style={{ backgroundColor: label.color }}
                        aria-hidden
                      />
                      <span className="min-w-0 truncate">{label.name}</span>
                    </label>
                  ))}
                </div>
              )
            : undefined
        }
      >
        {data.labels.length ? (
          <ul className="flex flex-wrap gap-1">
            {data.labels.map((label) => (
              <li key={label.id} className="max-w-full">
                <Link
                  to="/$owner/$repo/issues"
                  params={params}
                  search={{ labels: label.id }}
                  className="hover:opacity-80"
                >
                  {labelChip(label)}
                </Link>
              </li>
            ))}
          </ul>
        ) : (
          <p className={emptyText}>{t("repo.issues.new.no_label")}</p>
        )}
      </SidebarSection>

      <SidebarSection
        title={t("repo.issues.new.milestone")}
        editor={
          data.milestoneOptions
            ? (close) => (
                <OptionList
                  clearLabel={t("repo.issues.new.clear_milestone")}
                  groups={[
                    {
                      label: t("repo.issues.new.open_milestone"),
                      items: data.milestoneOptions?.filter((m) => !m.isClosed) ?? [],
                    },
                    {
                      label: t("repo.issues.new.closed_milestone"),
                      items: data.milestoneOptions?.filter((m) => m.isClosed) ?? [],
                    },
                  ]}
                  current={data.milestone?.id}
                  disabled={milestone.isPending}
                  render={(m) => <span className="min-w-0 truncate">{m.name}</span>}
                  onPick={(id) => milestone.mutate(id, { onSuccess: close })}
                />
              )
            : undefined
        }
      >
        {data.milestone ? (
          <Link
            to="/$owner/$repo/issues"
            params={params}
            search={{ milestone: data.milestone.id }}
            className="text-sm font-medium wrap-anywhere hover:text-(--color-primary) hover:underline"
          >
            {data.milestone.name}
          </Link>
        ) : (
          <p className={emptyText}>{t("repo.issues.new.no_milestone")}</p>
        )}
      </SidebarSection>

      <SidebarSection
        title={t("repo.issues.new.assignee")}
        editor={
          data.assigneeOptions
            ? (close) => (
                <OptionList
                  clearLabel={t("repo.issues.new.clear_assignee")}
                  groups={[{ items: data.assigneeOptions ?? [] }]}
                  current={data.assignee?.id}
                  disabled={assignee.isPending}
                  render={(u) => (
                    <>
                      <img src={u.avatarURL} alt="" className="size-5 shrink-0 rounded-full" />
                      <span className="min-w-0 truncate">{u.name}</span>
                    </>
                  )}
                  onPick={(id) => assignee.mutate(id, { onSuccess: close })}
                />
              )
            : undefined
        }
      >
        {data.assignee ? (
          <Link
            to="/$username"
            params={{ username: data.assignee.name }}
            className="inline-flex max-w-full items-center gap-2 text-sm font-medium hover:text-(--color-primary) hover:underline"
          >
            <img src={data.assignee.avatarURL} alt="" className="size-5 shrink-0 rounded-full" />
            <span className="truncate">{data.assignee.name}</span>
          </Link>
        ) : (
          <p className={emptyText}>{t("repo.issues.new.no_assignee")}</p>
        )}
      </SidebarSection>

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
    </aside>
  );
}
