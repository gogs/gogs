import { ISSUE_SORTS, type IssueSort } from "@/pages/user/DashboardIssues.search";

// Search-param schema for the issue and pull request lists of a repository.
// Defaults stay implicit, so they never appear in the URL: all issues, open,
// newest first, no label, milestone or assignee, the first page.
export interface RepoIssuesSearch {
  type?: "assigned" | "created_by" | "mentioned";
  labels?: number;
  milestone?: number;
  assignee?: number;
  state?: "closed";
  sort?: Exclude<IssueSort, "latest">;
  page?: number;
}

const TYPES = ["assigned", "created_by", "mentioned"] as const;

export function validateRepoIssuesSearch(search: Record<string, unknown>): RepoIssuesSearch {
  // A repeated parameter arrives as an array. Use the first value, like the server.
  const first = (value: unknown) => (Array.isArray(value) ? (value as unknown[])[0] : value);
  const positive = (value: unknown) => {
    const n = Number(first(value));
    return Number.isInteger(n) && n > 0 ? n : undefined;
  };
  const type = first(search.type);
  const sort = first(search.sort);
  const page = positive(search.page);
  return {
    type: TYPES.includes(type as (typeof TYPES)[number]) ? (type as RepoIssuesSearch["type"]) : undefined,
    labels: positive(search.labels),
    milestone: positive(search.milestone),
    assignee: positive(search.assignee),
    state: first(search.state) === "closed" ? "closed" : undefined,
    sort: sort !== "latest" && ISSUE_SORTS.includes(sort as IssueSort) ? (sort as RepoIssuesSearch["sort"]) : undefined,
    page: page && page > 1 ? page : undefined,
  };
}
