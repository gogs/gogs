// wikiPageName returns the name of the wiki page in a segment of the path.
// Server-rendered links escape page names like query values, with "+" for a
// space, so the segment is read from the path as it was given, not from the
// route params, which decode "%2B" to "+" too.
export function wikiPageName(pathname: string, fromEnd: number) {
  const segment =
    pathname
      .replace(/\/+$/, "")
      .split("/")
      .at(-1 - fromEnd) ?? "";
  try {
    return decodeURIComponent(segment.replaceAll("+", " "));
  } catch {
    return segment;
  }
}
