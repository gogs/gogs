import type { QueryClient } from "@tanstack/react-query";
import { type AnyRoute, createRoute, notFound, redirect } from "@tanstack/react-router";
import type { JSX } from "react";

import { AdminLayout } from "@/components/AdminLayout";
import { leaveAdmin } from "@/lib/admin-exit";
import { webContext } from "@/lib/context";
import { LoaderResponseError, authenticationRedirect, loadAuthenticatedPage } from "@/lib/loader-error";
import {
  type HealthSearch,
  type ListSearch,
  type PageSearch,
  type TraceSearch,
  validateHealthSearch,
  validateListSearch,
  validatePageSearch,
  validateTraceSearch,
} from "@/lib/page-search";
import {
  adminNoticesQuery,
  fetchAdminAuthSource,
  fetchAdminAuthSources,
  fetchAdminConfig,
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
import type { UserInfo } from "@/lib/user-info";
import { AdminAuthSourceEdit, AdminAuthSourceNew } from "@/pages/admin/Auths";
import { AdminServerHealth } from "@/pages/admin/Health";
import { AdminAuthSources, AdminNotices, AdminOrgs, AdminRepos, AdminUsers } from "@/pages/admin/Lists";
import { AdminOverview } from "@/pages/admin/Overview";
import { AdminError, AdminNotFound } from "@/pages/admin/Problem";
import { AdminConfig, AdminJobs, AdminOperations } from "@/pages/admin/System";
import { AdminTraceOverview, AdminTraceSearch, AdminTraceView } from "@/pages/admin/Traces";
import { AdminUserEdit, AdminUserNew } from "@/pages/admin/Users";

// A page of the console that is not found or fails to load says so in place of
// the page, so the navigation of the console stays.
const PAGE = { notFoundComponent: AdminNotFound, errorComponent: AdminError };

// loadAdminPage loads what a page of the console shows. When the admin API
// turns the viewer away, the page they are on stays until the one that takes
// them out of the console loads.
async function loadAdminPage<T>(location: { href: string }, load: () => Promise<T>): Promise<T> {
  try {
    return await loadAuthenticatedPage(load);
  } catch (err) {
    if (leaveAdmin(err, webContext.subURL + location.href)) await new Promise<never>(() => undefined);
    throw err;
  }
}

export function createAdminRoutes(rootRoute: AnyRoute) {
  // The pages share the console shell, which stays in place while one of them
  // loads. Only site admins get to see it.
  const adminRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/admin",
    beforeLoad: ({ context, location }) => {
      const { user } = context as { user: UserInfo | null };
      // eslint-disable-next-line @typescript-eslint/only-throw-error -- TanStack's redirect() returns a sentinel that must be thrown.
      if (!user) throw authenticationRedirect(location.href);
      // eslint-disable-next-line @typescript-eslint/only-throw-error -- `notFound()` is the documented TanStack Router signal for 404, not an Error subclass.
      if (!user.isAdmin) throw notFound();
    },
    component: AdminLayout,
  });

  const dashboardRoute = createRoute({
    getParentRoute: () => adminRoute,
    ...PAGE,
    path: "/",
    component: AdminOverview,
  });

  const operationsRoute = createRoute({
    getParentRoute: () => adminRoute,
    ...PAGE,
    path: "operations",
    component: AdminOperations,
  });

  const configRoute = createRoute({
    getParentRoute: () => adminRoute,
    ...PAGE,
    path: "config",
    loader: ({ location }) => loadAdminPage(location, fetchAdminConfig),
    component: function Config() {
      return <AdminConfig config={configRoute.useLoaderData()} />;
    },
  });

  const jobsRoute = createRoute({
    getParentRoute: () => adminRoute,
    ...PAGE,
    path: "jobs",
    loader: ({ location }) => loadAdminPage(location, fetchAdminMonitor),
    component: function Jobs() {
      return <AdminJobs monitor={jobsRoute.useLoaderData()} />;
    },
  });

  const healthRoute = createRoute({
    getParentRoute: () => adminRoute,
    ...PAGE,
    path: "health",
    validateSearch: validateHealthSearch,
    loaderDeps: ({ search }: { search: HealthSearch }) => ({ health: search.health ?? ("1h" as const) }),
    loader: ({ deps, location }) => loadAdminPage(location, () => fetchAdminHealth(deps.health)),
    component: function Health() {
      const health = healthRoute.useLoaderData();
      // Remount on a range switch, so that a pinned moment does not outlive its range.
      return <AdminServerHealth key={health.range} health={health} range={health.range} />;
    },
  });

  const tracesRoute = createRoute({
    getParentRoute: () => adminRoute,
    ...PAGE,
    path: "traces",
    validateSearch: validateTraceSearch,
    loaderDeps: ({ search }: { search: TraceSearch }) => search,
    loader: async ({ deps, location }) => {
      if (deps.view !== "search") {
        return {
          view: "overview" as const,
          overview: await loadAdminPage(location, () => fetchAdminTraceOverview(deps.range ?? "1h")),
        };
      }
      const result = await loadAdminPage(location, () => fetchAdminTraceSearch(deps));
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
    getParentRoute: () => adminRoute,
    ...PAGE,
    path: "traces/$id",
    loader: ({ params, location }) =>
      loadAdminPage(location, async () => {
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
    getParentRoute: () => adminRoute,
    ...PAGE,
    path: "users",
    validateSearch: validateListSearch,
    loaderDeps: ({ search }: { search: ListSearch }) => ({ q: search.q, page: search.page }),
    loader: ({ deps, location }) => loadAdminPage(location, () => fetchAdminUsers(deps)),
    component: function Users() {
      const search: ListSearch = usersRoute.useSearch();
      return <AdminUsers list={usersRoute.useLoaderData()} search={search} />;
    },
  });

  const userNewRoute = createRoute({
    getParentRoute: () => adminRoute,
    ...PAGE,
    path: "users/new",
    loader: ({ location }) => loadAdminPage(location, fetchAdminUserOptions),
    component: function UserNew() {
      return <AdminUserNew options={userNewRoute.useLoaderData()} />;
    },
  });

  const userEditRoute = createRoute({
    getParentRoute: () => adminRoute,
    ...PAGE,
    path: "users/$id",
    loader: ({ params, location }) =>
      loadAdminPage(location, () => Promise.all([fetchAdminUser(Number(params.id)), fetchAdminUserOptions()])),
    component: function UserEdit() {
      const [user, options] = userEditRoute.useLoaderData();
      // Remount for another user so the form starts from their values.
      return <AdminUserEdit key={user.id} user={user} options={options} />;
    },
  });

  const orgsRoute = createRoute({
    getParentRoute: () => adminRoute,
    ...PAGE,
    path: "orgs",
    validateSearch: validateListSearch,
    loaderDeps: ({ search }: { search: ListSearch }) => ({ q: search.q, page: search.page }),
    loader: ({ deps, location }) => loadAdminPage(location, () => fetchAdminOrgs(deps)),
    component: function Orgs() {
      const search: ListSearch = orgsRoute.useSearch();
      return <AdminOrgs list={orgsRoute.useLoaderData()} search={search} />;
    },
  });

  const reposRoute = createRoute({
    getParentRoute: () => adminRoute,
    ...PAGE,
    path: "repos",
    validateSearch: validateListSearch,
    loaderDeps: ({ search }: { search: ListSearch }) => ({ q: search.q, page: search.page }),
    loader: ({ deps, location }) => loadAdminPage(location, () => fetchAdminRepos(deps)),
    component: function Repos() {
      const search: ListSearch = reposRoute.useSearch();
      return <AdminRepos list={reposRoute.useLoaderData()} search={search} />;
    },
  });

  const authsRoute = createRoute({
    getParentRoute: () => adminRoute,
    ...PAGE,
    path: "auths",
    loader: ({ location }) => loadAdminPage(location, fetchAdminAuthSources),
    component: function Auths() {
      return <AdminAuthSources sources={authsRoute.useLoaderData()} />;
    },
  });

  const authNewRoute = createRoute({
    getParentRoute: () => adminRoute,
    ...PAGE,
    path: "auths/new",
    component: AdminAuthSourceNew,
  });

  const authEditRoute = createRoute({
    getParentRoute: () => adminRoute,
    ...PAGE,
    path: "auths/$id",
    loader: ({ params, location }) => loadAdminPage(location, () => fetchAdminAuthSource(Number(params.id))),
    component: function AuthEdit() {
      const source = authEditRoute.useLoaderData();
      // Remount for another source so the form starts from its values.
      return <AdminAuthSourceEdit key={source.id} source={source} />;
    },
  });

  const noticesRoute = createRoute({
    getParentRoute: () => adminRoute,
    ...PAGE,
    path: "notices",
    validateSearch: validatePageSearch,
    loaderDeps: ({ search }: { search: PageSearch }) => ({ page: search.page }),
    loader: async ({ deps, context, location }) => {
      const list = await loadAdminPage(location, () => fetchAdminNotices(deps));
      // The sidebar of the console counts the system notices from their first page.
      const { queryClient } = context as { queryClient: QueryClient };
      const firstPage = adminNoticesQuery({}).queryKey;
      if (list.page === 1) queryClient.setQueryData(firstPage, list);
      else void queryClient.invalidateQueries({ queryKey: firstPage });
      return list;
    },
    component: function Notices() {
      const list = noticesRoute.useLoaderData();
      // Remount for another page, so that a selection does not come back with its page.
      return <AdminNotices key={list.page} list={list} />;
    },
  });

  // Paths without a page are not found inside the console too.
  const missingRoute = createRoute({
    getParentRoute: () => adminRoute,
    ...PAGE,
    path: "$",
    loader: () => {
      // eslint-disable-next-line @typescript-eslint/only-throw-error -- `notFound()` is the documented TanStack Router signal for 404, not an Error subclass.
      throw notFound();
    },
  });

  return [
    adminRoute.addChildren([
      dashboardRoute,
      operationsRoute,
      configRoute,
      jobsRoute,
      healthRoute,
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
      missingRoute,
    ]),
  ];
}
