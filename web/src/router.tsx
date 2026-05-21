import {
  Outlet,
  RouterProvider,
  createRootRouteWithContext,
  createRoute,
  createRouter,
  redirect,
} from "@tanstack/react-router";

import { Footer } from "@/components/Footer";
import { Navbar } from "@/components/Navbar";
import { webContext } from "@/lib/context";
import { subUrl } from "@/lib/url";
import type { UserInfo } from "@/lib/user-info";
import { Landing } from "@/pages/Landing";
import { NotFound } from "@/pages/NotFound";
import { SignIn, type SignInPage } from "@/pages/SignIn";

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

const signInRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/user/sign-in",
  beforeLoad: ({ context }) => {
    if (context.user) {
      // eslint-disable-next-line @typescript-eslint/only-throw-error -- TanStack's redirect() returns a sentinel that must be thrown.
      throw redirect({ to: "/", replace: true });
    }
  },
  loader: async (): Promise<SignInPage> => {
    const res = await fetch(subUrl("/api/web/user/sign-in"), { credentials: "same-origin" });
    if (!res.ok) {
      return { loginSources: [] };
    }
    return (await res.json()) as SignInPage;
  },
  component: SignIn,
});

const routeTree = rootRoute.addChildren([landingRoute, signInRoute]);

function makeRouter(context: RouterContext) {
  return createRouter({
    routeTree,
    basepath: webContext.subURL || "/",
    defaultNotFoundComponent: NotFound,
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
