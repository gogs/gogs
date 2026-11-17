import type { SelectedLineRange } from "@pierre/diffs";
import { File as CodeFile } from "@pierre/diffs/react";
import { useSuspenseQuery } from "@tanstack/react-query";
import { Link, useNavigate, useSearch } from "@tanstack/react-router";
import {
  BookOpen,
  Check,
  ChevronDown,
  Code,
  Copy,
  CornerLeftUp,
  Download,
  Ellipsis,
  FileSymlink,
  FolderGit2,
  GitBranch,
  GitPullRequestArrow,
  History,
  Link2,
  List,
  PanelLeftClose,
  PanelLeftOpen,
  Pencil,
  Plus,
  Scale,
  ShieldCheck,
  Tag,
  Trash2,
  Upload,
  Users,
} from "lucide-react";
import { type ReactNode, useEffect, useMemo, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";

import { CloneBox } from "@/components/CloneBox";
import { CodeBlock } from "@/components/CodeBlock";
import { CommitAuthors } from "@/components/CommitAuthors";
import { FileIcon, FolderIcon } from "@/components/FileIcon";
import { GoToFile } from "@/components/GoToFile";
import { Markdown } from "@/components/Markdown";
import { PageTabs } from "@/components/PageTabs";
import { RefPicker } from "@/components/RefPicker";
import { RepoFileTree } from "@/components/RepoFileTree";
import { RepoHeader } from "@/components/RepoHeader";
import { ResizableSidebar } from "@/components/ResizableSidebar";
import { RichText } from "@/components/RichText";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { usePageTitle } from "@/lib/page-title";
import {
  type RepoCommitListItem,
  type RepoFile,
  type RepoOverviewFile,
  type RepoOverviewKind,
  type RepoSrcData,
  type RepoTreeEntry,
  refPath,
  repoHeaderQuery,
} from "@/lib/queries/repo";
import { formatAbsoluteTime, formatRelativeTime } from "@/lib/relative-time";
import { resolveTheme, useTheme } from "@/lib/theme-context";
import { subUrl } from "@/lib/url";
import { cn } from "@/lib/utils";

const LINK_CLASS = "text-(--color-primary) hover:underline";
const ICON_BUTTON_CLASS =
  "grid size-8 shrink-0 place-items-center rounded-md text-(--color-muted-foreground) hover:bg-(--color-surface) hover:text-(--color-foreground)";

// splatOf is the route path of a ref, followed by a path when given.
function splatOf(ref: string, path?: string) {
  return path ? `${ref}/${path}` : ref;
}

const SIZE_UNITS = ["B", "KiB", "MiB", "GiB"];

function formatFileSize(bytes: number) {
  let size = bytes;
  let unit = 0;
  while (size >= 1024 && unit < SIZE_UNITS.length - 1) {
    size /= 1024;
    unit++;
  }
  return `${unit ? size.toFixed(1) : size} ${SIZE_UNITS[unit]}`;
}

// RepoCode browses the files of a repository at a branch, a tag or a commit:
// a directory with its README and other overview files, or a file. An empty
// repository shows how to push to it.
export function RepoCode({
  owner,
  name,
  data,
  overviewTab,
}: {
  owner: string;
  name: string;
  data: RepoSrcData;
  overviewTab?: string;
}) {
  const { t } = useTranslation();
  const { data: repo } = useSuspenseQuery(repoHeaderQuery(owner, name));
  const [treeOpen, setTreeOpen] = useState(() => localStorage.getItem(FILE_TREE_KEY) !== "closed");
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
  const toggleTree = (open: boolean) => {
    setTreeOpen(open);
    localStorage.setItem(FILE_TREE_KEY, open ? "open" : "closed");
  };
  // The tree shows beside files and directories below the root, on wide
  // screens.
  const withTree = !summary;
  const treeShown = withTree && treeOpen;

  const refPicker = (
    <RefPicker
      current={data.ref}
      type={data.refType}
      branches={data.branches}
      tags={data.tags}
      renderLink={(ref, className, children) => srcLink(data.path, ref, className, children)}
    />
  );
  const compareButton = data.pullRequest ? (
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
  ) : null;
  const goToFile = (className?: string) => (
    <GoToFile owner={repo.owner} name={repo.name} gitRef={data.ref} commitID={data.commitID} className={className} />
  );
  const addFile =
    data.kind === "dir" && data.editor === "ok" ? (
      <AddFileMenu
        newFile={
          <Link
            to="/$owner/$repo/_new/$"
            params={{ ...params, _splat: splatOf(data.ref, data.path) }}
            className={ITEM_CLASS}
          >
            <Plus className="size-4 text-(--color-muted-foreground)" aria-hidden />
            {t("repo.editor.new_file")}
          </Link>
        }
        upload={
          data.canUploadFile ? (
            <Link
              to="/$owner/$repo/_upload/$"
              params={{ ...params, _splat: splatOf(data.ref, data.path) }}
              className={ITEM_CLASS}
            >
              <Upload className="size-4 text-(--color-muted-foreground)" aria-hidden />
              {t("repo.editor.upload_file")}
            </Link>
          ) : null
        }
      />
    ) : null;
  const latestCommit = data.latestCommit ? (
    <LatestCommit
      owner={repo.owner}
      name={repo.name}
      commit={data.latestCommit}
      history={
        <Link
          to="/$owner/$repo/commits/$"
          params={{ ...params, _splat: splatOf(data.ref, data.path) }}
          className="inline-flex shrink-0 items-center gap-1 text-(--color-foreground) hover:text-(--color-primary)"
        >
          <History className="size-4" aria-hidden />
          {summary ? (
            <span>
              <strong className="font-semibold">{summary.commitCount}</strong>{" "}
              {t(summary.commitCount === 1 ? "repo.commit" : "repo.commits")}
            </span>
          ) : (
            t("repo.file_history")
          )}
        </Link>
      }
    />
  ) : null;
  const entryList =
    data.kind === "dir" ? (
      <EntryList
        owner={repo.owner}
        name={repo.name}
        entries={data.entries ?? []}
        parent={pathParts.length ? srcLink(pathParts.slice(0, -1).join("/"), data.ref, LINK_CLASS, "..") : null}
        linkOf={(entry) =>
          srcLink(data.path ? `${data.path}/${entry.name}` : entry.name, data.ref, undefined, entry.name)
        }
      />
    ) : null;
  const readme =
    data.kind === "dir" && data.overview?.length ? <Overview files={data.overview} activeTab={overviewTab} /> : null;

  if (summary) {
    return (
      <div className="flex min-h-0 flex-1 flex-col">
        <RepoHeader repo={repo} activeTab="code" />
        <main className="mx-auto w-full max-w-7xl flex-1 px-4 py-6 sm:px-6">
          <h1 className="sr-only">{fullName}</h1>
          <div className="mb-4 lg:hidden">
            <AboutText summary={summary} />
          </div>
          <div className="grid gap-8 lg:grid-cols-[minmax(0,1fr)_18rem]">
            <div className="min-w-0">
              <div className="mb-4 flex flex-wrap items-center gap-2">
                {refPicker}
                {compareButton}
                <span className="order-3 hidden flex-wrap items-center gap-x-3 gap-y-1 text-sm sm:flex">
                  <Link
                    to="/$owner/$repo/branches"
                    params={params}
                    className="inline-flex items-center gap-1 hover:text-(--color-primary)"
                  >
                    <GitBranch className="size-4 text-(--color-muted-foreground)" aria-hidden />
                    <strong className="font-semibold">{data.branches.length}</strong>
                    <span className="text-(--color-muted-foreground)">
                      {t(data.branches.length === 1 ? "repo.branch" : "repo.branches")}
                    </span>
                  </Link>
                  <Link
                    to="/$owner/$repo/releases"
                    params={params}
                    className="inline-flex items-center gap-1 hover:text-(--color-primary)"
                  >
                    <Tag className="size-4 text-(--color-muted-foreground)" aria-hidden />
                    <strong className="font-semibold">{data.tags.length}</strong>
                    <span className="text-(--color-muted-foreground)">
                      {t(data.tags.length === 1 ? "repo.tag" : "repo.tags")}
                    </span>
                  </Link>
                </span>
                {goToFile("min-w-40 flex-1 sm:order-4 sm:ml-auto sm:max-w-64")}
                <span className="order-5 flex items-center gap-2">
                  {addFile}
                  <CloneMenu links={summary.cloneLinks} archiveLink={`${repoLink}/archive/${refPath(data.ref)}`} />
                </span>
              </div>
              <div className="overflow-hidden rounded-md border border-(--color-border)">
                {latestCommit}
                {entryList}
              </div>
              {readme}
            </div>
            <aside aria-labelledby="repo-about-heading" className="hidden min-w-0 lg:block">
              <h2 id="repo-about-heading" className="mb-3 font-semibold">
                {t("repo.about")}
              </h2>
              <AboutText summary={summary} />
              <AboutDocs
                entries={data.entries ?? []}
                hasReadme={!!data.overview?.some((file) => file.kind === "readme")}
                srcLink={srcLink}
              />
              <section aria-labelledby="repo-about-releases" className="border-t border-(--color-border) pt-4">
                <h3 id="repo-about-releases" className="mb-1 font-semibold">
                  <Link
                    to="/$owner/$repo/releases"
                    params={params}
                    className="inline-flex items-center gap-2 hover:text-(--color-primary)"
                  >
                    {t("repo.releases")}
                    {data.tags.length ? (
                      <span className="rounded-full bg-(--color-surface) px-2 text-xs font-semibold">
                        {data.tags.length}
                      </span>
                    ) : null}
                  </Link>
                </h3>
                {summary.latestRelease ? (
                  <>
                    <Link
                      to="/$owner/$repo/releases"
                      params={params}
                      className="mt-3 flex min-w-0 items-start gap-2 text-sm hover:text-(--color-primary)"
                    >
                      <Tag className="mt-0.5 size-4 shrink-0 text-(--color-success)" aria-hidden />
                      <span className="min-w-0">
                        <span className="flex flex-wrap items-center gap-x-2 gap-y-1">
                          <span className="font-semibold wrap-anywhere">
                            {summary.latestRelease.title || summary.latestRelease.tagName}
                          </span>
                          {summary.latestRelease.createdAt ? (
                            <span className="rounded-full border border-(--color-success) px-2 text-xs leading-5 font-medium text-(--color-success)">
                              {t("repo.release.latest")}
                            </span>
                          ) : null}
                        </span>
                        {summary.latestRelease.createdAt ? (
                          <span className="block text-xs text-(--color-muted-foreground)">
                            <RelativeTime when={summary.latestRelease.createdAt} />
                          </span>
                        ) : null}
                      </span>
                    </Link>
                    {data.tags.length > 1 ? (
                      <Link
                        to="/$owner/$repo/releases"
                        params={params}
                        className="mt-2 inline-block text-xs text-(--color-primary) hover:underline"
                      >
                        {t(data.tags.length === 2 ? "repo.release.n_more_release" : "repo.release.n_more_releases", {
                          count: data.tags.length - 1,
                        })}
                      </Link>
                    ) : null}
                  </>
                ) : (
                  <p className="text-sm text-(--color-muted-foreground)">{t("repo.no_releases")}</p>
                )}
              </section>
            </aside>
          </div>
        </main>
      </div>
    );
  }

  const breadcrumb = (
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
      <CopyButton text={data.path} label={t("repo.copy_file_path")} />
    </nav>
  );
  // The picker and the finder move to the tree while it shows.
  const inMain = treeShown ? "lg:hidden" : undefined;

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <RepoHeader repo={repo} activeTab="code" />
      <div className={cn("flex min-h-0 w-full flex-1", !treeShown && "mx-auto max-w-7xl")}>
        {treeShown ? (
          <ResizableSidebar
            defaultWidth={300}
            minWidth={220}
            maxWidth={480}
            className="hidden border-r border-(--color-border) lg:sticky lg:top-[calc(3.5rem+1px)] lg:flex lg:h-[calc(100dvh-3.5rem-1px)] lg:self-start"
          >
            <div className="flex flex-col gap-2 p-3">
              <div className="flex items-center gap-2">
                <Tooltip>
                  <TooltipTrigger asChild>
                    <button
                      type="button"
                      onClick={() => toggleTree(false)}
                      aria-label={t("repo.hide_file_tree")}
                      aria-expanded
                      className={ICON_BUTTON_CLASS}
                    >
                      <PanelLeftClose className="size-4" aria-hidden />
                    </button>
                  </TooltipTrigger>
                  <TooltipContent>{t("repo.hide_file_tree")}</TooltipContent>
                </Tooltip>
                <h2 className="font-semibold">{t("repo.files")}</h2>
              </div>
              <div className="flex min-w-0 items-center gap-2 [&>*:first-child]:min-w-0 [&>*:first-child]:flex-1 [&>*:first-child]:justify-start">
                {refPicker}
                {compareButton}
              </div>
              {goToFile()}
            </div>
            <div className="min-h-0 flex-1">
              <RepoFileTree
                owner={repo.owner}
                name={repo.name}
                gitRef={data.ref}
                commitID={data.commitID}
                path={data.path}
              />
            </div>
          </ResizableSidebar>
        ) : null}
        <main className={cn("min-w-0 flex-1 px-4 py-6 sm:px-6", treeShown && "lg:pt-3 lg:pl-3")}>
          <h1 className="sr-only">{data.path}</h1>
          <div className="mb-3 flex flex-wrap items-center gap-2">
            {withTree && !treeOpen ? (
              <Tooltip>
                <TooltipTrigger asChild>
                  <button
                    type="button"
                    onClick={() => toggleTree(true)}
                    aria-label={t("repo.show_file_tree")}
                    aria-expanded={false}
                    className={cn(ICON_BUTTON_CLASS, "hidden border border-(--color-border) lg:grid")}
                  >
                    <PanelLeftOpen className="size-4" aria-hidden />
                  </button>
                </TooltipTrigger>
                <TooltipContent>{t("repo.show_file_tree")}</TooltipContent>
              </Tooltip>
            ) : null}
            <span className={cn("flex min-w-0 items-center gap-2", inMain)}>
              {refPicker}
              {compareButton}
            </span>
            {breadcrumb}
            <div className="ml-auto flex items-center gap-2">
              {data.kind === "dir" ? (
                <>
                  {goToFile(cn("w-56", inMain))}
                  {addFile}
                </>
              ) : (
                <FileMenu
                  permalink={
                    data.refType !== "commit"
                      ? srcLink(data.path, data.commitID, ITEM_CLASS, t("repo.file_permalink"))
                      : null
                  }
                  permalinkURL={`${window.location.origin}${subUrl(`/${repo.owner}/${repo.name}/src/${refPath(splatOf(data.commitID, data.path))}`)}`}
                  canGoToLine={!!data.file?.content}
                />
              )}
            </div>
          </div>

          {data.kind === "dir" ? (
            <div className="overflow-hidden rounded-md border border-(--color-border)">
              {latestCommit}
              {entryList}
            </div>
          ) : (
            <div className="overflow-hidden rounded-md border border-(--color-border) [&>div]:border-b-0">
              {latestCommit}
            </div>
          )}
          {readme}

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
    </div>
  );
}

const FILE_TREE_KEY = "gogs.code.fileTree";

const ITEM_CLASS =
  "flex w-full cursor-pointer items-center gap-2 rounded-sm px-2 py-1.5 text-left text-sm hover:bg-(--color-surface) focus-visible:bg-(--color-surface) focus-visible:outline-none";

function CopyButton({ text, label }: { text: string; label: string }) {
  const { t } = useTranslation();
  const [copied, setCopied] = useState(false);
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <button
          type="button"
          aria-label={label}
          onClick={() =>
            void navigator.clipboard.writeText(text).then(
              () => {
                setCopied(true);
                setTimeout(() => setCopied(false), 1500);
              },
              () => toast.error(t("copy_failed")),
            )
          }
          className={cn(ICON_BUTTON_CLASS, "size-7")}
        >
          {copied ? (
            <Check className="size-4 text-(--color-success)" aria-hidden />
          ) : (
            <Copy className="size-4" aria-hidden />
          )}
        </button>
      </TooltipTrigger>
      <TooltipContent>{copied ? t("copied") : label}</TooltipContent>
    </Tooltip>
  );
}

function AddFileMenu({ newFile, upload }: { newFile: ReactNode; upload: ReactNode }) {
  const { t } = useTranslation();
  return (
    <Popover>
      <PopoverTrigger asChild>
        <Button variant="outline" size="sm">
          {t("repo.add_file")}
          <ChevronDown className="size-3.5" aria-hidden />
        </Button>
      </PopoverTrigger>
      <PopoverContent align="end" className="flex w-48 flex-col p-1">
        {newFile}
        {upload}
      </PopoverContent>
    </Popover>
  );
}

// FileMenu holds the actions on the file that GitHub keeps behind "...":
// its permalink, copying the permalink, and jumping to a line.
function FileMenu({
  permalink,
  permalinkURL,
  canGoToLine,
}: {
  permalink: ReactNode;
  permalinkURL: string;
  canGoToLine: boolean;
}) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const [goingToLine, setGoingToLine] = useState(false);
  const [line, setLine] = useState("");
  return (
    <>
      <Popover open={open} onOpenChange={setOpen}>
        <Tooltip>
          <TooltipTrigger asChild>
            <PopoverTrigger asChild>
              <Button variant="outline" size="icon" className="size-8" aria-label={t("repo.more_file_actions")}>
                <Ellipsis className="size-4" aria-hidden />
              </Button>
            </PopoverTrigger>
          </TooltipTrigger>
          <TooltipContent>{t("repo.more_file_actions")}</TooltipContent>
        </Tooltip>
        <PopoverContent align="end" className="flex w-52 flex-col p-1" onClick={() => setOpen(false)}>
          {permalink}
          <button
            type="button"
            className={ITEM_CLASS}
            onClick={() =>
              void navigator.clipboard.writeText(permalinkURL).then(
                () => toast.success(t("copied")),
                () => toast.error(t("copy_failed")),
              )
            }
          >
            {t("repo.copy_permalink")}
          </button>
          {canGoToLine ? (
            <button type="button" className={ITEM_CLASS} onClick={() => setGoingToLine(true)}>
              {t("repo.go_to_line")}
            </button>
          ) : null}
        </PopoverContent>
      </Popover>
      <Dialog
        open={goingToLine}
        onOpenChange={(next) => {
          setGoingToLine(next);
          if (!next) setLine("");
        }}
      >
        <DialogContent className="sm:max-w-sm">
          <form
            noValidate
            className="flex min-h-0 flex-1 flex-col"
            onSubmit={(e) => {
              e.preventDefault();
              const n = Number(line);
              if (!Number.isInteger(n) || n < 1) return;
              window.location.hash = `L${n}`;
              setGoingToLine(false);
              setLine("");
            }}
          >
            <DialogHeader>
              <DialogTitle>{t("repo.go_to_line")}</DialogTitle>
              <DialogDescription className="sr-only">{t("repo.go_to_line")}</DialogDescription>
            </DialogHeader>
            <DialogBody>
              <Input
                type="number"
                min={1}
                inputMode="numeric"
                autoFocus
                aria-label={t("repo.line_number")}
                placeholder={t("repo.line_number")}
                value={line}
                onChange={(e) => setLine(e.target.value)}
              />
            </DialogBody>
            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => setGoingToLine(false)}>
                {t("cancel")}
              </Button>
              <Button type="submit">{t("repo.go_to_line_submit")}</Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </>
  );
}

function AboutText({ summary }: { summary: NonNullable<RepoSrcData["summary"]> }) {
  const { t } = useTranslation();
  return (
    <div className="mb-3 flex flex-col gap-2">
      {summary.description ? (
        <p className="whitespace-pre-line wrap-anywhere">{summary.description}</p>
      ) : (
        <p className="text-sm text-(--color-muted-foreground) italic">{t("repo.no_desc")}</p>
      )}
      {summary.website ? (
        <a
          href={summary.website}
          className="inline-flex min-w-0 items-center gap-2 text-sm font-semibold text-(--color-primary) hover:underline"
          rel="nofollow noopener noreferrer"
        >
          <Link2 className="size-4 shrink-0" aria-hidden />
          <span className="truncate">{summary.website.replace(/^https?:\/\//, "").replace(/\/$/, "")}</span>
        </a>
      ) : null}
    </div>
  );
}

const DOC_FILES = [
  { pattern: /^(licen[cs]e|copying)(\..+)?$/i, label: "repo.license", Icon: Scale },
  { pattern: /^contributing(\..+)?$/i, label: "repo.contributing", Icon: Users },
  { pattern: /^security(\..+)?$/i, label: "repo.security_policy", Icon: ShieldCheck },
] as const;

// AboutDocs links the README and the license, contributing guide and
// security policy at the root, like GitHub's About.
function AboutDocs({
  entries,
  hasReadme,
  srcLink,
}: {
  entries: RepoTreeEntry[];
  hasReadme: boolean;
  srcLink: (path: string, ref?: string, className?: string, children?: ReactNode) => ReactNode;
}) {
  const { t } = useTranslation();
  const linkClass = "flex items-center gap-2 text-sm text-(--color-muted-foreground) hover:text-(--color-primary)";
  const docs = DOC_FILES.flatMap((doc) => {
    const entry = entries.find((e) => e.type === "file" && doc.pattern.test(e.name));
    return entry ? [{ ...doc, name: entry.name }] : [];
  });
  if (!hasReadme && !docs.length) return null;
  return (
    <ul className="mb-4 flex flex-col gap-2">
      {hasReadme ? (
        <li>
          <a href="#readme" className={linkClass}>
            <BookOpen className="size-4 shrink-0" aria-hidden />
            {t("repo.readme")}
          </a>
        </li>
      ) : null}
      {docs.map(({ name, label, Icon }) => (
        <li key={name}>
          {srcLink(
            name,
            undefined,
            linkClass,
            <>
              <Icon className="size-4 shrink-0" aria-hidden />
              {t(label)}
            </>,
          )}
        </li>
      ))}
    </ul>
  );
}

// OVERVIEW_TABS are the values of the "tab" search parameter that show each
// kind of overview file, the same as on GitHub.
const OVERVIEW_TABS: Record<RepoOverviewKind, string> = {
  readme: "readme-ov-file",
  code_of_conduct: "coc-ov-file",
  contributing: "contributing-ov-file",
  license: "license-ov-file",
  security: "security-ov-file",
};

interface OutlineHeading {
  level: number;
  text: string;
  slug: string;
}

// outlineOf lists the headings of rendered Markdown that have an id, which
// Markdown gives an anchor to link to.
function outlineOf(html: string): OutlineHeading[] {
  const body = new DOMParser().parseFromString(html, "text/html").body;
  return Array.from(body.querySelectorAll<HTMLElement>("h1, h2, h3, h4, h5, h6"))
    .filter((heading) => heading.id.startsWith("user-content-"))
    .map((heading) => ({
      level: Number(heading.tagName.slice(1)),
      text: heading.textContent?.trim() ?? "",
      slug: heading.id.slice("user-content-".length),
    }));
}

// Overview shows the README of a directory below its file list, and in the
// root directory also the code of conduct, contributing guide, license and
// security policy, in tabs as on GitHub.
function Overview({ files, activeTab }: { files: RepoOverviewFile[]; activeTab?: string }) {
  const { t } = useTranslation();
  const shown = files.find((file) => OVERVIEW_TABS[file.kind] === activeTab) ?? files[0];
  const outline = useMemo(() => (shown.html ? outlineOf(shown.html) : []), [shown.html]);
  const tabs = files.map((file) => ({ key: file.kind, label: t(`repo.overview_${file.kind}`) }));

  return (
    <section
      id="readme"
      aria-label={t("repo.overview_files")}
      className="mt-6 scroll-mt-20 rounded-md border border-(--color-border)"
    >
      <div className="flex items-center justify-between gap-2 border-b border-(--color-border) pr-2 pl-1">
        <PageTabs
          label={t("repo.overview_files")}
          tabs={tabs}
          active={shown.kind}
          inline={2}
          className="min-w-0 pt-1"
          renderLink={(tab, props) => (
            <Link
              to="."
              search={{ tab: tab.key === files[0].kind ? undefined : OVERVIEW_TABS[tab.key] }}
              resetScroll={false}
              activeOptions={{ exact: true }}
              {...props}
            />
          )}
        />
        {outline.length ? <OutlineMenu headings={outline} /> : null}
      </div>
      {shown.html !== undefined ? (
        <Markdown html={shown.html} className="p-4 sm:p-6" />
      ) : (
        <pre className="p-4 font-mono text-sm wrap-anywhere whitespace-pre-wrap">{shown.text}</pre>
      )}
    </section>
  );
}

// OutlineMenu lists the headings of the shown file in a menu that can be
// filtered, like the outline of a README on GitHub.
function OutlineMenu({ headings }: { headings: OutlineHeading[] }) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const [filter, setFilter] = useState("");
  const topLevel = Math.min(...headings.map((heading) => heading.level));
  const query = filter.trim().toLowerCase();
  const shown = query ? headings.filter((heading) => heading.text.toLowerCase().includes(query)) : headings;
  const label = t("repo.overview_outline");

  return (
    <Popover
      open={open}
      onOpenChange={(next) => {
        setOpen(next);
        if (!next) setFilter("");
      }}
    >
      <Tooltip>
        <TooltipTrigger asChild>
          <PopoverTrigger asChild>
            <button type="button" aria-label={label} className={cn(ICON_BUTTON_CLASS, "cursor-pointer")}>
              <List className="size-4" aria-hidden />
            </button>
          </PopoverTrigger>
        </TooltipTrigger>
        <TooltipContent>{label}</TooltipContent>
      </Tooltip>
      <PopoverContent align="end" className="w-80 max-w-[calc(100vw-2rem)] p-2">
        <Input
          type="search"
          value={filter}
          onChange={(e) => setFilter(e.target.value)}
          placeholder={t("repo.overview_filter_headings")}
          aria-label={t("repo.overview_filter_headings")}
        />
        <ul className="mt-2 max-h-[60vh] overflow-y-auto">
          {shown.map((heading) => (
            <li key={heading.slug}>
              <a
                href={`#${encodeURIComponent(heading.slug)}`}
                onClick={() => setOpen(false)}
                className={cn(
                  "block rounded py-1.5 pr-2 text-sm wrap-anywhere text-(--color-foreground) hover:bg-(--color-surface)",
                  heading.level === topLevel && "font-semibold",
                )}
                style={{ paddingLeft: `${0.5 + (heading.level - topLevel)}rem` }}
              >
                {heading.text}
              </a>
            </li>
          ))}
        </ul>
      </PopoverContent>
    </Popover>
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

function RelativeTime({ when }: { when: string }) {
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
  return (
    <div className="flex items-center gap-3 border-b border-(--color-border) bg-(--color-surface) px-4 py-2.5 text-sm">
      <CommitAuthors
        author={commit.author}
        coAuthors={commit.coAuthors}
        className="max-w-[60%] [&_img+img]:ring-(--color-surface)"
      />
      <span
        className="hidden min-w-0 flex-1 truncate text-(--color-muted-foreground) sm:block [&_a]:text-(--color-primary) [&_a]:hover:underline"
        dangerouslySetInnerHTML={{ __html: commit.subjectHTML }}
      />
      <span className="ml-auto flex shrink-0 items-center gap-3 text-(--color-muted-foreground)">
        <span className="hidden items-center gap-1 text-xs sm:flex">
          <Link
            to="/$owner/$repo/commit/$sha"
            params={{ owner, repo: name, sha: commit.sha }}
            className="font-mono hover:text-(--color-primary) hover:underline"
          >
            {commit.sha.slice(0, 7)}
          </Link>
          <span aria-hidden>·</span>
          <RelativeTime when={commit.author.when} />
        </span>
        {history}
      </span>
    </div>
  );
}

function EntryIcon({ type, name }: { type: RepoTreeEntry["type"]; name: string }) {
  if (type === "file") return <FileIcon name={name} />;
  if (type === "dir") return <FolderIcon />;
  const Icon = type === "symlink" ? FileSymlink : FolderGit2;
  return <Icon className="size-4 shrink-0 text-(--color-muted-foreground)" aria-hidden />;
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
            <EntryIcon type={entry.type} name={entry.name} />
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
  const { lineCount, locCount } = useMemo(() => {
    const lines = file.content ? file.content.replace(/\n$/, "").split("\n") : [];
    return { lineCount: lines.length, locCount: lines.filter((line) => line.trim() !== "").length };
  }, [file]);
  const [copied, setCopied] = useState(false);
  const navigate = useNavigate();
  const plain = useSearch({ strict: false, select: (search: { plain?: unknown }) => search.plain === true });
  // Rendered files can also show their source, like GitHub's "Preview" and
  // "Code".
  const hasPreview = (file.kind === "markup" || file.kind === "notebook") && file.content !== undefined;
  const showSource = hasPreview && plain;
  const setShowSource = (source: boolean) =>
    void navigate({
      to: ".",
      search: (prev: Record<string, unknown>) => ({ ...prev, plain: source ? 1 : undefined }),
      resetScroll: false,
    });
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
      className="mt-3 overflow-hidden rounded-md border border-(--color-border)"
    >
      <div className="flex flex-wrap items-center gap-x-3 gap-y-2 border-b border-(--color-border) bg-(--color-surface) px-4 py-2">
        {hasPreview ? (
          <div
            role="group"
            aria-label={t("repo.file_view_mode")}
            className="inline-flex h-7 items-stretch overflow-hidden rounded-md border border-(--color-border) text-xs"
          >
            <ViewModeButton active={!showSource} onClick={() => setShowSource(false)}>
              {t("repo.file_preview")}
            </ViewModeButton>
            <ViewModeButton active={showSource} onClick={() => setShowSource(true)}>
              {t("repo.code")}
            </ViewModeButton>
          </div>
        ) : null}
        <h2 id="repo-file-heading" className="flex min-w-0 items-center gap-2 text-sm">
          {file.isSymlink ? (
            <FileSymlink className="size-4 shrink-0 text-(--color-muted-foreground)" aria-hidden />
          ) : file.isReadme ? (
            <BookOpen className="size-4 shrink-0 text-(--color-muted-foreground)" aria-hidden />
          ) : (
            <FileIcon name={file.name} />
          )}
          <span className="font-semibold wrap-anywhere">{file.name}</span>
        </h2>
        <span className="font-mono text-xs text-(--color-muted-foreground)">
          {lineCount
            ? `${t(lineCount === 1 ? "repo.file_line" : "repo.file_lines", { count: lineCount })} (${t("repo.file_loc", { count: locCount })}) · `
            : null}
          {formatFileSize(file.size)}
        </span>
        <div className="ml-auto flex items-center gap-1">
          <div className="mr-1 flex h-8 overflow-hidden rounded-md border border-(--color-border) text-xs [&>*]:bg-(--color-background) [&>*:hover]:bg-(--color-surface) [&>*+*]:border-l [&>*+*]:border-(--color-border)">
            <a href={rawURL} className="grid place-items-center px-2.5">
              {t("repo.file_raw")}
            </a>
            {file.content !== undefined ? (
              <Tooltip>
                <TooltipTrigger asChild>
                  <button
                    type="button"
                    aria-label={t("repo.copy_raw_file")}
                    className="grid w-8 cursor-pointer place-items-center text-(--color-muted-foreground) hover:text-(--color-foreground)"
                    onClick={() =>
                      void navigator.clipboard.writeText(file.content ?? "").then(
                        () => {
                          setCopied(true);
                          setTimeout(() => setCopied(false), 1500);
                        },
                        () => toast.error(t("copy_failed")),
                      )
                    }
                  >
                    {copied ? (
                      <Check className="size-4 text-(--color-success)" aria-hidden />
                    ) : (
                      <Copy className="size-4" aria-hidden />
                    )}
                  </button>
                </TooltipTrigger>
                <TooltipContent>{copied ? t("copied") : t("repo.copy_raw_file")}</TooltipContent>
              </Tooltip>
            ) : null}
            <Tooltip>
              <TooltipTrigger asChild>
                <a
                  href={rawURL}
                  download={file.name}
                  aria-label={t("repo.download_raw_file")}
                  className="grid w-8 place-items-center text-(--color-muted-foreground) hover:text-(--color-foreground)"
                >
                  <Download className="size-4" aria-hidden />
                </a>
              </TooltipTrigger>
              <TooltipContent>{t("repo.download_raw_file")}</TooltipContent>
            </Tooltip>
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
      {showSource ? <SourceView file={file} /> : <FileBody file={file} rawURL={rawURL} />}
    </section>
  );
}

function ViewModeButton({ active, onClick, children }: { active: boolean; onClick: () => void; children: ReactNode }) {
  return (
    <button
      type="button"
      onClick={onClick}
      aria-pressed={active}
      className={cn(
        "min-w-11 cursor-pointer px-2.5",
        active
          ? "bg-(--color-background) font-semibold text-(--color-foreground)"
          : "text-(--color-muted-foreground) hover:text-(--color-foreground)",
      )}
    >
      {children}
    </button>
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
      return <NotebookView rawURL={rawURL} />;
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
        <iframe
          title={file.name}
          src={subUrl(`/plugins/pdfjs-5.2.133/web/viewer.html?file=${encodeURIComponent(rawURL)}`)}
          className="block h-[80vh] w-full"
        />
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

// Pierre's dark theme colors punctuation #636363 and tints selected lines
// strongly, which leaves code below 4.5:1 on them. Dark mode lifts the
// punctuation and tints the selection less.
const DARK_CONTRAST_CSS = `[data-line] span[style*="--diffs-token-dark:#636363"] {
  color: light-dark(var(--diffs-token-light), #9a9a9a);
}
[data-line][data-selected-line] {
  --mix-selection-dark: 85%;
}`;

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

  // Going to a line, e.g., from the file menu, changes the hash.
  useEffect(() => {
    const onHashChange = () => {
      const range = selectedLinesOf(window.location.hash);
      if (!range) return;
      setSelected(range);
      const root = wrapper.current?.querySelector("diffs-container")?.shadowRoot;
      root?.querySelector(`[data-line="${range.start}"]`)?.scrollIntoView({ block: "center" });
    };
    window.addEventListener("hashchange", onHashChange);
    return () => window.removeEventListener("hashchange", onHashChange);
  }, []);
  const tabCSS = file.tabWidth ? `pre, code { tab-size: ${file.tabWidth}; }` : "";

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
          unsafeCSS: DARK_CONTRAST_CSS + tabCSS,
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

const NOTEBOOK_SCRIPTS = [
  "/plugins/dompurify-3.4.8/purify.min.js",
  "/plugins/marked-4.3.0/marked.min.js",
  "/plugins/notebookjs-0.8.3/notebook.min.js",
];

interface NotebookGlobal {
  nb: { parse: (notebook: unknown) => { render: () => HTMLElement } };
}

let notebookScripts: Promise<void> | undefined;

// loadNotebookScripts loads notebook.js with the libraries it renders
// Markdown cells with, once.
function loadNotebookScripts() {
  notebookScripts ??= NOTEBOOK_SCRIPTS.reduce(
    (previous, src) =>
      previous.then(
        () =>
          new Promise<void>((resolve, reject) => {
            const script = document.createElement("script");
            script.src = subUrl(src);
            script.onload = () => resolve();
            script.onerror = () => reject(new Error(`load ${src}`));
            document.head.append(script);
          }),
      ),
    Promise.resolve(),
  );
  return notebookScripts;
}

// NotebookView renders a Jupyter notebook with notebook.js, sanitized by the
// server like the server-rendered pages, and highlights its input cells.
function NotebookView({ rawURL }: { rawURL: string }) {
  const { t } = useTranslation();
  const container = useRef<HTMLDivElement>(null);
  const [html, setHTML] = useState<string>();
  const [failed, setFailed] = useState(false);
  const [cells, setCells] = useState<{ node: Element; code: string }[]>([]);

  useEffect(() => {
    let cancelled = false;
    void (async () => {
      try {
        const [notebook] = await Promise.all([
          fetch(rawURL, { credentials: "same-origin" }).then((res) => res.json() as Promise<unknown>),
          loadNotebookScripts(),
        ]);
        const rendered = (window as unknown as NotebookGlobal).nb.parse(notebook).render();
        const res = await fetch(subUrl("/api/web/sanitize-html"), {
          method: "POST",
          credentials: "same-origin",
          body: rendered.outerHTML,
        });
        if (!res.ok) throw new Error(`sanitize notebook: ${res.status}`);
        const sanitized = await res.text();
        if (!cancelled) setHTML(sanitized);
      } catch {
        if (!cancelled) setFailed(true);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [rawURL]);

  useEffect(() => {
    const root = container.current;
    if (!root || html === undefined) return;
    // Set outside of React, which would otherwise put the input cells back
    // over their highlighted code.
    root.innerHTML = html;
    // Relative image sources resolve against the notebook's directory.
    const dir = rawURL.slice(0, rawURL.lastIndexOf("/"));
    for (const img of Array.from(root.querySelectorAll<HTMLImageElement>(".nb-markdown-cell img"))) {
      const src = img.getAttribute("src") ?? "";
      if (src && !src.startsWith("/") && !/^[a-z][a-z0-9+.-]*:|^\/\//i.test(src))
        img.setAttribute("src", `${dir}/${src}`);
    }
    setCells(
      Array.from(root.querySelectorAll(".nb-input pre")).map((pre) => {
        const code = pre.textContent ?? "";
        pre.replaceChildren();
        return { node: pre, code };
      }),
    );
  }, [html, rawURL]);

  if (failed) {
    return (
      <div className="flex flex-col items-center gap-3 px-4 py-10 text-sm">
        <Button asChild variant="outline" size="sm">
          <a href={rawURL} rel="nofollow">
            {t("repo.file_view_raw")}
          </a>
        </Button>
      </div>
    );
  }
  return (
    <>
      <div ref={container} className="markdown notebook p-4 sm:p-6" />
      {cells.map((cell, i) =>
        createPortal(<CodeBlock code={cell.code.replace(/\n$/, "")} lang="python" />, cell.node, i),
      )}
    </>
  );
}

function EmptyRepo({ data }: { data: RepoSrcData }) {
  const { t } = useTranslation();
  const links = data.cloneLinks ?? {};
  const [remote, setRemote] = useState(links.ssh ?? links.https ?? "");
  if (!data.viewerCanWrite || data.isMirror) {
    return (
      <p className="rounded-md border border-(--color-border) px-4 py-10 text-center text-sm text-(--color-muted-foreground)">
        {t("repo.empty_message")}
      </p>
    );
  }
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
            <CloneBox links={links} label={t("repo.clone_this_repo")} onURLChange={setRemote} />
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
