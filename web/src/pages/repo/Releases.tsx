import { useMutation, useSuspenseQuery } from "@tanstack/react-query";
import { Link, useNavigate } from "@tanstack/react-router";
import {
  ChevronLeft,
  ChevronRight,
  FileArchive,
  GitCommitHorizontal,
  Package,
  Pencil,
  Plus,
  Tag,
  X,
} from "lucide-react";
import { type FormEvent, type ReactNode, useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";

import { ConfirmButton } from "@/components/ConfirmButton";
import { CheckboxField, FormAlert, FormField } from "@/components/FormField";
import { AttachFilesButton, MarkdownEditor, PendingFiles } from "@/components/IssueForm";
import { Markdown } from "@/components/Markdown";
import { RepoHeader } from "@/components/RepoHeader";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { usePageTitle } from "@/lib/page-title";
import {
  type RepoRelease,
  type RepoReleaseAttachment,
  type RepoReleaseEdit,
  type RepoReleaseFormOptions,
  type RepoReleasesData,
  createRepoRelease,
  deleteRepoRelease,
  repoHeaderQuery,
  updateRepoRelease,
} from "@/lib/queries/repo";
import { formErrors } from "@/lib/queries/request";
import { formatAbsoluteTime, formatRelativeTime } from "@/lib/relative-time";
import { subUrl } from "@/lib/url";
import { useAttachmentUploads } from "@/lib/use-attachment-uploads";

const LINK_CLASS = "text-(--color-primary) hover:underline";

function ReleasesLayout({
  owner,
  name,
  title,
  action,
  children,
}: {
  owner: string;
  name: string;
  title: string;
  action?: ReactNode;
  children: ReactNode;
}) {
  const { data: repo } = useSuspenseQuery(repoHeaderQuery(owner, name));
  usePageTitle(`${title} - ${repo.owner}/${repo.name}`);
  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <RepoHeader repo={repo} activeTab="code" />
      <main className="mx-auto w-full max-w-7xl flex-1 px-4 py-6 sm:px-6">
        <div className="mb-4 flex flex-wrap items-center justify-between gap-2">
          <h1 className="text-2xl font-semibold">{title}</h1>
          {action}
        </div>
        {children}
      </main>
    </div>
  );
}

// RepoReleases lists the tags of a repository, newest first, with their
// releases. Writers also see the drafts and can open, change and delete
// releases.
export function RepoReleases({ owner, name, data }: { owner: string; name: string; data: RepoReleasesData }) {
  const { t } = useTranslation();
  const canCreate = data.viewerCanWrite && !data.isMirror;
  return (
    <ReleasesLayout
      owner={owner}
      name={name}
      title={t("repo.release.releases")}
      action={
        canCreate ? (
          <Button asChild variant="outline" size="sm">
            <Link to="/$owner/$repo/releases/new" params={{ owner, repo: name }}>
              <Plus className="size-4" aria-hidden />
              {t("repo.release.new_release")}
            </Link>
          </Button>
        ) : null
      }
    >
      {data.releases.length ? (
        <ul className="divide-y divide-(--color-border) rounded-md border border-(--color-border)">
          {data.releases.map((release) => (
            <ReleaseRow
              key={release.isDraft ? `draft:${release.tagName}` : release.tagName}
              owner={owner}
              name={name}
              release={release}
              canWrite={data.viewerCanWrite}
            />
          ))}
        </ul>
      ) : (
        <p className="rounded-md border border-(--color-border) py-8 text-center text-sm text-(--color-muted-foreground)">
          {t("repo.pulls.no_results")}
        </p>
      )}
      {data.hasPrevious || data.nextAfter ? (
        <nav aria-label={t("repo.release.releases")} className="mt-4 flex justify-center gap-2">
          <PageButton after={data.hasPrevious ? data.previousAfter : null}>
            <ChevronLeft className="size-4" aria-hidden />
            {t("repo.issues.previous")}
          </PageButton>
          <PageButton after={data.nextAfter ?? null}>
            {t("repo.issues.next")}
            <ChevronRight className="size-4" aria-hidden />
          </PageButton>
        </nav>
      ) : null}
    </ReleasesLayout>
  );
}

// PageButton links the page that starts after the tag, or is disabled when
// there is no such page. An empty tag is the first page.
function PageButton({ after, children }: { after: string | null; children: ReactNode }) {
  if (after === null) {
    return (
      <Button variant="outline" size="sm" disabled>
        {children}
      </Button>
    );
  }
  return (
    <Button asChild variant="outline" size="sm">
      <Link to="." search={{ after: after || undefined }}>
        {children}
      </Link>
    </Button>
  );
}

function ReleaseRow({
  owner,
  name,
  release,
  canWrite,
}: {
  owner: string;
  name: string;
  release: RepoRelease;
  canWrite: boolean;
}) {
  const { t } = useTranslation();
  const repoLink = `/${owner}/${name}`;
  const archive = (ext: string) => subUrl(`${repoLink}/archive/${encodeURIComponent(release.tagName)}${ext}`);
  const tagLink = (
    <a
      href={subUrl(`${repoLink}/src/${release.tagName}`)}
      rel="nofollow"
      className={`${LINK_CLASS} inline-flex items-center gap-1.5 break-all`}
    >
      <Tag className="size-4 shrink-0" aria-hidden />
      {release.tagName}
    </a>
  );
  const commitLink = (
    <a
      href={subUrl(`${repoLink}/src/${release.sha}`)}
      rel="nofollow"
      className="inline-flex items-center gap-1.5 font-mono text-(--color-muted-foreground) hover:text-(--color-primary) hover:underline"
    >
      <GitCommitHorizontal className="size-4 shrink-0" aria-hidden />
      {release.sha.slice(0, 10)}
    </a>
  );

  // A tag that has no release only offers its source code.
  if (!release.publisher) {
    return (
      <li className="flex flex-wrap items-center gap-x-6 gap-y-2 px-4 py-3 text-sm">
        <span className="font-semibold">{tagLink}</span>
        {commitLink}
        <span className="flex gap-4 sm:ml-auto">
          <a href={archive(".zip")} rel="nofollow" className={`${LINK_CLASS} inline-flex items-center gap-1.5`}>
            <FileArchive className="size-4" aria-hidden />
            ZIP
          </a>
          <a href={archive(".tar.gz")} rel="nofollow" className={`${LINK_CLASS} inline-flex items-center gap-1.5`}>
            <FileArchive className="size-4" aria-hidden />
            TAR.GZ
          </a>
        </span>
      </li>
    );
  }

  return (
    <li className="grid gap-4 px-4 py-5 md:grid-cols-[12rem_minmax(0,1fr)]">
      <div className="flex flex-wrap items-center gap-x-4 gap-y-2 text-sm md:flex-col md:items-start">
        {release.isDraft ? (
          <span className="rounded-full border border-(--color-border) bg-(--color-surface) px-2 text-xs leading-5 font-medium">
            {t("repo.release.draft")}
          </span>
        ) : release.isPrerelease ? (
          <span className="rounded-full border border-(--color-border) bg-(--color-surface) px-2 text-xs leading-5 font-medium">
            {t("repo.release.prerelease")}
          </span>
        ) : null}
        {tagLink}
        {commitLink}
      </div>
      <div className="min-w-0">
        <div className="flex flex-wrap items-start justify-between gap-2">
          <h2 className="min-w-0 text-xl font-semibold break-words">
            <a href={subUrl(`${repoLink}/src/${release.tagName}`)} className={LINK_CLASS}>
              {release.title}
            </a>
          </h2>
          {canWrite ? (
            <Button asChild variant="outline" size="sm">
              <Link
                to="/$owner/$repo/releases/edit/$"
                params={{ owner, repo: name, _splat: release.tagName }}
                aria-label={`${t("repo.release.edit")} ${release.title}`}
              >
                <Pencil className="size-4" aria-hidden />
                {t("repo.release.edit")}
              </Link>
            </Button>
          ) : null}
        </div>
        <p className="mt-1 flex flex-wrap items-center gap-x-2 gap-y-1 text-sm text-(--color-muted-foreground)">
          <Publisher publisher={release.publisher} />
          {release.createdAt ? (
            <Tooltip>
              <TooltipTrigger asChild>
                <time dateTime={release.createdAt}>{formatRelativeTime(t, release.createdAt)}</time>
              </TooltipTrigger>
              <TooltipContent>{formatAbsoluteTime(release.createdAt)}</TooltipContent>
            </Tooltip>
          ) : null}
          {release.target ? (
            <>
              <span aria-hidden className="max-sm:hidden">
                ·
              </span>
              <span className="max-sm:w-full">
                <Ahead count={release.numCommitsBehind ?? 0} target={release.target} />
              </span>
            </>
          ) : null}
        </p>
        {release.noteHTML ? <Markdown html={release.noteHTML} className="mt-4" /> : null}
        {/* A draft has no tag yet, so no source code to download. */}
        {release.attachments?.length || !release.isDraft ? (
          <>
            <h3 className="mt-5 mb-2 text-sm font-semibold">{t("repo.release.downloads")}</h3>
            <ul className="divide-y divide-(--color-border) rounded-md border border-(--color-border) text-sm">
              {release.attachments?.map((a) => (
                <li key={a.uuid}>
                  <a
                    href={a.url}
                    rel="nofollow"
                    className={`${LINK_CLASS} flex items-center gap-2 px-3 py-2 break-all`}
                  >
                    <Package className="size-4 shrink-0" aria-hidden />
                    {a.name}
                  </a>
                </li>
              ))}
              {release.isDraft ? null : (
                <>
                  <li>
                    <a
                      href={archive(".zip")}
                      rel="nofollow"
                      className={`${LINK_CLASS} flex items-center gap-2 px-3 py-2`}
                    >
                      <FileArchive className="size-4 shrink-0" aria-hidden />
                      {t("repo.release.source_code")} (ZIP)
                    </a>
                  </li>
                  <li>
                    <a
                      href={archive(".tar.gz")}
                      rel="nofollow"
                      className={`${LINK_CLASS} flex items-center gap-2 px-3 py-2`}
                    >
                      <FileArchive className="size-4 shrink-0" aria-hidden />
                      {t("repo.release.source_code")} (TAR.GZ)
                    </a>
                  </li>
                </>
              )}
            </ul>
          </>
        ) : null}
      </div>
    </li>
  );
}

function Publisher({ publisher }: { publisher: NonNullable<RepoRelease["publisher"]> }) {
  const avatar = <img src={publisher.avatarURL} alt="" className="size-5 rounded-full" />;
  if (publisher.isGhost) {
    return (
      <span className="inline-flex items-center gap-1.5">
        {avatar}
        {publisher.name}
      </span>
    );
  }
  return (
    <Link
      to="/$username"
      params={{ username: publisher.name }}
      className="inline-flex items-center gap-1.5 font-medium text-(--color-foreground) hover:text-(--color-primary) hover:underline"
    >
      {avatar}
      {publisher.name}
    </Link>
  );
}

// Ahead says how many commits the target branch has since the release.
function Ahead({ count, target }: { count: number; target: string }) {
  const { t } = useTranslation();
  const parts = t("repo.release.ahead").split(/(<strong>%(?:\[1\])?d<\/strong>|%(?:\[1\])?d|%(?:\[2\])?s)/);
  return (
    <>
      {parts.map((part, i) => {
        if (part.startsWith("<strong>")) return <strong key={i}>{count}</strong>;
        if (/^%(?:\[1\])?d$/.test(part)) return count;
        if (/^%(?:\[2\])?s$/.test(part)) return target;
        return part;
      })}
    </>
  );
}

// RepoReleaseEditor is the form to open a release, or to change one when
// `release` is given. Drafts get a tag only once published.
export function RepoReleaseEditor({
  owner,
  name,
  options,
  release,
}: {
  owner: string;
  name: string;
  options?: RepoReleaseFormOptions;
  release?: RepoReleaseEdit;
}) {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const [tagName, setTagName] = useState("");
  const [target, setTarget] = useState(options?.defaultBranch ?? "");
  const [title, setTitle] = useState(release?.title ?? "");
  const [content, setContent] = useState(release?.content ?? "");
  const [isPrerelease, setIsPrerelease] = useState(release?.isPrerelease ?? false);
  const [kept, setKept] = useState<RepoReleaseAttachment[]>(release?.attachments ?? []);
  const settings = release ? release.attachment : options?.attachment;
  // The files already attached count toward the limit.
  const uploads = useAttachmentUploads(
    owner,
    name,
    settings && { ...settings, maxFiles: Math.max(settings.maxFiles - kept.length, 0) },
    "releases",
  );
  const toList = () => navigate({ to: "/$owner/$repo/releases", params: { owner, repo: name }, search: {} });

  const mutation = useMutation({
    mutationFn: (isDraft: boolean) => {
      const form = {
        title,
        content,
        isDraft,
        isPrerelease,
        attachments: [...kept.map((a) => a.uuid), ...uploads.uploaded],
      };
      return release
        ? updateRepoRelease(owner, name, release.tagName, form)
        : createRepoRelease(owner, name, { ...form, tagName, target });
    },
    onSuccess: toList,
  });
  const { fields, formError } = formErrors(mutation.error);
  const onSubmit = (e: FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    const submitter = (e.nativeEvent as SubmitEvent).submitter;
    mutation.mutate(submitter?.getAttribute("value") === "draft");
  };
  // A published release has its tag, so it cannot go back to a draft.
  const canSaveDraft = !release || release.isDraft;

  return (
    <ReleasesLayout
      owner={owner}
      name={name}
      title={release ? t("repo.release.edit_release") : t("repo.release.new_release")}
    >
      <p className="-mt-2 mb-6 text-sm text-(--color-muted-foreground)">
        {release ? t("repo.release.edit_subheader") : t("repo.release.new_subheader")}
      </p>
      <form onSubmit={onSubmit} className="max-w-3xl">
        <fieldset disabled={mutation.isPending} className="flex flex-col gap-5">
          <FormAlert message={formError} />
          {release ? (
            <p className="flex flex-wrap items-center gap-1.5 text-sm">
              <Tag className="size-4 text-(--color-muted-foreground)" aria-hidden />
              <strong className="break-all">{release.tagName}</strong>
              <span className="text-(--color-muted-foreground)">@</span>
              <strong className="break-all">{release.target}</strong>
            </p>
          ) : (
            <div className="grid gap-4 sm:grid-cols-2">
              <FormField
                id="release-tag-name"
                label={t("repo.release.tag_name")}
                helper={t("repo.release.tag_helper")}
                error={fields.tagName}
                invalid={Boolean(fields.tagName)}
              >
                <Input
                  id="release-tag-name"
                  value={tagName}
                  required
                  autoFocus
                  onChange={(e) => setTagName(e.target.value)}
                />
              </FormField>
              <FormField
                id="release-target"
                label={t("repo.release.target")}
                error={fields.target}
                invalid={Boolean(fields.target)}
              >
                <Select value={target} onValueChange={setTarget}>
                  <SelectTrigger id="release-target" aria-invalid={Boolean(fields.target) || undefined}>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {options?.branches.map((branch) => (
                      <SelectItem key={branch} value={branch}>
                        {branch}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </FormField>
            </div>
          )}
          <FormField
            id="release-title"
            label={t("repo.release.title")}
            error={fields.title}
            invalid={Boolean(fields.title)}
          >
            <Input
              id="release-title"
              value={title}
              required
              maxLength={255}
              autoFocus={Boolean(release)}
              onChange={(e) => setTitle(e.target.value)}
            />
          </FormField>
          <div className="rounded-md border border-(--color-border) p-3">
            <MarkdownEditor
              repo={{ owner, name }}
              label={t("repo.release.content")}
              value={content}
              onChange={setContent}
              disabled={mutation.isPending}
              uploads={uploads}
            />
            {kept.length ? (
              <ul className="mt-2 flex flex-col gap-1">
                {kept.map((a) => (
                  <li key={a.uuid} className="flex items-center gap-2 text-sm">
                    <Package className="size-4 shrink-0 text-(--color-muted-foreground)" aria-hidden />
                    <a
                      href={a.url}
                      target="_blank"
                      rel="noopener noreferrer"
                      className={`${LINK_CLASS} min-w-0 flex-1 truncate`}
                    >
                      {a.name}
                    </a>
                    <Button
                      type="button"
                      variant="ghost"
                      size="icon"
                      aria-label={`${t("dropzone.remove_file")}: ${a.name}`}
                      className="size-7 text-(--color-muted-foreground)"
                      onClick={() => setKept((prev) => prev.filter((k) => k.uuid !== a.uuid))}
                    >
                      <X className="size-4" aria-hidden />
                    </Button>
                  </li>
                ))}
              </ul>
            ) : null}
            <PendingFiles uploads={uploads} />
            <div className="mt-2">
              <AttachFilesButton uploads={uploads} />
            </div>
          </div>
          <div>
            <CheckboxField
              id="release-prerelease"
              label={<strong>{t("repo.release.prerelease_desc")}</strong>}
              checked={isPrerelease}
              onChange={setIsPrerelease}
            />
            <p className="mt-1 ml-6.5 text-sm text-(--color-muted-foreground)">{t("repo.release.prerelease_helper")}</p>
          </div>
          <div className="flex flex-wrap items-center gap-2 border-t border-(--color-border) pt-5">
            <Button type="submit" value="publish" disabled={uploads.uploading}>
              {release && !release.isDraft ? t("repo.release.edit_release") : t("repo.release.publish")}
            </Button>
            {canSaveDraft ? (
              <Button type="submit" value="draft" variant="outline" disabled={uploads.uploading}>
                {t("repo.release.save_draft")}
              </Button>
            ) : null}
            <Button asChild variant="ghost">
              <Link to="/$owner/$repo/releases" params={{ owner, repo: name }} search={{}}>
                {t("repo.release.cancel")}
              </Link>
            </Button>
            {release ? (
              <span className="sm:ml-auto">
                <ConfirmButton
                  label={t("repo.release.deletion")}
                  title={t("repo.release.deletion")}
                  description={t("repo.release.deletion_desc")}
                  action={() => deleteRepoRelease(owner, name, release.tagName)}
                  onDone={async () => {
                    toast.success(t("repo.release.deletion_success"));
                    await toList();
                  }}
                />
              </span>
            ) : null}
          </div>
        </fieldset>
      </form>
    </ReleasesLayout>
  );
}
