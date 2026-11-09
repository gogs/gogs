import { type AnyRoute, createRoute } from "@tanstack/react-router";

import { loadSignedInPage } from "@/lib/loader-error";
import { type ListSearch, type PageSearch, validateListSearch, validatePageSearch } from "@/lib/page-search";
import { fetchAdminNotices, fetchAdminOrgs, fetchAdminRepos, fetchAdminUsers } from "@/lib/queries/admin";
import { AdminNotices, AdminOrgs, AdminRepos, AdminUsers } from "@/pages/admin/Lists";

type AdminUsersSearch = ListSearch & { deleted?: true };

export function createAdminRoutes(rootRoute: AnyRoute) {
  const usersRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/admin/users",
    // Deleting a user on their edit page lands here with `deleted`.
    validateSearch: (search: Record<string, unknown>): AdminUsersSearch => ({
      ...validateListSearch(search),
      ...(search.deleted === 1 || search.deleted === "1" ? { deleted: true } : {}),
    }),
    loaderDeps: ({ search }: { search: AdminUsersSearch }) => ({ q: search.q, page: search.page }),
    loader: ({ deps }) => loadSignedInPage(() => fetchAdminUsers(deps)),
    component: function Users() {
      const search: AdminUsersSearch = usersRoute.useSearch();
      return <AdminUsers list={usersRoute.useLoaderData()} search={search} deleted={search.deleted === true} />;
    },
  });

  const orgsRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/admin/orgs",
    validateSearch: validateListSearch,
    loaderDeps: ({ search }: { search: ListSearch }) => ({ q: search.q, page: search.page }),
    loader: ({ deps }) => loadSignedInPage(() => fetchAdminOrgs(deps)),
    component: function Orgs() {
      const search: ListSearch = orgsRoute.useSearch();
      return <AdminOrgs list={orgsRoute.useLoaderData()} search={search} />;
    },
  });

  const reposRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/admin/repos",
    validateSearch: validateListSearch,
    loaderDeps: ({ search }: { search: ListSearch }) => ({ q: search.q, page: search.page }),
    loader: ({ deps }) => loadSignedInPage(() => fetchAdminRepos(deps)),
    component: function Repos() {
      const search: ListSearch = reposRoute.useSearch();
      return <AdminRepos list={reposRoute.useLoaderData()} search={search} />;
    },
  });

  const noticesRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/admin/notices",
    validateSearch: validatePageSearch,
    loaderDeps: ({ search }: { search: PageSearch }) => ({ page: search.page }),
    loader: ({ deps }) => loadSignedInPage(() => fetchAdminNotices(deps)),
    component: function Notices() {
      return <AdminNotices list={noticesRoute.useLoaderData()} />;
    },
  });

  return [usersRoute, orgsRoute, reposRoute, noticesRoute];
}
