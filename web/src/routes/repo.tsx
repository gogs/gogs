import type { QueryClient } from "@tanstack/react-query";
import { type AnyRoute, createRoute, notFound, redirect } from "@tanstack/react-router";

import { LoaderResponseError, loadSignedInPage, loaderResponseError } from "@/lib/loader-error";
import { type ListSearch, type PageSearch, validateListSearch, validatePageSearch } from "@/lib/page-search";
import {
  type RepoCompareData,
  type RepoSrcData,
  fetchRepoBranchSettings,
  fetchRepoBranches,
  fetchRepoCollaborators,
  fetchRepoCommits,
  fetchRepoDeployKeys,
  fetchRepoEditorTarget,
  fetchRepoForkOptions,
  fetchRepoForks,
  fetchRepoGitHook,
  fetchRepoGitHooks,
  fetchRepoIssue,
  fetchRepoIssues,
  fetchRepoLabels,
  fetchRepoMilestone,
  fetchRepoMilestones,
  fetchRepoNewIssue,
  fetchRepoNewOptions,
  fetchRepoNewRelease,
  fetchRepoPeople,
  fetchRepoProtectedBranch,
  fetchRepoRelease,
  fetchRepoReleases,
  fetchRepoSrc,
  fetchRepoWiki,
  fetchRepoWikiPageSource,
  fetchRepoWikiPages,
  repoHeaderQuery,
  repoSettingsQuery,
} from "@/lib/queries/repo";
import { type WebhookType, fetchWebhook, fetchWebhooks } from "@/lib/queries/webhooks";
import { subUrl } from "@/lib/url";
import { WebhookEdit, WebhookNew, Webhooks } from "@/pages/Webhooks";
import { RepoBranches } from "@/pages/repo/Branches";
import { RepoCode } from "@/pages/repo/Code";
import { RepoCommit, type RepoCommitPage } from "@/pages/repo/Commit";
import { type RepoCommitSearch, validateRepoCommitSearch } from "@/pages/repo/Commit.search";
import { RepoCommits } from "@/pages/repo/Commits";
import { RepoCompare } from "@/pages/repo/Compare";
import { RepoFileDelete, RepoFileEditor, RepoFileUpload } from "@/pages/repo/Editor";
import { RepoForks } from "@/pages/repo/Forks";
import { RepoIssue } from "@/pages/repo/Issue";
import { RepoIssues } from "@/pages/repo/Issues";
import { type RepoIssuesSearch, validateRepoIssuesSearch } from "@/pages/repo/Issues.search";
import { RepoLabels } from "@/pages/repo/Labels";
import { RepoMilestoneEditor, RepoMilestones, type RepoMilestonesSearch } from "@/pages/repo/Milestones";
import { RepoCreate, RepoFork, RepoMigrate } from "@/pages/repo/New";
import { RepoNewIssue } from "@/pages/repo/NewIssue";
import { type RepoNewIssueSearch, validateRepoNewIssueSearch } from "@/pages/repo/NewIssue.search";
import { RepoPeople } from "@/pages/repo/People";
import { RepoReleaseEditor, RepoReleases } from "@/pages/repo/Releases";
import { type RepoReleasesSearch, validateRepoReleasesSearch } from "@/pages/repo/Releases.search";
import {
  RepoBranchProtection,
  RepoBranchSettings,
  RepoCollaboration,
  RepoDeployKeys,
  RepoGitHookEdit,
  RepoGitHooks,
  RepoOptions,
} from "@/pages/repo/Settings";
import { RepoWiki, RepoWikiEditor, RepoWikiPages } from "@/pages/repo/Wiki";
import { wikiPageName } from "@/pages/repo/Wiki.path";

// Match the legacy server-side route constraint (see `web.go` near the
// `/commit/:sha([a-f0-9]{7,64})$` declaration). The server enforces the same
// shape for SEO and to skip the SPA shell for malformed paths; this client
// check short-circuits the loader so we render 404 without a wasted fetch.
const SHA_RE = /^[a-f0-9]{7,64}$/;
const COMPARE_RANGE_RE = /^([a-f0-9]{40}|[a-f0-9]{64})\.\.\.([a-f0-9]{40}|[a-f0-9]{64})$/;

interface RouterContext {
  queryClient: QueryClient;
}

// loadDiffPage loads the header of a repository, the metadata of a diff page
// at the path and the diff itself. The diff is fetched as raw text from the
// path with a ".diff" suffix, which skips JSON-string escaping and lets the
// browser cache the (often large) diff separately from the metadata.
async function loadDiffPage<T>(
  queryClient: QueryClient,
  owner: string,
  repo: string,
  path: string,
  whitespace: RepoCommitSearch["whitespace"],
): Promise<T & { patch: string }> {
  const rawBase = subUrl(`/${owner}/${repo}${path}.diff`);
  const rawURL = whitespace ? `${rawBase}?whitespace=${encodeURIComponent(whitespace)}` : rawBase;
  try {
    const [, meta, patch] = await Promise.all([
      queryClient.ensureQueryData(repoHeaderQuery(owner, repo)),
      fetch(subUrl(`/api/web/${owner}/${repo}${path}`), { credentials: "same-origin" }).then(async (res) => {
        if (!res.ok) throw await loaderResponseError(res);
        return (await res.json()) as T;
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
}

export function createRepoRoutes(rootRoute: AnyRoute) {
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
    loaderDeps: ({ search }: { search: RepoCommitSearch }) => ({ whitespace: search.whitespace }),
    loader: ({ params, deps, context }) =>
      loadDiffPage<Omit<RepoCommitPage, "patch">>(
        (context as RouterContext).queryClient,
        params.owner,
        params.repo,
        `/commit/${params.sha}`,
        deps.whitespace,
      ),
    component: RepoCommit,
  });

  const repoCompareRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/$owner/$repo/compare/$range",
    validateSearch: validateRepoCommitSearch,
    // Only full commit IDs are compared here, the same as the server route in
    // `web.go`. Comparing branches goes to the pull request compare page.
    params: {
      parse: (raw: { owner: string; repo: string; range: string }) => {
        if (!COMPARE_RANGE_RE.test(raw.range)) {
          // eslint-disable-next-line @typescript-eslint/only-throw-error -- `notFound()` is the documented TanStack Router signal for 404, not an Error subclass.
          throw notFound();
        }
        return raw;
      },
      stringify: (params: { owner: string; repo: string; range: string }) => params,
    },
    loaderDeps: ({ search }: { search: RepoCommitSearch }) => ({ whitespace: search.whitespace }),
    loader: ({ params, deps, context }) =>
      loadDiffPage<RepoCompareData>(
        (context as RouterContext).queryClient,
        params.owner,
        params.repo,
        `/compare/${params.range}`,
        deps.whitespace,
      ),
    component: function Compare() {
      const { owner, repo } = repoCompareRoute.useParams();
      return (
        <RepoCompare
          owner={owner}
          name={repo}
          data={repoCompareRoute.useLoaderData()}
          search={repoCompareRoute.useSearch()}
        />
      );
    },
  });

  // loadRepoPage loads the header of a repository along with the data of the
  // page.
  async function loadRepoPage<T>(queryClient: QueryClient, owner: string, repo: string, load: () => Promise<T>) {
    try {
      const [, data] = await Promise.all([queryClient.ensureQueryData(repoHeaderQuery(owner, repo)), load()]);
      return data;
    } catch (err) {
      if (err instanceof LoaderResponseError && err.status === 404) {
        // eslint-disable-next-line @typescript-eslint/only-throw-error -- `notFound()` is the documented TanStack Router signal for 404, not an Error subclass.
        throw notFound();
      }
      if (err instanceof LoaderResponseError && (err.status === 401 || err.status === 403)) {
        // The viewer is not allowed to see site content. Reload the page so the
        // server sends them to sign in, account activation, or the login
        // prohibited page.
        // eslint-disable-next-line @typescript-eslint/only-throw-error -- TanStack's redirect() returns a sentinel that must be thrown.
        throw redirect({ href: window.location.href, reloadDocument: true });
      }
      throw err;
    }
  }

  const repoStargazersRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/$owner/$repo/stars",
    validateSearch: validatePageSearch,
    loaderDeps: ({ search }: { search: PageSearch }) => ({ page: search.page }),
    loader: ({ params, deps, context }) =>
      loadRepoPage((context as RouterContext).queryClient, params.owner, params.repo, () =>
        fetchRepoPeople(params.owner, params.repo, "stargazers", deps.page),
      ),
    component: function RepoStargazers() {
      const { owner, repo } = repoStargazersRoute.useParams();
      return <RepoPeople owner={owner} name={repo} list="stargazers" data={repoStargazersRoute.useLoaderData()} />;
    },
  });

  const repoWatchersRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/$owner/$repo/watchers",
    validateSearch: validatePageSearch,
    loaderDeps: ({ search }: { search: PageSearch }) => ({ page: search.page }),
    loader: ({ params, deps, context }) =>
      loadRepoPage((context as RouterContext).queryClient, params.owner, params.repo, () =>
        fetchRepoPeople(params.owner, params.repo, "watchers", deps.page),
      ),
    component: function RepoWatchers() {
      const { owner, repo } = repoWatchersRoute.useParams();
      return <RepoPeople owner={owner} name={repo} list="watchers" data={repoWatchersRoute.useLoaderData()} />;
    },
  });

  interface RepoNewSearch {
    // The organization to create the repository for.
    org?: number;
    // Whether a migration starts out as a mirror.
    mirror?: boolean;
  }

  function validateRepoNewSearch(search: Record<string, unknown>): RepoNewSearch {
    const org = Number(search.org);
    return {
      org: Number.isInteger(org) && org > 0 ? org : undefined,
      mirror: String(search.mirror) === "1" || search.mirror === true ? true : undefined,
    };
  }

  async function loadRepoNewOptions(org: number | undefined) {
    try {
      return await fetchRepoNewOptions(org);
    } catch (err) {
      // An organization the viewer cannot create repositories for is not
      // found.
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

  const repoCreateRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/repo/create",
    validateSearch: validateRepoNewSearch,
    loaderDeps: ({ search }: { search: RepoNewSearch }) => ({ org: search.org }),
    loader: ({ deps }) => loadRepoNewOptions(deps.org),
    component: function Create() {
      const options = repoCreateRoute.useLoaderData();
      // Start over when the owner asked for changes.
      return <RepoCreate key={options.ownerID} options={options} />;
    },
  });

  const repoMigrateRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/repo/migrate",
    validateSearch: validateRepoNewSearch,
    loaderDeps: ({ search }: { search: RepoNewSearch }) => ({ org: search.org, mirror: search.mirror ?? false }),
    loader: async ({ deps }) => ({ options: await loadRepoNewOptions(deps.org), mirror: deps.mirror }),
    component: function Migrate() {
      const { options, mirror } = repoMigrateRoute.useLoaderData();
      return <RepoMigrate key={`${options.ownerID}-${mirror}`} options={options} mirror={mirror} />;
    },
  });

  const repoForkRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/repo/fork/$id",
    loader: async ({ params }) => {
      const id = Number(params.id);
      if (!Number.isInteger(id) || id <= 0) {
        // eslint-disable-next-line @typescript-eslint/only-throw-error -- `notFound()` is the documented TanStack Router signal for 404, not an Error subclass.
        throw notFound();
      }
      try {
        return { id, options: await fetchRepoForkOptions(id) };
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
    },
    component: function Fork() {
      const { id, options } = repoForkRoute.useLoaderData();
      return <RepoFork key={id} id={id} options={options} />;
    },
  });

  // loadSettingsPage loads the header and the settings of a repository, which
  // every settings page shows, along with the data of the page. Only admins of
  // the repository get past it.
  function loadSettingsPage<T>(queryClient: QueryClient, owner: string, repo: string, load: () => Promise<T>) {
    return loadSignedInPage(async () => {
      const [, , data] = await Promise.all([
        queryClient.ensureQueryData(repoHeaderQuery(owner, repo)),
        queryClient.ensureQueryData(repoSettingsQuery(owner, repo)),
        load(),
      ]);
      return data;
    });
  }

  const repoSettingsRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/$owner/$repo/settings",
    loader: ({ params, context }) =>
      loadSettingsPage((context as RouterContext).queryClient, params.owner, params.repo, () => Promise.resolve()),
    component: function Settings() {
      const { owner, repo } = repoSettingsRoute.useParams();
      // Start over when the repository changes, e.g., after renaming it.
      return <RepoOptions key={`${owner}/${repo}`} owner={owner} name={repo} />;
    },
  });

  const repoBranchSettingsRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/$owner/$repo/settings/branches",
    loader: ({ params, context }) =>
      loadSettingsPage((context as RouterContext).queryClient, params.owner, params.repo, () =>
        fetchRepoBranchSettings(params.owner, params.repo),
      ),
    component: function BranchSettings() {
      const { owner, repo } = repoBranchSettingsRoute.useParams();
      const settings = repoBranchSettingsRoute.useLoaderData();
      // Start over once the default branch is saved, so the choice shows it.
      return <RepoBranchSettings key={settings.defaultBranch} owner={owner} name={repo} settings={settings} />;
    },
  });

  // Branch names can contain slashes, so the branch is the rest of the path.
  const repoBranchProtectionRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/$owner/$repo/settings/branches/$",
    // Without a branch the path is the branch settings, which a splat would
    // otherwise take over.
    params: { parse: (params) => (params._splat ? params : false) },
    loader: ({ params, context }) =>
      loadSettingsPage((context as RouterContext).queryClient, params.owner, params.repo, () =>
        fetchRepoProtectedBranch(params.owner, params.repo, params._splat ?? ""),
      ),
    component: function BranchProtection() {
      const { owner, repo, _splat: branch } = repoBranchProtectionRoute.useParams();
      return (
        <RepoBranchProtection
          key={`${owner}/${repo}@${branch}`}
          owner={owner}
          name={repo}
          branch={repoBranchProtectionRoute.useLoaderData()}
        />
      );
    },
  });

  const repoGitHooksRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/$owner/$repo/settings/hooks/git",
    loader: ({ params, context }) =>
      loadSettingsPage((context as RouterContext).queryClient, params.owner, params.repo, () =>
        fetchRepoGitHooks(params.owner, params.repo),
      ),
    component: function GitHooks() {
      const { owner, repo } = repoGitHooksRoute.useParams();
      return <RepoGitHooks owner={owner} name={repo} hooks={repoGitHooksRoute.useLoaderData()} />;
    },
  });

  const repoGitHookRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/$owner/$repo/settings/hooks/git/$name",
    loader: ({ params, context }) =>
      loadSettingsPage((context as RouterContext).queryClient, params.owner, params.repo, () =>
        fetchRepoGitHook(params.owner, params.repo, params.name),
      ),
    component: function GitHook() {
      const { owner, repo, name } = repoGitHookRoute.useParams();
      return (
        <RepoGitHookEdit
          key={`${owner}/${repo}@${name}`}
          owner={owner}
          name={repo}
          hook={repoGitHookRoute.useLoaderData()}
        />
      );
    },
  });

  const repoCollaborationRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/$owner/$repo/settings/collaboration",
    loader: ({ params, context }) =>
      loadSettingsPage((context as RouterContext).queryClient, params.owner, params.repo, () =>
        fetchRepoCollaborators(params.owner, params.repo),
      ),
    component: function Collaboration() {
      const { owner, repo } = repoCollaborationRoute.useParams();
      return <RepoCollaboration owner={owner} name={repo} collaborators={repoCollaborationRoute.useLoaderData()} />;
    },
  });

  const repoDeployKeysRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/$owner/$repo/settings/keys",
    loader: ({ params, context }) =>
      loadSettingsPage((context as RouterContext).queryClient, params.owner, params.repo, () =>
        fetchRepoDeployKeys(params.owner, params.repo),
      ),
    component: function DeployKeys() {
      const { owner, repo } = repoDeployKeysRoute.useParams();
      return <RepoDeployKeys owner={owner} name={repo} keys={repoDeployKeysRoute.useLoaderData()} />;
    },
  });

  const repoWebhooksRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/$owner/$repo/settings/hooks",
    loader: ({ params, context }) =>
      loadSettingsPage((context as RouterContext).queryClient, params.owner, params.repo, () =>
        fetchWebhooks({ owner: params.owner, repo: params.repo }),
      ),
    component: function RepoWebhooks() {
      const { owner, repo } = repoWebhooksRoute.useParams();
      return <Webhooks scope={{ owner, repo }} list={repoWebhooksRoute.useLoaderData()} />;
    },
  });

  const repoWebhookNewRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/$owner/$repo/settings/hooks/$type/new",
    loader: ({ params, context }) =>
      loadSettingsPage((context as RouterContext).queryClient, params.owner, params.repo, async () => {
        const { types } = await fetchWebhooks({ owner: params.owner, repo: params.repo });
        if (!types.includes(params.type as WebhookType)) {
          // eslint-disable-next-line @typescript-eslint/only-throw-error -- `notFound()` is the documented TanStack Router signal for 404, not an Error subclass.
          throw notFound();
        }
      }),
    component: function RepoWebhookNew() {
      const { owner, repo, type } = repoWebhookNewRoute.useParams();
      return <WebhookNew key={type} scope={{ owner, repo }} type={type as WebhookType} />;
    },
  });

  const repoWebhookRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/$owner/$repo/settings/hooks/$id",
    loader: ({ params, context }) => {
      const id = Number(params.id);
      if (!Number.isInteger(id) || id <= 0) {
        // eslint-disable-next-line @typescript-eslint/only-throw-error -- `notFound()` is the documented TanStack Router signal for 404, not an Error subclass.
        throw notFound();
      }
      return loadSettingsPage((context as RouterContext).queryClient, params.owner, params.repo, () =>
        fetchWebhook({ owner: params.owner, repo: params.repo }, id),
      );
    },
    component: function RepoWebhook() {
      const { owner, repo } = repoWebhookRoute.useParams();
      const hook = repoWebhookRoute.useLoaderData();
      return <WebhookEdit key={hook.id} scope={{ owner, repo }} hook={hook} />;
    },
  });

  const repoLabelsRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/$owner/$repo/labels",
    loader: ({ params, context }) =>
      loadRepoPage((context as RouterContext).queryClient, params.owner, params.repo, () =>
        fetchRepoLabels(params.owner, params.repo),
      ),
    component: function Labels() {
      const { owner, repo } = repoLabelsRoute.useParams();
      return <RepoLabels key={`${owner}/${repo}`} owner={owner} name={repo} data={repoLabelsRoute.useLoaderData()} />;
    },
  });

  const repoMilestonesRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/$owner/$repo/milestones",
    validateSearch: (search: Record<string, unknown>): RepoMilestonesSearch => ({
      state: search.state === "closed" ? "closed" : undefined,
      ...validatePageSearch(search),
    }),
    loaderDeps: ({ search }: { search: RepoMilestonesSearch }) => ({ state: search.state, page: search.page }),
    loader: ({ params, deps, context }) =>
      loadRepoPage((context as RouterContext).queryClient, params.owner, params.repo, () =>
        fetchRepoMilestones(params.owner, params.repo, deps.state === "closed", deps.page),
      ),
    component: function Milestones() {
      const { owner, repo } = repoMilestonesRoute.useParams();
      return (
        <RepoMilestones
          owner={owner}
          name={repo}
          data={repoMilestonesRoute.useLoaderData()}
          search={repoMilestonesRoute.useSearch()}
        />
      );
    },
  });

  const repoMilestoneNewRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/$owner/$repo/milestones/new",
    loader: ({ params, context }) =>
      loadRepoPage((context as RouterContext).queryClient, params.owner, params.repo, async () => {
        // Like the server-rendered page, only writers find it.
        const { viewerCanWrite } = await fetchRepoMilestones(params.owner, params.repo, false);
        if (!viewerCanWrite) {
          // eslint-disable-next-line @typescript-eslint/only-throw-error -- `notFound()` is the documented TanStack Router signal for 404, not an Error subclass.
          throw notFound();
        }
      }),
    component: function MilestoneNew() {
      const { owner, repo } = repoMilestoneNewRoute.useParams();
      return <RepoMilestoneEditor key={`${owner}/${repo}`} owner={owner} name={repo} />;
    },
  });

  const repoMilestoneEditRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/$owner/$repo/milestones/$id/edit",
    loader: async ({ params, context }) => {
      const id = Number(params.id);
      if (!Number.isInteger(id) || id <= 0) {
        // eslint-disable-next-line @typescript-eslint/only-throw-error -- `notFound()` is the documented TanStack Router signal for 404, not an Error subclass.
        throw notFound();
      }
      const milestone = await loadRepoPage((context as RouterContext).queryClient, params.owner, params.repo, () =>
        fetchRepoMilestone(params.owner, params.repo, id),
      );
      return { id, milestone };
    },
    component: function MilestoneEdit() {
      const { owner, repo } = repoMilestoneEditRoute.useParams();
      const { id, milestone } = repoMilestoneEditRoute.useLoaderData();
      return <RepoMilestoneEditor key={id} owner={owner} name={repo} id={id} initial={milestone} />;
    },
  });

  function createIssueListRoute(kind: "issues" | "pulls") {
    const route = createRoute({
      getParentRoute: () => rootRoute,
      path: `/$owner/$repo/${kind}`,
      validateSearch: validateRepoIssuesSearch,
      loaderDeps: ({ search }: { search: RepoIssuesSearch }) => search,
      loader: ({ params, deps, context }) =>
        loadRepoPage((context as RouterContext).queryClient, params.owner, params.repo, () =>
          fetchRepoIssues(params.owner, params.repo, kind, { ...deps }),
        ),
      component: function IssueList() {
        const { owner, repo } = route.useParams();
        return (
          <RepoIssues owner={owner} name={repo} kind={kind} data={route.useLoaderData()} search={route.useSearch()} />
        );
      },
    });
    return route;
  }
  const repoIssuesRoute = createIssueListRoute("issues");
  const repoPullsRoute = createIssueListRoute("pulls");

  const repoNewIssueRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/$owner/$repo/issues/new",
    validateSearch: validateRepoNewIssueSearch,
    loader: ({ params, context }) =>
      loadRepoPage((context as RouterContext).queryClient, params.owner, params.repo, () =>
        fetchRepoNewIssue(params.owner, params.repo),
      ),
    component: function NewIssue() {
      const { owner, repo } = repoNewIssueRoute.useParams();
      const search: RepoNewIssueSearch = repoNewIssueRoute.useSearch();
      return (
        <RepoNewIssue
          key={`${owner}/${repo}`}
          owner={owner}
          name={repo}
          options={repoNewIssueRoute.useLoaderData()}
          search={search}
        />
      );
    },
  });

  const repoIssueRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/$owner/$repo/issues/$index",
    loader: ({ params, context }) => {
      const index = Number(params.index);
      if (!Number.isInteger(index) || index <= 0) {
        // eslint-disable-next-line @typescript-eslint/only-throw-error -- `notFound()` is the documented TanStack Router signal for 404, not an Error subclass.
        throw notFound();
      }
      return loadRepoPage((context as RouterContext).queryClient, params.owner, params.repo, () =>
        fetchRepoIssue(params.owner, params.repo, index),
      );
    },
    component: function Issue() {
      const { owner, repo } = repoIssueRoute.useParams();
      const data = repoIssueRoute.useLoaderData();
      // Start over on another issue, e.g., after following a reference.
      return <RepoIssue key={`${owner}/${repo}#${data.index}`} owner={owner} name={repo} data={data} />;
    },
  });

  const repoForksRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/$owner/$repo/forks",
    loader: ({ params, context }) =>
      loadRepoPage((context as RouterContext).queryClient, params.owner, params.repo, () =>
        fetchRepoForks(params.owner, params.repo),
      ),
    component: function Forks() {
      const { owner, repo } = repoForksRoute.useParams();
      return <RepoForks owner={owner} name={repo} forks={repoForksRoute.useLoaderData()} />;
    },
  });

  const repoBranchesRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/$owner/$repo/branches",
    loader: ({ params, context }) =>
      loadRepoPage((context as RouterContext).queryClient, params.owner, params.repo, () =>
        fetchRepoBranches(params.owner, params.repo),
      ),
    component: function Branches() {
      const { owner, repo } = repoBranchesRoute.useParams();
      return <RepoBranches owner={owner} name={repo} view="overview" data={repoBranchesRoute.useLoaderData()} />;
    },
  });

  const repoAllBranchesRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/$owner/$repo/branches/all",
    loader: ({ params, context }) =>
      loadRepoPage((context as RouterContext).queryClient, params.owner, params.repo, () =>
        fetchRepoBranches(params.owner, params.repo),
      ),
    component: function AllBranches() {
      const { owner, repo } = repoAllBranchesRoute.useParams();
      return <RepoBranches owner={owner} name={repo} view="all" data={repoAllBranchesRoute.useLoaderData()} />;
    },
  });

  // The ref is followed by the path whose history to list, or by "search" to
  // search the commit messages.
  const repoCommitsRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/$owner/$repo/commits/$",
    validateSearch: validateListSearch,
    loaderDeps: ({ search }: { search: ListSearch }) => ({ page: search.page, q: search.q }),
    loader: async ({ params, deps, context }) => {
      const data = await loadRepoPage((context as RouterContext).queryClient, params.owner, params.repo, () =>
        fetchRepoCommits(params.owner, params.repo, params._splat ?? "", deps),
      );
      if (data.search && !data.keyword) {
        // eslint-disable-next-line @typescript-eslint/only-throw-error -- TanStack's redirect() returns a sentinel that must be thrown.
        throw redirect({
          to: "/$owner/$repo/commits/$",
          params: { owner: params.owner, repo: params.repo, _splat: data.ref },
        });
      }
      return data;
    },
    component: function Commits() {
      const { owner, repo } = repoCommitsRoute.useParams();
      const data = repoCommitsRoute.useLoaderData();
      return (
        <RepoCommits
          key={`${owner}/${repo}@${data.ref}/${data.path}?${data.keyword}`}
          owner={owner}
          name={repo}
          data={data}
        />
      );
    },
  });

  function RepoCodePage({ owner, repo, data }: { owner: string; repo: string; data: RepoSrcData }) {
    return <RepoCode key={`${owner}/${repo}@${data.ref}/${data.path}`} owner={owner} name={repo} data={data} />;
  }

  const repoHomeRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/$owner/$repo",
    loader: ({ params, context }) =>
      loadRepoPage((context as RouterContext).queryClient, params.owner, params.repo, () =>
        fetchRepoSrc(params.owner, params.repo, ""),
      ),
    component: function Home() {
      const { owner, repo } = repoHomeRoute.useParams();
      return <RepoCodePage owner={owner} repo={repo} data={repoHomeRoute.useLoaderData()} />;
    },
  });

  const repoSrcRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/$owner/$repo/src/$",
    loader: ({ params, context }) =>
      loadRepoPage((context as RouterContext).queryClient, params.owner, params.repo, () =>
        fetchRepoSrc(params.owner, params.repo, params._splat ?? ""),
      ),
    component: function Src() {
      const { owner, repo } = repoSrcRoute.useParams();
      return <RepoCodePage owner={owner} repo={repo} data={repoSrcRoute.useLoaderData()} />;
    },
  });

  const repoReleasesRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/$owner/$repo/releases",
    validateSearch: validateRepoReleasesSearch,
    loaderDeps: ({ search }: { search: RepoReleasesSearch }) => ({ after: search.after }),
    loader: ({ params, deps, context }) =>
      loadRepoPage((context as RouterContext).queryClient, params.owner, params.repo, () =>
        fetchRepoReleases(params.owner, params.repo, deps.after),
      ),
    component: function Releases() {
      const { owner, repo } = repoReleasesRoute.useParams();
      return <RepoReleases owner={owner} name={repo} data={repoReleasesRoute.useLoaderData()} />;
    },
  });

  const repoReleaseNewRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/$owner/$repo/releases/new",
    loader: ({ params, context }) =>
      loadRepoPage((context as RouterContext).queryClient, params.owner, params.repo, () =>
        fetchRepoNewRelease(params.owner, params.repo),
      ),
    component: function ReleaseNew() {
      const { owner, repo } = repoReleaseNewRoute.useParams();
      return (
        <RepoReleaseEditor
          key={`${owner}/${repo}`}
          owner={owner}
          name={repo}
          options={repoReleaseNewRoute.useLoaderData()}
        />
      );
    },
  });

  // Tag names can contain slashes, so the tag is the rest of the path.
  const repoReleaseEditRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/$owner/$repo/releases/edit/$",
    loader: ({ params, context }) =>
      loadRepoPage((context as RouterContext).queryClient, params.owner, params.repo, () =>
        fetchRepoRelease(params.owner, params.repo, params._splat ?? ""),
      ),
    component: function ReleaseEdit() {
      const { owner, repo, _splat: tag } = repoReleaseEditRoute.useParams();
      return (
        <RepoReleaseEditor
          key={`${owner}/${repo}@${tag}`}
          owner={owner}
          name={repo}
          release={repoReleaseEditRoute.useLoaderData()}
        />
      );
    },
  });

  // The editor pages take the branch followed by the path, and are not found
  // when nothing of the kind they change is at the path.
  async function loadEditorTarget(
    queryClient: QueryClient,
    params: { owner: string; repo: string; _splat?: string },
    kind: "file" | "dir",
  ) {
    const target = await loadRepoPage(queryClient, params.owner, params.repo, () =>
      fetchRepoEditorTarget(params.owner, params.repo, params._splat ?? ""),
    );
    if (target.kind !== kind) {
      // eslint-disable-next-line @typescript-eslint/only-throw-error -- `notFound()` is the documented TanStack Router signal for 404, not an Error subclass.
      throw notFound();
    }
    return target;
  }

  const repoFileEditRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/$owner/$repo/_edit/$",
    loader: async ({ params, context }) => {
      const target = await loadEditorTarget((context as RouterContext).queryClient, params, "file");
      // Only text files can be edited.
      if (target.content === undefined) {
        // eslint-disable-next-line @typescript-eslint/only-throw-error -- `notFound()` is the documented TanStack Router signal for 404, not an Error subclass.
        throw notFound();
      }
      return target;
    },
    component: function FileEdit() {
      const { owner, repo } = repoFileEditRoute.useParams();
      const target = repoFileEditRoute.useLoaderData();
      return (
        <RepoFileEditor
          key={`${owner}/${repo}@${target.branch}/${target.path}`}
          owner={owner}
          name={repo}
          target={target}
          isNew={false}
        />
      );
    },
  });

  const repoFileNewRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/$owner/$repo/_new/$",
    loader: ({ params, context }) => loadEditorTarget((context as RouterContext).queryClient, params, "dir"),
    component: function FileNew() {
      const { owner, repo } = repoFileNewRoute.useParams();
      const target = repoFileNewRoute.useLoaderData();
      return (
        <RepoFileEditor
          key={`${owner}/${repo}@${target.branch}/${target.path}`}
          owner={owner}
          name={repo}
          target={target}
          isNew
        />
      );
    },
  });

  const repoFileDeleteRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/$owner/$repo/_delete/$",
    loader: ({ params, context }) => loadEditorTarget((context as RouterContext).queryClient, params, "file"),
    component: function FileDelete() {
      const { owner, repo } = repoFileDeleteRoute.useParams();
      const target = repoFileDeleteRoute.useLoaderData();
      return (
        <RepoFileDelete
          key={`${owner}/${repo}@${target.branch}/${target.path}`}
          owner={owner}
          name={repo}
          target={target}
        />
      );
    },
  });

  const repoFileUploadRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/$owner/$repo/_upload/$",
    loader: async ({ params, context }) => {
      const target = await loadEditorTarget((context as RouterContext).queryClient, params, "dir");
      if (!target.upload) {
        // eslint-disable-next-line @typescript-eslint/only-throw-error -- `notFound()` is the documented TanStack Router signal for 404, not an Error subclass.
        throw notFound();
      }
      return target;
    },
    component: function FileUpload() {
      const { owner, repo } = repoFileUploadRoute.useParams();
      const target = repoFileUploadRoute.useLoaderData();
      return (
        <RepoFileUpload
          key={`${owner}/${repo}@${target.branch}/${target.path}`}
          owner={owner}
          name={repo}
          target={target}
        />
      );
    },
  });

  // A link to a missing page leads to the list of pages.
  async function fetchWikiPage(owner: string, repo: string, page: string) {
    try {
      return await fetchRepoWiki(owner, repo, page);
    } catch (err) {
      if (err instanceof LoaderResponseError && err.errorField === "page does not exist") {
        // eslint-disable-next-line @typescript-eslint/only-throw-error -- TanStack's redirect() returns a sentinel that must be thrown.
        throw redirect({ to: "/$owner/$repo/wiki/_pages", params: { owner, repo } });
      }
      throw err;
    }
  }

  const repoWikiRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/$owner/$repo/wiki",
    loader: ({ params, context }) =>
      loadRepoPage((context as RouterContext).queryClient, params.owner, params.repo, () =>
        fetchWikiPage(params.owner, params.repo, "Home"),
      ),
    component: function Wiki() {
      const { owner, repo } = repoWikiRoute.useParams();
      return <RepoWiki owner={owner} name={repo} data={repoWikiRoute.useLoaderData()} />;
    },
  });

  const repoWikiPageRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/$owner/$repo/wiki/$page",
    loader: ({ params, context, location }) =>
      loadRepoPage((context as RouterContext).queryClient, params.owner, params.repo, () =>
        fetchWikiPage(params.owner, params.repo, wikiPageName(location.pathname, 0)),
      ),
    component: function WikiPage() {
      const { owner, repo } = repoWikiPageRoute.useParams();
      const data = repoWikiPageRoute.useLoaderData();
      return <RepoWiki key={data.page?.name} owner={owner} name={repo} data={data} />;
    },
  });

  const repoWikiPagesRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/$owner/$repo/wiki/_pages",
    loader: ({ params, context }) =>
      loadRepoPage((context as RouterContext).queryClient, params.owner, params.repo, () =>
        fetchRepoWikiPages(params.owner, params.repo),
      ),
    component: function WikiPages() {
      const { owner, repo } = repoWikiPagesRoute.useParams();
      return <RepoWikiPages owner={owner} name={repo} data={repoWikiPagesRoute.useLoaderData()} />;
    },
  });

  const repoWikiNewRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/$owner/$repo/wiki/_new",
    loader: async ({ params, context }) => {
      const data = await loadRepoPage((context as RouterContext).queryClient, params.owner, params.repo, () =>
        fetchRepoWikiPages(params.owner, params.repo),
      );
      if (!data.viewerCanWrite) {
        // eslint-disable-next-line @typescript-eslint/only-throw-error -- `notFound()` is the documented TanStack Router signal for 404, not an Error subclass.
        throw notFound();
      }
      return data;
    },
    component: function WikiNew() {
      const { owner, repo } = repoWikiNewRoute.useParams();
      // The first page is the home page of the wiki.
      const defaultTitle = repoWikiNewRoute.useLoaderData().pages.length ? "" : "Home";
      return <RepoWikiEditor key={`${owner}/${repo}`} owner={owner} name={repo} defaultTitle={defaultTitle} />;
    },
  });

  const repoWikiEditRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/$owner/$repo/wiki/$page/_edit",
    loader: ({ params, context, location }) =>
      loadRepoPage((context as RouterContext).queryClient, params.owner, params.repo, () =>
        fetchRepoWikiPageSource(params.owner, params.repo, wikiPageName(location.pathname, 1)),
      ),
    component: function WikiEdit() {
      const { owner, repo } = repoWikiEditRoute.useParams();
      const page = repoWikiEditRoute.useLoaderData();
      return <RepoWikiEditor key={page.name} owner={owner} name={repo} page={page} />;
    },
  });

  return [
    repoCommitRoute,
    repoCompareRoute,
    repoHomeRoute,
    repoSrcRoute,
    repoCommitsRoute,
    repoStargazersRoute,
    repoWatchersRoute,
    repoIssuesRoute,
    repoPullsRoute,
    repoNewIssueRoute,
    repoIssueRoute,
    repoForksRoute,
    repoBranchesRoute,
    repoAllBranchesRoute,
    repoReleasesRoute,
    repoReleaseNewRoute,
    repoReleaseEditRoute,
    repoWikiRoute,
    repoWikiPagesRoute,
    repoWikiNewRoute,
    repoWikiPageRoute,
    repoWikiEditRoute,
    repoFileEditRoute,
    repoFileNewRoute,
    repoFileDeleteRoute,
    repoFileUploadRoute,
    repoLabelsRoute,
    repoMilestonesRoute,
    repoMilestoneNewRoute,
    repoMilestoneEditRoute,
    repoCreateRoute,
    repoMigrateRoute,
    repoForkRoute,
    repoSettingsRoute,
    repoCollaborationRoute,
    repoBranchSettingsRoute,
    repoBranchProtectionRoute,
    repoGitHooksRoute,
    repoGitHookRoute,
    repoDeployKeysRoute,
    repoWebhooksRoute,
    repoWebhookNewRoute,
    repoWebhookRoute,
  ];
}
