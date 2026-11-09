// Search-param schema for the user profile route. Defined here (and not in the
// page component) so both the route definition and the page can import from it.

// The repositories tab and the first page stay implicit, so they never appear
// in the URL. Like the server-rendered page, any other `tab` value shows the
// repositories.
export interface ProfileSearch {
  tab?: "activity";
  page?: number;
}

export function validateProfileSearch(search: Record<string, unknown>): ProfileSearch {
  // A repeated parameter arrives as an array. Use the first value, like the server.
  const first = (value: unknown) => (Array.isArray(value) ? (value as unknown[])[0] : value);
  if (first(search.tab) === "activity") return { tab: "activity", page: undefined };
  const page = Number(first(search.page));
  return { tab: undefined, page: Number.isInteger(page) && page > 1 ? page : undefined };
}
