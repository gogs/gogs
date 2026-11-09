import { type AnyRoute, createRoute } from "@tanstack/react-router";

import { loadAuthenticatedPage } from "@/lib/loader-error";
import { type ListSearch, type PageSearch, validateListSearch, validatePageSearch } from "@/lib/page-search";
import {
  fetchAdminConfig,
  fetchAdminDashboard,
  fetchAdminMonitor,
  fetchAdminNotices,
  fetchAdminOrgs,
  fetchAdminRepos,
  fetchAdminUser,
  fetchAdminUserOptions,
  fetchAdminUsers,
} from "@/lib/queries/admin";
import { AdminNotices, AdminOrgs, AdminRepos, AdminUsers } from "@/pages/admin/Lists";
import { AdminConfig, AdminDashboard, AdminMonitor } from "@/pages/admin/System";
import { AdminUserEdit, AdminUserNew } from "@/pages/admin/Users";

export function createAdminRoutes(rootRoute: AnyRoute) {
  const dashboardRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/admin",
    loader: () => loadAuthenticatedPage(fetchAdminDashboard),
    component: function Dashboard() {
      return <AdminDashboard dashboard={dashboardRoute.useLoaderData()} />;
    },
  });

  const configRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/admin/config",
    loader: () => loadAuthenticatedPage(fetchAdminConfig),
    component: function Config() {
      return <AdminConfig config={configRoute.useLoaderData()} />;
    },
  });

  const monitorRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/admin/monitor",
    loader: () => loadAuthenticatedPage(fetchAdminMonitor),
    component: function Monitor() {
      return <AdminMonitor monitor={monitorRoute.useLoaderData()} />;
    },
  });

  const usersRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/admin/users",
    validateSearch: validateListSearch,
    loaderDeps: ({ search }: { search: ListSearch }) => ({ q: search.q, page: search.page }),
    loader: ({ deps }) => loadAuthenticatedPage(() => fetchAdminUsers(deps)),
    component: function Users() {
      const search: ListSearch = usersRoute.useSearch();
      return <AdminUsers list={usersRoute.useLoaderData()} search={search} />;
    },
  });

  const userNewRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/admin/users/new",
    loader: () => loadAuthenticatedPage(fetchAdminUserOptions),
    component: function UserNew() {
      return <AdminUserNew options={userNewRoute.useLoaderData()} />;
    },
  });

  const userEditRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/admin/users/$id",
    loader: ({ params }) =>
      loadAuthenticatedPage(() => Promise.all([fetchAdminUser(Number(params.id)), fetchAdminUserOptions()])),
    component: function UserEdit() {
      const [user, options] = userEditRoute.useLoaderData();
      // Remount for another user so the form starts from their values.
      return <AdminUserEdit key={user.id} user={user} options={options} />;
    },
  });

  const orgsRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/admin/orgs",
    validateSearch: validateListSearch,
    loaderDeps: ({ search }: { search: ListSearch }) => ({ q: search.q, page: search.page }),
    loader: ({ deps }) => loadAuthenticatedPage(() => fetchAdminOrgs(deps)),
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
    loader: ({ deps }) => loadAuthenticatedPage(() => fetchAdminRepos(deps)),
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
    loader: ({ deps }) => loadAuthenticatedPage(() => fetchAdminNotices(deps)),
    component: function Notices() {
      return <AdminNotices list={noticesRoute.useLoaderData()} />;
    },
  });

  return [
    dashboardRoute,
    configRoute,
    monitorRoute,
    usersRoute,
    userNewRoute,
    userEditRoute,
    orgsRoute,
    reposRoute,
    noticesRoute,
  ];
}
