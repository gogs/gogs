import { type AnyRoute, createRoute } from "@tanstack/react-router";

import { loadSignedInPage } from "@/lib/loader-error";
import { type ListSearch, type PageSearch, validateListSearch, validatePageSearch } from "@/lib/page-search";
import {
  fetchAdminAuthSource,
  fetchAdminAuthSources,
  fetchAdminConfig,
  fetchAdminDashboard,
  fetchAdminEmailPreview,
  fetchAdminMonitor,
  fetchAdminNotices,
  fetchAdminOrgs,
  fetchAdminRepos,
  fetchAdminUser,
  fetchAdminUserOptions,
  fetchAdminUsers,
} from "@/lib/queries/admin";
import { AdminAuthSourceEdit, AdminAuthSourceNew } from "@/pages/admin/Auths";
import { AdminEmails } from "@/pages/admin/Emails";
import { AdminAuthSources, AdminNotices, AdminOrgs, AdminRepos, AdminUsers } from "@/pages/admin/Lists";
import { AdminConfig, AdminDashboard, AdminMonitor, AdminOperations } from "@/pages/admin/System";
import { AdminUserEdit, AdminUserNew } from "@/pages/admin/Users";

interface EmailSearch {
  name?: string;
  language?: string;
}

export function createAdminRoutes(rootRoute: AnyRoute) {
  const dashboardRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/admin",
    loader: () => loadSignedInPage(fetchAdminDashboard),
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
    loader: () => loadSignedInPage(fetchAdminConfig),
    component: function Config() {
      return <AdminConfig config={configRoute.useLoaderData()} />;
    },
  });

  const monitorRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/admin/monitor",
    loader: () => loadSignedInPage(fetchAdminMonitor),
    component: function Monitor() {
      return <AdminMonitor monitor={monitorRoute.useLoaderData()} />;
    },
  });

  const usersRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/admin/users",
    validateSearch: validateListSearch,
    loaderDeps: ({ search }: { search: ListSearch }) => ({ q: search.q, page: search.page }),
    loader: ({ deps }) => loadSignedInPage(() => fetchAdminUsers(deps)),
    component: function Users() {
      const search: ListSearch = usersRoute.useSearch();
      return <AdminUsers list={usersRoute.useLoaderData()} search={search} />;
    },
  });

  const userNewRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/admin/users/new",
    loader: () => loadSignedInPage(fetchAdminUserOptions),
    component: function UserNew() {
      return <AdminUserNew options={userNewRoute.useLoaderData()} />;
    },
  });

  const userEditRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/admin/users/$id",
    loader: ({ params }) =>
      loadSignedInPage(() => Promise.all([fetchAdminUser(Number(params.id)), fetchAdminUserOptions()])),
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

  const authsRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/admin/auths",
    loader: () => loadSignedInPage(fetchAdminAuthSources),
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
    loader: ({ params }) => loadSignedInPage(() => fetchAdminAuthSource(Number(params.id))),
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
    loader: ({ deps }) => loadSignedInPage(() => fetchAdminNotices(deps)),
    component: function Notices() {
      return <AdminNotices list={noticesRoute.useLoaderData()} />;
    },
  });

  const emailsRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/admin/emails",
    validateSearch: (search: Record<string, unknown>): EmailSearch => ({
      name: typeof search.name === "string" ? search.name : undefined,
      language: typeof search.language === "string" ? search.language : undefined,
    }),
    loaderDeps: ({ search }: { search: EmailSearch }) => ({ name: search.name, language: search.language }),
    loader: ({ deps }) => loadSignedInPage(() => fetchAdminEmailPreview(deps)),
    component: function Emails() {
      return <AdminEmails preview={emailsRoute.useLoaderData()} />;
    },
  });

  return [
    dashboardRoute,
    operationsRoute,
    configRoute,
    monitorRoute,
    usersRoute,
    userNewRoute,
    userEditRoute,
    orgsRoute,
    reposRoute,
    authsRoute,
    authNewRoute,
    authEditRoute,
    noticesRoute,
    emailsRoute,
  ];
}
