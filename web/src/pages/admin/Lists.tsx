import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Link, useNavigate, useRouter } from "@tanstack/react-router";
import { Pencil, Plus } from "lucide-react";
import { type ReactNode, useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";

import { AdminPageHeader } from "@/components/AdminPage";
import { ConfirmButton } from "@/components/ConfirmButton";
import { Pagination } from "@/components/Pagination";
import { SearchForm } from "@/components/SearchForm";
import { Button } from "@/components/ui/button";
import { Card, CardFooter } from "@/components/ui/card";
import { Checkbox } from "@/components/ui/checkbox";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { webContext } from "@/lib/context";
import { fill, formatNumber } from "@/lib/format";
import type { ListSearch } from "@/lib/page-search";
import {
  type AdminAuthSource,
  type AdminList,
  type AdminNotice,
  type AdminOrg,
  type AdminRepo,
  type AdminUser,
  deleteAdminNotices,
  deleteAdminRepo,
  deleteAllAdminNotices,
} from "@/lib/queries/admin";
import { cn } from "@/lib/utils";

const DATE_FMT = new Intl.DateTimeFormat(webContext.lang, { year: "numeric", month: "short", day: "2-digit" });

const BADGE_CLASS =
  "rounded-full border border-(--color-border) px-2 py-0.5 text-xs font-normal whitespace-nowrap text-(--color-muted-foreground)";

// The link is at least 24px in both directions, however short the name is.
const NAME_LINK =
  "inline-flex min-h-6 min-w-6 items-center font-medium wrap-anywhere text-(--color-primary) hover:underline";

const MUTED_CELL = "text-(--color-muted-foreground)";
const NUMBER_CELL = "w-px text-right whitespace-nowrap tabular-nums";
// A cell that is only as wide as what it holds, so the name gets the rest.
const FIT_CELL = "w-px whitespace-nowrap";
const ACTION_CELL = "w-px text-right";

// A table hides a column until its card is wide enough for it, which depends
// on the sidebar of the console as well as on the window. Until then the name
// of the row tells what the column would, in a `Fact` of the same width.
type CardWidth = "xl" | "2xl" | "3xl" | "4xl";

const SHOWN_FROM: Record<CardWidth, string> = {
  xl: "hidden @xl:table-cell",
  "2xl": "hidden @2xl:table-cell",
  "3xl": "hidden @3xl:table-cell",
  "4xl": "hidden @4xl:table-cell",
};

const HIDDEN_FROM: Record<CardWidth, string> = {
  xl: "@xl:hidden",
  "2xl": "@2xl:hidden",
  "3xl": "@3xl:hidden",
  "4xl": "@4xl:hidden",
};

// Facts holds the facts below the name of a row. Give it the width from which
// every column shows.
function Facts({ until, children }: { until: CardWidth; children: ReactNode }) {
  return (
    <p
      className={cn(
        "mt-0.5 flex flex-wrap gap-x-3 gap-y-0.5 text-xs text-(--color-muted-foreground) tabular-nums",
        HIDDEN_FROM[until],
      )}
    >
      {children}
    </p>
  );
}

function Fact({ until, children }: { until: CardWidth; children: ReactNode }) {
  return <span className={HIDDEN_FROM[until]}>{children}</span>;
}

function Avatar({ src, square = false }: { src: string; square?: boolean }) {
  return (
    <img
      src={src}
      alt=""
      className={cn(
        "size-8 shrink-0 border border-(--color-border) object-cover",
        square ? "rounded-md" : "rounded-full",
      )}
    />
  );
}

// ListPage heads a list with its title, the number of all its items and its
// actions, and shows one page of it as a table in a card, or that there are
// no items, with the links to the other pages in the footer of the card.
function ListPage<T>({
  title,
  list,
  actions,
  search,
  head,
  children,
}: {
  title: string;
  list: AdminList<T>;
  actions?: ReactNode;
  // Given for a list that can be searched by keyword.
  search?: ListSearch;
  head: ReactNode;
  children: (item: T) => ReactNode;
}) {
  const { t } = useTranslation();
  return (
    <div className="grid gap-4">
      <AdminPageHeader title={title} description={fill(t("admin.total"), formatNumber(list.total))}>
        {actions}
      </AdminPageHeader>
      {search ? (
        <div className="sm:max-w-md">
          <SearchForm q={search.q} />
        </div>
      ) : null}
      <Card className="@container min-w-0 overflow-hidden">
        {list.items.length > 0 ? (
          <Table>
            <TableHeader className="bg-(--color-surface)">
              <TableRow className="hover:bg-transparent">{head}</TableRow>
            </TableHeader>
            <TableBody>{list.items.map(children)}</TableBody>
          </Table>
        ) : (
          <p className="px-5 py-10 text-center text-sm text-(--color-muted-foreground) sm:px-6">
            {t("repo.pulls.no_results")}
          </p>
        )}
        {list.totalPages > 1 ? (
          <CardFooter className="justify-center border-t border-(--color-border) py-3 sm:py-3">
            <Pagination page={list.page} totalPages={list.totalPages} />
          </CardFooter>
        ) : null}
      </Card>
    </div>
  );
}

// useReloadList reloads a list after items on the page shown were deleted.
// It moves to `page` instead when the deletion leaves the page shown empty.
function useReloadList(current: number) {
  const router = useRouter();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  return async (page: number) => {
    // The sidebar of the console counts the system notices.
    void queryClient.invalidateQueries({ queryKey: ["admin", "notices"] });
    if (page >= current) {
      await router.invalidate();
      return;
    }
    await navigate({
      to: ".",
      search: (prev: Record<string, unknown>) => ({ ...prev, page: page > 1 ? page : undefined }),
    });
  };
}

// AdminUsers lists the users of the site.
export function AdminUsers({ list, search }: { list: AdminList<AdminUser>; search: ListSearch }) {
  const { t } = useTranslation();

  return (
    <ListPage
      title={t("admin.users")}
      list={list}
      search={search}
      actions={
        <Button asChild variant="outline" size="sm">
          <Link to="/admin/users/new">
            <Plus className="size-4" aria-hidden />
            {t("admin.users.new_account")}
          </Link>
        </Button>
      }
      head={
        <>
          <TableHead>{t("username")}</TableHead>
          <TableHead className={SHOWN_FROM.xl}>{t("email")}</TableHead>
          <TableHead className={cn(SHOWN_FROM["3xl"], "text-right")}>ID</TableHead>
          <TableHead className={cn(SHOWN_FROM["3xl"], "text-right")}>{t("admin.users.repos")}</TableHead>
          <TableHead className={SHOWN_FROM["3xl"]}>{t("admin.users.created")}</TableHead>
          <TableHead>
            <span className="sr-only">{t("admin.users.edit")}</span>
          </TableHead>
        </>
      }
    >
      {(user) => {
        const created = DATE_FMT.format(new Date(user.createdAt));
        return (
          <TableRow key={user.id}>
            <TableCell>
              <div className="flex min-w-0 items-center gap-3">
                <Avatar src={user.avatarURL} />
                <div className="min-w-0">
                  <p className="flex flex-wrap items-center gap-x-2 gap-y-1">
                    <Link to="/$username" params={{ username: user.name }} search={{}} className={NAME_LINK}>
                      {user.name}
                    </Link>
                    {user.isAdmin ? <span className={BADGE_CLASS}>{t("admin.users.admin")}</span> : null}
                    {user.isActive ? null : <span className={BADGE_CLASS}>{t("admin.users.not_activated")}</span>}
                  </p>
                  <p className={cn("wrap-anywhere text-(--color-muted-foreground)", HIDDEN_FROM.xl)}>{user.email}</p>
                  <Facts until="3xl">
                    <Fact until="3xl">ID {user.id}</Fact>
                    <Fact until="3xl">
                      {t("admin.users.repos")} {user.repoCount}
                    </Fact>
                    <Fact until="3xl">
                      {t("admin.users.created")} {created}
                    </Fact>
                  </Facts>
                </div>
              </div>
            </TableCell>
            <TableCell className={cn(SHOWN_FROM.xl, MUTED_CELL, "wrap-anywhere")}>{user.email}</TableCell>
            <TableCell className={cn(SHOWN_FROM["3xl"], NUMBER_CELL, MUTED_CELL)}>{user.id}</TableCell>
            <TableCell className={cn(SHOWN_FROM["3xl"], NUMBER_CELL)}>{user.repoCount}</TableCell>
            <TableCell className={cn(SHOWN_FROM["3xl"], FIT_CELL)}>{created}</TableCell>
            <TableCell className={ACTION_CELL}>
              <Button asChild variant="outline" size="sm" className="shrink-0">
                <Link
                  to="/admin/users/$id"
                  params={{ id: String(user.id) }}
                  aria-label={`${t("admin.users.edit")} ${user.name}`}
                >
                  <Pencil className="size-3.5" aria-hidden />
                  {t("admin.users.edit")}
                </Link>
              </Button>
            </TableCell>
          </TableRow>
        );
      }}
    </ListPage>
  );
}

// AdminAuthSources lists the authentication sources users can sign in
// through, besides local accounts.
export function AdminAuthSources({ sources }: { sources: AdminAuthSource[] }) {
  const { t } = useTranslation();
  return (
    <ListPage
      title={t("admin.auths.auth_sources")}
      list={{ items: sources, page: 1, totalPages: 1, total: sources.length }}
      actions={
        <Button asChild variant="outline" size="sm">
          <Link to="/admin/auths/new">
            <Plus className="size-4" aria-hidden />
            {t("admin.auths.new")}
          </Link>
        </Button>
      }
      head={
        <>
          <TableHead>{t("admin.auths.auth_name")}</TableHead>
          <TableHead className={cn(SHOWN_FROM["3xl"], "text-right")}>ID</TableHead>
          <TableHead className={SHOWN_FROM.xl}>{t("admin.auths.auth_type")}</TableHead>
          <TableHead className={SHOWN_FROM["3xl"]}>{t("admin.auths.updated")}</TableHead>
          <TableHead className={SHOWN_FROM["3xl"]}>{t("admin.users.created")}</TableHead>
          <TableHead>
            <span className="sr-only">{t("admin.users.edit")}</span>
          </TableHead>
        </>
      }
    >
      {(source) => {
        const updated = source.updatedAt ? DATE_FMT.format(new Date(source.updatedAt)) : null;
        const created = source.createdAt ? DATE_FMT.format(new Date(source.createdAt)) : null;
        return (
          <TableRow key={source.id}>
            <TableCell>
              <p className="flex flex-wrap items-center gap-x-2 gap-y-1">
                <Link to="/admin/auths/$id" params={{ id: String(source.id) }} className={NAME_LINK}>
                  {source.name}
                </Link>
                {source.isDefault ? <span className={BADGE_CLASS}>{t("admin.auths.default")}</span> : null}
                {source.isActive ? null : <span className={BADGE_CLASS}>{t("admin.users.not_activated")}</span>}
              </p>
              <Facts until="3xl">
                <Fact until="3xl">ID {source.id}</Fact>
                <Fact until="xl">{source.typeName}</Fact>
                {updated ? (
                  <Fact until="3xl">
                    {t("admin.auths.updated")} {updated}
                  </Fact>
                ) : null}
                {created ? (
                  <Fact until="3xl">
                    {t("admin.users.created")} {created}
                  </Fact>
                ) : null}
              </Facts>
            </TableCell>
            <TableCell className={cn(SHOWN_FROM["3xl"], NUMBER_CELL, MUTED_CELL)}>{source.id}</TableCell>
            <TableCell className={SHOWN_FROM.xl}>{source.typeName}</TableCell>
            <TableCell className={cn(SHOWN_FROM["3xl"], FIT_CELL)}>{updated}</TableCell>
            <TableCell className={cn(SHOWN_FROM["3xl"], FIT_CELL)}>{created}</TableCell>
            <TableCell className={ACTION_CELL}>
              <Button asChild variant="outline" size="sm" className="shrink-0">
                <Link
                  to="/admin/auths/$id"
                  params={{ id: String(source.id) }}
                  aria-label={`${t("admin.users.edit")} ${source.name}`}
                >
                  <Pencil className="size-3.5" aria-hidden />
                  {t("admin.users.edit")}
                </Link>
              </Button>
            </TableCell>
          </TableRow>
        );
      }}
    </ListPage>
  );
}

// AdminOrgs lists the organizations of the site.
export function AdminOrgs({ list, search }: { list: AdminList<AdminOrg>; search: ListSearch }) {
  const { t } = useTranslation();
  return (
    <ListPage
      title={t("admin.organizations")}
      list={list}
      search={search}
      head={
        <>
          <TableHead>{t("organization")}</TableHead>
          <TableHead className={cn(SHOWN_FROM["3xl"], "text-right")}>ID</TableHead>
          <TableHead className={cn(SHOWN_FROM.xl, "text-right")}>{t("admin.orgs.teams")}</TableHead>
          <TableHead className={cn(SHOWN_FROM.xl, "text-right")}>{t("admin.orgs.members")}</TableHead>
          <TableHead className={cn(SHOWN_FROM.xl, "text-right")}>{t("admin.users.repos")}</TableHead>
          <TableHead className={SHOWN_FROM["3xl"]}>{t("admin.users.created")}</TableHead>
          <TableHead>
            <span className="sr-only">{t("admin.users.edit")}</span>
          </TableHead>
        </>
      }
    >
      {(org) => {
        const created = DATE_FMT.format(new Date(org.createdAt));
        return (
          <TableRow key={org.id}>
            <TableCell>
              <div className="flex min-w-0 items-center gap-3">
                <Avatar src={org.avatarURL} square />
                <div className="min-w-0">
                  <Link to="/$username" params={{ username: org.name }} search={{}} className={NAME_LINK}>
                    {org.name}
                  </Link>
                  <Facts until="3xl">
                    <Fact until="3xl">ID {org.id}</Fact>
                    <Fact until="xl">
                      {t("admin.orgs.teams")} {org.teamCount}
                    </Fact>
                    <Fact until="xl">
                      {t("admin.orgs.members")} {org.memberCount}
                    </Fact>
                    <Fact until="xl">
                      {t("admin.users.repos")} {org.repoCount}
                    </Fact>
                    <Fact until="3xl">
                      {t("admin.users.created")} {created}
                    </Fact>
                  </Facts>
                </div>
              </div>
            </TableCell>
            <TableCell className={cn(SHOWN_FROM["3xl"], NUMBER_CELL, MUTED_CELL)}>{org.id}</TableCell>
            <TableCell className={cn(SHOWN_FROM.xl, NUMBER_CELL)}>{org.teamCount}</TableCell>
            <TableCell className={cn(SHOWN_FROM.xl, NUMBER_CELL)}>{org.memberCount}</TableCell>
            <TableCell className={cn(SHOWN_FROM.xl, NUMBER_CELL)}>{org.repoCount}</TableCell>
            <TableCell className={cn(SHOWN_FROM["3xl"], FIT_CELL)}>{created}</TableCell>
            <TableCell className={ACTION_CELL}>
              <Button asChild variant="outline" size="sm" className="shrink-0">
                <Link
                  to="/org/$org/settings"
                  params={{ org: org.name }}
                  aria-label={`${t("admin.users.edit")} ${org.name}`}
                >
                  <Pencil className="size-3.5" aria-hidden />
                  {t("admin.users.edit")}
                </Link>
              </Button>
            </TableCell>
          </TableRow>
        );
      }}
    </ListPage>
  );
}

// AdminRepos lists the repositories of the site, including private ones, and
// deletes them.
export function AdminRepos({ list, search }: { list: AdminList<AdminRepo>; search: ListSearch }) {
  const { t } = useTranslation();
  const reload = useReloadList(list.page);
  const description = `${t("repo.settings.delete_desc")} ${t("repo.settings.delete_notices_2").replace(/^-\s*/, "")}`;
  return (
    <ListPage
      title={t("admin.repositories")}
      list={list}
      search={search}
      head={
        <>
          <TableHead>{t("repository")}</TableHead>
          <TableHead className={cn(SHOWN_FROM["4xl"], "text-right")}>ID</TableHead>
          <TableHead className={cn(SHOWN_FROM["2xl"], "text-right")}>{t("admin.repos.watches")}</TableHead>
          <TableHead className={cn(SHOWN_FROM["2xl"], "text-right")}>{t("admin.repos.stars")}</TableHead>
          <TableHead className={cn(SHOWN_FROM["2xl"], "text-right")}>{t("admin.repos.issues")}</TableHead>
          <TableHead className={cn(SHOWN_FROM["2xl"], "text-right")}>{t("admin.repos.size")}</TableHead>
          <TableHead className={SHOWN_FROM["4xl"]}>{t("admin.users.created")}</TableHead>
          <TableHead>
            <span className="sr-only">{t("admin.repos.delete")}</span>
          </TableHead>
        </>
      }
    >
      {(repo) => {
        const created = DATE_FMT.format(new Date(repo.createdAt));
        return (
          <TableRow key={repo.id}>
            <TableCell>
              <p className="flex flex-wrap items-center gap-x-2 gap-y-1">
                <Link activeOptions={{ exact: true }} to={`/${repo.owner}/${repo.name}`} className={NAME_LINK}>
                  {repo.owner}/{repo.name}
                </Link>
                {repo.isPrivate ? <span className={BADGE_CLASS}>{t("admin.repos.private")}</span> : null}
              </p>
              <Facts until="4xl">
                <Fact until="4xl">ID {repo.id}</Fact>
                <Fact until="2xl">
                  {t("admin.repos.watches")} {repo.watchCount}
                </Fact>
                <Fact until="2xl">
                  {t("admin.repos.stars")} {repo.starCount}
                </Fact>
                <Fact until="2xl">
                  {t("admin.repos.issues")} {repo.issueCount}
                </Fact>
                <Fact until="2xl">
                  {t("admin.repos.size")} {repo.size}
                </Fact>
                <Fact until="4xl">
                  {t("admin.users.created")} {created}
                </Fact>
              </Facts>
            </TableCell>
            <TableCell className={cn(SHOWN_FROM["4xl"], NUMBER_CELL, MUTED_CELL)}>{repo.id}</TableCell>
            <TableCell className={cn(SHOWN_FROM["2xl"], NUMBER_CELL)}>{repo.watchCount}</TableCell>
            <TableCell className={cn(SHOWN_FROM["2xl"], NUMBER_CELL)}>{repo.starCount}</TableCell>
            <TableCell className={cn(SHOWN_FROM["2xl"], NUMBER_CELL)}>{repo.issueCount}</TableCell>
            <TableCell className={cn(SHOWN_FROM["2xl"], NUMBER_CELL)}>{repo.size}</TableCell>
            <TableCell className={cn(SHOWN_FROM["4xl"], FIT_CELL)}>{created}</TableCell>
            <TableCell className={ACTION_CELL}>
              <ConfirmButton
                label={t("admin.repos.delete")}
                ariaLabel={`${t("admin.repos.delete")} ${repo.owner}/${repo.name}`}
                title={t("admin.repos.delete_title", { repo: `${repo.owner}/${repo.name}` })}
                description={description}
                size="sm"
                action={() => deleteAdminRepo(repo.id)}
                onDone={async () => {
                  toast.success(t("repo.settings.deletion_success"));
                  await reload(list.items.length === 1 ? list.page - 1 : list.page);
                }}
              />
            </TableCell>
          </TableRow>
        );
      }}
    </ListPage>
  );
}

// Descriptions longer than this are cut until the admin asks for the rest.
const NOTICE_PREVIEW_LENGTH = 200;

function NoticeDescription({ text }: { text: string }) {
  const { t } = useTranslation();
  const [expanded, setExpanded] = useState(false);
  const long = text.length > NOTICE_PREVIEW_LENGTH;
  return (
    <>
      <p className="font-mono text-xs whitespace-pre-wrap wrap-anywhere">
        {long && !expanded ? `${text.slice(0, NOTICE_PREVIEW_LENGTH)}…` : text}
      </p>
      {long ? (
        <button
          type="button"
          aria-expanded={expanded}
          className="mt-0.5 inline-flex min-h-6 cursor-pointer items-center rounded-sm text-xs text-(--color-primary) outline-none hover:underline focus-visible:ring-1 focus-visible:ring-(--color-ring)"
          onClick={() => setExpanded(!expanded)}
        >
          {expanded ? t("show_less") : t("show_more")}
        </button>
      ) : null}
    </>
  );
}

// The label widens the target of a checkbox to 32px, and its margin keeps
// the box itself in line with the edge of the card.
const CHECKBOX_TARGET = "-my-1 -ml-2 flex size-8 cursor-pointer items-center justify-center";

// AdminNotices lists the system notices, such as failures of background
// tasks, and deletes the selected ones or all of them.
export function AdminNotices({ list }: { list: AdminList<AdminNotice> }) {
  const { t } = useTranslation();
  const reload = useReloadList(list.page);
  const [picked, setPicked] = useState<Set<number>>(new Set());
  // The selection only covers notices on the page shown, so notices that were
  // deleted or are on another page drop out of it.
  const selected = new Set(list.items.filter((n) => picked.has(n.id)).map((n) => n.id));

  const onDeleted = async (page: number) => {
    toast.success(t("admin.notices.delete_success"));
    await reload(page);
  };
  const deleteSelected = useMutation({
    mutationFn: () => deleteAdminNotices([...selected]),
    // The notices after an emptied page move up into it, unless it was the last one.
    onSuccess: () =>
      onDeleted(selected.size === list.items.length && list.page >= list.totalPages ? list.page - 1 : list.page),
    onError: (err) => toast.error(err.message),
  });

  const allSelected = list.items.length > 0 && selected.size === list.items.length;
  const toggle = (id: number, checked: boolean) => {
    const next = new Set(selected);
    if (checked) next.add(id);
    else next.delete(id);
    setPicked(next);
  };

  return (
    <ListPage
      title={t("admin.notices")}
      list={list}
      actions={
        list.total > 0 ? (
          <>
            {list.items.length > 0 ? (
              <Button
                variant="outline"
                size="sm"
                className="text-(--color-destructive)"
                disabled={selected.size === 0 || deleteSelected.isPending}
                onClick={() => deleteSelected.mutate()}
              >
                {t("admin.notices.delete_selected")}
                {selected.size > 0 ? ` (${selected.size})` : null}
              </Button>
            ) : null}
            <ConfirmButton
              label={t("admin.notices.delete_all")}
              title={t("admin.notices.delete_all")}
              description={t("admin.notices.delete_all_desc")}
              size="sm"
              action={deleteAllAdminNotices}
              onDone={() => onDeleted(1)}
            />
          </>
        ) : null
      }
      head={
        <>
          <TableHead className="w-px">
            <label className={CHECKBOX_TARGET}>
              <Checkbox
                checked={allSelected ? true : selected.size > 0 ? "indeterminate" : false}
                onCheckedChange={(checked) =>
                  setPicked(checked === true ? new Set(list.items.map((n) => n.id)) : new Set())
                }
                aria-label={t("admin.notices.select_all")}
              />
            </label>
          </TableHead>
          <TableHead>{t("admin.notices.desc")}</TableHead>
          <TableHead className={cn(SHOWN_FROM.xl, "text-right")}>ID</TableHead>
          <TableHead className={SHOWN_FROM.xl}>{t("admin.notices.type")}</TableHead>
          <TableHead className={SHOWN_FROM.xl}>{t("admin.users.created")}</TableHead>
        </>
      }
    >
      {(notice) => {
        const created = DATE_FMT.format(new Date(notice.createdAt));
        return (
          <TableRow key={notice.id} data-state={selected.has(notice.id) ? "selected" : undefined}>
            <TableCell className="w-px align-top">
              <label className={CHECKBOX_TARGET}>
                <Checkbox
                  checked={selected.has(notice.id)}
                  onCheckedChange={(checked) => toggle(notice.id, checked === true)}
                  aria-label={`${t("admin.notices.select")} ${notice.id}`}
                />
              </label>
            </TableCell>
            <TableCell className="align-top">
              <NoticeDescription text={notice.description} />
              <Facts until="xl">
                <Fact until="xl">ID {notice.id}</Fact>
                <Fact until="xl">{notice.type}</Fact>
                <Fact until="xl">{created}</Fact>
              </Facts>
            </TableCell>
            <TableCell className={cn(SHOWN_FROM.xl, NUMBER_CELL, MUTED_CELL, "align-top")}>{notice.id}</TableCell>
            <TableCell className={cn(SHOWN_FROM.xl, FIT_CELL, "align-top")}>{notice.type}</TableCell>
            <TableCell className={cn(SHOWN_FROM.xl, FIT_CELL, "align-top")}>{created}</TableCell>
          </TableRow>
        );
      }}
    </ListPage>
  );
}
