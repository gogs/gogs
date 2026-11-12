import { type AnyRoute, createRoute } from "@tanstack/react-router";

import { loadSignedInPage } from "@/lib/loader-error";
import {
  fetchAccessTokens,
  fetchLoginSessions,
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
import { UserSessionSettings } from "@/pages/user/Sessions";
import {
  UserAvatarSettings,
  UserDelete,
  UserOrgSettings,
  UserPasswordSettings,
  UserProfileSettings,
  UserRepoSettings,
} from "@/pages/user/Settings";
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
    loader: () => loadSignedInPage(fetchUserSettings),
    component: function Profile() {
      return <UserProfileSettings settings={profileRoute.useLoaderData()} />;
    },
  });

  const avatarRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/user/settings/avatar",
    loader: () => loadSignedInPage(fetchUserSettings),
    component: function Avatar() {
      return <UserAvatarSettings settings={avatarRoute.useLoaderData()} />;
    },
  });

  const passwordRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/user/settings/password",
    loader: () => loadSignedInPage(fetchUserSettings),
    component: function Password() {
      return <UserPasswordSettings isLocal={passwordRoute.useLoaderData().isLocal} />;
    },
  });

  const emailRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/user/settings/email",
    // The link that confirms an email address lands here with `activated`.
    validateSearch: (search: Record<string, unknown>): { activated?: true } =>
      search.activated === 1 || search.activated === "1" ? { activated: true } : {},
    loader: () => loadSignedInPage(fetchUserEmails),
    component: function Emails() {
      const search: { activated?: true } = emailRoute.useSearch();
      return <UserEmailSettings emails={emailRoute.useLoaderData()} activated={search.activated === true} />;
    },
  });

  const sshRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/user/settings/ssh",
    loader: () => loadSignedInPage(fetchSSHKeys),
    component: function SSHKeys() {
      return <UserSSHKeySettings keys={sshRoute.useLoaderData()} />;
    },
  });

  const securityRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/user/settings/security",
    loader: () =>
      loadSignedInPage(async () => {
        const [passkeys, twoFactorEnabled] = await Promise.all([fetchPasskeys(), fetchTwoFactorEnabled()]);
        return { passkeys, twoFactorEnabled };
      }),
    component: function Security() {
      const { passkeys, twoFactorEnabled } = securityRoute.useLoaderData();
      return <UserSecuritySettings passkeys={passkeys} enabled={twoFactorEnabled} />;
    },
  });

  const twoFactorEnableRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/user/settings/security/two_factor_enable",
    loader: () => loadSignedInPage(fetchTwoFactorEnrollment),
    component: function TwoFactorEnable() {
      return <UserTwoFactorEnable enrollment={twoFactorEnableRoute.useLoaderData()} />;
    },
  });

  const recoveryCodesRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/user/settings/security/two_factor_recovery_codes",
    loader: () => loadSignedInPage(fetchRecoveryCodes),
    component: function RecoveryCodes() {
      return <UserRecoveryCodes codes={recoveryCodesRoute.useLoaderData()} />;
    },
  });

  const sessionsRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/user/settings/sessions",
    loader: () => loadSignedInPage(fetchLoginSessions),
    component: function Sessions() {
      return <UserSessionSettings sessions={sessionsRoute.useLoaderData()} />;
    },
  });

  const applicationsRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/user/settings/applications",
    loader: () => loadSignedInPage(fetchAccessTokens),
    component: function Applications() {
      return <UserTokenSettings tokens={applicationsRoute.useLoaderData()} />;
    },
  });

  const reposRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/user/settings/repositories",
    loader: () => loadSignedInPage(fetchSettingsRepos),
    component: function Repos() {
      return <UserRepoSettings repos={reposRoute.useLoaderData()} />;
    },
  });

  const orgsRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/user/settings/organizations",
    loader: () => loadSignedInPage(fetchSettingsOrgs),
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
    avatarRoute,
    passwordRoute,
    emailRoute,
    sshRoute,
    securityRoute,
    twoFactorEnableRoute,
    recoveryCodesRoute,
    sessionsRoute,
    applicationsRoute,
    reposRoute,
    orgsRoute,
    deleteRoute,
  ];
}
