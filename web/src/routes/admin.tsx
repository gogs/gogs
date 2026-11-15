import { type AnyRoute, createRoute } from "@tanstack/react-router";

import { loadAuthenticatedPage } from "@/lib/loader-error";
import { type ListSearch, type PageSearch, validateListSearch, validatePageSearch } from "@/lib/page-search";
import {
  fetchAdminAuthSource,
  fetchAdminAuthSources,
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
import { AdminAuthSourceEdit, AdminAuthSourceNew } from "@/pages/admin/Auths";
import { AdminAuthSources, AdminNotices, AdminOrgs, AdminRepos, AdminUsers } from "@/pages/admin/Lists";
import { AdminConfig, AdminCron, AdminDashboard, AdminOperations, AdminProcesses } from "@/pages/admin/System";
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

  const operationsRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/admin/operations",
    component: AdminOperations,
  });

  const configRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/admin/config",
    loader: () => loadAuthenticatedPage(fetchAdminConfig),
    component: function Config() {
      return <AdminConfig config={configRoute.useLoaderData()} />;
    },
  });

  const cronRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/admin/cron",
    loader: () => loadAuthenticatedPage(fetchAdminMonitor),
    component: function Cron() {
      return <AdminCron monitor={cronRoute.useLoaderData()} />;
    },
  });

  const processesRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/admin/processes",
    loader: () => loadAuthenticatedPage(fetchAdminMonitor),
    component: function Processes() {
      return <AdminProcesses monitor={processesRoute.useLoaderData()} />;
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

  const authsRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/admin/auths",
    loader: () => loadAuthenticatedPage(fetchAdminAuthSources),
    component: function Auths() {
      return <AdminAuthSources sources={authsRoute.useLoaderData()} />;
    },
  });

  const authNewRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/admin/auths/new",
    component: AdminAuthSourceNew,
  });

  const authEditRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/admin/auths/$id",
    loader: ({ params }) => loadAuthenticatedPage(() => fetchAdminAuthSource(Number(params.id))),
    component: function AuthEdit() {
      const source = authEditRoute.useLoaderData();
      // Remount for another source so the form starts from its values.
      return <AdminAuthSourceEdit key={source.id} source={source} />;
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
    operationsRoute,
    configRoute,
    cronRoute,
    processesRoute,
    usersRoute,
    userNewRoute,
    userEditRoute,
    orgsRoute,
    reposRoute,
    authsRoute,
    authNewRoute,
    authEditRoute,
    noticesRoute,
  ];
}
