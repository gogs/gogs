import { type AnyRoute, createRoute } from "@tanstack/react-router";

import { loadAuthenticatedPage } from "@/lib/loader-error";
import {
  fetchAccessTokens,
  fetchPasskeys,
  fetchRecoveryCodes,
  fetchSSHKeys,
  fetchSettingsOrgs,
  fetchSettingsRepos,
  fetchTwoFactorEnabled,
  fetchTwoFactorEnrollment,
  fetchUserEmails,
  fetchUserSettings,
} from "@/lib/queries/settings";
import { UserDelete, UserOrgSettings, UserProfileSettings, UserRepoSettings } from "@/pages/user/Settings";
import {
  UserEmailSettings,
  UserRecoveryCodes,
  UserSSHKeySettings,
  UserSecuritySettings,
  UserTokenSettings,
  UserTwoFactorEnable,
} from "@/pages/user/SettingsSecurity";

export function createSettingsRoutes(rootRoute: AnyRoute) {
  const profileRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/user/settings",
    loader: () => loadAuthenticatedPage(fetchUserSettings),
    component: function Profile() {
      return <UserProfileSettings settings={profileRoute.useLoaderData()} />;
    },
  });

  const emailRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/user/settings/email",
    // The link that confirms an email address lands here with `activated`.
    validateSearch: (search: Record<string, unknown>): { activated?: true } =>
      search.activated === 1 || search.activated === "1" ? { activated: true } : {},
    loader: () => loadAuthenticatedPage(fetchUserEmails),
    component: function Emails() {
      const search: { activated?: true } = emailRoute.useSearch();
      return <UserEmailSettings emails={emailRoute.useLoaderData()} activated={search.activated === true} />;
    },
  });

  const sshRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/user/settings/ssh",
    loader: () => loadAuthenticatedPage(fetchSSHKeys),
    component: function SSHKeys() {
      return <UserSSHKeySettings keys={sshRoute.useLoaderData()} />;
    },
  });

  const securityRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/user/settings/security",
    loader: () =>
      loadAuthenticatedPage(() => Promise.all([fetchUserSettings(), fetchPasskeys(), fetchTwoFactorEnabled()])),
    component: function Security() {
      const [settings, passkeys, enabled] = securityRoute.useLoaderData();
      return <UserSecuritySettings isLocal={settings.isLocal} passkeys={passkeys} enabled={enabled} />;
    },
  });

  const twoFactorEnableRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/user/settings/security/two_factor_enable",
    loader: () => loadAuthenticatedPage(fetchTwoFactorEnrollment),
    component: function TwoFactorEnable() {
      return <UserTwoFactorEnable enrollment={twoFactorEnableRoute.useLoaderData()} />;
    },
  });

  const recoveryCodesRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/user/settings/security/two_factor_recovery_codes",
    loader: () => loadAuthenticatedPage(fetchRecoveryCodes),
    component: function RecoveryCodes() {
      return <UserRecoveryCodes codes={recoveryCodesRoute.useLoaderData()} />;
    },
  });

  const applicationsRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/user/settings/applications",
    loader: () => loadAuthenticatedPage(fetchAccessTokens),
    component: function Applications() {
      return <UserTokenSettings tokens={applicationsRoute.useLoaderData()} />;
    },
  });

  const reposRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/user/settings/repositories",
    loader: () => loadAuthenticatedPage(fetchSettingsRepos),
    component: function Repos() {
      return <UserRepoSettings repos={reposRoute.useLoaderData()} />;
    },
  });

  const orgsRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/user/settings/organizations",
    loader: () => loadAuthenticatedPage(fetchSettingsOrgs),
    component: function Orgs() {
      return <UserOrgSettings orgs={orgsRoute.useLoaderData()} />;
    },
  });

  const deleteRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/user/settings/delete",
    component: UserDelete,
  });

  return [
    profileRoute,
    emailRoute,
    sshRoute,
    securityRoute,
    twoFactorEnableRoute,
    recoveryCodesRoute,
    applicationsRoute,
    reposRoute,
    orgsRoute,
    deleteRoute,
  ];
}
