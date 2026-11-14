// Matching for the command palette. Text matches are substrings, which rank
// higher at the start of the text or of a word. File paths also match letters
// in order, the way "go to file" does.

const isMac = typeof navigator !== "undefined" && /Mac|iPhone|iPad/.test(navigator.platform || navigator.userAgent);

// paletteShortcut is the key that opens the palette, as shown to the viewer
// and as aria-keyshortcuts takes it.
export const paletteShortcut = isMac
  ? { label: "⌘K", aria: "Meta+K", matches: (e: KeyboardEvent) => e.metaKey && !e.ctrlKey }
  : { label: "Ctrl K", aria: "Control+K", matches: (e: KeyboardEvent) => e.ctrlKey && !e.metaKey };

export interface Segment {
  text: string;
  match: boolean;
}

const WORD_BOUNDARY = " /:-_.";

// scoreText returns how well the query matches the label or its extra
// keywords, from 3 for a match at the start down to 0 for none.
export function scoreText(label: string, keywords: string, query: string) {
  const text = `${label} ${keywords}`.toLowerCase();
  const at = text.indexOf(query.toLowerCase());
  if (at < 0) return 0;
  if (at === 0) return 3;
  return WORD_BOUNDARY.includes(text[at - 1]) ? 2 : 1;
}

// filterByText keeps the items that match the query, the best first, and at
// most limit of them.
export function filterByText<T extends { label: string; keywords?: string }>(items: T[], query: string, limit: number) {
  return items
    .map((item) => ({ item, score: scoreText(item.label, item.keywords ?? "", query) }))
    .filter(({ score }) => score > 0)
    .sort((a, b) => b.score - a.score)
    .slice(0, limit)
    .map(({ item }) => item);
}

function hasInOrder(text: string, query: string) {
  let found = 0;
  for (const ch of text) {
    if (ch === query[found]) found++;
    if (found === query.length) return true;
  }
  return false;
}

// scoreFile returns how well the query matches the path: a match in the file
// name beats one in the directories, and both beat letters in order.
export function scoreFile(path: string, query: string) {
  const q = query.toLowerCase().replace(/\s+/g, "");
  if (q.length < 2) return 0;
  const lower = path.toLowerCase();
  const base = lower.slice(lower.lastIndexOf("/") + 1);
  if (base.startsWith(q)) return 5;
  if (base.includes(q)) return 4;
  if (lower.includes(q)) return 3;
  if (hasInOrder(base, q)) return 2;
  if (hasInOrder(lower, q)) return 1;
  return 0;
}

// filterFiles returns the paths that match the query, the best first, and
// the shorter of two equally good ones first.
export function filterFiles(paths: string[], query: string, limit: number) {
  const scored: { path: string; score: number }[] = [];
  for (const path of paths) {
    const score = scoreFile(path, query);
    if (score > 0) scored.push({ path, score });
  }
  return scored
    .sort((a, b) => b.score - a.score || a.path.length - b.path.length)
    .slice(0, limit)
    .map(({ path }) => path);
}

function toSegments(text: string, matched: boolean[]) {
  const segments: Segment[] = [];
  for (let i = 0; i < text.length; i++) {
    const last = segments.at(-1);
    if (last && last.match === matched[i]) last.text += text[i];
    else segments.push({ text: text[i], match: matched[i] });
  }
  return segments;
}

// highlightText marks the first place the query is in the text.
export function highlightText(text: string, query: string): Segment[] {
  const at = query ? text.toLowerCase().indexOf(query.toLowerCase()) : -1;
  if (at < 0) return [{ text, match: false }];
  return toSegments(
    text,
    Array.from(text, (_, i) => i >= at && i < at + query.length),
  );
}

// highlightPath marks what scoreFile matched: the query in the file name, else
// in the path, else its letters in order.
export function highlightPath(path: string, query: string): Segment[] {
  const q = query.toLowerCase().replace(/\s+/g, "");
  const lower = path.toLowerCase();
  const baseStart = lower.lastIndexOf("/") + 1;
  let at = lower.indexOf(q, baseStart);
  if (at < 0) at = lower.indexOf(q);
  if (q && at >= 0) {
    return toSegments(
      path,
      Array.from(path, (_, i) => i >= at && i < at + q.length),
    );
  }
  // Letters in order are marked in the file name when they all are there.
  const from = hasInOrder(lower.slice(baseStart), q) ? baseStart : 0;
  let found = 0;
  return toSegments(
    path,
    Array.from(lower, (ch, i) => {
      if (i < from || found >= q.length || ch !== q[found]) return false;
      found++;
      return true;
    }),
  );
}
