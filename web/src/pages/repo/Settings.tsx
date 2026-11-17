import { useMutation, useQueryClient, useSuspenseQuery } from "@tanstack/react-query";
import { Link, useNavigate, useRouter } from "@tanstack/react-router";
import {
  GitBranch,
  KeyRound,
  Pencil,
  RefreshCw,
  Settings,
  ShieldCheck,
  SquareTerminal,
  TriangleAlert,
  Users,
  Webhook,
} from "lucide-react";
import { type FormEvent, type ReactNode, useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";

import { AvatarForm } from "@/components/AvatarForm";
import { CodeEditor } from "@/components/CodeEditor";
import { ConfirmButton } from "@/components/ConfirmButton";
import { CredentialRow, ROW_CLASS } from "@/components/CredentialRow";
import { CheckboxField, FormAlert, FormField, RadioField } from "@/components/FormField";
import { RepoHeader } from "@/components/RepoHeader";
import { RichText } from "@/components/RichText";
import { SettingsLayout, SettingsNavDivider, SettingsNavGroup, SettingsNavItem } from "@/components/SettingsLayout";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { webContext } from "@/lib/context";
import { usePageTitle } from "@/lib/page-title";
import {
  type RepoAdvancedSettings,
  type RepoBranchSettingsData,
  type RepoCollaborator,
  type RepoCollaboratorMode,
  type RepoGitHook,
  type RepoGitHookContent,
  type RepoMirrorSettings,
  type RepoProtectedBranch,
  type RepoProtectedBranchForm,
  type RepoSettingsData,
  type RepoSettingsForm,
  addRepoCollaborator,
  addRepoDeployKey,
  changeRepoCollaborator,
  convertRepo,
  deleteRepo,
  deleteRepoAvatar,
  deleteRepoDeployKey,
  deleteRepoGitHook,
  deleteRepoWiki,
  removeRepoCollaborator,
  repoHeaderQuery,
  repoSettingsQuery,
  syncRepoMirror,
  transferRepo,
  updateRepoAdvanced,
  updateRepoDefaultBranch,
  updateRepoGitHook,
  updateRepoMirror,
  updateRepoProtectedBranch,
  updateRepoSettings,
  uploadRepoAvatar,
} from "@/lib/queries/repo";
import { formErrors } from "@/lib/queries/request";
import type { SSHKey } from "@/lib/queries/settings";
import { toastError } from "@/lib/toast-error";
import { subUrl } from "@/lib/url";
import { cn } from "@/lib/utils";
import { DescriptionField } from "@/pages/repo/New";

const TEXTAREA_CLASS =
  "block w-full rounded-md border border-(--color-input) bg-(--color-field) px-3 py-2 font-mono text-sm text-(--color-foreground) outline-none transition-colors focus-visible:border-(--color-ring) focus-visible:ring-1 focus-visible:ring-(--color-ring) disabled:cursor-not-allowed disabled:opacity-60 aria-invalid:border-(--color-destructive)";
const DATE_TIME_FMT = new Intl.DateTimeFormat(webContext.lang, { dateStyle: "medium", timeStyle: "short" });
const MODES: RepoCollaboratorMode[] = ["admin", "write", "read"];

type SettingsPage = "general" | "mirror" | "danger" | "collaboration" | "branches" | "hooks" | "githooks" | "keys";

// focusOnError moves focus to the first field of a failed form that has an
// error.
function focusOnError(err: Error, order: readonly string[]) {
  const { fields } = formErrors(err);
  const first = order.find((f) => f in fields);
  if (first) requestAnimationFrame(() => document.getElementById(first)?.focus());
}

// notice turns a line of a translated warning, which starts with a dash, into
// a list item.
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
export function RepoSettingsLayout({
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
  const titles: Record<SettingsPage, string> = {
    general: t("repo.settings.general"),
    mirror: t("repo.settings.mirror"),
    danger: t("repo.settings.danger_zone"),
    collaboration: t("repo.settings.collaboration"),
    branches: t("repo.settings.branches"),
    hooks: t("repo.settings.hooks"),
    githooks: t("repo.settings.githooks"),
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
              <SettingsNavGroup>
                <SettingsNavItem active={active === "general"} icon={Settings}>
                  <Link to="/$owner/$repo/settings" params={params} activeOptions={{ exact: true }}>
                    {titles.general}
                  </Link>
                </SettingsNavItem>
              </SettingsNavGroup>
              <SettingsNavGroup title={t("settings.group_access")}>
                <SettingsNavItem active={active === "collaboration"} icon={Users}>
                  <Link to="/$owner/$repo/settings/collaboration" params={params} activeOptions={{ exact: true }}>
                    {titles.collaboration}
                  </Link>
                </SettingsNavItem>
              </SettingsNavGroup>
              <SettingsNavGroup title={t("repo.settings.group_code")}>
                {/* Mirrors take their branches from the source repository. */}
                {settings.mirror ? (
                  <SettingsNavItem active={active === "mirror"} icon={RefreshCw}>
                    <Link to="/$owner/$repo/settings/mirror" params={params} activeOptions={{ exact: true }}>
                      {titles.mirror}
                    </Link>
                  </SettingsNavItem>
                ) : (
                  <SettingsNavItem active={active === "branches"} icon={GitBranch}>
                    <Link to="/$owner/$repo/settings/branches" params={params}>
                      {titles.branches}
                    </Link>
                  </SettingsNavItem>
                )}
                <SettingsNavItem active={active === "hooks"} icon={Webhook}>
                  <Link to="/$owner/$repo/settings/hooks" params={params} activeOptions={{ exact: true }}>
                    {titles.hooks}
                  </Link>
                </SettingsNavItem>
                {settings.viewerCanEditGitHooks ? (
                  <SettingsNavItem active={active === "githooks"} icon={SquareTerminal}>
                    <Link to="/$owner/$repo/settings/hooks/git" params={params}>
                      {titles.githooks}
                    </Link>
                  </SettingsNavItem>
                ) : null}
              </SettingsNavGroup>
              <SettingsNavGroup title={t("repo.settings.group_security")}>
                <SettingsNavItem active={active === "keys"} icon={KeyRound}>
                  <Link to="/$owner/$repo/settings/keys" params={params} activeOptions={{ exact: true }}>
                    {titles.keys}
                  </Link>
                </SettingsNavItem>
              </SettingsNavGroup>
              {hasDangerZone(settings) ? (
                <>
                  <SettingsNavDivider />
                  <SettingsNavItem active={active === "danger"} icon={TriangleAlert} destructive>
                    <Link to="/$owner/$repo/settings/danger" params={params} activeOptions={{ exact: true }}>
                      {titles.danger}
                    </Link>
                  </SettingsNavItem>
                </>
              ) : null}
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
    <section className="mt-8 border-t border-(--color-border) pt-6 first-of-type:mt-0 first-of-type:border-t-0 first-of-type:pt-0">
      <h2 className="mb-4 text-lg font-semibold">{title}</h2>
      {children}
    </section>
  );
}

// RepoGeneral changes the name, the avatar and the features of a repository.
export function RepoGeneral({ owner, name }: { owner: string; name: string }) {
  const { t } = useTranslation();
  const { data: settings } = useSuspenseQuery(repoSettingsQuery(owner, name));
  const refresh = useRefresh(owner, name);
  return (
    <RepoSettingsLayout owner={owner} name={name} active="general">
      <h1 className="mb-4 text-xl font-semibold">{t("repo.settings.general")}</h1>
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
      <Section title={t("repo.settings.advanced_settings")}>
        <AdvancedSettings owner={owner} name={name} settings={settings} />
      </Section>
    </RepoSettingsLayout>
  );
}

// RepoMirror changes how a mirror syncs from its source. Only mirrors get here.
export function RepoMirror({ owner, name }: { owner: string; name: string }) {
  const { t } = useTranslation();
  const { data: settings } = useSuspenseQuery(repoSettingsQuery(owner, name));
  return (
    <RepoSettingsLayout owner={owner} name={name} active="mirror">
      <h1 className="mb-4 text-xl font-semibold">{t("repo.settings.mirror_settings")}</h1>
      {settings.mirror ? <MirrorSettings owner={owner} name={name} mirror={settings.mirror} /> : null}
    </RepoSettingsLayout>
  );
}

// hasDangerZone tells whether the viewer has anything to do in the danger zone.
// Admins can change the visibility, and only the owner can do the rest. Forks
// may keep the visibility of their base repository.
function hasDangerZone(settings: RepoSettingsData) {
  return settings.viewerIsOwner || !settings.visibilityFollowsBase;
}

// RepoDangerZone holds the actions on a repository that are hard or impossible
// to take back.
export function RepoDangerZone({ owner, name }: { owner: string; name: string }) {
  const { t } = useTranslation();
  const { data: settings } = useSuspenseQuery(repoSettingsQuery(owner, name));
  return (
    <RepoSettingsLayout owner={owner} name={name} active="danger">
      <h1 className="mb-4 text-xl font-semibold">{t("repo.settings.danger_zone")}</h1>
      <DangerZone owner={owner} name={name} settings={settings} />
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
          <div>
            <Button type="submit">{t("repo.settings.update_settings")}</Button>
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
    onError: (err) => toastError(err),
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
              <Button type="submit">{t("repo.settings.update_settings")}</Button>
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
  const { fields, formError } = formErrors(save.error);
  const set = (change: Partial<RepoAdvancedSettings>) => setForm({ ...form, ...change });
  const noMergeMethod = form.enablePulls && !form.pullsAllowMerge && !form.pullsAllowSquash && !form.pullsAllowRebase;

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
                  disabled={!settings.canAllowPublicWiki}
                />
                {settings.canAllowPublicWiki ? null : (
                  <p className="mt-1 ml-6.5 text-sm text-(--color-muted-foreground)">
                    {t("repo.settings.fork_public_access_limited")}
                  </p>
                )}
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
                  error={fields.externalWikiURL}
                  invalid={"externalWikiURL" in fields}
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
                  disabled={!settings.canAllowPublicIssues}
                />
                {settings.canAllowPublicIssues ? null : (
                  <p className="mt-1 ml-6.5 text-sm text-(--color-muted-foreground)">
                    {t("repo.settings.fork_public_access_limited")}
                  </p>
                )}
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
                  error={fields.externalTrackerURL}
                  invalid={"externalTrackerURL" in fields}
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
              <p className="text-sm">{t("repo.settings.pulls.merge_methods_desc")}</p>
              <div className="divide-y divide-(--color-border) glass rounded-md border border-(--color-border)">
                <div className="flex flex-col gap-3 px-4 py-3">
                  <CheckboxField
                    id="pullsAllowMerge"
                    label={t("repo.settings.pulls.allow_merge_commit")}
                    description={t("repo.settings.pulls.allow_merge_commit_desc")}
                    checked={form.pullsAllowMerge}
                    onChange={(pullsAllowMerge) => set({ pullsAllowMerge })}
                  />
                  {form.pullsAllowMerge ? (
                    <MessagePresetField
                      id="pullsMergeMessage"
                      description={t("repo.settings.pulls.default_merge_message_desc")}
                      presets={["", "title", "title_description"]}
                      value={form.pullsMergeMessage}
                      onChange={(pullsMergeMessage) => set({ pullsMergeMessage })}
                    />
                  ) : null}
                </div>
                <div className="flex flex-col gap-3 px-4 py-3">
                  <CheckboxField
                    id="pullsAllowSquash"
                    label={t("repo.settings.pulls.allow_squash_merge")}
                    description={t("repo.settings.pulls.allow_squash_merge_desc")}
                    checked={form.pullsAllowSquash}
                    onChange={(pullsAllowSquash) => set({ pullsAllowSquash })}
                  />
                  {form.pullsAllowSquash ? (
                    <MessagePresetField
                      id="pullsSquashMessage"
                      description={t("repo.settings.pulls.default_squash_message_desc")}
                      presets={["", "title", "title_commits", "title_description"]}
                      value={form.pullsSquashMessage}
                      onChange={(pullsSquashMessage) => set({ pullsSquashMessage })}
                    />
                  ) : null}
                </div>
                <div className="px-4 py-3">
                  <CheckboxField
                    id="pullsAllowRebase"
                    label={t("repo.settings.pulls.allow_rebase_merge")}
                    description={t("repo.settings.pulls.allow_rebase_merge_desc")}
                    checked={form.pullsAllowRebase}
                    onChange={(pullsAllowRebase) => set({ pullsAllowRebase })}
                  />
                </div>
              </div>
              {noMergeMethod ? (
                <p role="alert" className="text-sm text-(--color-destructive)">
                  {t("repo.settings.pulls.no_merge_method")}
                </p>
              ) : null}
              <CheckboxField
                id="pullsDeleteBranch"
                label={t("repo.settings.pulls.delete_branch")}
                description={t("repo.settings.pulls.delete_branch_desc")}
                checked={form.pullsDeleteBranch}
                onChange={(pullsDeleteBranch) => set({ pullsDeleteBranch })}
              />
              <CheckboxField
                id="pullsIgnoreWhitespace"
                label={t("repo.settings.pulls.ignore_whitespace")}
                checked={form.pullsIgnoreWhitespace}
                onChange={(pullsIgnoreWhitespace) => set({ pullsIgnoreWhitespace })}
              />
            </FeatureGroup>
          ) : null}
          <div>
            <Button type="submit" disabled={noMergeMethod}>
              {t("repo.settings.update_settings")}
            </Button>
          </div>
        </div>
      </fieldset>
    </form>
  );
}

// MessagePresetField picks the message a merge style starts with, where the
// empty preset is the default message.
function MessagePresetField<P extends string>({
  id,
  description,
  presets,
  value,
  onChange,
}: {
  id: string;
  description: string;
  presets: P[];
  value: P;
  onChange: (value: P) => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="ml-6.5 flex flex-col gap-1">
      <span id={`${id}-label`} className="text-sm font-medium">
        {t("repo.settings.pulls.default_commit_message")}
      </span>
      <span id={`${id}-description`} className="text-sm text-(--color-muted-foreground)">
        {description}
      </span>
      {/* Radix Select does not allow an empty value, so the default preset
          is "default" here. */}
      <Select value={value || "default"} onValueChange={(v) => onChange((v === "default" ? "" : v) as P)}>
        <SelectTrigger
          id={id}
          aria-labelledby={`${id}-label`}
          aria-describedby={`${id}-description`}
          className="mt-1 h-8 w-auto max-w-full gap-2"
        >
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {presets.map((preset) => (
            <SelectItem key={preset || "default"} value={preset || "default"}>
              {t(`repo.settings.pulls.message_${preset || "default"}`)}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  );
}

// DangerZone changes the visibility of the repository. For its owner, it also
// converts, transfers or deletes the repository, or erases its wiki, once the
// owner types its name.
function DangerZone({ owner, name, settings }: { owner: string; name: string; settings: RepoSettingsData }) {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const refresh = useRefresh(owner, name);
  const repoHome = (repoOwner: string, repoName: string) => window.location.assign(subUrl(`/${repoOwner}/${repoName}`));

  return (
    <div className="rounded-md border border-(--color-destructive)">
      {settings.visibilityFollowsBase ? null : <VisibilityAction owner={owner} name={name} settings={settings} />}
      {settings.viewerIsOwner ? (
        <>
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
                t("repo.settings.wiki_delete_notices_1", { repo: settings.name }),
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
        </>
      ) : null}
    </div>
  );
}

// VisibilityAction makes the repository private or unlisted, or public again.
function VisibilityAction({ owner, name, settings }: { owner: string; name: string; settings: RepoSettingsData }) {
  const { t } = useTranslation();
  const refresh = useRefresh(owner, name);
  const [open, setOpen] = useState(false);
  const [form, setForm] = useState({ private: settings.private, unlisted: settings.unlisted });
  const mutation = useMutation({
    // The settings are saved together, so the rest stays as it is.
    mutationFn: () =>
      updateRepoSettings(owner, name, {
        name: settings.name,
        description: settings.description,
        website: settings.website,
        ...form,
      }),
    onSuccess: async () => {
      setOpen(false);
      toast.success(t("repo.settings.update_settings_success"));
      await refresh();
    },
  });
  const { formError } = formErrors(mutation.error);
  const changed = form.private !== settings.private || form.unlisted !== settings.unlisted;
  // The private checkbox already says why when the site forces it.
  const visibilityLimited = (!settings.canBePublic && !settings.forcePrivate) || !settings.canBeListed;
  const title = t("repo.settings.change_visibility");
  const description = [
    t(settings.private ? "repo.settings.visibility_private" : "repo.settings.visibility_public"),
    settings.unlisted ? t("repo.settings.visibility_unlisted") : "",
  ]
    .filter(Boolean)
    .join(" ");

  const onOpenChange = (next: boolean) => {
    setOpen(next);
    if (!next) {
      setForm({ private: settings.private, unlisted: settings.unlisted });
      mutation.reset();
    }
  };
  const onSubmit = (e: FormEvent) => {
    e.preventDefault();
    if (changed) mutation.mutate();
  };

  return (
    <div className="flex flex-wrap items-center justify-between gap-x-4 gap-y-3 border-t border-(--color-border) px-4 py-3 first:border-t-0">
      <div className="min-w-0 flex-1 basis-56">
        <p className="text-sm font-semibold">{title}</p>
        <p className="text-sm text-(--color-muted-foreground)">{description}</p>
      </div>
      <Dialog open={open} onOpenChange={onOpenChange}>
        <DialogTrigger asChild>
          <Button type="button" variant="outline" size="sm" className="shrink-0 text-(--color-destructive)">
            {title}
          </Button>
        </DialogTrigger>
        <DialogContent>
          <form onSubmit={onSubmit} noValidate className="flex min-h-0 flex-1 flex-col">
            <fieldset disabled={mutation.isPending} className="contents">
              <DialogHeader>
                <DialogTitle>{title}</DialogTitle>
                <DialogDescription>{description}</DialogDescription>
              </DialogHeader>
              <DialogBody>
                <FormAlert message={formError} />
                <fieldset
                  className="flex flex-col gap-3"
                  aria-describedby={
                    [visibilityLimited ? "visibility-limited" : "", settings.forkCount ? "visibility-helper" : ""]
                      .filter(Boolean)
                      .join(" ") || undefined
                  }
                >
                  <legend className="mb-3 text-sm font-medium">{t("repo.visibility")}</legend>
                  {settings.forcePrivate ? (
                    <label htmlFor="private" className="flex items-start gap-2.5 text-sm">
                      <Checkbox id="private" className="mt-0.5" checked disabled />
                      <span>
                        <RichText text={t("repo.visiblity_helper_forced")} />
                      </span>
                    </label>
                  ) : (
                    <CheckboxField
                      id="private"
                      label={<RichText text={t("repo.visiblity_helper")} />}
                      checked={form.private || !settings.canBePublic}
                      onChange={(checked) => setForm((f) => ({ ...f, private: checked }))}
                      disabled={!settings.canBePublic}
                    />
                  )}
                  <CheckboxField
                    id="unlisted"
                    label={<RichText text={t("repo.unlisted_helper")} />}
                    checked={form.unlisted || !settings.canBeListed}
                    onChange={(checked) => setForm((f) => ({ ...f, unlisted: checked }))}
                    disabled={!settings.canBeListed}
                  />
                  {visibilityLimited ? (
                    <p id="visibility-limited" className="text-sm text-(--color-muted-foreground)">
                      {t("repo.settings.fork_visibility_limited")}
                    </p>
                  ) : null}
                  {settings.forkCount ? (
                    <p id="visibility-helper" className="text-sm text-(--color-muted-foreground)">
                      {t("repo.visiblity_fork_helper")}
                    </p>
                  ) : null}
                </fieldset>
              </DialogBody>
              <DialogFooter>
                <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
                  {t("cancel")}
                </Button>
                <Button
                  type="submit"
                  variant="destructive"
                  // A long label wraps on narrow screens, so the button grows with it.
                  className="h-auto min-h-10"
                  disabled={!changed}
                >
                  {t("repo.settings.change_visibility_confirm")}
                </Button>
              </DialogFooter>
            </fieldset>
          </form>
        </DialogContent>
      </Dialog>
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
      <Dialog open={open} onOpenChange={onOpenChange}>
        <DialogTrigger asChild>
          <Button type="button" variant="outline" size="sm" className="shrink-0 text-(--color-destructive)">
            {title}
          </Button>
        </DialogTrigger>
        <DialogContent>
          <form onSubmit={onSubmit} noValidate className="flex min-h-0 flex-1 flex-col">
            <fieldset disabled={mutation.isPending} className="contents">
              <DialogHeader>
                <DialogTitle>{title}</DialogTitle>
                <DialogDescription className="sr-only">{description}</DialogDescription>
              </DialogHeader>
              <DialogBody>
                <ul className="list-disc space-y-1 pl-5 text-sm text-(--color-muted-foreground)">
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
              </DialogBody>
              <DialogFooter>
                <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
                  {t("cancel")}
                </Button>
                <Button
                  type="submit"
                  variant="destructive"
                  // A long label wraps on narrow screens, so the button grows with it.
                  className="h-auto min-h-10"
                  disabled={!ready}
                >
                  {confirmLabel}
                </Button>
              </DialogFooter>
            </fieldset>
          </form>
        </DialogContent>
      </Dialog>
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
              <Button type="submit">{t("repo.settings.add_collaborator")}</Button>
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
    onError: (err) => toastError(err),
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
      toast.success(t("repo.settings.add_key_success", { name: form.title }));
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
              <Button type="submit">{t("repo.settings.add_deploy_key")}</Button>
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

// RepoBranchSettings changes the default branch of a repository and leads to
// the protection of its branches.
export function RepoBranchSettings({
  owner,
  name,
  settings,
}: {
  owner: string;
  name: string;
  settings: RepoBranchSettingsData;
}) {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const refresh = useRefresh(owner, name);
  const [branch, setBranch] = useState(settings.defaultBranch);
  const update = useMutation({
    mutationFn: () => updateRepoDefaultBranch(owner, name, branch),
    onSuccess: async () => {
      toast.success(t("repo.settings.update_default_branch_success"));
      await refresh();
    },
  });
  const { fields, formError } = formErrors(update.error);

  const onSubmit = (e: FormEvent) => {
    e.preventDefault();
    update.mutate();
  };

  return (
    <RepoSettingsLayout owner={owner} name={name} active="branches">
      <h1 className="sr-only">{t("repo.settings.branches")}</h1>
      {settings.isEmpty ? (
        <p className="glass rounded-md border border-(--color-border) px-4 py-3 text-sm text-(--color-muted-foreground)">
          {t("repo.settings.branches_empty")}
        </p>
      ) : (
        <div>
          <Section title={t("repo.settings.default_branch")}>
            <p className="mb-4 text-sm text-(--color-muted-foreground)">{t("repo.settings.default_branch_desc")}</p>
            <form onSubmit={onSubmit} noValidate>
              <fieldset disabled={update.isPending} className="contents">
                <FormAlert message={formError} />
                <div className="flex flex-wrap gap-2">
                  <Select value={branch} onValueChange={setBranch}>
                    <SelectTrigger
                      id="branch"
                      aria-label={t("repo.settings.default_branch")}
                      aria-invalid={"branch" in fields ? true : undefined}
                      aria-describedby={fields.branch ? "branch-error" : undefined}
                      className="w-auto max-w-full min-w-48 gap-2 font-mono"
                    >
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      {settings.branches.map((b) => (
                        <SelectItem key={b} value={b} className="font-mono">
                          {b}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                  <Button type="submit">{t("repo.settings.update")}</Button>
                </div>
                {fields.branch ? (
                  <p id="branch-error" className="mt-1.5 text-sm text-(--color-destructive)">
                    {fields.branch}
                  </p>
                ) : null}
              </fieldset>
            </form>
          </Section>
          <Section title={t("repo.settings.protected_branches")}>
            <p className="mb-4 text-sm text-(--color-muted-foreground)">{t("repo.settings.protected_branches_desc")}</p>
            <Select
              value=""
              onValueChange={(b) =>
                void navigate({ to: "/$owner/$repo/settings/branches/$", params: { owner, repo: name, _splat: b } })
              }
            >
              <SelectTrigger
                aria-label={t("repo.settings.choose_a_branch")}
                className="w-auto max-w-full min-w-48 gap-2"
              >
                <SelectValue placeholder={t("repo.settings.choose_a_branch")} />
              </SelectTrigger>
              <SelectContent>
                {settings.branches.map((b) => (
                  <SelectItem key={b} value={b} className="font-mono">
                    {b}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            {settings.protectedBranches.length ? (
              <ul className="mt-4 glass rounded-md border border-(--color-border)">
                {settings.protectedBranches.map((b) => (
                  <li key={b} className="border-b border-(--color-border) last:border-b-0">
                    <Link
                      to="/$owner/$repo/settings/branches/$"
                      params={{ owner, repo: name, _splat: b }}
                      className="flex items-center gap-2 px-4 py-2.5 font-mono text-sm wrap-anywhere text-(--color-primary) hover:bg-(--color-surface) hover:underline"
                    >
                      <ShieldCheck className="size-4 shrink-0" aria-hidden />
                      {b}
                    </Link>
                  </li>
                ))}
              </ul>
            ) : null}
          </Section>
        </div>
      )}
    </RepoSettingsLayout>
  );
}

// RepoBranchProtection changes how a branch is protected. Only repositories
// of organizations can let people and teams push past the protection.
export function RepoBranchProtection({
  owner,
  name,
  branch,
}: {
  owner: string;
  name: string;
  branch: RepoProtectedBranch;
}) {
  const { t } = useTranslation();
  const router = useRouter();
  const [form, setForm] = useState<RepoProtectedBranchForm>({
    protected: branch.protected,
    requirePullRequest: branch.requirePullRequest,
    requiredApprovals: branch.requiredApprovals,
    dismissStaleApprovals: branch.dismissStaleApprovals,
    requireResolvedConversations: branch.requireResolvedConversations,
    enableAllowlist: branch.enableAllowlist,
    allowlistUserIDs: branch.allowlistUserIDs,
    allowlistTeamIDs: branch.allowlistTeamIDs,
  });
  const set = (patch: Partial<RepoProtectedBranchForm>) => setForm({ ...form, ...patch });
  const toggle = (ids: number[], id: number, on: boolean) => (on ? [...ids, id] : ids.filter((x) => x !== id));
  const update = useMutation({
    mutationFn: () => updateRepoProtectedBranch(owner, name, branch.name, form),
    onSuccess: async () => {
      toast.success(t("repo.settings.update_protect_branch_success"));
      await router.invalidate();
    },
  });
  const { formError } = formErrors(update.error);
  const [before, after = ""] = t("repo.settings.branch_protection_desc").split(/<b>\{branch\}<\/b>|\{branch\}/);
  const isOrg = branch.userOptions !== undefined;

  const onSubmit = (e: FormEvent) => {
    e.preventDefault();
    update.mutate();
  };

  return (
    <RepoSettingsLayout owner={owner} name={name} active="branches">
      <h1 className="mb-2 text-xl font-semibold">{t("repo.settings.branch_protection")}</h1>
      <p className="mb-6 text-sm text-(--color-muted-foreground)">
        {before}
        <code className="rounded bg-(--color-surface) px-1 font-mono text-[0.9em] font-semibold wrap-anywhere text-(--color-foreground)">
          {branch.name}
        </code>
        {after}
      </p>
      <form onSubmit={onSubmit} noValidate>
        <fieldset disabled={update.isPending} className="contents">
          <FormAlert message={formError} />
          <div className="flex flex-col gap-5">
            <CheckboxField
              id="protected"
              checked={form.protected}
              onChange={(on) => set({ protected: on })}
              label={
                <OptionLabel
                  title={t("repo.settings.protect_this_branch")}
                  desc={t("repo.settings.protect_this_branch_desc")}
                />
              }
            />
            {form.protected ? (
              <div className="ml-2 flex flex-col gap-5 border-l-2 border-(--color-border) pl-4">
                <CheckboxField
                  id="requirePullRequest"
                  checked={form.requirePullRequest}
                  onChange={(on) => set({ requirePullRequest: on })}
                  label={
                    <OptionLabel
                      title={t("repo.settings.protect_require_pull_request")}
                      desc={t("repo.settings.protect_require_pull_request_desc")}
                    />
                  }
                />
                <div className="flex flex-col gap-1.5">
                  <label htmlFor="requiredApprovals" className="text-sm font-medium">
                    {t("repo.settings.protect_required_approvals")}
                  </label>
                  <p id="requiredApprovals-desc" className="text-sm text-(--color-muted-foreground)">
                    {t("repo.settings.protect_required_approvals_desc")}
                  </p>
                  <Input
                    id="requiredApprovals"
                    type="number"
                    inputMode="numeric"
                    min={0}
                    max={10}
                    className="w-24"
                    aria-describedby="requiredApprovals-desc"
                    value={form.requiredApprovals}
                    onChange={(e) => set({ requiredApprovals: Math.min(10, Math.max(0, Number(e.target.value) || 0)) })}
                  />
                </div>
                {form.requiredApprovals > 0 ? (
                  <div className="ml-2 flex flex-col gap-5 border-l-2 border-(--color-border) pl-4">
                    <CheckboxField
                      id="dismissStaleApprovals"
                      checked={form.dismissStaleApprovals}
                      onChange={(on) => set({ dismissStaleApprovals: on })}
                      label={
                        <OptionLabel
                          title={t("repo.settings.protect_dismiss_stale_approvals")}
                          desc={t("repo.settings.protect_dismiss_stale_approvals_desc")}
                        />
                      }
                    />
                    <CheckboxField
                      id="requireResolvedConversations"
                      checked={form.requireResolvedConversations}
                      onChange={(on) => set({ requireResolvedConversations: on })}
                      label={
                        <OptionLabel
                          title={t("repo.settings.protect_require_resolved_conversations")}
                          desc={t("repo.settings.protect_require_resolved_conversations_desc")}
                        />
                      }
                    />
                  </div>
                ) : null}
                {isOrg ? (
                  <CheckboxField
                    id="enableAllowlist"
                    checked={form.enableAllowlist}
                    onChange={(on) => set({ enableAllowlist: on })}
                    label={
                      <OptionLabel
                        title={t("repo.settings.protect_allowlist_committers")}
                        desc={t("repo.settings.protect_allowlist_committers_desc")}
                      />
                    }
                  />
                ) : null}
                {isOrg && form.enableAllowlist ? (
                  <div className="ml-2 flex flex-col gap-5 border-l-2 border-(--color-border) pl-4">
                    {branch.userOptions?.length ? (
                      <fieldset className="flex flex-col gap-2.5">
                        <legend className="mb-2.5 text-sm font-medium">
                          {t("repo.settings.protect_allowlist_users")}
                        </legend>
                        {branch.userOptions.map((u) => (
                          <CheckboxField
                            key={u.id}
                            id={`allowlist-user-${u.id}`}
                            checked={form.allowlistUserIDs.includes(u.id)}
                            onChange={(on) => set({ allowlistUserIDs: toggle(form.allowlistUserIDs, u.id, on) })}
                            label={
                              <span className="inline-flex items-center gap-2">
                                <img src={u.avatarURL} alt="" className="size-5 rounded-full" />
                                {u.name}
                              </span>
                            }
                          />
                        ))}
                      </fieldset>
                    ) : null}
                    {branch.teamOptions?.length ? (
                      <fieldset className="flex flex-col gap-2.5">
                        <legend className="mb-2.5 text-sm font-medium">
                          {t("repo.settings.protect_allowlist_teams")}
                        </legend>
                        {branch.teamOptions.map((team) => (
                          <CheckboxField
                            key={team.id}
                            id={`allowlist-team-${team.id}`}
                            checked={form.allowlistTeamIDs.includes(team.id)}
                            onChange={(on) => set({ allowlistTeamIDs: toggle(form.allowlistTeamIDs, team.id, on) })}
                            label={
                              <span className="inline-flex items-center gap-2">
                                <Users className="size-4 text-(--color-muted-foreground)" aria-hidden />
                                {team.name}
                              </span>
                            }
                          />
                        ))}
                      </fieldset>
                    ) : null}
                  </div>
                ) : null}
              </div>
            ) : null}
            <div className="flex flex-wrap items-center gap-2 border-t border-(--color-border) pt-5">
              <Button type="submit">{t("repo.settings.update_settings")}</Button>
              <Button asChild variant="outline">
                <Link to="/$owner/$repo/settings/branches" params={{ owner, repo: name }}>
                  {t("cancel")}
                </Link>
              </Button>
            </div>
          </div>
        </fieldset>
      </form>
    </RepoSettingsLayout>
  );
}

function OptionLabel({ title, desc }: { title: string; desc: string }) {
  return (
    <>
      <span className="font-medium">{title}</span>
      <span className="block text-(--color-muted-foreground)">{desc}</span>
    </>
  );
}

function HookState({ isActive }: { isActive: boolean }) {
  const { t } = useTranslation();
  return (
    <span className="inline-flex items-center gap-1.5 rounded-full border border-(--color-border) px-2 text-xs leading-5 font-medium text-(--color-muted-foreground)">
      <span
        aria-hidden
        className={cn("size-2 rounded-full", isActive ? "bg-(--color-success)" : "bg-(--color-muted-foreground)")}
      />
      {isActive ? t("repo.settings.active") : t("repo.settings.githook_inactive")}
    </span>
  );
}

// RepoGitHooks lists the server-side Git hooks of a repository. Only site
// admins and the users they allow can edit them, since hooks run on the
// server.
export function RepoGitHooks({ owner, name, hooks }: { owner: string; name: string; hooks: RepoGitHook[] }) {
  const { t } = useTranslation();
  return (
    <RepoSettingsLayout owner={owner} name={name} active="githooks">
      <h1 className="mb-2 text-xl font-semibold">{t("repo.settings.githooks")}</h1>
      <p className="mb-4 text-sm text-(--color-muted-foreground)">{t("repo.settings.githooks_desc")}</p>
      <ul className="glass rounded-md border border-(--color-border)">
        {hooks.map((hook) => (
          <li key={hook.name} className="border-b border-(--color-border) last:border-b-0">
            <Link
              to="/$owner/$repo/settings/hooks/git/$name"
              params={{ owner, repo: name, name: hook.name }}
              className="flex flex-wrap items-center gap-x-3 gap-y-1 px-4 py-3 hover:bg-(--color-surface)"
            >
              <span className="font-mono text-sm text-(--color-primary) hover:underline">{hook.name}</span>
              <HookState isActive={hook.isActive} />
              <Pencil className="ml-auto size-4 text-(--color-muted-foreground)" aria-hidden />
            </Link>
          </li>
        ))}
      </ul>
    </RepoSettingsLayout>
  );
}

// RepoGitHookEdit changes the content of a Git hook. An inactive hook shows
// its sample, and an active one can be deleted to make it inactive again.
export function RepoGitHookEdit({ owner, name, hook }: { owner: string; name: string; hook: RepoGitHookContent }) {
  const { t } = useTranslation();
  const router = useRouter();
  const navigate = useNavigate();
  const [content, setContent] = useState(hook.content);
  const update = useMutation({
    mutationFn: () => updateRepoGitHook(owner, name, hook.name, content),
    onSuccess: async () => {
      toast.success(t("repo.settings.update_githook_success"));
      await router.invalidate();
    },
  });
  const { fields, formError } = formErrors(update.error);

  const onSubmit = (e: FormEvent) => {
    e.preventDefault();
    update.mutate();
  };

  return (
    <RepoSettingsLayout owner={owner} name={name} active="githooks">
      <div className="mb-2 flex flex-wrap items-center gap-3">
        <h1 className="font-mono text-xl font-semibold">{hook.name}</h1>
        <HookState isActive={hook.isActive} />
      </div>
      <p className="mb-6 text-sm text-(--color-muted-foreground)">{t("repo.settings.githook_edit_desc")}</p>
      <form onSubmit={onSubmit} noValidate>
        <fieldset disabled={update.isPending} className="contents">
          <FormAlert message={formError} />
          <div className="flex flex-col gap-5">
            <FormField
              id="content"
              label={t("repo.settings.githook_content")}
              error={fields.content}
              invalid={"content" in fields}
            >
              <CodeEditor
                id="content"
                label={t("repo.settings.githook_content")}
                lang="shell"
                code
                minLines={20}
                disabled={update.isPending}
                value={content}
                onChange={setContent}
              />
            </FormField>
            <div className="flex flex-wrap items-center gap-2">
              <Button type="submit">{t("repo.settings.update_githook")}</Button>
              <Button asChild variant="outline">
                <Link to="/$owner/$repo/settings/hooks/git" params={{ owner, repo: name }}>
                  {t("cancel")}
                </Link>
              </Button>
              {hook.isActive && (
                <div className="ml-auto">
                  <ConfirmButton
                    label={t("repo.settings.delete_githook")}
                    title={t("repo.settings.delete_githook")}
                    description={t("repo.settings.githook_deletion_desc")}
                    action={() => deleteRepoGitHook(owner, name, hook.name)}
                    onDone={async () => {
                      toast.success(t("repo.settings.delete_githook_success"));
                      await navigate({ to: "/$owner/$repo/settings/hooks/git", params: { owner, repo: name } });
                    }}
                  />
                </div>
              )}
            </div>
          </div>
        </fieldset>
      </form>
    </RepoSettingsLayout>
  );
}
