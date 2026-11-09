import type { QueryClient } from "@tanstack/react-query";
import { type AnyRoute, createRoute, notFound, redirect } from "@tanstack/react-router";

import { LoaderResponseError } from "@/lib/loader-error";
import {
  fetchOrgMembers,
  fetchOrgTeam,
  fetchOrgTeamMembers,
  fetchOrgTeamRepos,
  fetchOrgTeams,
} from "@/lib/queries/org";
import { userProfileQuery } from "@/lib/queries/user";
import { OrgMembers } from "@/pages/org/Members";
import { OrgTeam, type TeamPageData } from "@/pages/org/Team";
import { OrgTeams } from "@/pages/org/Teams";

interface RouterContext {
  queryClient: QueryClient;
}

// loadOrgPage loads the organization for the header along with the page, and
// maps the errors to router signals. Like the server-rendered pages, only
// members see them.
async function loadOrgPage<T>(queryClient: QueryClient, org: string, load: () => Promise<T>): Promise<T> {
  try {
    const [, data] = await Promise.all([queryClient.ensureQueryData(userProfileQuery(org)), load()]);
    return data;
  } catch (err) {
    if (err instanceof LoaderResponseError && err.status === 404) {
      // eslint-disable-next-line @typescript-eslint/only-throw-error -- `notFound()` is the documented TanStack Router signal for 404, not an Error subclass.
      throw notFound();
    }
    if (err instanceof LoaderResponseError && (err.status === 401 || err.status === 403)) {
      // The viewer is signed out, has not confirmed their email, or is
      // prohibited from signing in. Reload the page so the server sends them to
      // sign in, account activation, or the login prohibited page.
      // eslint-disable-next-line @typescript-eslint/only-throw-error -- TanStack's redirect() returns a sentinel that must be thrown.
      throw redirect({ href: window.location.href, reloadDocument: true });
    }
    throw err;
  }
}

export function createOrgRoutes(rootRoute: AnyRoute) {
  const membersRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/org/$org/members",
    loader: ({ params, context }) =>
      loadOrgPage((context as RouterContext).queryClient, params.org, () => fetchOrgMembers(params.org)),
    component: function Members() {
      const { org } = membersRoute.useParams();
      return <OrgMembers org={org} members={membersRoute.useLoaderData()} />;
    },
  });

  const teamsRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/org/$org/teams",
    loader: ({ params, context }) =>
      loadOrgPage((context as RouterContext).queryClient, params.org, () => fetchOrgTeams(params.org)),
    component: function Teams() {
      const { org } = teamsRoute.useParams();
      return <OrgTeams org={org} teams={teamsRoute.useLoaderData()} />;
    },
  });

  const teamRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/org/$org/teams/$team",
    loader: ({ params, context }) =>
      loadOrgPage((context as RouterContext).queryClient, params.org, async (): Promise<TeamPageData> => {
        const [team, members] = await Promise.all([
          fetchOrgTeam(params.org, params.team),
          fetchOrgTeamMembers(params.org, params.team),
        ]);
        return { tab: "members", team, members };
      }),
    component: function Team() {
      const { org } = teamRoute.useParams();
      return <OrgTeam org={org} data={teamRoute.useLoaderData()} />;
    },
  });

  const teamReposRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/org/$org/teams/$team/repositories",
    loader: ({ params, context }) =>
      loadOrgPage((context as RouterContext).queryClient, params.org, async (): Promise<TeamPageData> => {
        const [team, repos] = await Promise.all([
          fetchOrgTeam(params.org, params.team),
          fetchOrgTeamRepos(params.org, params.team),
        ]);
        return { tab: "repositories", team, repos };
      }),
    component: function TeamRepos() {
      const { org } = teamReposRoute.useParams();
      return <OrgTeam org={org} data={teamReposRoute.useLoaderData()} />;
    },
  });

  return [membersRoute, teamsRoute, teamRoute, teamReposRoute];
}
