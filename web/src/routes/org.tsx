import type { QueryClient } from "@tanstack/react-query";
import { type AnyRoute, createRoute, notFound, redirect } from "@tanstack/react-router";

import { LoaderResponseError } from "@/lib/loader-error";
import {
  fetchOrgMembers,
  fetchOrgSettings,
  fetchOrgTeam,
  fetchOrgTeamMembers,
  fetchOrgTeamRepos,
  fetchOrgTeams,
} from "@/lib/queries/org";
import { userProfileQuery } from "@/lib/queries/user";
import { type WebhookType, fetchWebhook, fetchWebhooks } from "@/lib/queries/webhooks";
import type { UserInfo } from "@/lib/user-info";
import { WebhookEdit, WebhookNew, Webhooks } from "@/pages/Webhooks";
import { OrgCreate } from "@/pages/org/Create";
import { OrgInvite } from "@/pages/org/Invite";
import { OrgMembers } from "@/pages/org/Members";
import { OrgDelete, OrgSettings } from "@/pages/org/Settings";
import { OrgTeam, type TeamPageData } from "@/pages/org/Team";
import { TeamForm } from "@/pages/org/TeamForm";
import { OrgTeams } from "@/pages/org/Teams";

interface RouterContext {
  user: UserInfo | null;
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

// requireOwner turns away members who are not owners, like the
// server-rendered pages, once the organization is loaded.
async function requireOwner(queryClient: QueryClient, org: string) {
  const profile = await queryClient.ensureQueryData(userProfileQuery(org));
  if (profile.kind !== "organization" || !profile.viewerIsOwner) {
    // eslint-disable-next-line @typescript-eslint/only-throw-error -- `notFound()` is the documented TanStack Router signal for 404, not an Error subclass.
    throw notFound();
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

  const inviteRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/org/$org/invitations/new",
    loader: ({ params, context }) =>
      loadOrgPage((context as RouterContext).queryClient, params.org, () =>
        requireOwner((context as RouterContext).queryClient, params.org),
      ),
    component: function Invite() {
      const { org } = inviteRoute.useParams();
      return <OrgInvite org={org} />;
    },
  });

  const newTeamRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/org/$org/teams/new",
    loader: ({ params, context }) =>
      loadOrgPage((context as RouterContext).queryClient, params.org, () =>
        requireOwner((context as RouterContext).queryClient, params.org),
      ),
    component: function NewTeam() {
      const { org } = newTeamRoute.useParams();
      return <TeamForm org={org} />;
    },
  });

  const editTeamRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/org/$org/teams/$team/edit",
    loader: ({ params, context }) => {
      const { queryClient } = context as RouterContext;
      return loadOrgPage(queryClient, params.org, async () => {
        const [, team] = await Promise.all([
          requireOwner(queryClient, params.org),
          fetchOrgTeam(params.org, params.team),
        ]);
        return team;
      });
    },
    component: function EditTeam() {
      const { org } = editTeamRoute.useParams();
      const team = editTeamRoute.useLoaderData();
      // Start over when the team changes, e.g., after renaming it.
      return <TeamForm key={team.lowerName} org={org} team={team} />;
    },
  });

  const orgCreateRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/org/create",
    loader: ({ context }) => {
      // Like the server-rendered page, only users allowed to create
      // organizations get here.
      if (!(context as RouterContext).user?.canCreateOrganization) {
        // eslint-disable-next-line @typescript-eslint/only-throw-error -- `notFound()` is the documented TanStack Router signal for 404, not an Error subclass.
        throw notFound();
      }
    },
    component: OrgCreate,
  });

  const settingsRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/org/$org/settings",
    loader: ({ params, context }) => {
      const { queryClient } = context as RouterContext;
      return loadOrgPage(queryClient, params.org, async () => {
        const [, settings] = await Promise.all([requireOwner(queryClient, params.org), fetchOrgSettings(params.org)]);
        return settings;
      });
    },
    component: function Settings() {
      const { org } = settingsRoute.useParams();
      const settings = settingsRoute.useLoaderData();
      // Start over when the organization changes, e.g., after renaming it.
      return <OrgSettings key={settings.name} org={org} settings={settings} />;
    },
  });

  const deleteRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/org/$org/settings/delete",
    loader: ({ params, context }) =>
      loadOrgPage((context as RouterContext).queryClient, params.org, () =>
        requireOwner((context as RouterContext).queryClient, params.org),
      ),
    component: function Delete() {
      const { org } = deleteRoute.useParams();
      return <OrgDelete org={org} />;
    },
  });

  // loadOwnerPage loads a page of the organization only its owners see.
  function loadOwnerPage<T>(context: unknown, org: string, load: () => Promise<T>) {
    const { queryClient } = context as RouterContext;
    return loadOrgPage(queryClient, org, async () => {
      const [, data] = await Promise.all([requireOwner(queryClient, org), load()]);
      return data;
    });
  }

  const webhooksRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/org/$org/settings/hooks",
    loader: ({ params, context }) => loadOwnerPage(context, params.org, () => fetchWebhooks({ org: params.org })),
    component: function OrgWebhooks() {
      const { org } = webhooksRoute.useParams();
      return <Webhooks scope={{ org }} list={webhooksRoute.useLoaderData()} />;
    },
  });

  const webhookNewRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/org/$org/settings/hooks/$type/new",
    loader: ({ params, context }) =>
      loadOwnerPage(context, params.org, async () => {
        const { types } = await fetchWebhooks({ org: params.org });
        if (!types.includes(params.type as WebhookType)) {
          // eslint-disable-next-line @typescript-eslint/only-throw-error -- `notFound()` is the documented TanStack Router signal for 404, not an Error subclass.
          throw notFound();
        }
      }),
    component: function OrgWebhookNew() {
      const { org, type } = webhookNewRoute.useParams();
      return <WebhookNew key={type} scope={{ org }} type={type as WebhookType} />;
    },
  });

  const webhookRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/org/$org/settings/hooks/$id",
    loader: ({ params, context }) => {
      const id = Number(params.id);
      if (!Number.isInteger(id) || id <= 0) {
        // eslint-disable-next-line @typescript-eslint/only-throw-error -- `notFound()` is the documented TanStack Router signal for 404, not an Error subclass.
        throw notFound();
      }
      return loadOwnerPage(context, params.org, () => fetchWebhook({ org: params.org }, id));
    },
    component: function OrgWebhook() {
      const { org } = webhookRoute.useParams();
      const hook = webhookRoute.useLoaderData();
      return <WebhookEdit key={hook.id} scope={{ org }} hook={hook} />;
    },
  });

  return [
    orgCreateRoute,
    membersRoute,
    teamsRoute,
    teamRoute,
    teamReposRoute,
    inviteRoute,
    newTeamRoute,
    editTeamRoute,
    settingsRoute,
    webhooksRoute,
    webhookNewRoute,
    webhookRoute,
    deleteRoute,
  ];
}
