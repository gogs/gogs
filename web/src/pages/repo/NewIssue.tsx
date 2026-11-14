import { useMutation, useSuspenseQuery } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { type FormEvent, useState } from "react";
import { useTranslation } from "react-i18next";

import { FormAlert } from "@/components/FormField";
import { AttachFilesButton, IssueMeta, MarkdownEditor, PendingFiles } from "@/components/IssueForm";
import { RepoHeader } from "@/components/RepoHeader";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { usePageTitle } from "@/lib/page-title";
import { type RepoNewIssueOptions, createRepoIssue, repoHeaderQuery } from "@/lib/queries/repo";
import { formErrors } from "@/lib/queries/request";
import { useAttachmentUploads } from "@/lib/use-attachment-uploads";
import type { RepoNewIssueSearch } from "@/pages/repo/NewIssue.search";

// RepoNewIssue is the form to open an issue. Writers also set its labels,
// milestone and assignee.
export function RepoNewIssue({
  owner,
  name,
  options,
  search,
}: {
  owner: string;
  name: string;
  options: RepoNewIssueOptions & { template: string };
  search: RepoNewIssueSearch;
}) {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const { data: repo } = useSuspenseQuery(repoHeaderQuery(owner, name));
  usePageTitle(`${t("repo.issues.new")} - ${repo.owner}/${repo.name}`);
  const repoRef = { owner: repo.owner, name: repo.name };
  const [title, setTitle] = useState(search.title ?? "");
  // Like the server-rendered page, the issue template wins over the content
  // in the URL.
  const [content, setContent] = useState(options.template || (search.content ?? ""));
  const [labelIDs, setLabelIDs] = useState<number[]>([]);
  const [milestoneID, setMilestoneID] = useState(0);
  const [assigneeID, setAssigneeID] = useState(0);
  const uploads = useAttachmentUploads(repo.owner, repo.name, options.attachment);

  const mutation = useMutation({
    mutationFn: () =>
      createRepoIssue(repo.owner, repo.name, {
        title,
        content,
        labelIDs,
        milestoneID,
        assigneeID,
        attachments: uploads.uploaded,
      }),
    onSuccess: ({ index }) =>
      navigate({
        to: "/$owner/$repo/issues/$index",
        params: { owner: repo.owner, repo: repo.name, index: String(index) },
      }),
  });
  const { fields, formError } = formErrors(mutation.error);
  const onSubmit = (e: FormEvent) => {
    e.preventDefault();
    mutation.mutate();
  };
  const milestone = options.milestoneOptions?.find((m) => m.id === milestoneID);
  const assignee = options.assigneeOptions?.find((u) => u.id === assigneeID);
  // The choices are kept in the form until it is submitted.
  const pick = (set: (id: number) => void) => (id: number) => {
    set(id);
    return Promise.resolve();
  };

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <RepoHeader repo={repo} activeTab="issues" />
      <main className="mx-auto w-full max-w-7xl flex-1 px-4 py-6 sm:px-6">
        <h1 className="mb-4 text-2xl font-semibold">{t("repo.issues.new")}</h1>
        <form onSubmit={onSubmit} className="grid content-start gap-8 md:grid-cols-[minmax(0,1fr)_16rem]">
          <fieldset disabled={mutation.isPending} className="contents">
            <div className="min-w-0">
              <FormAlert message={formError} />
              <div className="flex flex-col gap-1.5">
                <Input
                  aria-label={t("repo.milestones.title")}
                  placeholder={t("repo.milestones.title")}
                  value={title}
                  maxLength={255}
                  autoFocus
                  aria-invalid={Boolean(fields.title) || undefined}
                  aria-describedby={fields.title ? "new-issue-title-error" : undefined}
                  onChange={(e) => setTitle(e.target.value)}
                />
                {fields.title ? (
                  <p id="new-issue-title-error" className="text-sm text-(--color-destructive)">
                    {fields.title}
                  </p>
                ) : null}
              </div>
              <div className="mt-4 rounded-md border border-(--color-border) p-3">
                <MarkdownEditor
                  repo={repoRef}
                  label={t("repo.issues.new")}
                  value={content}
                  onChange={setContent}
                  disabled={mutation.isPending}
                  uploads={uploads}
                />
                <PendingFiles uploads={uploads} />
                <div className="mt-3 flex flex-wrap items-center justify-between gap-2">
                  <AttachFilesButton uploads={uploads} />
                  <Button type="submit" size="sm" disabled={uploads.uploading}>
                    {t("repo.issues.create")}
                  </Button>
                </div>
              </div>
            </div>
            <IssueMeta
              repo={repoRef}
              value={{
                labels: options.labelOptions?.filter((l) => labelIDs.includes(l.id)) ?? [],
                milestone,
                assignee,
              }}
              options={options}
              pending={mutation.isPending}
              onLabels={setLabelIDs}
              onMilestone={pick(setMilestoneID)}
              onAssignee={pick(setAssigneeID)}
              linkChoices={false}
            />
          </fieldset>
        </form>
      </main>
    </div>
  );
}
