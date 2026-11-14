import { useMutation, useQueryClient, useSuspenseQuery } from "@tanstack/react-query";
import { useRouter } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";

import { RepoHeader } from "@/components/RepoHeader";
import { UserList, type UserListItemData, type UserListPage } from "@/components/UserListItem";
import { Button } from "@/components/ui/button";
import { usePageTitle } from "@/lib/page-title";
import { removeRepoWatcher, repoHeaderQuery } from "@/lib/queries/repo";
import { useUserInfo } from "@/lib/use-user-info";

// RepoPeople lists the stargazers or the watchers of a repository.
export function RepoPeople({
  owner,
  name,
  list,
  data,
}: {
  owner: string;
  name: string;
  list: "stargazers" | "watchers";
  data: UserListPage;
}) {
  const { t } = useTranslation();
  const { data: repo } = useSuspenseQuery(repoHeaderQuery(owner, name));
  const user = useUserInfo();
  const title = list === "stargazers" ? t("repo.stargazers") : t("repo.watchers");
  usePageTitle(`${title} - ${owner}/${name}`);

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <RepoHeader repo={repo} />
      <main className="mx-auto w-full max-w-7xl flex-1 px-4 py-6 sm:px-6">
        <h1 className="flex items-center gap-2 border-b border-(--color-border) pb-2 text-xl font-semibold">
          {title}
          <span className="rounded-full bg-(--color-secondary) px-2 text-xs/5 font-medium tabular-nums text-(--color-secondary-foreground)">
            {list === "stargazers" ? repo.starCount : repo.watchCount}
          </span>
        </h1>
        <UserList
          data={data}
          action={
            // Site admins can make anyone stop watching, like on the server-rendered page.
            list === "watchers" && user?.isAdmin
              ? (watcher) => <UnwatchButton owner={owner} name={name} watcher={watcher} />
              : undefined
          }
        />
      </main>
    </div>
  );
}

function UnwatchButton({ owner, name, watcher }: { owner: string; name: string; watcher: UserListItemData }) {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const router = useRouter();
  const mutation = useMutation({
    mutationFn: () => removeRepoWatcher(owner, name, watcher.name),
    onSuccess: async (result) => {
      queryClient.setQueryData(repoHeaderQuery(owner, name).queryKey, (prev) =>
        prev ? { ...prev, watchCount: result.watchCount } : prev,
      );
      await router.invalidate();
    },
    onError: (err) => toast.error(t("status.internal_server_error"), { description: err.message }),
  });
  return (
    <Button
      variant="outline"
      size="sm"
      aria-label={`${t("repo.unwatch")} ${watcher.name}`}
      disabled={mutation.isPending}
      onClick={() => mutation.mutate()}
    >
      {t("repo.unwatch")}
    </Button>
  );
}
