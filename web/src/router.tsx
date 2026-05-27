import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  Outlet,
  RouterProvider,
  createRootRouteWithContext,
  createRoute,
  createRouter,
  notFound,
} from "@tanstack/react-router";

import { Footer } from "@/components/Footer";
import { Navbar } from "@/components/Navbar";
import { TooltipProvider } from "@/components/ui/tooltip";
import { webContext } from "@/lib/context";
import { LoaderResponseError, loaderResponseError } from "@/lib/loader-error";
import { repoHeaderQuery } from "@/lib/queries/repo";
import { subUrl } from "@/lib/url";
import type { UserInfo } from "@/lib/user-info";
import { Landing } from "@/pages/Landing";
import { NotFound } from "@/pages/NotFound";
import { ServerError } from "@/pages/ServerError";
import { RepoCommit, type RepoCommitPage } from "@/pages/repo/Commit";
import { validateRepoCommitSearch } from "@/pages/repo/Commit.search";
import { createUserRoutes } from "@/routes/user";

// Match the legacy server-side route constraint (see `web.go` near the
// `/commit/:sha([a-f0-9]{7,40})$` declaration). The server enforces the same
// shape for SEO and to skip the SPA shell for malformed paths; this client
// check short-circuits the loader so we render 404 without a wasted fetch.
const SHA_RE = /^[a-f0-9]{7,40}$/;

interface RouterContext {
  user: UserInfo | null;
  queryClient: QueryClient;
}

function RootLayout() {
  return (
    <div className="flex min-h-dvh flex-col">
      <Navbar />
      <Outlet />
      <Footer />
    </div>
  );
}

const rootRoute = createRootRouteWithContext<RouterContext>()({
  component: RootLayout,
  notFoundComponent: NotFound,
});

const landingRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/",
  component: Landing,
});

const repoCommitRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/$owner/$repo/commit/$sha",
  validateSearch: validateRepoCommitSearch,
  // Reject malformed SHA at parse time so the route doesn't match for paths
  // like `/owner/repo/commit/garbage`. The thrown `notFound()` bubbles to the
  // root route's NotFound component.
  params: {
    parse: (raw: { owner: string; repo: string; sha: string }) => {
      if (!SHA_RE.test(raw.sha)) {
        // eslint-disable-next-line @typescript-eslint/only-throw-error -- `notFound()` is the documented TanStack Router signal for 404, not an Error subclass.
        throw notFound();
      }
      return raw;
    },
    stringify: (params: { owner: string; repo: string; sha: string }) => params,
  },
  loaderDeps: ({ search }) => ({ whitespace: search.whitespace }),
  loader: async ({ params, deps, context }): Promise<RepoCommitPage> => {
    const metaURL = subUrl(`/api/web/${params.owner}/${params.repo}/commit/${params.sha}`);
    const rawBase = subUrl(`/${params.owner}/${params.repo}/commit/${params.sha}.diff`);
    const rawURL = deps.whitespace ? `${rawBase}?whitespace=${encodeURIComponent(deps.whitespace)}` : rawBase;
    // Three requests in parallel: repo header (via Query cache for cross-page
    // reuse), commit metadata JSON, raw patch text. Splitting the patch out
    // skips JSON-string escaping and lets the browser cache the (often large)
    // patch separately from the metadata.
    try {
      const [, meta, patch] = await Promise.all([
        context.queryClient.ensureQueryData(repoHeaderQuery(params.owner, params.repo)),
        fetch(metaURL, { credentials: "same-origin" }).then(async (res) => {
          if (!res.ok) throw await loaderResponseError(res);
          return (await res.json()) as Omit<RepoCommitPage, "patch">;
        }),
        fetch(rawURL, { credentials: "same-origin" }).then(async (res) => {
          if (!res.ok) throw await loaderResponseError(res);
          return res.text();
        }),
      ]);
      return { ...meta, patch };
    } catch (err) {
      if (err instanceof LoaderResponseError && err.status === 404) {
        // eslint-disable-next-line @typescript-eslint/only-throw-error -- `notFound()` is the documented TanStack Router signal for 404, not an Error subclass.
        throw notFound();
      }
      throw err;
    }
  },
  component: RepoCommit,
});

const routeTree = rootRoute.addChildren([landingRoute, ...createUserRoutes(rootRoute), repoCommitRoute]);

function makeRouter(context: RouterContext) {
  return createRouter({
    routeTree,
    basepath: webContext.subURL || "/",
    defaultNotFoundComponent: NotFound,
    defaultErrorComponent: ServerError,
    context,
  });
}

type AppRouterInstance = ReturnType<typeof makeRouter>;

declare module "@tanstack/react-router" {
  interface Register {
    router: AppRouterInstance;
  }
}

const queryClient = new QueryClient();

export function AppRouter({ user }: { user: UserInfo | null }) {
  const router = makeRouter({ user, queryClient });
  return (
    <QueryClientProvider client={queryClient}>
      <TooltipProvider delayDuration={300}>
        <RouterProvider router={router} />
      </TooltipProvider>
    </QueryClientProvider>
  );
}
