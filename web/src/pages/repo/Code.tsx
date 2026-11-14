import { type SelectedLineRange, getFiletypeFromFileName } from "@pierre/diffs";
import { File as CodeFile } from "@pierre/diffs/react";
import { useSuspenseQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import {
  BookOpen,
  ChevronDown,
  Code,
  CornerLeftUp,
  Download,
  File,
  FileSymlink,
  Folder,
  FolderGit2,
  GitBranch,
  GitPullRequestArrow,
  History,
  Pencil,
  Plus,
  Tag,
  Trash2,
  Upload,
} from "lucide-react";
import { type ReactNode, Suspense, lazy, useEffect, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";

import { CloneBox } from "@/components/CloneBox";
import { CodeBlock } from "@/components/CodeBlock";
import { Markdown } from "@/components/Markdown";
import { RefPicker } from "@/components/RefPicker";
import { RepoHeader } from "@/components/RepoHeader";
import { RichText } from "@/components/RichText";
import { Button } from "@/components/ui/button";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { formatFileSize } from "@/lib/file-size";
import { usePageTitle } from "@/lib/page-title";
import {
  type RepoCommitListItem,
  type RepoFile,
  type RepoNotebook,
  type RepoNotebookOutput,
  type RepoSrcData,
  type RepoTreeEntry,
  refPath,
  repoHeaderQuery,
} from "@/lib/queries/repo";
import { formatAbsoluteTime, formatRelativeTime } from "@/lib/relative-time";
import { resolveTheme, useTheme } from "@/lib/theme-context";
import { subUrl } from "@/lib/url";
import { cn } from "@/lib/utils";

const PdfView = lazy(() => import("@/components/PdfView"));

const LINK_CLASS = "text-(--color-primary) hover:underline";
const ICON_BUTTON_CLASS =
  "grid size-8 shrink-0 place-items-center rounded-md text-(--color-muted-foreground) hover:bg-(--color-surface) hover:text-(--color-foreground)";

// splatOf is the route path of a ref, followed by a path when given.
function splatOf(ref: string, path?: string) {
  return path ? `${ref}/${path}` : ref;
}

// RepoCode browses the files of a repository at a branch, a tag or a commit:
// a directory with its README, or a file. An empty repository shows how to
// push to it.
export function RepoCode({ owner, name, data }: { owner: string; name: string; data: RepoSrcData }) {
  const { t } = useTranslation();
  const { data: repo } = useSuspenseQuery(repoHeaderQuery(owner, name));
  const summary = data.summary;
  const fullName = `${repo.owner}/${repo.name}`;
  let title = summary?.description ? `${fullName}: ${summary.description}` : fullName;
  if (data.file) title = `${data.file.name} - ${fullName}`;
  else if (!data.empty && data.ref !== data.defaultBranch) title += ` @ ${data.ref}`;
  usePageTitle(title);

  if (data.empty) {
    return (
      <div className="flex min-h-0 flex-1 flex-col">
        <RepoHeader repo={repo} activeTab="code" />
        <main className="mx-auto w-full max-w-7xl flex-1 px-4 py-6 sm:px-6">
          <EmptyRepo data={data} />
        </main>
      </div>
    );
  }

  const params = { owner: repo.owner, repo: repo.name };
  const srcLink = (path: string, ref = data.ref, className?: string, children?: ReactNode) => (
    <Link to="/$owner/$repo/src/$" params={{ ...params, _splat: splatOf(ref, path) }} className={className}>
      {children}
    </Link>
  );
  const pathParts = data.path ? data.path.split("/") : [];
  const repoLink = subUrl(`/${repo.owner}/${repo.name}`);
  const editorPath = refPath(splatOf(data.ref, data.path));

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <RepoHeader repo={repo} activeTab="code" />
      <main className="mx-auto w-full max-w-7xl flex-1 px-4 py-6 sm:px-6">
        <h1 className="sr-only">{data.path || fullName}</h1>
        {summary ? (
          <p className="mb-4 text-sm wrap-anywhere">
            {summary.description ? (
              <span className="whitespace-pre-line">{summary.description}</span>
            ) : (
              <span className="text-(--color-muted-foreground) italic">{t("repo.no_desc")}</span>
            )}
            {summary.website ? (
              <a href={summary.website} className={cn(LINK_CLASS, "ml-2")} rel="nofollow noopener noreferrer">
                {summary.website}
              </a>
            ) : null}
          </p>
        ) : null}

        <div className="mb-4 flex flex-wrap items-center gap-2">
          <RefPicker
            current={data.ref}
            type={data.refType}
            branches={data.branches}
            tags={data.tags}
            renderLink={(ref, className, children) => srcLink(data.path, ref, className, children)}
          />
          {data.pullRequest ? (
            <Tooltip>
              <TooltipTrigger asChild>
                <a
                  href={subUrl(
                    `${data.pullRequest.repoLink}/compare/${refPath(data.pullRequest.baseBranch)}...${refPath(data.pullRequest.head).replace(/%3A/g, ":")}`,
                  )}
                  aria-label={t("repo.pulls.compare_changes")}
                  className="grid size-8 shrink-0 place-items-center rounded-md border border-(--color-border) hover:bg-(--color-surface)"
                >
                  <GitPullRequestArrow className="size-4 text-(--color-success)" aria-hidden />
                </a>
              </TooltipTrigger>
              <TooltipContent>{t("repo.pulls.compare_changes")}</TooltipContent>
            </Tooltip>
          ) : null}
          {pathParts.length ? (
            <nav aria-label={t("repo.files")} className="flex min-w-0 flex-wrap items-center gap-x-1 text-sm">
              {srcLink("", data.ref, cn(LINK_CLASS, "font-semibold wrap-anywhere"), repo.name)}
              {pathParts.map((part, i) => (
                <span key={i} className="contents">
                  <span className="text-(--color-muted-foreground)" aria-hidden>
                    /
                  </span>
                  {i === pathParts.length - 1 ? (
                    <span className="font-semibold wrap-anywhere" aria-current="page">
                      {part}
                    </span>
                  ) : (
                    srcLink(pathParts.slice(0, i + 1).join("/"), data.ref, cn(LINK_CLASS, "wrap-anywhere"), part)
                  )}
                </span>
              ))}
            </nav>
          ) : summary ? (
            <span className="flex flex-wrap items-center gap-x-3 gap-y-1 text-sm">
              <Link
                to="/$owner/$repo/branches"
                params={params}
                className="inline-flex items-center gap-1 hover:underline"
              >
                <GitBranch className="size-4 text-(--color-muted-foreground)" aria-hidden />
                <strong className="font-semibold">{data.branches.length}</strong>
                <span className="text-(--color-muted-foreground)">{t("repo.branches")}</span>
              </Link>
              <Link
                to="/$owner/$repo/releases"
                params={params}
                className="inline-flex items-center gap-1 hover:underline"
              >
                <Tag className="size-4 text-(--color-muted-foreground)" aria-hidden />
                <strong className="font-semibold">{data.tags.length}</strong>
                <span className="text-(--color-muted-foreground)">{t("repo.releases")}</span>
              </Link>
            </span>
          ) : null}
          <div className="ml-auto flex flex-wrap items-center gap-2">
            {data.kind === "dir" && data.editor === "ok" ? (
              <>
                <Button asChild variant="outline" size="sm">
                  <Link to="/$owner/$repo/_new/$" params={{ ...params, _splat: splatOf(data.ref, data.path) }}>
                    <Plus className="size-4" aria-hidden />
                    {t("repo.editor.new_file")}
                  </Link>
                </Button>
                {data.canUploadFile ? (
                  <Button asChild variant="outline" size="sm">
                    <Link to="/$owner/$repo/_upload/$" params={{ ...params, _splat: splatOf(data.ref, data.path) }}>
                      <Upload className="size-4" aria-hidden />
                      {t("repo.editor.upload_file")}
                    </Link>
                  </Button>
                ) : null}
              </>
            ) : null}
            {summary ? (
              <CloneMenu links={summary.cloneLinks} archiveLink={`${repoLink}/archive/${refPath(data.ref)}`} />
            ) : null}
          </div>
        </div>

        <div className="overflow-hidden rounded-md border border-(--color-border)">
          {data.latestCommit ? (
            <LatestCommit
              owner={repo.owner}
              name={repo.name}
              commit={data.latestCommit}
              history={
                <Link
                  to="/$owner/$repo/commits/$"
                  params={{ ...params, _splat: splatOf(data.ref, data.path) }}
                  className="inline-flex shrink-0 items-center gap-1 text-sm hover:text-(--color-primary)"
                >
                  <History className="size-4" aria-hidden />
                  {summary ? (
                    <span>
                      <strong className="font-semibold">{summary.commitCount}</strong> {t("repo.commits")}
                    </span>
                  ) : (
                    t("repo.file_history")
                  )}
                </Link>
              }
            />
          ) : null}
          {data.kind === "dir" ? (
            <EntryList
              owner={repo.owner}
              name={repo.name}
              entries={data.entries ?? []}
              parent={pathParts.length ? srcLink(pathParts.slice(0, -1).join("/"), data.ref, LINK_CLASS, "..") : null}
              linkOf={(entry) =>
                srcLink(data.path ? `${data.path}/${entry.name}` : entry.name, data.ref, undefined, entry.name)
              }
            />
          ) : null}
        </div>

        {data.kind === "dir" && data.readme ? (
          <section
            aria-labelledby="repo-readme-heading"
            className="mt-6 overflow-hidden rounded-md border border-(--color-border)"
          >
            <h2
              id="repo-readme-heading"
              className="flex items-center gap-2 border-b border-(--color-border) bg-(--color-surface) px-4 py-2 text-sm font-semibold"
            >
              <BookOpen className="size-4 text-(--color-muted-foreground)" aria-hidden />
              {data.readme.name}
            </h2>
            {data.readme.html !== undefined ? (
              <Markdown html={data.readme.html} className="p-4 sm:p-6" />
            ) : (
              <pre className="p-4 font-mono text-sm wrap-anywhere whitespace-pre-wrap">{data.readme.text}</pre>
            )}
          </section>
        ) : null}

        {data.file ? (
          <FileView
            owner={repo.owner}
            name={repo.name}
            data={data}
            file={data.file}
            rawURL={subUrl(`/${repo.owner}/${repo.name}/raw/${editorPath}`)}
          />
        ) : null}
      </main>
    </div>
  );
}

function CloneMenu({ links, archiveLink }: { links: RepoSrcData["cloneLinks"]; archiveLink: string }) {
  const { t } = useTranslation();
  return (
    <Popover>
      <PopoverTrigger asChild>
        <Button size="sm">
          <Code className="size-4" aria-hidden />
          {t("repo.code")}
          <ChevronDown className="size-3.5" aria-hidden />
        </Button>
      </PopoverTrigger>
      <PopoverContent align="end" className="flex w-80 max-w-[calc(100vw-2rem)] flex-col gap-3 p-3">
        {links?.https || links?.ssh ? (
          <section aria-labelledby="repo-clone-heading">
            <h2 id="repo-clone-heading" className="mb-2 text-sm font-semibold">
              {t("repo.clone_this_repo")}
            </h2>
            <CloneBox links={links} label={t("repo.clone_this_repo")} />
          </section>
        ) : null}
        <div className="flex flex-col border-t border-(--color-border) pt-2 text-sm">
          {[".zip", ".tar.gz"].map((ext) => (
            <a
              key={ext}
              href={`${archiveLink}${ext}`}
              className="flex items-center gap-2 rounded-sm px-2 py-1.5 hover:bg-(--color-surface)"
            >
              <Download className="size-4 text-(--color-muted-foreground)" aria-hidden />
              {t("repo.download_archive", { format: ext === ".zip" ? "ZIP" : "TAR.GZ" })}
            </a>
          ))}
        </div>
      </PopoverContent>
    </Popover>
  );
}

export function RelativeTime({ when }: { when: string }) {
  const { t } = useTranslation();
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <time dateTime={when} className="whitespace-nowrap">
          {formatRelativeTime(t, when)}
        </time>
      </TooltipTrigger>
      <TooltipContent>{formatAbsoluteTime(when)}</TooltipContent>
    </Tooltip>
  );
}

const FILE_MODE_CLASS =
  "bg-(--color-background) px-2.5 py-1.5 hover:bg-(--color-surface) aria-[current=page]:bg-(--color-surface) aria-[current=page]:font-semibold";

// FileModeNav switches a text file between its code and its blame.
export function FileModeNav({
  owner,
  name,
  splat,
  mode,
}: {
  owner: string;
  name: string;
  splat: string;
  mode: "code" | "blame";
}) {
  const { t } = useTranslation();
  const params = { owner, repo: name, _splat: splat };
  return (
    <nav
      aria-label={t("repo.file_view")}
      className="flex overflow-hidden rounded-md border border-(--color-border) text-xs [&>*+*]:border-l [&>*+*]:border-(--color-border)"
    >
      <Link
        to="/$owner/$repo/src/$"
        params={params}
        aria-current={mode === "code" ? "page" : undefined}
        className={FILE_MODE_CLASS}
      >
        {t("repo.code")}
      </Link>
      <Link
        to="/$owner/$repo/blame/$"
        params={params}
        aria-current={mode === "blame" ? "page" : undefined}
        className={FILE_MODE_CLASS}
      >
        {t("repo.blame")}
      </Link>
    </nav>
  );
}

function LatestCommit({
  owner,
  name,
  commit,
  history,
}: {
  owner: string;
  name: string;
  commit: RepoCommitListItem;
  history: ReactNode;
}) {
  const { author } = commit;
  return (
    <div className="flex flex-wrap items-center gap-x-3 gap-y-1 border-b border-(--color-border) bg-(--color-surface) px-4 py-2.5 text-sm">
      <span className="flex min-w-0 flex-1 basis-60 items-center gap-2">
        <img src={author.avatarURL} alt="" className="size-5 shrink-0 rounded-full" />
        {author.profileURL ? (
          <a href={author.profileURL} className="shrink-0 font-semibold hover:underline">
            {author.name}
          </a>
        ) : (
          <span className="shrink-0 font-semibold">{author.name}</span>
        )}
        <span
          className="min-w-0 truncate text-(--color-muted-foreground) [&_a]:text-(--color-primary) [&_a]:hover:underline"
          dangerouslySetInnerHTML={{ __html: commit.subjectHTML }}
        />
      </span>
      <span className="flex items-center gap-3 text-(--color-muted-foreground)">
        <Link
          to="/$owner/$repo/commit/$sha"
          params={{ owner, repo: name, sha: commit.sha }}
          className="font-mono text-xs hover:text-(--color-primary) hover:underline"
        >
          {commit.sha.slice(0, 7)}
        </Link>
        <RelativeTime when={author.when} />
        {history}
      </span>
    </div>
  );
}

function EntryIcon({ type }: { type: RepoTreeEntry["type"] }) {
  const Icon = { dir: Folder, file: File, symlink: FileSymlink, submodule: FolderGit2 }[type];
  return (
    <Icon
      className={cn("size-4 shrink-0", type === "dir" ? "text-(--color-primary)" : "text-(--color-muted-foreground)")}
      aria-hidden
    />
  );
}

function EntryList({
  owner,
  name,
  entries,
  parent,
  linkOf,
}: {
  owner: string;
  name: string;
  entries: RepoTreeEntry[];
  parent: ReactNode;
  linkOf: (entry: RepoTreeEntry) => ReactNode;
}) {
  const params = { owner, repo: name };
  return (
    <ul className="divide-y divide-(--color-border) text-sm">
      {parent ? (
        <li className="flex items-center gap-2 px-4 py-2 hover:bg-(--color-surface)">
          <CornerLeftUp className="size-4 text-(--color-muted-foreground)" aria-hidden />
          <span className="font-mono">{parent}</span>
        </li>
      ) : null}
      {entries.map((entry) => (
        <li key={entry.name} className="flex items-center gap-x-4 px-4 py-2 hover:bg-(--color-surface)">
          <span className="flex min-w-0 flex-1 items-center gap-2 sm:max-w-xs sm:flex-none sm:basis-1/3">
            <EntryIcon type={entry.type} />
            {entry.type === "submodule" ? (
              <span className="min-w-0 truncate">
                {entry.submoduleURL ? (
                  <a href={entry.submoduleURL} className={LINK_CLASS}>
                    {entry.name}
                  </a>
                ) : (
                  entry.name
                )}
                {entry.submoduleCommit ? (
                  <span className="ml-1 font-mono text-xs text-(--color-muted-foreground)">
                    @ {entry.submoduleCommit.slice(0, 7)}
                  </span>
                ) : null}
              </span>
            ) : (
              <span className="min-w-0 truncate [&>a]:hover:text-(--color-primary) [&>a]:hover:underline">
                {linkOf(entry)}
              </span>
            )}
          </span>
          {entry.commit ? (
            <>
              <span
                className="hidden min-w-0 flex-1 truncate text-(--color-muted-foreground) sm:block [&_a]:hover:text-(--color-primary) [&_a]:hover:underline"
                dangerouslySetInnerHTML={{ __html: entry.commit.subjectHTML }}
              />
              <Link
                to="/$owner/$repo/commit/$sha"
                params={{ ...params, sha: entry.commit.sha }}
                className="shrink-0 text-(--color-muted-foreground) hover:text-(--color-primary)"
                aria-label={`${entry.name}: ${entry.commit.sha.slice(0, 7)}`}
              >
                <RelativeTime when={entry.commit.when} />
              </Link>
            </>
          ) : null}
        </li>
      ))}
    </ul>
  );
}

function FileView({
  owner,
  name,
  data,
  file,
  rawURL,
}: {
  owner: string;
  name: string;
  data: RepoSrcData;
  file: RepoFile;
  rawURL: string;
}) {
  const { t } = useTranslation();
  const params = { owner, repo: name };
  const splat = splatOf(data.ref, data.path);
  const lineCount = useMemo(() => (file.content ? file.content.replace(/\n$/, "").split("\n").length : 0), [file]);
  const isText =
    file.kind === "text" || file.kind === "markup" || file.kind === "notebook" || file.kind === "too_large";
  const canEdit = data.editor === "ok" && isText && !file.isSymlink;
  let editHint = t("repo.editor.edit_this_file");
  if (data.editor === "not_branch") editHint = t("repo.editor.must_be_on_a_branch");
  else if (data.editor === "no_write") editHint = t("repo.editor.fork_before_edit");
  else if (file.isSymlink) editHint = t("repo.editor.file_is_a_symlink").replace(/%(?:\[1\])?s/, file.name);
  else if (!isText) editHint = t("repo.editor.cannot_edit_non_text_files");
  const deleteHint =
    data.editor === "ok"
      ? t("repo.editor.delete_this_file")
      : data.editor === "not_branch"
        ? t("repo.editor.must_be_on_a_branch")
        : t("repo.editor.must_have_write_access");

  return (
    <section
      aria-labelledby="repo-file-heading"
      className="mt-4 overflow-hidden rounded-md border border-(--color-border)"
    >
      <div className="flex flex-wrap items-center gap-x-3 gap-y-2 border-b border-(--color-border) bg-(--color-surface) px-4 py-2">
        <h2 id="repo-file-heading" className="flex min-w-0 items-center gap-2 text-sm">
          {file.isSymlink ? (
            <FileSymlink className="size-4 shrink-0 text-(--color-muted-foreground)" aria-hidden />
          ) : file.isReadme ? (
            <BookOpen className="size-4 shrink-0 text-(--color-muted-foreground)" aria-hidden />
          ) : (
            <File className="size-4 shrink-0 text-(--color-muted-foreground)" aria-hidden />
          )}
          <span className="font-semibold wrap-anywhere">{file.name}</span>
        </h2>
        <span className="text-xs text-(--color-muted-foreground)">
          {lineCount ? `${t(lineCount === 1 ? "repo.file_line" : "repo.file_lines", { count: lineCount })} · ` : null}
          {formatFileSize(file.size)}
        </span>
        <div className="ml-auto flex flex-wrap items-center gap-1">
          {file.kind !== "too_large" && isText ? (
            <span className="mr-1">
              <FileModeNav owner={owner} name={name} splat={splat} mode="code" />
            </span>
          ) : null}
          <div className="mr-1 flex overflow-hidden rounded-md border border-(--color-border) text-xs [&>*+*]:border-l [&>*+*]:border-(--color-border)">
            {data.refType !== "commit" ? (
              <Link
                to="/$owner/$repo/src/$"
                params={{ ...params, _splat: splatOf(data.commitID, data.path) }}
                className="bg-(--color-background) px-2.5 py-1.5 hover:bg-(--color-surface)"
              >
                {t("repo.file_permalink")}
              </Link>
            ) : null}
            <a href={rawURL} className="bg-(--color-background) px-2.5 py-1.5 hover:bg-(--color-surface)">
              {t("repo.file_raw")}
            </a>
          </div>
          {data.editor ? (
            <>
              <Tooltip>
                <TooltipTrigger asChild>
                  {canEdit ? (
                    <Link
                      to="/$owner/$repo/_edit/$"
                      params={{ ...params, _splat: splat }}
                      aria-label={editHint}
                      className={ICON_BUTTON_CLASS}
                    >
                      <Pencil className="size-4" aria-hidden />
                    </Link>
                  ) : (
                    <span
                      role="link"
                      aria-disabled="true"
                      aria-label={editHint}
                      tabIndex={0}
                      className={cn(ICON_BUTTON_CLASS, "opacity-50")}
                    >
                      <Pencil className="size-4" aria-hidden />
                    </span>
                  )}
                </TooltipTrigger>
                <TooltipContent>{editHint}</TooltipContent>
              </Tooltip>
              <Tooltip>
                <TooltipTrigger asChild>
                  {data.editor === "ok" ? (
                    <Link
                      to="/$owner/$repo/_delete/$"
                      params={{ ...params, _splat: splat }}
                      aria-label={deleteHint}
                      className={cn(ICON_BUTTON_CLASS, "hover:text-(--color-destructive)")}
                    >
                      <Trash2 className="size-4" aria-hidden />
                    </Link>
                  ) : (
                    <span
                      role="link"
                      aria-disabled="true"
                      aria-label={deleteHint}
                      tabIndex={0}
                      className={cn(ICON_BUTTON_CLASS, "opacity-50")}
                    >
                      <Trash2 className="size-4" aria-hidden />
                    </span>
                  )}
                </TooltipTrigger>
                <TooltipContent>{deleteHint}</TooltipContent>
              </Tooltip>
            </>
          ) : null}
        </div>
      </div>
      <FileBody file={file} rawURL={rawURL} />
    </section>
  );
}

function FileBody({ file, rawURL }: { file: RepoFile; rawURL: string }) {
  const { t } = useTranslation();
  switch (file.kind) {
    case "text":
      return <SourceView file={file} />;
    case "markup":
      return <Markdown html={file.html ?? ""} className="p-4 sm:p-6" />;
    case "notebook":
      return file.notebook ? <NotebookView notebook={file.notebook} rawURL={rawURL} /> : null;
    case "image":
      return (
        <div className="grid place-items-center bg-(--color-background) p-4">
          <img src={rawURL} alt={file.name} className="max-w-full" />
        </div>
      );
    case "video":
      return (
        <div className="grid place-items-center p-4">
          <video controls src={rawURL} className="max-w-full">
            {t("repo.video_not_supported_in_browser")}
          </video>
        </div>
      );
    case "pdf":
      return (
        <Suspense fallback={null}>
          <PdfView url={rawURL} />
        </Suspense>
      );
    default:
      return (
        <div className="flex flex-col items-center gap-3 px-4 py-10 text-sm">
          {file.kind === "too_large" ? <p className="font-semibold">{t("repo.file_too_large")}</p> : null}
          <Button asChild variant="outline" size="sm">
            <a href={rawURL} rel="nofollow">
              {t("repo.file_view_raw")}
            </a>
          </Button>
        </div>
      );
  }
}

const LINE_HASH = /^#L(\d+)(?:-L?(\d+))?$/;

function selectedLinesOf(hash: string): SelectedLineRange | null {
  const match = LINE_HASH.exec(hash);
  if (!match) return null;
  const start = Number(match[1]);
  const end = match[2] ? Number(match[2]) : start;
  return { start: Math.min(start, end), end: Math.max(start, end) };
}

// SourceView shows a text file highlighted, with line numbers that select
// lines into the URL hash, e.g., "#L5-L9" like the server-rendered pages.
function SourceView({ file }: { file: RepoFile }) {
  const { theme } = useTheme();
  const [selected, setSelected] = useState(() => selectedLinesOf(window.location.hash));
  const wrapper = useRef<HTMLDivElement>(null);
  const initialLine = useRef(selected?.start);

  // The lines render after the highlighter loads, so wait for the selected
  // line of the URL to appear before scrolling to it.
  useEffect(() => {
    const line = initialLine.current;
    if (!line) return;
    let frame = 0;
    let tries = 0;
    const scroll = () => {
      const root = wrapper.current?.querySelector("diffs-container")?.shadowRoot;
      const el = root?.querySelector(`[data-line="${line}"]`);
      if (el) el.scrollIntoView({ block: "center" });
      else if (tries++ < 300) frame = window.requestAnimationFrame(scroll);
    };
    frame = window.requestAnimationFrame(scroll);
    return () => window.cancelAnimationFrame(frame);
  }, []);
  const tabCSS = file.tabWidth ? `pre, code { tab-size: ${file.tabWidth}; }` : undefined;

  return (
    <div ref={wrapper} className="text-sm">
      <CodeFile
        file={{ name: file.name, contents: (file.content ?? "").replace(/\n$/, "") }}
        selectedLines={selected}
        style={{ border: "none" }}
        options={{
          theme: { light: "pierre-light", dark: "pierre-dark" },
          themeType: resolveTheme(theme),
          disableFileHeader: true,
          overflow: "scroll",
          enableLineSelection: true,
          unsafeCSS: tabCSS,
          onLineSelected: (range) => {
            setSelected(range);
            const hash = range
              ? range.start === range.end
                ? `#L${range.start}`
                : `#L${range.start}-L${range.end}`
              : "";
            window.history.replaceState(
              window.history.state,
              "",
              `${window.location.pathname}${window.location.search}${hash}`,
            );
          },
        }}
      />
    </div>
  );
}

// resolveImages points relative image sources in the Markdown of a notebook at
// the files next to it, e.g., "img/plot.png".
function resolveImages(html: string, dir: string) {
  if (!html.includes("<img")) return html;
  const body = new DOMParser().parseFromString(html, "text/html").body;
  for (const img of Array.from(body.querySelectorAll("img"))) {
    const src = img.getAttribute("src") ?? "";
    if (src && !/^([a-z][a-z0-9+.-]*:|\/|#)/i.test(src)) img.setAttribute("src", `${dir}/${src}`);
  }
  return body.innerHTML;
}

// NotebookPrompt labels the input or output of a code cell like Jupyter, e.g.,
// "In [3]:".
function NotebookPrompt({ label, count }: { label?: "In" | "Out"; count?: number }) {
  return (
    <div className="font-mono text-xs text-(--color-muted-foreground) sm:pt-2 sm:text-right">
      {label ? `${label} [${count ?? " "}]:` : null}
    </div>
  );
}

const NOTEBOOK_ROW = "flex flex-col gap-1 sm:grid sm:grid-cols-[4.5rem_minmax(0,1fr)] sm:gap-x-3";

// themeAnsiColors replaces the 256-color and true-color codes of ANSI escape
// sequences with the default color, so text only takes the 16 colors of the
// theme, which stay readable in both modes.
function themeAnsiColors(text: string) {
  // eslint-disable-next-line no-control-regex
  return text.replace(/\x1b\[([\d;]*)m/g, (_, params: string) => {
    const codes = params.split(";");
    const kept: string[] = [];
    for (let i = 0; i < codes.length; i++) {
      if ((codes[i] === "38" || codes[i] === "48") && (codes[i + 1] === "5" || codes[i + 1] === "2")) {
        kept.push(codes[i] === "38" ? "39" : "49");
        i += codes[i + 1] === "5" ? 2 : 4;
        continue;
      }
      kept.push(codes[i]);
    }
    return `\x1b[${kept.join(";")}m`;
  });
}

function NotebookOutputView({ output }: { output: RepoNotebookOutput }) {
  const { t } = useTranslation();
  switch (output.type) {
    case "text":
    case "error":
      return (
        <CodeBlock
          code={themeAnsiColors(output.text ?? "").replace(/\n$/, "")}
          lang="ansi"
          className={cn("rounded-none border-0", output.type === "error" && "border-l-2 border-(--color-destructive)")}
        />
      );
    case "image":
      return <img src={output.src} alt={t("repo.notebook_output_image")} className="max-w-full bg-white" />;
    case "html":
      return <Markdown html={output.html ?? ""} className="overflow-x-auto" />;
  }
}

// NotebookView shows a Jupyter notebook from the cells the server parsed, with
// Markdown and HTML it rendered and sanitized.
function NotebookView({ notebook, rawURL }: { notebook: RepoNotebook; rawURL: string }) {
  const lang = getFiletypeFromFileName(`cell${notebook.fileExtension || ".py"}`);
  const dir = rawURL.slice(0, rawURL.lastIndexOf("/"));
  return (
    <div className="flex flex-col gap-4 p-4 sm:p-6">
      {notebook.cells.map((cell, i) => {
        switch (cell.type) {
          case "markdown":
            return (
              <div key={i} className={NOTEBOOK_ROW}>
                <NotebookPrompt />
                <Markdown html={resolveImages(cell.html ?? "", dir)} />
              </div>
            );
          case "raw":
            return (
              <div key={i} className={NOTEBOOK_ROW}>
                <NotebookPrompt />
                <pre className="font-mono text-xs whitespace-pre-wrap">{cell.source}</pre>
              </div>
            );
          case "code":
            return (
              <div key={i} className="flex flex-col gap-2">
                <div className={NOTEBOOK_ROW}>
                  <NotebookPrompt label="In" count={cell.executionCount} />
                  <CodeBlock code={cell.source ?? ""} lang={lang} />
                </div>
                {cell.outputs?.map((output, j) => (
                  <div key={j} className={NOTEBOOK_ROW}>
                    <NotebookPrompt label={output.executionCount ? "Out" : undefined} count={output.executionCount} />
                    <div className="min-w-0">
                      <NotebookOutputView output={output} />
                    </div>
                  </div>
                ))}
              </div>
            );
        }
      })}
    </div>
  );
}

function EmptyRepo({ data }: { data: RepoSrcData }) {
  const { t } = useTranslation();
  if (!data.viewerCanWrite || data.isMirror) {
    return (
      <p className="rounded-md border border-(--color-border) px-4 py-10 text-center text-sm text-(--color-muted-foreground)">
        {t("repo.empty_message")}
      </p>
    );
  }
  const links = data.cloneLinks ?? {};
  const remote = links.ssh ?? links.https ?? "";
  const branch = data.defaultBranch;
  return (
    <section aria-labelledby="repo-quick-guide" className="overflow-hidden rounded-md border border-(--color-border)">
      <h2
        id="repo-quick-guide"
        className="border-b border-(--color-border) bg-(--color-surface) px-4 py-2 text-sm font-semibold"
      >
        {t("repo.quick_guide")}
      </h2>
      <div className="flex flex-col divide-y divide-(--color-border)">
        <div className="flex flex-col gap-2 p-4">
          <h3 className="font-semibold">{t("repo.clone_this_repo")}</h3>
          <p className="text-sm text-(--color-muted-foreground)">
            <RichText
              text={t("repo.clone_helper")}
              links={["http://git-scm.com/book/en/Git-Basics-Getting-a-Git-Repository"]}
            />
          </p>
          <div className="max-w-xl">
            <CloneBox links={links} label={t("repo.clone_this_repo")} />
          </div>
        </div>
        <div className="flex flex-col gap-2 p-4">
          <h3 className="font-semibold">{t("repo.create_new_repo_command")}</h3>
          <CodeBlock
            lang="shellscript"
            code={`touch README.md
git init
git add README.md
git commit -m "first commit"
git remote add origin ${remote}
git push -u origin ${branch}`}
          />
        </div>
        <div className="flex flex-col gap-2 p-4">
          <h3 className="font-semibold">{t("repo.push_exist_repo")}</h3>
          <CodeBlock lang="shellscript" code={`git remote add origin ${remote}\ngit push -u origin ${branch}`} />
        </div>
      </div>
    </section>
  );
}
