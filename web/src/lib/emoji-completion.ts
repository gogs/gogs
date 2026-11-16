import type { Completion, CompletionContext, CompletionResult } from "@codemirror/autocomplete";
import { syntaxTree } from "@codemirror/language";

// emoji.json lists the names of emoji shortcodes with their text, sorted by
// name. It is generated from internal/markup/emoji.go, the names that Markdown
// renders as emoji, and must be regenerated with it.
import emojiURL from "@/lib/emoji.json?url";

// NOT_TEXT are the Markdown syntax nodes whose text keeps its emoji
// shortcodes, or that are not text at all.
const NOT_TEXT = new Set(["InlineCode", "FencedCode", "CodeBlock", "URL", "Autolink", "HTMLTag", "CommentBlock"]);

let table: Promise<Completion[]> | undefined;

// options loads the emoji on the first shortcode, so editors that never see
// one do not pay for the table. It is fetched rather than imported because
// browsers keep a failed import failed, while a failed fetch, e.g., during a
// deploy, is tried again on the next shortcode.
function options() {
  table ??= fetch(emojiURL)
    .then((res) => {
      if (!res.ok) throw new Error(`load emoji: ${res.status}`);
      return res.json() as Promise<[name: string, emoji: string][]>;
    })
    .then((emoji) => emoji.map(([name, text]) => ({ label: `:${name}:`, displayLabel: `${text} ${name}` })))
    .catch((err: unknown) => {
      table = undefined;
      throw err;
    });
  return table;
}

// emojiCompletion completes emoji shortcodes, e.g., ":ta" to ":tada:", that
// start a line or follow whitespace, outside code. Names that start with what
// was typed come first, then names that contain it.
export async function emojiCompletion(context: CompletionContext): Promise<CompletionResult | null> {
  const typed = context.matchBefore(/:[\w+-]+$/);
  if (!typed) return null;
  if (typed.from > context.state.doc.lineAt(typed.from).from) {
    const before = context.state.sliceDoc(typed.from - 1, typed.from);
    if (before !== " " && before !== "\t") return null;
  }
  let node = syntaxTree(context.state).resolveInner(typed.from, 1);
  for (;;) {
    if (NOT_TEXT.has(node.name)) return null;
    if (!node.parent) break;
    node = node.parent;
  }

  const query = typed.text.slice(1).toLowerCase();
  // A source that rejects is not queried again until the cursor moves, so a
  // failed load shows no completions instead.
  const all = await options().catch(() => null);
  if (!all || context.aborted) return null;
  const prefix: Completion[] = [];
  const inside: Completion[] = [];
  for (const option of all) {
    const at = option.label.indexOf(query, 1);
    if (at === 1) prefix.push(option);
    else if (at > 1) inside.push(option);
  }
  if (prefix.length + inside.length === 0) return null;
  return { from: typed.from, options: [...prefix, ...inside], filter: false };
}
