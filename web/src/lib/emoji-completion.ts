import type { Completion, CompletionContext, CompletionResult } from "@codemirror/autocomplete";
import { syntaxTree } from "@codemirror/language";

// NOT_TEXT are the Markdown syntax nodes whose text keeps its emoji
// shortcodes, or that are not text at all.
const NOT_TEXT = new Set(["InlineCode", "FencedCode", "CodeBlock", "URL", "Autolink", "HTMLTag", "CommentBlock"]);

let table: Promise<Completion[]> | undefined;

// options loads the emoji on the first shortcode, so editors that never see
// one do not pay for the table.
function options() {
  table ??= import("@/lib/emoji").then(({ EMOJI }) =>
    EMOJI.map(([name, emoji]) => ({ label: `:${name}:`, displayLabel: `${emoji} ${name}` })),
  );
  return table;
}

// emojiCompletion completes emoji shortcodes, e.g., ":ta" to ":tada:", that
// start a line or follow whitespace, outside code, the way GitHub does. Names
// that start with what was typed come first, then names that contain it.
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
  const all = await options();
  if (context.aborted) return null;
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
