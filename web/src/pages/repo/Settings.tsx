import { useMutation, useQueryClient, useSuspenseQuery } from "@tanstack/react-query";
import { Link, useNavigate, useRouter } from "@tanstack/react-router";
import { type FormEvent, type ReactNode, useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";

import { AvatarForm } from "@/components/AvatarForm";
import { ConfirmButton } from "@/components/ConfirmButton";
import { CredentialRow, ROW_CLASS } from "@/components/CredentialRow";
import { CheckboxField, FormAlert, FormField } from "@/components/FormField";
import { RepoHeader } from "@/components/RepoHeader";
import { RichText } from "@/components/RichText";
import { SettingsLayout, SettingsNavItem } from "@/components/SettingsLayout";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { webContext } from "@/lib/context";
import { usePageTitle } from "@/lib/page-title";
import {
  type RepoAdvancedSettings,
  type RepoCollaborator,
  type RepoCollaboratorMode,
  type RepoMirrorSettings,
  type RepoSettingsData,
  type RepoSettingsForm,
  addRepoCollaborator,
  addRepoDeployKey,
  changeRepoCollaborator,
  convertRepo,
  deleteRepo,
  deleteRepoAvatar,
  deleteRepoDeployKey,
  deleteRepoWiki,
  removeRepoCollaborator,
  repoHeaderQuery,
  repoSettingsQuery,
  syncRepoMirror,
  transferRepo,
  updateRepoAdvanced,
  updateRepoMirror,
  updateRepoSettings,
  uploadRepoAvatar,
} from "@/lib/queries/repo";
import { formErrors } from "@/lib/queries/request";
import type { SSHKey } from "@/lib/queries/settings";
import { subUrl } from "@/lib/url";
import { DescriptionField } from "@/pages/repo/New";

const TEXTAREA_CLASS =
  "block w-full rounded-md border border-(--color-input) bg-(--color-background) px-3 py-2 font-mono text-sm text-(--color-foreground) outline-none transition-colors focus-visible:border-(--color-ring) focus-visible:ring-1 focus-visible:ring-(--color-ring) disabled:cursor-not-allowed disabled:opacity-60 aria-invalid:border-(--color-destructive)";
const DATE_TIME_FMT = new Intl.DateTimeFormat(webContext.lang, { dateStyle: "medium", timeStyle: "short" });
const MODES: RepoCollaboratorMode[] = ["admin", "write", "read"];

type SettingsPage = "options" | "collaboration" | "keys";

// focusOnError moves focus to the first field of a failed form that has an
// error.
function focusOnError(err: Error, order: readonly string[]) {
  const { fields } = formErrors(err);
  const first = order.find((f) => f in fields);
  if (first) requestAnimationFrame(() => document.getElementById(first)?.focus());
}

// notice turns a line of the warnings of the server-rendered pages, which
// start with a dash, into a list item.
function notice(text: string) {
  return text.replace(/^-\s*/, "");
}

// useRefresh reloads the settings and the header of the repository after a
// change.
function useRefresh(owner: string, name: string) {
  const queryClient = useQueryClient();
  const router = useRouter();
  return async () => {
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: repoSettingsQuery(owner, name).queryKey }),
      queryClient.invalidateQueries({ queryKey: repoHeaderQuery(owner, name).queryKey }),
    ]);
    await router.invalidate();
  };
}

// RepoSettingsLayout heads the settings pages of a repository, with links
// between them. Only admins of the repository get here.
function RepoSettingsLayout({
  owner,
  name,
  active,
  children,
}: {
  owner: string;
  name: string;
  active: SettingsPage;
  children: ReactNode;
}) {
  const { t } = useTranslation();
  const { data: repo } = useSuspenseQuery(repoHeaderQuery(owner, name));
  const { data: settings } = useSuspenseQuery(repoSettingsQuery(owner, name));
  usePageTitle(`${t("repo.settings")} - ${repo.owner}/${repo.name}`);
  const params = { owner: repo.owner, repo: repo.name };
  const legacyLink = (path: string) => subUrl(`/${repo.owner}/${repo.name}/settings${path}`);
  const titles: Record<SettingsPage, string> = {
    options: t("repo.settings.options"),
    collaboration: t("repo.settings.collaboration"),
    keys: t("repo.settings.deploy_keys"),
  };

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <RepoHeader repo={repo} activeTab="settings" />
      <main className="flex-1">
        <SettingsLayout
          label={t("repo.settings")}
          current={titles[active]}
          className="max-w-7xl"
          nav={
            <>
              <SettingsNavItem active={active === "options"}>
                <Link to="/$owner/$repo/settings" params={params} activeOptions={{ exact: true }}>
                  {titles.options}
                </Link>
              </SettingsNavItem>
              <SettingsNavItem active={active === "collaboration"}>
                <Link to="/$owner/$repo/settings/collaboration" params={params} activeOptions={{ exact: true }}>
                  {titles.collaboration}
                </Link>
              </SettingsNavItem>
              {/* Branches, webhooks and Git hooks are still served by the server-rendered pages. */}
              {settings.mirror ? null : (
                <SettingsNavItem active={false}>
                  <a href={legacyLink("/branches")}>{t("repo.settings.branches")}</a>
                </SettingsNavItem>
              )}
              <SettingsNavItem active={false}>
                <a href={legacyLink("/hooks")}>{t("repo.settings.hooks")}</a>
              </SettingsNavItem>
              {settings.viewerCanEditGitHooks ? (
                <SettingsNavItem active={false}>
                  <a href={legacyLink("/hooks/git")}>{t("repo.settings.githooks")}</a>
                </SettingsNavItem>
              ) : null}
              <SettingsNavItem active={active === "keys"}>
                <Link to="/$owner/$repo/settings/keys" params={params} activeOptions={{ exact: true }}>
                  {titles.keys}
                </Link>
              </SettingsNavItem>
            </>
          }
        >
          {children}
        </SettingsLayout>
      </main>
    </div>
  );
}

function Section({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section className="mt-8 border-t border-(--color-border) pt-6 first:mt-0 first:border-t-0 first:pt-0">
      <h2 className="mb-4 text-xl font-semibold">{title}</h2>
      {children}
    </section>
  );
}

function RadioField({
  id,
  name,
  checked,
  onChange,
  label,
}: {
  id: string;
  name: string;
  checked: boolean;
  onChange: () => void;
  label: ReactNode;
}) {
  return (
    <label htmlFor={id} className="flex cursor-pointer items-start gap-2.5 text-sm">
      <input
        id={id}
        type="radio"
        name={name}
        checked={checked}
        onChange={onChange}
        className="mt-0.5 size-4 shrink-0 cursor-pointer accent-(--color-primary)"
      />
      <span>{label}</span>
    </label>
  );
}

// RepoOptions changes the name, the visibility, the avatar, the mirroring and
// the features of a repository, and holds the actions that cannot be taken
// back.
export function RepoOptions({ owner, name }: { owner: string; name: string }) {
  const { t } = useTranslation();
  const { data: settings } = useSuspenseQuery(repoSettingsQuery(owner, name));
  const refresh = useRefresh(owner, name);
  return (
    <RepoSettingsLayout owner={owner} name={name} active="options">
      <Section title={t("repo.settings.basic_settings")}>
        <BasicSettings owner={owner} name={name} settings={settings} />
        <div className="mt-8">
          <AvatarForm
            avatarURL={settings.avatarURL}
            hasCustomAvatar={settings.hasCustomAvatar}
            upload={(file) => uploadRepoAvatar(owner, name, file)}
            remove={() => deleteRepoAvatar(owner, name)}
            onChanged={async (uploaded) => {
              if (uploaded) toast.success(t("settings.update_avatar_success"));
              await refresh();
            }}
          />
        </div>
      </Section>
      {settings.mirror ? (
        <Section title={t("repo.settings.mirror_settings")}>
          <MirrorSettings owner={owner} name={name} mirror={settings.mirror} />
        </Section>
      ) : null}
      <Section title={t("repo.settings.advanced_settings")}>
        <AdvancedSettings owner={owner} name={name} settings={settings} />
      </Section>
      {settings.viewerIsOwner ? (
        <Section title={t("repo.settings.danger_zone")}>
          <DangerZone owner={owner} name={name} settings={settings} />
        </Section>
      ) : null}
    </RepoSettingsLayout>
  );
}

const BASIC_FIELDS = ["name", "description", "website"] as const;

function BasicSettings({ owner, name, settings }: { owner: string; name: string; settings: RepoSettingsData }) {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const refresh = useRefresh(owner, name);
  const [form, setForm] = useState<RepoSettingsForm>({
    name: settings.name,
    description: settings.description,
    website: settings.website,
    private: settings.private,
    unlisted: settings.unlisted,
  });
  const save = useMutation({
    mutationFn: () => updateRepoSettings(owner, name, form),
    onSuccess: async (repo) => {
      toast.success(t("repo.settings.update_settings_success"));
      if (repo.name !== name) {
        // The old name no longer exists, so drop it rather than refetch it.
        queryClient.removeQueries({ queryKey: ["repo", owner, name] });
        await navigate({ to: "/$owner/$repo/settings", params: { owner: repo.owner, repo: repo.name }, replace: true });
        return;
      }
      await refresh();
    },
    onError: (err) => focusOnError(err, BASIC_FIELDS),
  });
  const { fields, formError } = formErrors(save.error);
  const renaming = form.name.toLowerCase() !== settings.name.toLowerCase();

  const onSubmit = (e: FormEvent) => {
    e.preventDefault();
    save.mutate();
  };

  return (
    <form onSubmit={onSubmit} noValidate>
      <fieldset disabled={save.isPending} className="contents">
        <FormAlert message={formError} />
        <div className="flex flex-col gap-5">
          <FormField
            id="name"
            label={t("repo.repo_name")}
            // Renaming changes every link to the repository.
            helper={renaming ? t("repo.settings.change_reponame_prompt") : undefined}
            error={fields.name}
            invalid={"name" in fields}
          >
            <Input
              id="name"
              name="name"
              autoComplete="off"
              required
              maxLength={100}
              value={form.name}
              onChange={(e) => setForm({ ...form, name: e.target.value })}
            />
          </FormField>
          <DescriptionField
            value={form.description}
            onChange={(description) => setForm({ ...form, description })}
            error={fields.description}
            invalid={"description" in fields}
          />
          <FormField id="website" label={t("repo.settings.site")} error={fields.website} invalid={"website" in fields}>
            <Input
              id="website"
              name="website"
              type="url"
              autoComplete="url"
              value={form.website}
              onChange={(e) => setForm({ ...form, website: e.target.value })}
            />
          </FormField>
          {/* Forks keep the visibility of their base repository. */}
          {settings.isFork ? null : (
            <fieldset
              className="flex flex-col gap-3"
              aria-describedby={settings.forkCount ? "visibility-helper" : undefined}
            >
              <legend className="mb-3 text-sm font-medium">{t("repo.visibility")}</legend>
              <CheckboxField
                id="private"
                label={<RichText text={t("repo.visiblity_helper")} />}
                checked={form.private}
                onChange={(checked) => setForm({ ...form, private: checked })}
              />
              <CheckboxField
                id="unlisted"
                label={<RichText text={t("repo.unlisted_helper")} />}
                checked={form.unlisted}
                onChange={(checked) => setForm({ ...form, unlisted: checked })}
              />
              {settings.forkCount ? (
                <p id="visibility-helper" className="text-sm text-(--color-muted-foreground)">
                  {t("repo.visiblity_fork_helper")}
                </p>
              ) : null}
            </fieldset>
          )}
          <div>
            <Button type="submit" variant="outline">
              {t("repo.settings.update_settings")}
            </Button>
          </div>
        </div>
      </fieldset>
    </form>
  );
}

const MIRROR_FIELDS = ["interval", "address"] as const;

function MirrorSettings({ owner, name, mirror }: { owner: string; name: string; mirror: RepoMirrorSettings }) {
  const { t } = useTranslation();
  const refresh = useRefresh(owner, name);
  const [form, setForm] = useState({
    address: mirror.address,
    interval: String(mirror.interval),
    enablePrune: mirror.enablePrune,
  });
  const save = useMutation({
    mutationFn: () => updateRepoMirror(owner, name, { ...form, interval: Number(form.interval) }),
    onSuccess: async () => {
      toast.success(t("repo.settings.update_settings_success"));
      await refresh();
    },
    onError: (err) => focusOnError(err, MIRROR_FIELDS),
  });
  const sync = useMutation({
    mutationFn: () => syncRepoMirror(owner, name),
    onSuccess: () => toast.info(t("repo.settings.mirror_sync_in_progress")),
    onError: (err) => toast.error(err.message),
  });
  const { fields, formError } = formErrors(save.error);

  const onSubmit = (e: FormEvent) => {
    e.preventDefault();
    save.mutate();
  };

  return (
    <>
      <form onSubmit={onSubmit} noValidate>
        <fieldset disabled={save.isPending} className="contents">
          <FormAlert message={formError} />
          <div className="flex flex-col gap-5">
            <CheckboxField
              id="enablePrune"
              label={
                <>
                  <span className="font-medium">{t("repo.mirror_prune")}</span>
                  <span className="mt-0.5 block text-(--color-muted-foreground)">{t("repo.mirror_prune_desc")}</span>
                </>
              }
              checked={form.enablePrune}
              onChange={(enablePrune) => setForm({ ...form, enablePrune })}
            />
            <FormField
              id="interval"
              label={t("repo.mirror_interval")}
              error={fields.interval}
              invalid={"interval" in fields}
            >
              <Input
                id="interval"
                name="interval"
                type="number"
                min={1}
                className="max-w-40"
                value={form.interval}
                onChange={(e) => setForm({ ...form, interval: e.target.value })}
              />
            </FormField>
            <FormField
              id="address"
              label={t("repo.mirror_address")}
              helper={t("repo.mirror_address_desc")}
              error={fields.address}
              invalid={"address" in fields}
            >
              <Input
                id="address"
                name="address"
                autoComplete="off"
                required
                value={form.address}
                onChange={(e) => setForm({ ...form, address: e.target.value })}
              />
            </FormField>
            <div>
              <Button type="submit" variant="outline">
                {t("repo.settings.update_settings")}
              </Button>
            </div>
          </div>
        </fieldset>
      </form>
      <div className="mt-6 flex flex-wrap items-center gap-x-4 gap-y-2">
        <Button type="button" variant="outline" disabled={sync.isPending} onClick={() => sync.mutate()}>
          {t("repo.settings.sync_mirror")}
        </Button>
        {mirror.lastSyncedAt ? (
          <p className="text-sm text-(--color-muted-foreground)">
            {t("repo.mirror_last_synced")} {DATE_TIME_FMT.format(new Date(mirror.lastSyncedAt))}
          </p>
        ) : null}
      </div>
    </>
  );
}

// FeatureGroup turns a feature of the repository on or off, and shows its
// options while it is on.
function FeatureGroup({
  id,
  title,
  label,
  checked,
  onChange,
  children,
}: {
  id: string;
  title: string;
  label: string;
  checked: boolean;
  onChange: (checked: boolean) => void;
  children: ReactNode;
}) {
  return (
    <fieldset className="flex flex-col gap-3">
      <legend className="mb-3 text-sm font-medium">{title}</legend>
      <CheckboxField id={id} label={label} checked={checked} onChange={onChange} />
      {checked ? (
        <div className="ml-2 flex flex-col gap-3 border-l-2 border-(--color-border) pl-4">{children}</div>
      ) : null}
    </fieldset>
  );
}

function AdvancedSettings({ owner, name, settings }: { owner: string; name: string; settings: RepoSettingsData }) {
  const { t } = useTranslation();
  const refresh = useRefresh(owner, name);
  const [form, setForm] = useState<RepoAdvancedSettings>(settings.advanced);
  const save = useMutation({
    mutationFn: () => updateRepoAdvanced(owner, name, form),
    onSuccess: async () => {
      toast.success(t("repo.settings.update_settings_success"));
      await refresh();
    },
  });
  const { formError } = formErrors(save.error);
  const set = (change: Partial<RepoAdvancedSettings>) => setForm({ ...form, ...change });

  const onSubmit = (e: FormEvent) => {
    e.preventDefault();
    save.mutate();
  };

  return (
    <form onSubmit={onSubmit} noValidate>
      <fieldset disabled={save.isPending} className="contents">
        <FormAlert message={formError} />
        <div className="flex flex-col gap-6">
          <FeatureGroup
            id="enableWiki"
            title={t("repo.wiki")}
            label={t("repo.settings.wiki_desc")}
            checked={form.enableWiki}
            onChange={(enableWiki) => set({ enableWiki })}
          >
            <RadioField
              id="internalWiki"
              name="wiki"
              checked={!form.enableExternalWiki}
              onChange={() => set({ enableExternalWiki: false })}
              label={t("repo.settings.use_internal_wiki")}
            />
            {form.enableExternalWiki ? null : (
              <div className="ml-6.5">
                <CheckboxField
                  id="allowPublicWiki"
                  label={t("repo.settings.allow_public_wiki_desc")}
                  checked={form.allowPublicWiki}
                  onChange={(allowPublicWiki) => set({ allowPublicWiki })}
                />
              </div>
            )}
            <RadioField
              id="externalWiki"
              name="wiki"
              checked={form.enableExternalWiki}
              onChange={() => set({ enableExternalWiki: true })}
              label={t("repo.settings.use_external_wiki")}
            />
            {form.enableExternalWiki ? (
              <div className="ml-6.5">
                <FormField
                  id="externalWikiURL"
                  label={t("repo.settings.external_wiki_url")}
                  helper={t("repo.settings.external_wiki_url_desc")}
                  invalid={false}
                >
                  <Input
                    id="externalWikiURL"
                    name="externalWikiURL"
                    type="url"
                    value={form.externalWikiURL}
                    onChange={(e) => set({ externalWikiURL: e.target.value })}
                  />
                </FormField>
              </div>
            ) : null}
          </FeatureGroup>
          <FeatureGroup
            id="enableIssues"
            title={t("issues")}
            label={t("repo.settings.issues_desc")}
            checked={form.enableIssues}
            onChange={(enableIssues) => set({ enableIssues })}
          >
            <RadioField
              id="internalTracker"
              name="tracker"
              checked={!form.enableExternalTracker}
              onChange={() => set({ enableExternalTracker: false })}
              label={t("repo.settings.use_internal_issue_tracker")}
            />
            {form.enableExternalTracker ? null : (
              <div className="ml-6.5">
                <CheckboxField
                  id="allowPublicIssues"
                  label={t("repo.settings.allow_public_issues_desc")}
                  checked={form.allowPublicIssues}
                  onChange={(allowPublicIssues) => set({ allowPublicIssues })}
                />
              </div>
            )}
            <RadioField
              id="externalTracker"
              name="tracker"
              checked={form.enableExternalTracker}
              onChange={() => set({ enableExternalTracker: true })}
              label={t("repo.settings.use_external_issue_tracker")}
            />
            {form.enableExternalTracker ? (
              <div className="ml-6.5 flex flex-col gap-5">
                <FormField
                  id="externalTrackerURL"
                  label={t("repo.settings.external_tracker_url")}
                  helper={t("repo.settings.external_tracker_url_desc")}
                  invalid={false}
                >
                  <Input
                    id="externalTrackerURL"
                    name="externalTrackerURL"
                    type="url"
                    value={form.externalTrackerURL}
                    onChange={(e) => set({ externalTrackerURL: e.target.value })}
                  />
                </FormField>
                <FormField
                  id="externalTrackerFormat"
                  label={t("repo.settings.tracker_url_format")}
                  helper={<RichText text={t("repo.settings.tracker_url_format_desc")} />}
                  invalid={false}
                >
                  <Input
                    id="externalTrackerFormat"
                    name="externalTrackerFormat"
                    type="url"
                    placeholder="https://github.com/{user}/{repo}/issues/{index}"
                    value={form.externalTrackerFormat}
                    onChange={(e) => set({ externalTrackerFormat: e.target.value })}
                  />
                </FormField>
                <fieldset className="flex flex-col gap-3">
                  <legend className="mb-3 text-sm font-medium">{t("repo.settings.tracker_issue_style")}</legend>
                  <RadioField
                    id="trackerStyleNumeric"
                    name="trackerStyle"
                    checked={form.externalTrackerStyle !== "alphanumeric"}
                    onChange={() => set({ externalTrackerStyle: "numeric" })}
                    label={
                      <>
                        {t("repo.settings.tracker_issue_style.numeric")}{" "}
                        <span className="text-(--color-muted-foreground)">(#1234)</span>
                      </>
                    }
                  />
                  <RadioField
                    id="trackerStyleAlphanumeric"
                    name="trackerStyle"
                    checked={form.externalTrackerStyle === "alphanumeric"}
                    onChange={() => set({ externalTrackerStyle: "alphanumeric" })}
                    label={
                      <>
                        {t("repo.settings.tracker_issue_style.alphanumeric")}{" "}
                        <span className="text-(--color-muted-foreground)">(ABC-123, DEFG-234)</span>
                      </>
                    }
                  />
                </fieldset>
              </div>
            ) : null}
          </FeatureGroup>
          {settings.canEnablePulls ? (
            <FeatureGroup
              id="enablePulls"
              title={t("pull_requests")}
              label={t("repo.settings.pulls_desc")}
              checked={form.enablePulls}
              onChange={(enablePulls) => set({ enablePulls })}
            >
              <CheckboxField
                id="pullsIgnoreWhitespace"
                label={t("repo.settings.pulls.ignore_whitespace")}
                checked={form.pullsIgnoreWhitespace}
                onChange={(pullsIgnoreWhitespace) => set({ pullsIgnoreWhitespace })}
              />
              <CheckboxField
                id="pullsAllowRebase"
                label={t("repo.settings.pulls.allow_rebase_merge")}
                checked={form.pullsAllowRebase}
                onChange={(pullsAllowRebase) => set({ pullsAllowRebase })}
              />
            </FeatureGroup>
          ) : null}
          <div>
            <Button type="submit" variant="outline">
              {t("repo.settings.update_settings")}
            </Button>
          </div>
        </div>
      </fieldset>
    </form>
  );
}

// DangerZone converts, transfers or deletes the repository, or erases its
// wiki, once the viewer types its name.
function DangerZone({ owner, name, settings }: { owner: string; name: string; settings: RepoSettingsData }) {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const refresh = useRefresh(owner, name);
  const repoHome = (repoOwner: string, repoName: string) =>
    // The repository pages are still served by the server.
    window.location.assign(subUrl(`/${repoOwner}/${repoName}`));

  return (
    <div className="rounded-md border border-(--color-destructive)">
      {settings.mirror ? (
        <DangerAction
          id="convert"
          name={settings.name}
          title={t("repo.settings.convert")}
          description={t("repo.settings.convert_desc")}
          notices={[t("repo.settings.convert_notices_1")]}
          confirmLabel={t("repo.settings.convert_confirm")}
          action={(form) => convertRepo(owner, name, form.repoName)}
          onDone={() => repoHome(owner, name)}
        />
      ) : null}
      <DangerAction
        id="transfer"
        name={settings.name}
        title={t("repo.settings.transfer")}
        description={t("repo.settings.transfer_desc")}
        notices={[t("repo.settings.transfer_notices_1"), t("repo.settings.transfer_notices_2")]}
        confirmLabel={t("repo.settings.make_transfer")}
        askNewOwner
        action={(form) => transferRepo(owner, name, form)}
        onDone={(repo) => repoHome(repo.owner, repo.name)}
      />
      {settings.advanced.enableWiki ? (
        <DangerAction
          id="deleteWiki"
          name={settings.name}
          title={t("repo.settings.wiki_delete")}
          description={t("repo.settings.wiki_delete_desc")}
          notices={[
            t("repo.settings.delete_notices_1"),
            t("repo.settings.wiki_delete_notices_1").replace(/%(?:\[1\])?s/, settings.name),
          ]}
          confirmLabel={t("repo.settings.confirm_delete")}
          action={(form) => deleteRepoWiki(owner, name, form.repoName)}
          onDone={async () => {
            toast.success(t("repo.settings.wiki_deletion_success"));
            await refresh();
          }}
        />
      ) : null}
      <DangerAction
        id="delete"
        name={settings.name}
        title={t("repo.settings.delete")}
        description={t("repo.settings.delete_desc")}
        notices={[
          t("repo.settings.delete_notices_1"),
          t("repo.settings.delete_notices_2"),
          ...(settings.forkCount ? [t("repo.settings.delete_notices_fork_1")] : []),
        ]}
        confirmLabel={t("repo.settings.confirm_delete")}
        action={(form) => deleteRepo(owner, name, form.repoName)}
        onDone={async () => {
          toast.success(t("repo.settings.deletion_success"));
          if (settings.ownerIsOrganization) {
            await navigate({ to: "/org/$org/dashboard", params: { org: owner } });
          } else {
            await navigate({ to: "/" });
          }
          queryClient.removeQueries({ queryKey: ["repo", owner, name] });
        }}
      />
    </div>
  );
}

// DangerAction asks in a popover for the name of the repository before an
// action that cannot be taken back.
function DangerAction<T>({
  id,
  name,
  title,
  description,
  notices,
  confirmLabel,
  askNewOwner = false,
  action,
  onDone,
}: {
  id: string;
  name: string;
  title: string;
  description: string;
  notices: string[];
  confirmLabel: string;
  // askNewOwner also asks who to transfer the repository to.
  askNewOwner?: boolean;
  action: (form: { repoName: string; newOwner: string }) => Promise<T>;
  onDone: (result: T) => Promise<void> | void;
}) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const [form, setForm] = useState({ repoName: "", newOwner: "" });
  const mutation = useMutation({
    mutationFn: () => action(form),
    onSuccess: async (result) => {
      setOpen(false);
      await onDone(result);
    },
    onError: (err) => {
      const { fields } = formErrors(err);
      const first = ["repoName", "newOwner"].find((f) => f in fields);
      if (first) requestAnimationFrame(() => document.getElementById(`${id}-${first}`)?.focus());
    },
  });
  const { fields, formError } = formErrors(mutation.error);
  const ready = form.repoName === name && (!askNewOwner || form.newOwner.trim() !== "");

  const onOpenChange = (next: boolean) => {
    setOpen(next);
    if (!next) {
      setForm({ repoName: "", newOwner: "" });
      mutation.reset();
    }
  };
  const onSubmit = (e: FormEvent) => {
    e.preventDefault();
    if (ready) mutation.mutate();
  };

  return (
    <div className="flex flex-wrap items-center justify-between gap-x-4 gap-y-3 border-t border-(--color-border) px-4 py-3 first:border-t-0">
      <div className="min-w-0 flex-1 basis-56">
        <p className="text-sm font-semibold">{title}</p>
        <p className="text-sm text-(--color-muted-foreground)">{description}</p>
      </div>
      <Popover open={open} onOpenChange={onOpenChange}>
        <PopoverTrigger asChild>
          <Button type="button" variant="outline" size="sm" className="shrink-0 text-(--color-destructive)">
            {title}
          </Button>
        </PopoverTrigger>
        <PopoverContent
          align="end"
          collisionPadding={16}
          // The warnings and the fields can be taller than a short screen.
          className="max-h-(--radix-popover-content-available-height) w-80 max-w-[calc(100vw-2rem)] overflow-y-auto p-4"
        >
          <form onSubmit={onSubmit} noValidate>
            <fieldset disabled={mutation.isPending} className="contents">
              <p className="font-semibold">{title}</p>
              <ul className="mt-2 list-disc space-y-1 pl-5 text-sm text-(--color-muted-foreground)">
                {notices.map((text) => (
                  <li key={text}>
                    <RichText text={notice(text)} />
                  </li>
                ))}
              </ul>
              <div className="mt-4 flex flex-col gap-4">
                <FormAlert message={formError} />
                <FormField
                  id={`${id}-repoName`}
                  label={
                    <>
                      {t("repo.settings.transfer_form_title")} <strong className="wrap-anywhere">{name}</strong>
                    </>
                  }
                  error={fields.repoName}
                  invalid={"repoName" in fields}
                >
                  <Input
                    id={`${id}-repoName`}
                    name="repoName"
                    autoComplete="off"
                    autoFocus
                    required
                    value={form.repoName}
                    onChange={(e) => setForm({ ...form, repoName: e.target.value })}
                  />
                </FormField>
                {askNewOwner ? (
                  <FormField
                    id={`${id}-newOwner`}
                    label={t("repo.settings.transfer_owner")}
                    error={fields.newOwner}
                    invalid={"newOwner" in fields}
                  >
                    <Input
                      id={`${id}-newOwner`}
                      name="newOwner"
                      autoComplete="off"
                      required
                      value={form.newOwner}
                      onChange={(e) => setForm({ ...form, newOwner: e.target.value })}
                    />
                  </FormField>
                ) : null}
              </div>
              <div className="mt-4 flex justify-end gap-2">
                <Button type="button" variant="outline" size="sm" onClick={() => onOpenChange(false)}>
                  {t("cancel")}
                </Button>
                <Button
                  type="submit"
                  variant="destructive"
                  size="sm"
                  // A long label wraps in narrow popovers, so the button grows with it.
                  className="h-auto min-h-8 py-1.5"
                  disabled={!ready}
                >
                  {confirmLabel}
                </Button>
              </div>
            </fieldset>
          </form>
        </PopoverContent>
      </Popover>
    </div>
  );
}

// RepoCollaboration lists who else can access a repository and how much, and
// adds or removes them.
export function RepoCollaboration({
  owner,
  name,
  collaborators,
}: {
  owner: string;
  name: string;
  collaborators: RepoCollaborator[];
}) {
  const { t } = useTranslation();
  const router = useRouter();
  const [username, setUsername] = useState("");
  const add = useMutation({
    mutationFn: () => addRepoCollaborator(owner, name, username.trim()),
    onSuccess: async () => {
      toast.success(t("repo.settings.add_collaborator_success"));
      setUsername("");
      await router.invalidate();
    },
    onError: (err) => focusOnError(err, ["username"]),
  });
  const { fields, formError } = formErrors(add.error);

  const onSubmit = (e: FormEvent) => {
    e.preventDefault();
    if (username.trim()) add.mutate();
  };

  return (
    <RepoSettingsLayout owner={owner} name={name} active="collaboration">
      <h1 className="mb-4 text-xl font-semibold">{t("repo.settings.collaboration")}</h1>
      {collaborators.length ? (
        <ul className="mb-8 border-t border-(--color-border)">
          {collaborators.map((collaborator) => (
            <CollaboratorRow key={collaborator.name} owner={owner} name={name} collaborator={collaborator} />
          ))}
        </ul>
      ) : null}
      <form onSubmit={onSubmit} noValidate>
        <fieldset disabled={add.isPending} className="contents">
          <h2 className="mb-4 text-base font-semibold">{t("repo.settings.add_collaborator")}</h2>
          <FormAlert message={formError} />
          <div className="flex flex-col gap-1.5">
            <div className="flex flex-wrap gap-2">
              <Input
                id="username"
                name="username"
                autoComplete="off"
                required
                aria-label={t("username")}
                placeholder={t("repo.settings.search_user_placeholder")}
                value={username}
                onChange={(e) => setUsername(e.target.value)}
                aria-invalid={"username" in fields ? true : undefined}
                aria-describedby={fields.username ? "username-error" : undefined}
                className="min-w-0 flex-1 basis-56"
              />
              <Button type="submit" variant="outline">
                {t("repo.settings.add_collaborator")}
              </Button>
            </div>
            {fields.username ? (
              <p id="username-error" className="text-sm text-(--color-destructive)">
                {fields.username}
              </p>
            ) : null}
          </div>
        </fieldset>
      </form>
    </RepoSettingsLayout>
  );
}

function CollaboratorRow({
  owner,
  name,
  collaborator,
}: {
  owner: string;
  name: string;
  collaborator: RepoCollaborator;
}) {
  const { t } = useTranslation();
  const router = useRouter();
  const change = useMutation({
    mutationFn: (mode: RepoCollaboratorMode) => changeRepoCollaborator(owner, name, collaborator.name, mode),
    onSuccess: () => router.invalidate(),
    onError: (err) => toast.error(err.message),
  });
  const nameID = `collaborator-${collaborator.name}`;
  return (
    <li className={ROW_CLASS}>
      <Link
        to="/$username"
        params={{ username: collaborator.name }}
        className="flex min-w-0 flex-1 basis-40 items-center gap-3 hover:underline"
      >
        <img src={collaborator.avatarURL} alt="" className="size-8 shrink-0 rounded-full" />
        <span id={nameID} className="min-w-0 truncate font-semibold">
          {collaborator.fullName || collaborator.name}
        </span>
      </Link>
      <div className="flex items-center gap-2">
        <Select
          value={change.isPending ? change.variables : collaborator.mode}
          onValueChange={(mode) => change.mutate(mode as RepoCollaboratorMode)}
          disabled={change.isPending}
        >
          <SelectTrigger aria-labelledby={nameID} className="h-8 w-28">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {MODES.map((mode) => (
              <SelectItem key={mode} value={mode}>
                {t(`repo.settings.collaboration.${mode}`)}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <ConfirmButton
          label={t("repo.settings.delete_collaborator")}
          ariaLabel={`${t("repo.settings.delete_collaborator")} ${collaborator.name}`}
          title={t("repo.settings.collaborator_deletion")}
          description={t("repo.settings.collaborator_deletion_desc")}
          size="sm"
          action={() => removeRepoCollaborator(owner, name, collaborator.name)}
          onDone={async () => {
            toast.success(t("repo.settings.remove_collaborator_success"));
            await router.invalidate();
          }}
        />
      </div>
    </li>
  );
}

const DEPLOY_KEY_FIELDS = ["title", "content"] as const;

// RepoDeployKeys lists the SSH keys with read access to a repository, and adds
// new ones.
export function RepoDeployKeys({ owner, name, keys }: { owner: string; name: string; keys: SSHKey[] }) {
  const { t } = useTranslation();
  const router = useRouter();
  const [form, setForm] = useState({ title: "", content: "" });
  const add = useMutation({
    mutationFn: () => addRepoDeployKey(owner, name, form),
    onSuccess: async () => {
      toast.success(t("repo.settings.add_key_success").replace(/%(?:\[1\])?s/, form.title));
      setForm({ title: "", content: "" });
      await router.invalidate();
    },
    onError: (err) => focusOnError(err, DEPLOY_KEY_FIELDS),
  });
  const { fields, formError } = formErrors(add.error);

  const onSubmit = (e: FormEvent) => {
    e.preventDefault();
    add.mutate();
  };

  return (
    <RepoSettingsLayout owner={owner} name={name} active="keys">
      <h1 className="mb-2 text-xl font-semibold">{t("repo.settings.deploy_keys")}</h1>
      <p className="mb-4 text-sm text-(--color-muted-foreground)">{t("repo.settings.deploy_key_desc")}</p>
      {keys.length ? (
        <ul>
          {keys.map((key) => (
            <CredentialRow
              key={key.id}
              item={key}
              detail={key.fingerprint}
              recentLabel={t("settings.key_state_desc")}
              deletion={{
                label: t("settings.delete_key"),
                title: t("repo.settings.deploy_key_deletion"),
                description: t("repo.settings.deploy_key_deletion_desc"),
                success: t("repo.settings.deploy_key_deletion_success"),
                action: () => deleteRepoDeployKey(owner, name, key.id),
              }}
            />
          ))}
        </ul>
      ) : (
        <p className="border-y border-(--color-border) py-3 text-sm text-(--color-muted-foreground)">
          {t("repo.settings.no_deploy_keys")}
        </p>
      )}
      <form onSubmit={onSubmit} noValidate className="mt-8">
        <fieldset disabled={add.isPending} className="contents">
          <h2 className="mb-4 text-base font-semibold">{t("repo.settings.add_deploy_key")}</h2>
          <FormAlert message={formError} />
          <div className="flex flex-col gap-5">
            <FormField id="title" label={t("repo.settings.title")} error={fields.title} invalid={"title" in fields}>
              <Input
                id="title"
                name="title"
                autoComplete="off"
                required
                value={form.title}
                onChange={(e) => setForm({ ...form, title: e.target.value })}
              />
            </FormField>
            <FormField
              id="content"
              label={t("repo.settings.deploy_key_content")}
              error={fields.content}
              invalid={"content" in fields}
            >
              <textarea
                id="content"
                name="content"
                rows={5}
                required
                spellCheck={false}
                placeholder="ssh-ed25519 AAAA…"
                className={TEXTAREA_CLASS}
                value={form.content}
                onChange={(e) => setForm({ ...form, content: e.target.value })}
              />
            </FormField>
            <div>
              <Button type="submit" variant="outline">
                {t("repo.settings.add_deploy_key")}
              </Button>
            </div>
          </div>
        </fieldset>
      </form>
      <p className="mt-8 text-sm text-(--color-muted-foreground)">
        <RichText text={t("repo.settings.deploy_keys_helper")} links={["/user/settings/ssh"]} />
      </p>
    </RepoSettingsLayout>
  );
}
