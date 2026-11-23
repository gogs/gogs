import { useSuspenseQuery } from "@tanstack/react-query";
import { Link, useLoaderData, useParams, useSearch } from "@tanstack/react-router";
import { Check, Copy, FileCode2 } from "lucide-react";
import { useCallback, useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";

import { DiffWorkspace } from "@/components/DiffWorkspace";
import { RelativeTime } from "@/components/RelativeTime";
import { RepoHeader } from "@/components/RepoHeader";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { usePageTitle } from "@/lib/page-title";
import { repoHeaderQuery } from "@/lib/queries/repo";
import { routePath, subUrl } from "@/lib/url";
import { useAppLinkClick } from "@/lib/use-app-link-click";

import type { RepoCommitSearch } from "./Commit.search";

export interface RepoCommitSignature {
  name: string;
  email: string;
  avatarURL: string;
  profileURL?: string;
  when: string;
}

export interface RepoCommitPage {
  sha: string;
  // subject is the first line of the message as plain text, for the page title.
  subject: string;
  // The first line and the rest of the message, escaped, with links to the
  // users and issues they mention.
  subjectHTML: string;
  bodyHTML: string;
  author: RepoCommitSignature;
  parents: string[];
  // markdownExtensions are the extensions of files that show a rich diff.
  markdownExtensions: string[];
  patch: string;
}

const BODY_CLAMP_LINES = 3;

const messageLinkClass = "[&_a]:text-(--color-primary) [&_a]:hover:underline";

function CommitBody({ html }: { html: string }) {
  const { t } = useTranslation();
  // Links never span lines, so the HTML splits into whole lines.
  const lines = html.split("\n");
  const needsClamp = lines.length > BODY_CLAMP_LINES;
  const [expanded, setExpanded] = useState(!needsClamp);
  const visible = expanded ? html : lines.slice(0, BODY_CLAMP_LINES).join("\n");
  const onLinkClick = useAppLinkClick();

  return (
    <div className="mt-2 max-w-3xl">
      <pre
        className={`text-sm wrap-anywhere whitespace-pre-wrap text-(--color-muted-foreground) ${messageLinkClass}`}
        onClick={onLinkClick}
        dangerouslySetInnerHTML={{ __html: visible }}
      />
      {needsClamp ? (
        <button
          type="button"
          onClick={() => setExpanded((e) => !e)}
          className="glass-control mt-1.5 cursor-pointer rounded border border-(--color-border) bg-(--color-surface) px-2 py-0.5 text-xs text-(--color-muted-foreground) hover:bg-(--color-surface)/80 hover:text-(--color-foreground)"
        >
          {expanded ? t("show_less") : t("show_more")}
        </button>
      ) : null}
    </div>
  );
}

export function RepoCommit() {
  const data = useLoaderData({ from: "/$owner/$repo/commit/$sha" });
  const { sha, subject, subjectHTML, bodyHTML, author, parents, markdownExtensions, patch } = data;
  const { owner, repo } = useParams({ from: "/$owner/$repo/commit/$sha" });
  const search: RepoCommitSearch = useSearch({ from: "/$owner/$repo/commit/$sha" });
  const { data: repoHeader } = useSuspenseQuery(repoHeaderQuery(owner, repo));
  const { t } = useTranslation();
  const [copied, setCopied] = useState(false);
  const onLinkClick = useAppLinkClick();
  usePageTitle(`${subject} - ${owner}/${repo}@${sha.slice(0, 7)}`);

  const authorLabel = author.profileURL ? (
    <Link
      activeOptions={{ exact: true }}
      to={routePath(author.profileURL)}
      className="font-semibold text-(--color-foreground) hover:underline"
    >
      {author.name}
    </Link>
  ) : (
    <span className="font-semibold text-(--color-foreground)">{author.name}</span>
  );

  const repoLink = `/${owner}/${repo}`;
  const browseFilesHref = `${repoLink}/src/${sha}`;

  const copySha = useCallback(() => {
    void (async () => {
      try {
        await navigator.clipboard.writeText(sha);
        setCopied(true);
        window.setTimeout(() => setCopied(false), 1500);
      } catch {
        toast.error(t("copy_failed"));
      }
    })();
  }, [sha, t]);

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <RepoHeader repo={repoHeader} activeTab="code" />

      <section className="mx-auto w-full max-w-7xl px-4 pt-6 pb-4 sm:px-6">
        <div className="flex flex-wrap items-start justify-between gap-x-4 gap-y-2">
          <h2
            className={`min-w-0 flex-1 text-xl font-semibold break-words text-(--color-foreground) ${messageLinkClass}`}
            onClick={onLinkClick}
            dangerouslySetInnerHTML={{ __html: subjectHTML }}
          />
          <div className="basis-full sm:basis-auto">
            <Link
              activeOptions={{ exact: true }}
              to={browseFilesHref}
              className="glass-control inline-flex h-7 shrink-0 items-center gap-1 rounded-md border border-(--color-border) px-2 text-sm hover:bg-(--color-surface)"
            >
              <FileCode2 className="size-3.5" aria-hidden />
              <span>{t("repo.browse_files")}</span>
            </Link>
          </div>
        </div>
        {bodyHTML ? <CommitBody key={sha} html={bodyHTML} /> : null}

        <div className="mt-4 flex flex-wrap items-center gap-x-3 gap-y-2 text-sm text-(--color-muted-foreground)">
          <span className="inline-flex items-center gap-1.5">
            <img src={author.avatarURL} alt="" className="size-6 rounded-full" />
            {authorLabel}
            <span>{t("repo.authored")}</span>
            <RelativeTime time={author.when} />
          </span>
          {/* TODO: Verified pill once the backend exposes signature verification. */}

          <span aria-hidden className="hidden h-4 w-px bg-(--color-border) sm:inline-block" />

          {/* Wraps between groups on narrow screens, keeping each label with its value. Each
              separator leads its group, so a wrapped line never ends with one. */}
          <span className="inline-flex flex-wrap items-center gap-x-1 gap-y-1.5 font-mono text-xs">
            <a href={subUrl(`/${owner}/${repo}/commit/${sha}.patch`)} className="hover:underline">
              {t("repo.patch_label")}
            </a>
            <span className="inline-flex items-center gap-1">
              <span aria-hidden>·</span>
              <a href={subUrl(`/${owner}/${repo}/commit/${sha}.diff`)} className="hover:underline">
                {t("repo.diff_label")}
              </a>
            </span>
            {parents.length > 0 ? (
              <span className="inline-flex items-center gap-1">
                <span aria-hidden>·</span>
                <span>
                  {parents.length > 1
                    ? `${parents.length} ${t("repo.parents", { count: parents.length })}`
                    : t("repo.commit_parent")}
                </span>
                {parents.map((p) => (
                  <Link
                    activeOptions={{ exact: true }}
                    key={p}
                    to={`/${owner}/${repo}/commit/${p}`}
                    className="rounded bg-(--color-surface) px-1.5 py-0.5 text-(--color-foreground) hover:underline"
                  >
                    {p.slice(0, 7)}
                  </Link>
                ))}
              </span>
            ) : null}
            <span className="inline-flex items-center gap-1">
              <span aria-hidden>·</span>
              <span>{t("repo.commit_label")}</span>
              <code className="rounded bg-(--color-surface) px-1.5 py-0.5 text-(--color-foreground)">
                {sha.slice(0, 10)}
              </code>
              <Tooltip>
                <TooltipTrigger asChild>
                  <button
                    type="button"
                    onClick={copySha}
                    aria-label={t("repo.copy_full_sha")}
                    className="grid size-6 cursor-pointer place-items-center rounded hover:bg-(--color-surface)"
                  >
                    {copied ? (
                      <Check className="size-3.5 text-(--color-success)" aria-hidden />
                    ) : (
                      <Copy className="size-3.5" aria-hidden />
                    )}
                  </button>
                </TooltipTrigger>
                <TooltipContent>{t("repo.copy_full_sha")}</TooltipContent>
              </Tooltip>
            </span>
          </span>
        </div>
      </section>

      <DiffWorkspace
        // Files keep their ids from commit to commit, which would keep showing
        // the diff of the previous commit.
        key={sha}
        owner={owner}
        repo={repo}
        patch={patch}
        oldRef={parents[0]}
        newRef={sha}
        markdownExtensions={markdownExtensions}
        search={search}
      />
    </div>
  );
}
