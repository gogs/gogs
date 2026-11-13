import { defaultKeymap, history, historyKeymap, indentWithTab } from "@codemirror/commands";
import { Language, StreamLanguage, defineLanguageFacet, syntaxHighlighting } from "@codemirror/language";
import { shell } from "@codemirror/legacy-modes/mode/shell";
import { Compartment, EditorState } from "@codemirror/state";
import { EditorView, keymap, lineNumbers } from "@codemirror/view";
import { classHighlighter, tagHighlighter, tags } from "@lezer/highlight";
import { GFM, parser as markdownParser } from "@lezer/markdown";
import { useEffect, useEffectEvent, useRef, useState } from "react";

import type { CodeEditorProps } from "@/components/CodeEditor";

// MARKDOWN_HIGHLIGHTER marks what classHighlighter leaves out of Markdown:
// inline code, quotes, and the characters of the markup.
const MARKDOWN_HIGHLIGHTER = tagHighlighter([
  { tag: tags.monospace, class: "tok-monospace" },
  { tag: tags.quote, class: "tok-quote" },
  { tag: tags.processingInstruction, class: "tok-meta" },
]);

// Markdown takes the parser alone: @codemirror/lang-markdown loads the HTML,
// CSS and JavaScript languages too, which triples the size of the editor.
const markdownLanguage = new Language(defineLanguageFacet(), markdownParser.configure(GFM), [], "markdown");

const LANGUAGES = {
  markdown: () => markdownLanguage,
  shell: () => StreamLanguage.define(shell),
};

// The token colors live in index.css as the .tok-* classes, so they follow the
// theme without reconfiguring the editor.
const THEME = EditorView.theme({
  "&": { backgroundColor: "transparent" },
  "&.cm-focused": { outline: "none" },
  ".cm-scroller": { fontFamily: "var(--font-mono)", lineHeight: "1.5" },
  ".cm-content": { padding: "0.5rem 0", caretColor: "var(--color-primary)" },
  ".cm-line": { padding: "0 0.75rem" },
  ".cm-gutters": {
    backgroundColor: "var(--color-surface)",
    color: "var(--color-muted-foreground)",
    border: "none",
    borderRight: "1px solid var(--color-border)",
  },
  ".cm-lineNumbers .cm-gutterElement": { padding: "0 0.5rem 0 0.75rem" },
});

// CodeEditorView is the CodeMirror half of CodeEditor, loaded on demand. The
// language and the layout are fixed once it mounts.
export default function CodeEditorView({
  id,
  label,
  value,
  onChange,
  lang,
  code = false,
  minLines = 6,
  disabled = false,
  autoFocus = false,
  "aria-invalid": invalid,
  "aria-describedby": describedBy,
}: CodeEditorProps) {
  const host = useRef<HTMLDivElement>(null);
  const view = useRef<EditorView | null>(null);
  const [attrs] = useState(() => new Compartment());
  const [editable] = useState(() => new Compartment());
  const [initial] = useState(() => ({ value, lang, code, minLines, autoFocus }));
  const emitChange = useEffectEvent((text: string) => onChange(text));

  useEffect(() => {
    const v = new EditorView({
      parent: host.current!,
      state: EditorState.create({
        doc: initial.value,
        extensions: [
          history(),
          keymap.of([...defaultKeymap, ...historyKeymap, ...(initial.code ? [indentWithTab] : [])]),
          LANGUAGES[initial.lang](),
          syntaxHighlighting(classHighlighter),
          syntaxHighlighting(MARKDOWN_HIGHLIGHTER),
          THEME,
          EditorView.theme({ ".cm-content": { minHeight: `${initial.minLines * 1.5}em` } }),
          initial.code ? lineNumbers() : EditorView.lineWrapping,
          EditorView.contentAttributes.of(
            initial.code ? { spellcheck: "false", autocorrect: "off", autocapitalize: "off" } : { spellcheck: "true" },
          ),
          // Dropping a file would otherwise insert its bytes as text.
          EditorView.domEventHandlers({
            drop: (e) => {
              if (!e.dataTransfer?.files.length) return false;
              e.preventDefault();
              return true;
            },
          }),
          attrs.of([]),
          editable.of([]),
          EditorView.updateListener.of((update) => {
            if (update.docChanged) emitChange(update.state.doc.toString());
          }),
        ],
      }),
    });
    view.current = v;
    if (initial.autoFocus) v.focus();
    return () => {
      v.destroy();
      view.current = null;
    };
  }, [initial, attrs, editable]);

  // Take the value from outside when it changes there, e.g., cleared after
  // submitting.
  useEffect(() => {
    const v = view.current;
    if (v && v.state.doc.toString() !== value) {
      v.dispatch({ changes: { from: 0, to: v.state.doc.length, insert: value } });
    }
  }, [value]);

  useEffect(() => {
    view.current?.dispatch({
      effects: attrs.reconfigure(
        EditorView.contentAttributes.of({
          id,
          "aria-label": label,
          ...(invalid ? { "aria-invalid": "true" } : {}),
          ...(describedBy ? { "aria-describedby": describedBy } : {}),
        }),
      ),
    });
  }, [attrs, id, label, invalid, describedBy]);

  useEffect(() => {
    view.current?.dispatch({
      effects: editable.reconfigure([EditorView.editable.of(!disabled), EditorState.readOnly.of(disabled)]),
    });
  }, [editable, disabled]);

  return <div ref={host} />;
}
