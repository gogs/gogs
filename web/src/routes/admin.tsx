import { type AnyRoute, createRoute, redirect } from "@tanstack/react-router";
import type { JSX } from "react";

import { LoaderResponseError, loadAuthenticatedPage } from "@/lib/loader-error";
import {
  type ListSearch,
  type PageSearch,
  type ProcessesSearch,
  type TraceSearch,
  validateListSearch,
  validatePageSearch,
  validateProcessesSearch,
  validateTraceSearch,
} from "@/lib/page-search";
import {
  fetchAdminAuthSource,
  fetchAdminAuthSources,
  fetchAdminConfig,
  fetchAdminDashboard,
  fetchAdminHealth,
  fetchAdminMonitor,
  fetchAdminNotices,
  fetchAdminOrgs,
  fetchAdminRepos,
  fetchAdminTrace,
  fetchAdminTraceOverview,
  fetchAdminTraceSearch,
  fetchAdminUser,
  fetchAdminUserOptions,
  fetchAdminUsers,
} from "@/lib/queries/admin";
import { AdminAuthSourceEdit, AdminAuthSourceNew } from "@/pages/admin/Auths";
import { AdminAuthSources, AdminNotices, AdminOrgs, AdminRepos, AdminUsers } from "@/pages/admin/Lists";
import { AdminConfig, AdminCron, AdminDashboard, AdminOperations, AdminProcesses } from "@/pages/admin/System";
import { AdminTraceOverview, AdminTraceSearch, AdminTraceView } from "@/pages/admin/Traces";
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
    validateSearch: validateProcessesSearch,
    loaderDeps: ({ search }: { search: ProcessesSearch }) => ({ health: search.health ?? ("1h" as const) }),
    loader: ({ deps }) =>
      loadAuthenticatedPage(() => Promise.all([fetchAdminMonitor(), fetchAdminHealth(deps.health)])),
    component: function Processes() {
      const [monitor, health] = processesRoute.useLoaderData();
      return <AdminProcesses monitor={monitor} health={health} />;
    },
  });

  const tracesRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/admin/traces",
    validateSearch: validateTraceSearch,
    loaderDeps: ({ search }: { search: TraceSearch }) => search,
    loader: async ({ deps }) => {
      if (deps.view !== "search") {
        return {
          view: "overview" as const,
          overview: await loadAuthenticatedPage(() => fetchAdminTraceOverview(deps.range ?? "1h")),
        };
      }
      const result = await loadAuthenticatedPage(() => fetchAdminTraceSearch(deps));
      // A trace ID, or a prefix of one that only one trace has, opens the trace.
      if (deps.q && result.total === 1) {
        // eslint-disable-next-line @typescript-eslint/only-throw-error -- TanStack's redirect() returns a sentinel that must be thrown.
        throw redirect({ to: "/admin/traces/$id", params: { id: result.traces[0].id }, replace: true });
      }
      return { view: "search" as const, result };
    },
    // Without the declared return type, inferring it reads the loader data of this
    // route while its type is still being inferred, which turns the loader data and
    // search params of other routes into `any`.
    component: function Traces(): JSX.Element {
      const data = tracesRoute.useLoaderData();
      const search: TraceSearch = tracesRoute.useSearch();
      return data.view === "overview" ? (
        <AdminTraceOverview overview={data.overview} />
      ) : (
        <AdminTraceSearch result={data.result} search={search} />
      );
    },
  });

  const traceRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/admin/traces/$id",
    loader: ({ params }) =>
      loadAuthenticatedPage(async () => {
        try {
          return await fetchAdminTrace(params.id);
        } catch (err) {
          // Search for the ID instead, which explains why the trace is missing.
          if (err instanceof LoaderResponseError && err.status === 404) {
            // eslint-disable-next-line @typescript-eslint/only-throw-error -- TanStack's redirect() returns a sentinel that must be thrown.
            throw redirect({ to: "/admin/traces", search: { view: "search", q: params.id }, replace: true });
          }
          throw err;
        }
      }),
    component: function Trace() {
      const trace = traceRoute.useLoaderData();
      // Remount for another trace so expanded spans start closed.
      return <AdminTraceView key={trace.id} trace={trace} />;
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
    tracesRoute,
    traceRoute,
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
