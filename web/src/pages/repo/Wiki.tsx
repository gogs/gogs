import { useMutation, useSuspenseQuery } from "@tanstack/react-query";
import { Link, useNavigate } from "@tanstack/react-router";
import { BookOpen, FileText, Pencil, Plus } from "lucide-react";
import { type FormEvent, type ReactNode, useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";

import { CloneBox } from "@/components/CloneBox";
import { ConfirmButton } from "@/components/ConfirmButton";
import { FormAlert, FormField } from "@/components/FormField";
import { MarkdownEditor } from "@/components/IssueForm";
import { Markdown } from "@/components/Markdown";
import { RepoHeader } from "@/components/RepoHeader";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { usePageTitle } from "@/lib/page-title";
import {
  type RepoCloneLinks,
  type RepoWikiData,
  type RepoWikiPagesData,
  deleteRepoWikiPage,
  repoHeaderQuery,
  saveRepoWikiPage,
} from "@/lib/queries/repo";
import { formErrors } from "@/lib/queries/request";
import { formatAbsoluteTime, formatRelativeTime } from "@/lib/relative-time";
import { cn } from "@/lib/utils";

const LINK_CLASS = "text-(--color-primary) hover:underline";

function WikiLayout({
  owner,
  name,
  title,
  children,
}: {
  owner: string;
  name: string;
  title: string;
  children: ReactNode;
}) {
  const { data: repo } = useSuspenseQuery(repoHeaderQuery(owner, name));
  usePageTitle(`${title} - ${repo.owner}/${repo.name}`);
  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <RepoHeader repo={repo} activeTab="wiki" />
      <main className="mx-auto w-full max-w-7xl flex-1 px-4 py-6 sm:px-6">{children}</main>
    </div>
  );
}

// fill puts the values in the placeholders of a translated sentence, in order.
function fill(text: string, values: ReactNode[]) {
  let next = 0;
  return text.split(/(%(?:\[\d\])?[sd])/).map((part, i) => {
    if (!/^%(?:\[\d\])?[sd]$/.test(part)) return part;
    return <span key={i}>{values[next++]}</span>;
  });
}

function RelativeTime({ when }: { when: string }) {
  const { t } = useTranslation();
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <time dateTime={when}>{formatRelativeTime(t, when)}</time>
      </TooltipTrigger>
      <TooltipContent>{formatAbsoluteTime(when)}</TooltipContent>
    </Tooltip>
  );
}

function PageLink({
  owner,
  name,
  page,
  className,
  children,
}: {
  owner: string;
  name: string;
  page: string;
  className?: string;
  children: ReactNode;
}) {
  return (
    <Link to="/$owner/$repo/wiki/$page" params={{ owner, repo: name, page }} className={className}>
      {children}
    </Link>
  );
}

function NewPageButton({ owner, name, primary }: { owner: string; name: string; primary?: boolean }) {
  const { t } = useTranslation();
  return (
    <Button asChild variant={primary ? "default" : "outline"} size="sm">
      <Link to="/$owner/$repo/wiki/_new" params={{ owner, repo: name }}>
        <Plus className="size-4" aria-hidden />
        {t("repo.wiki.new_page_button")}
      </Link>
    </Button>
  );
}

// WikiStart welcomes viewers to a wiki that has no pages yet.
function WikiStart({ owner, name, canWrite }: { owner: string; name: string; canWrite: boolean }) {
  const { t } = useTranslation();
  return (
    <section className="flex flex-col items-center rounded-md border border-(--color-border) px-6 py-16 text-center">
      <BookOpen className="size-12 text-(--color-muted-foreground)" aria-hidden />
      <h1 className="mt-4 text-2xl font-semibold">{t("repo.wiki.welcome")}</h1>
      <p className="mt-2 max-w-prose text-(--color-muted-foreground)">{t("repo.wiki.welcome_desc")}</p>
      {canWrite ? (
        <Button asChild className="mt-6">
          <Link to="/$owner/$repo/wiki/_new" params={{ owner, repo: name }}>
            {t("repo.wiki.create_first_page")}
          </Link>
        </Button>
      ) : null}
    </section>
  );
}

// RepoWiki shows a page of the wiki of a repository, with the list of its
// pages and the links to clone it.
export function RepoWiki({ owner, name, data }: { owner: string; name: string; data: RepoWikiData }) {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const page = data.page;
  if (!data.hasWiki || !page) {
    return (
      <WikiLayout owner={owner} name={name} title={t("repo.wiki")}>
        <WikiStart owner={owner} name={name} canWrite={data.viewerCanWrite} />
      </WikiLayout>
    );
  }

  return (
    <WikiLayout owner={owner} name={name} title={page.name}>
      <div className="grid gap-8 md:grid-cols-[minmax(0,1fr)_16rem] lg:grid-cols-[minmax(0,1fr)_18rem]">
        <article className="min-w-0">
          <header className="mb-6 border-b border-(--color-border) pb-4">
            <div className="flex flex-wrap items-start justify-between gap-x-4 gap-y-3">
              <h1 className="min-w-0 text-2xl font-semibold break-words">{page.name}</h1>
              {data.viewerCanWrite ? (
                <div className="flex flex-wrap items-center gap-2">
                  <Button asChild variant="outline" size="sm">
                    <Link
                      to="/$owner/$repo/wiki/$page/_edit"
                      params={{ owner, repo: name, page: page.name }}
                      aria-label={`${t("repo.wiki.edit_page_button")} ${page.name}`}
                    >
                      <Pencil className="size-4" aria-hidden />
                      {t("repo.wiki.edit_page_button")}
                    </Link>
                  </Button>
                  <NewPageButton owner={owner} name={name} primary />
                  <ConfirmButton
                    label={t("repo.wiki.delete_page_button")}
                    size="sm"
                    title={t("repo.wiki.delete_page_button")}
                    description={t("repo.wiki.delete_page_notice_1")
                      .replace(/<\/?code>/g, "")
                      .replace(/%(?:\[1\])?s/, page.name)}
                    action={() => deleteRepoWikiPage(owner, name, page.name)}
                    onDone={async () => {
                      toast.success(t("repo.wiki.delete_page_success"));
                      await navigate({ to: "/$owner/$repo/wiki", params: { owner, repo: name } });
                    }}
                  />
                </div>
              ) : null}
            </div>
            <p className="mt-2 text-sm text-(--color-muted-foreground)">
              {fill(t("repo.wiki.last_commit_info"), [
                <strong key="author" className="font-medium text-(--color-foreground)">
                  {page.author.name}
                </strong>,
                <RelativeTime key="when" when={page.author.when} />,
              ])}
            </p>
          </header>
          <Markdown html={page.html} />
        </article>
        <aside className="flex min-w-0 flex-col gap-6 max-md:border-t max-md:border-(--color-border) max-md:pt-6">
          <PageList owner={owner} name={name} pages={data.pages ?? []} current={page.name} />
          {data.cloneLinks?.https || data.cloneLinks?.ssh ? <WikiCloneBox links={data.cloneLinks} /> : null}
        </aside>
      </div>
    </WikiLayout>
  );
}

function PageList({ owner, name, pages, current }: { owner: string; name: string; pages: string[]; current: string }) {
  const { t } = useTranslation();
  const [filter, setFilter] = useState("");
  const shown = pages.filter((p) => p.toLowerCase().includes(filter.trim().toLowerCase()));
  return (
    <section aria-labelledby="wiki-pages-heading">
      <h2 id="wiki-pages-heading" className="mb-2 flex items-center gap-2 text-sm font-semibold">
        <Link
          to="/$owner/$repo/wiki/_pages"
          params={{ owner, repo: name }}
          className="hover:text-(--color-primary) hover:underline"
        >
          {t("repo.wiki.pages")}
        </Link>
        <span className="rounded-full bg-(--color-surface) px-1.5 text-xs leading-5 font-normal tabular-nums text-(--color-muted-foreground)">
          {pages.length}
        </span>
      </h2>
      {pages.length > 5 ? (
        <Input
          type="search"
          value={filter}
          onChange={(e) => setFilter(e.target.value)}
          placeholder={t("repo.wiki.filter_page")}
          aria-label={t("repo.wiki.filter_page")}
          className="mb-2 h-8 text-sm"
        />
      ) : null}
      <ul className="max-h-96 overflow-y-auto rounded-md border border-(--color-border) text-sm">
        {shown.map((p) => (
          <li key={p} className="border-b border-(--color-border) last:border-b-0">
            <PageLink
              owner={owner}
              name={name}
              page={p}
              className={cn(
                "flex items-center gap-2 px-3 py-2 break-all hover:bg-(--color-surface)",
                p === current ? "font-semibold text-(--color-foreground)" : "text-(--color-primary)",
              )}
            >
              <FileText className="size-4 shrink-0 text-(--color-muted-foreground)" aria-hidden />
              <span aria-current={p === current ? "page" : undefined}>{p}</span>
            </PageLink>
          </li>
        ))}
      </ul>
    </section>
  );
}

function WikiCloneBox({ links }: { links: RepoCloneLinks }) {
  const { t } = useTranslation();
  return (
    <section aria-labelledby="wiki-clone-heading">
      <h2 id="wiki-clone-heading" className="mb-2 text-sm font-semibold">
        {t("repo.wiki.clone")}
      </h2>
      <CloneBox links={links} label={t("repo.wiki.clone")} />
    </section>
  );
}

// RepoWikiPages lists every page of the wiki with when it last changed.
export function RepoWikiPages({ owner, name, data }: { owner: string; name: string; data: RepoWikiPagesData }) {
  const { t } = useTranslation();
  return (
    <WikiLayout owner={owner} name={name} title={t("repo.wiki.pages")}>
      <div className="mb-4 flex flex-wrap items-center justify-between gap-2">
        <h1 className="text-2xl font-semibold">{t("repo.wiki.pages")}</h1>
        {data.viewerCanWrite ? <NewPageButton owner={owner} name={name} /> : null}
      </div>
      {data.pages.length ? (
        <ul className="divide-y divide-(--color-border) rounded-md border border-(--color-border)">
          {data.pages.map((p) => (
            <li key={p.name} className="flex flex-wrap items-center gap-x-4 gap-y-1 px-4 py-3 text-sm">
              <PageLink
                owner={owner}
                name={name}
                page={p.name}
                className={`${LINK_CLASS} flex min-w-0 items-center gap-2 font-medium break-all`}
              >
                <FileText className="size-4 shrink-0 text-(--color-muted-foreground)" aria-hidden />
                {p.name}
              </PageLink>
              <span className="text-(--color-muted-foreground) sm:ml-auto">
                {fill(t("repo.wiki.last_updated"), [<RelativeTime key="when" when={p.updatedAt} />])}
              </span>
            </li>
          ))}
        </ul>
      ) : (
        <WikiStart owner={owner} name={name} canWrite={data.viewerCanWrite} />
      )}
    </WikiLayout>
  );
}

// RepoWikiEditor is the form to create a page, or to change one when `page`
// is given.
export function RepoWikiEditor({
  owner,
  name,
  page,
  defaultTitle = "",
}: {
  owner: string;
  name: string;
  page?: { name: string; content: string };
  // defaultTitle names a new page, e.g., "Home" for the first page.
  defaultTitle?: string;
}) {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const [title, setTitle] = useState(page?.name ?? defaultTitle);
  const [content, setContent] = useState(page?.content ?? "");
  const [message, setMessage] = useState("");
  const save = useMutation({
    mutationFn: () => saveRepoWikiPage(owner, name, { oldTitle: page?.name, title, content, message }),
    onSuccess: (saved) => navigate({ to: "/$owner/$repo/wiki/$page", params: { owner, repo: name, page: saved } }),
  });
  const { fields, formError } = formErrors(save.error);
  const onSubmit = (e: FormEvent) => {
    e.preventDefault();
    save.mutate();
  };
  const heading = page ? t("repo.wiki.edit_page") : t("repo.wiki.new_page");

  return (
    <WikiLayout owner={owner} name={name} title={heading}>
      <div className="mb-4 flex flex-wrap items-center justify-between gap-2">
        <h1 className="text-2xl font-semibold">{heading}</h1>
        {page ? <NewPageButton owner={owner} name={name} /> : null}
      </div>
      <form onSubmit={onSubmit} noValidate>
        <fieldset disabled={save.isPending} className="flex flex-col gap-5">
          <FormAlert message={formError} />
          <FormField
            id="wiki-title"
            label={t("repo.wiki.page_title")}
            error={fields.title}
            invalid={Boolean(fields.title)}
          >
            <Input
              id="wiki-title"
              value={title}
              required
              autoFocus={!page}
              onChange={(e) => setTitle(e.target.value)}
            />
          </FormField>
          <div className="rounded-md border border-(--color-border) p-3">
            <MarkdownEditor
              repo={{ owner, name }}
              label={t("repo.wiki.page_content")}
              value={content}
              onChange={setContent}
              error={fields.content}
              autoFocus={Boolean(page)}
              disabled={save.isPending}
              minLines={16}
            />
          </div>
          <FormField
            id="wiki-message"
            label={t("repo.wiki.commit_message")}
            helper={t("repo.wiki.default_commit_message")}
            invalid={false}
          >
            <Input id="wiki-message" value={message} onChange={(e) => setMessage(e.target.value)} />
          </FormField>
          <div className="flex flex-wrap items-center gap-2 border-t border-(--color-border) pt-5">
            <Button type="submit">{t("repo.wiki.save_page")}</Button>
            <Button asChild variant="ghost">
              {page ? (
                <Link to="/$owner/$repo/wiki/$page" params={{ owner, repo: name, page: page.name }}>
                  {t("cancel")}
                </Link>
              ) : (
                <Link to="/$owner/$repo/wiki" params={{ owner, repo: name }}>
                  {t("cancel")}
                </Link>
              )}
            </Button>
          </div>
        </fieldset>
      </form>
    </WikiLayout>
  );
}
