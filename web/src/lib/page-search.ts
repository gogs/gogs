// Search-param schema for pages whose only parameter is the page number. The
// first page stays implicit, so it never appears in the URL.
export interface PageSearch {
  page?: number;
}

export function validatePageSearch(search: Record<string, unknown>): PageSearch {
  // A repeated parameter arrives as an array. Use the first value, like the server.
  const value: unknown = Array.isArray(search.page) ? (search.page as unknown[])[0] : search.page;
  const page = Number(value);
  return { page: Number.isInteger(page) && page > 1 ? page : undefined };
}
