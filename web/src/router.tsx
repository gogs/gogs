import { Outlet, RouterProvider, createRootRouteWithContext, createRoute, createRouter } from "@tanstack/react-router";

import { Footer } from "@/components/Footer";
import { Navbar } from "@/components/Navbar";
import { webContext } from "@/lib/context";
import type { UserInfo } from "@/lib/user-info";
import { Landing } from "@/pages/Landing";
import { NotFound } from "@/pages/NotFound";
import { ServerError } from "@/pages/ServerError";
import { createUserRoutes } from "@/routes/user";

interface RouterContext {
  user: UserInfo | null;
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

const routeTree = rootRoute.addChildren([landingRoute, ...createUserRoutes(rootRoute)]);

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

export function AppRouter({ user }: { user: UserInfo | null }) {
  const router = makeRouter({ user });
  return <RouterProvider router={router} />;
}
