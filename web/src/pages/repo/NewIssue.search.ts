export interface RepoNewIssueSearch {
  // title and content prefill the form, like on the server-rendered page.
  title?: string;
  content?: string;
}

export function validateRepoNewIssueSearch(search: Record<string, unknown>): RepoNewIssueSearch {
  const text = (value: unknown) => (typeof value === "string" && value ? value : undefined);
  return { title: text(search.title), content: text(search.content) };
}
