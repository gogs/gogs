import type { QueryClient } from "@tanstack/react-query";
import { type AnyRoute, createRoute, notFound, redirect } from "@tanstack/react-router";

import { CustomSlot } from "@/components/CustomSlot";
import { webContext } from "@/lib/context";
import { LoaderResponseError, authenticationRedirect } from "@/lib/loader-error";
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
async function withDashboardErrors<T>(load: Promise<T>): Promise<T> {
  try {
    return await load;
  } catch (err) {
    if (err instanceof LoaderResponseError && err.status === 404) {
      // eslint-disable-next-line @typescript-eslint/only-throw-error -- `notFound()` is the documented TanStack Router signal for 404, not an Error subclass.
      throw notFound();
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
      if (!user) {
        // The landing page decides whether visitors need to sign in.
        if (webContext.landingURL) {
          // eslint-disable-next-line @typescript-eslint/only-throw-error -- TanStack's redirect() returns a sentinel that must be thrown.
          throw redirect({ href: webContext.subURL + webContext.landingURL, replace: true, reloadDocument: true });
        }
        if (webContext.requireAuthenticatedViewer) {
          // eslint-disable-next-line @typescript-eslint/only-throw-error -- TanStack's redirect() returns a sentinel that must be thrown.
          throw authenticationRedirect("/");
        }
        return;
      }
      await withDashboardErrors(loadDashboard(queryClient));
    },
    // Visitors get the landing page, authenticated users their dashboard.
    component: function Home() {
      return useUserInfo() ? <Dashboard /> : <CustomSlot name="landing" fallback={Landing} />;
    },
  });

  const orgDashboardRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/org/$org/dashboard",
    loader: async ({ params, context }) => {
      const { queryClient } = context as RouterContext;
      await withDashboardErrors(loadDashboard(queryClient, params.org));
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
    loader: ({ deps }) => withDashboardErrors(fetchDashboardIssues(undefined, "issues", { ...deps })),
    component: function Issues() {
      return <DashboardIssues data={issuesRoute.useLoaderData()} search={issuesRoute.useSearch()} kind="issues" />;
    },
  });

  const pullsRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/pulls",
    validateSearch: validateDashboardIssuesSearch,
    loaderDeps: issuesLoaderDeps,
    loader: ({ deps }) => withDashboardErrors(fetchDashboardIssues(undefined, "pulls", { ...deps })),
    component: function Pulls() {
      return <DashboardIssues data={pullsRoute.useLoaderData()} search={pullsRoute.useSearch()} kind="pulls" />;
    },
  });

  const orgIssuesRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/org/$org/issues",
    validateSearch: validateDashboardIssuesSearch,
    loaderDeps: issuesLoaderDeps,
    loader: ({ params, deps }) => withDashboardErrors(fetchDashboardIssues(params.org, "issues", { ...deps })),
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
    loader: ({ params, deps }) => withDashboardErrors(fetchDashboardIssues(params.org, "pulls", { ...deps })),
    component: function OrgPulls() {
      return <DashboardIssues data={orgPullsRoute.useLoaderData()} search={orgPullsRoute.useSearch()} kind="pulls" />;
    },
  });

  return [homeRoute, orgDashboardRoute, issuesRoute, pullsRoute, orgIssuesRoute, orgPullsRoute];
}
