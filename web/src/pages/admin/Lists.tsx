import { useMutation } from "@tanstack/react-query";
import { Link, useNavigate, useRouter } from "@tanstack/react-router";
import { Pencil, Plus } from "lucide-react";
import { type ReactNode, useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";

import { AdminLayout, AdminListHeading } from "@/components/AdminLayout";
import { ConfirmButton } from "@/components/ConfirmButton";
import { Pagination } from "@/components/Pagination";
import { SearchForm } from "@/components/SearchForm";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { webContext } from "@/lib/context";
import type { ListSearch } from "@/lib/page-search";
import {
  type AdminList,
  type AdminNotice,
  type AdminOrg,
  type AdminRepo,
  type AdminUser,
  deleteAdminNotices,
  deleteAdminRepo,
  deleteAllAdminNotices,
} from "@/lib/queries/admin";
import { subUrl } from "@/lib/url";
import { cn } from "@/lib/utils";

const DATE_FMT = new Intl.DateTimeFormat(webContext.lang, { year: "numeric", month: "short", day: "2-digit" });

const ROW_CLASS = "flex flex-wrap items-center gap-x-3 gap-y-2 border-b border-(--color-border) py-3";
const BADGE_CLASS =
  "rounded-full border border-(--color-border) px-2 py-0.5 text-xs whitespace-nowrap text-(--color-muted-foreground)";

// Meta lists facts about an item of a list, e.g., when it was created.
function Meta({ children }: { children: ReactNode }) {
  return (
    <p className="mt-1 flex flex-wrap gap-x-3 gap-y-1 text-xs text-(--color-muted-foreground) tabular-nums">
      {children}
    </p>
  );
}

function Avatar({ src, square = false }: { src: string; square?: boolean }) {
  return (
    <img
      src={src}
      alt=""
      className={cn(
        "size-10 shrink-0 border border-(--color-border) object-cover",
        square ? "rounded-md" : "rounded-full",
      )}
    />
  );
}

// ListBody shows the items of a page of a list, or that there are none, and
// links to the other pages.
function ListBody<T>({ list, children }: { list: AdminList<T>; children: (item: T) => ReactNode }) {
  const { t } = useTranslation();
  return (
    <>
      {list.items.length > 0 ? (
        <ul className="mt-4 border-t border-(--color-border)">{list.items.map(children)}</ul>
      ) : (
        <p className="mt-4 border-t border-(--color-border) py-6 text-sm text-(--color-muted-foreground)">
          {t("repo.pulls.no_results")}
        </p>
      )}
      {list.totalPages > 1 ? (
        <div className="mt-4">
          <Pagination page={list.page} totalPages={list.totalPages} />
        </div>
      ) : null}
    </>
  );
}

// useReloadList reloads a list after items on the page shown were deleted.
// It moves to `page` instead when the deletion leaves the page shown empty.
function useReloadList(current: number) {
  const router = useRouter();
  const navigate = useNavigate();
  return async (page: number) => {
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

// AdminUsers lists the users of the site. deleted is true right after a user
// was deleted from their edit page.
export function AdminUsers({
  list,
  search,
  deleted,
}: {
  list: AdminList<AdminUser>;
  search: ListSearch;
  deleted: boolean;
}) {
  const { t } = useTranslation();
  const navigate = useNavigate();

  useEffect(() => {
    if (!deleted) return;
    toast.success(t("admin.users.deletion_success"));
    void navigate({ to: "/admin/users", search: { q: search.q, page: search.page }, replace: true });
  }, [deleted, navigate, search.q, search.page, t]);

  return (
    <AdminLayout active="users">
      <AdminListHeading
        title={t("admin.users")}
        total={list.total}
        action={
          <Button asChild variant="outline" size="sm">
            <a href={subUrl("/admin/users/new")}>
              <Plus className="size-4" aria-hidden />
              {t("admin.users.new_account")}
            </a>
          </Button>
        }
      />
      <SearchForm q={search.q} />
      <ListBody list={list}>
        {(user) => (
          <li key={user.id} className={ROW_CLASS}>
            <div className="flex min-w-0 flex-1 basis-56 items-center gap-3">
              <Avatar src={user.avatarURL} />
              <div className="min-w-0">
                <p className="flex flex-wrap items-center gap-2">
                  <Link
                    to="/$username"
                    params={{ username: user.name }}
                    search={{}}
                    className="font-semibold wrap-anywhere text-(--color-primary) hover:underline"
                  >
                    {user.name}
                  </Link>
                  {user.isAdmin ? <span className={BADGE_CLASS}>{t("admin.users.admin")}</span> : null}
                  {user.isActive ? null : <span className={BADGE_CLASS}>{t("admin.users.not_activated")}</span>}
                </p>
                <p className="text-sm wrap-anywhere text-(--color-muted-foreground)">{user.email}</p>
                <Meta>
                  <span>ID {user.id}</span>
                  <span>
                    {t("admin.users.repos")} {user.repoCount}
                  </span>
                  <span>
                    {t("admin.users.created")} {DATE_FMT.format(new Date(user.createdAt))}
                  </span>
                </Meta>
              </div>
            </div>
            <Button asChild variant="outline" size="sm" className="shrink-0">
              <a href={subUrl(`/admin/users/${user.id}`)} aria-label={`${t("admin.users.edit")} ${user.name}`}>
                <Pencil className="size-3.5" aria-hidden />
                {t("admin.users.edit")}
              </a>
            </Button>
          </li>
        )}
      </ListBody>
    </AdminLayout>
  );
}

// AdminOrgs lists the organizations of the site.
export function AdminOrgs({ list, search }: { list: AdminList<AdminOrg>; search: ListSearch }) {
  const { t } = useTranslation();
  return (
    <AdminLayout active="orgs">
      <AdminListHeading title={t("admin.organizations")} total={list.total} />
      <SearchForm q={search.q} />
      <ListBody list={list}>
        {(org) => (
          <li key={org.id} className={ROW_CLASS}>
            <div className="flex min-w-0 flex-1 basis-56 items-center gap-3">
              <Avatar src={org.avatarURL} square />
              <div className="min-w-0">
                <Link
                  to="/$username"
                  params={{ username: org.name }}
                  search={{}}
                  className="font-semibold wrap-anywhere text-(--color-primary) hover:underline"
                >
                  {org.name}
                </Link>
                <Meta>
                  <span>ID {org.id}</span>
                  <span>
                    {t("admin.orgs.teams")} {org.teamCount}
                  </span>
                  <span>
                    {t("admin.orgs.members")} {org.memberCount}
                  </span>
                  <span>
                    {t("admin.users.repos")} {org.repoCount}
                  </span>
                  <span>
                    {t("admin.users.created")} {DATE_FMT.format(new Date(org.createdAt))}
                  </span>
                </Meta>
              </div>
            </div>
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
          </li>
        )}
      </ListBody>
    </AdminLayout>
  );
}

// AdminRepos lists the repositories of the site, including private ones, and
// deletes them.
export function AdminRepos({ list, search }: { list: AdminList<AdminRepo>; search: ListSearch }) {
  const { t } = useTranslation();
  const reload = useReloadList(list.page);
  const description = `${t("repo.settings.delete_desc")} ${t("repo.settings.delete_notices_2").replace(/^-\s*/, "")}`;
  return (
    <AdminLayout active="repos">
      <AdminListHeading title={t("admin.repositories")} total={list.total} />
      <SearchForm q={search.q} />
      <ListBody list={list}>
        {(repo) => (
          <li key={repo.id} className={ROW_CLASS}>
            <div className="min-w-0 flex-1 basis-56">
              <p className="flex flex-wrap items-center gap-2">
                <a
                  href={subUrl(`/${repo.owner}/${repo.name}`)}
                  className="font-semibold wrap-anywhere text-(--color-primary) hover:underline"
                >
                  {repo.owner}/{repo.name}
                </a>
                {repo.isPrivate ? <span className={BADGE_CLASS}>{t("admin.repos.private")}</span> : null}
              </p>
              <Meta>
                <span>ID {repo.id}</span>
                <span>
                  {t("admin.repos.watches")} {repo.watchCount}
                </span>
                <span>
                  {t("admin.repos.stars")} {repo.starCount}
                </span>
                <span>
                  {t("admin.repos.issues")} {repo.issueCount}
                </span>
                <span>
                  {t("admin.repos.size")} {repo.size}
                </span>
                <span>
                  {t("admin.users.created")} {DATE_FMT.format(new Date(repo.createdAt))}
                </span>
              </Meta>
            </div>
            <ConfirmButton
              label={t("admin.repos.delete")}
              ariaLabel={`${t("admin.repos.delete")} ${repo.owner}/${repo.name}`}
              title={t("repo.settings.delete")}
              description={description}
              size="sm"
              action={() => deleteAdminRepo(repo.id)}
              onDone={async () => {
                toast.success(t("repo.settings.deletion_success"));
                await reload(list.items.length === 1 ? list.page - 1 : list.page);
              }}
            />
          </li>
        )}
      </ListBody>
    </AdminLayout>
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
          className="mt-1 cursor-pointer text-xs text-(--color-primary) hover:underline"
          onClick={() => setExpanded(!expanded)}
        >
          {expanded ? t("show_less") : t("show_more")}
        </button>
      ) : null}
    </>
  );
}

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
    onSuccess: () => onDeleted(selected.size === list.items.length ? list.page - 1 : list.page),
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
    <AdminLayout active="notices">
      <AdminListHeading
        title={t("admin.notices")}
        total={list.total}
        action={
          list.total > 0 ? (
            <ConfirmButton
              label={t("admin.notices.delete_all")}
              title={t("admin.notices.delete_all")}
              description={t("admin.notices.delete_all_desc")}
              size="sm"
              action={deleteAllAdminNotices}
              onDone={() => onDeleted(1)}
            />
          ) : null
        }
      />
      {list.items.length > 0 ? (
        <div className="flex flex-wrap items-center justify-between gap-3">
          <label className="flex min-h-8 cursor-pointer items-center gap-2 text-sm">
            <Checkbox
              checked={allSelected ? true : selected.size > 0 ? "indeterminate" : false}
              onCheckedChange={(checked) =>
                setPicked(checked === true ? new Set(list.items.map((n) => n.id)) : new Set())
              }
            />
            {t("admin.notices.select_all")}
          </label>
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
        </div>
      ) : null}
      <ListBody list={list}>
        {(notice) => (
          <li key={notice.id} className="flex gap-1 border-b border-(--color-border) py-3">
            {/* The label widens the target of the checkbox to 32px. */}
            <label className="-my-1.5 -ml-2 flex size-8 shrink-0 cursor-pointer items-center justify-center">
              <Checkbox
                checked={selected.has(notice.id)}
                onCheckedChange={(checked) => toggle(notice.id, checked === true)}
                aria-label={`${t("admin.notices.select")} ${notice.id}`}
              />
            </label>
            <div className="min-w-0 flex-1">
              <NoticeDescription text={notice.description} />
              <Meta>
                <span>ID {notice.id}</span>
                <span>{notice.type}</span>
                <span>{DATE_FMT.format(new Date(notice.createdAt))}</span>
              </Meta>
            </div>
          </li>
        )}
      </ListBody>
    </AdminLayout>
  );
}
