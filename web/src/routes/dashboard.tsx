import type { QueryClient } from "@tanstack/react-query";
import { type AnyRoute, createRoute, notFound, redirect } from "@tanstack/react-router";

import { LoaderResponseError } from "@/lib/loader-error";
import { dashboardActivityQuery, dashboardQuery, fetchDashboardIssues } from "@/lib/queries/user";
import { useUserInfo } from "@/lib/use-user-info";
import type { UserInfo } from "@/lib/user-info";
import { Landing } from "@/pages/Landing";
import { Dashboard } from "@/pages/user/Dashboard";
import { DashboardIssues } from "@/pages/user/DashboardIssues";
import { type DashboardIssuesSearch, validateDashboardIssuesSearch } from "@/pages/user/DashboardIssues.search";

interface RouterContext {
  user: UserInfo | null;
  queryClient: QueryClient;
}

function loadDashboard(queryClient: QueryClient, org?: string) {
  return Promise.all([
    queryClient.ensureQueryData(dashboardQuery(org)),
    queryClient.ensureInfiniteQueryData(dashboardActivityQuery(org)),
  ]);
}

// withDashboardErrors maps the errors of loading a dashboard page to router
// signals.
async function withDashboardErrors<T>(load: Promise<T>, reload: () => unknown): Promise<T> {
  try {
    return await load;
  } catch (err) {
    if (err instanceof LoaderResponseError && err.status === 404) {
      // eslint-disable-next-line @typescript-eslint/only-throw-error -- `notFound()` is the documented TanStack Router signal for 404, not an Error subclass.
      throw notFound();
    }
    if (err instanceof LoaderResponseError && (err.status === 401 || err.status === 403)) {
      // The viewer is signed out, has not confirmed their email, or is
      // prohibited from signing in. Reload the page so the server sends them to
      // sign in, account activation, or the login prohibited page.
      throw reload();
    }
    throw err;
  }
}

export function createDashboardRoutes(rootRoute: AnyRoute) {
  const homeRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/",
    loader: async ({ context }) => {
      const { user, queryClient } = context as RouterContext;
      if (!user) return;
      await withDashboardErrors(loadDashboard(queryClient), () => redirect({ to: "/", reloadDocument: true }));
    },
    // Visitors get the landing page, signed-in users their dashboard.
    component: function Home() {
      return useUserInfo() ? <Dashboard /> : <Landing />;
    },
  });

  const orgDashboardRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/org/$org/dashboard",
    loader: async ({ params, context }) => {
      const { queryClient } = context as RouterContext;
      await withDashboardErrors(loadDashboard(queryClient, params.org), () =>
        redirect({ to: "/org/$org/dashboard", params: { org: params.org }, reloadDocument: true }),
      );
    },
    component: function OrgDashboard() {
      const { org } = orgDashboardRoute.useParams();
      // Remount when switching organizations so the list tab starts over.
      return <Dashboard key={org} org={org} />;
    },
  });

  const issuesLoaderDeps = ({ search }: { search: DashboardIssuesSearch }) => search;

  const issuesRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/issues",
    validateSearch: validateDashboardIssuesSearch,
    loaderDeps: issuesLoaderDeps,
    loader: ({ deps, location }) =>
      withDashboardErrors(fetchDashboardIssues(undefined, "issues", { ...deps }), () =>
        redirect({ href: location.href, reloadDocument: true }),
      ),
    component: function Issues() {
      return <DashboardIssues data={issuesRoute.useLoaderData()} search={issuesRoute.useSearch()} kind="issues" />;
    },
  });

  const pullsRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/pulls",
    validateSearch: validateDashboardIssuesSearch,
    loaderDeps: issuesLoaderDeps,
    loader: ({ deps, location }) =>
      withDashboardErrors(fetchDashboardIssues(undefined, "pulls", { ...deps }), () =>
        redirect({ href: location.href, reloadDocument: true }),
      ),
    component: function Pulls() {
      return <DashboardIssues data={pullsRoute.useLoaderData()} search={pullsRoute.useSearch()} kind="pulls" />;
    },
  });

  const orgIssuesRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/org/$org/issues",
    validateSearch: validateDashboardIssuesSearch,
    loaderDeps: issuesLoaderDeps,
    loader: ({ params, deps, location }) =>
      withDashboardErrors(fetchDashboardIssues(params.org, "issues", { ...deps }), () =>
        redirect({ href: location.href, reloadDocument: true }),
      ),
    component: function OrgIssues() {
      return (
        <DashboardIssues data={orgIssuesRoute.useLoaderData()} search={orgIssuesRoute.useSearch()} kind="issues" />
      );
    },
  });

  const orgPullsRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/org/$org/pulls",
    validateSearch: validateDashboardIssuesSearch,
    loaderDeps: issuesLoaderDeps,
    loader: ({ params, deps, location }) =>
      withDashboardErrors(fetchDashboardIssues(params.org, "pulls", { ...deps }), () =>
        redirect({ href: location.href, reloadDocument: true }),
      ),
    component: function OrgPulls() {
      return <DashboardIssues data={orgPullsRoute.useLoaderData()} search={orgPullsRoute.useSearch()} kind="pulls" />;
    },
  });

  return [homeRoute, orgDashboardRoute, issuesRoute, pullsRoute, orgIssuesRoute, orgPullsRoute];
}
