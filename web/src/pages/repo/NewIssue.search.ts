export interface RepoNewIssueSearch {
  // title and content prefill the form, like on the server-rendered page.
  title?: string;
  content?: string;
  // template is the file of the issue template to open the issue with, and
  // blank skips the template chooser.
  template?: string;
  blank?: boolean;
}

export function validateRepoNewIssueSearch(search: Record<string, unknown>): RepoNewIssueSearch {
  const text = (value: unknown) => (typeof value === "string" && value ? value : undefined);
  return {
    title: text(search.title),
    content: text(search.content),
    template: text(search.template),
    blank: search.blank === true || undefined,
  };
}
