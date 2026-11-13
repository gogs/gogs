import { defaultKeymap, history, historyKeymap, indentWithTab } from "@codemirror/commands";
import {
  Language,
  StreamLanguage,
  type StreamParser,
  defineLanguageFacet,
  indentUnit as indentUnitFacet,
  syntaxHighlighting,
} from "@codemirror/language";
import { Compartment, EditorState, type Extension } from "@codemirror/state";
import { EditorView, keymap, lineNumbers } from "@codemirror/view";
import { classHighlighter, tagHighlighter, tags } from "@lezer/highlight";
import { GFM, parser as markdownParser } from "@lezer/markdown";
import { useEffect, useEffectEvent, useRef, useState } from "react";

import type { CodeEditorProps } from "@/components/CodeEditor";
import type { CodeLanguage } from "@/lib/code-language";

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

// Each mode loads on demand, so a page only fetches the languages it shows.
const MODES: Record<Exclude<CodeLanguage, "markdown" | "plain">, () => Promise<StreamParser<unknown>>> = {
  shell: async () => (await import("@codemirror/legacy-modes/mode/shell")).shell,
  go: async () => (await import("@codemirror/legacy-modes/mode/go")).go,
  javascript: async () => (await import("@codemirror/legacy-modes/mode/javascript")).javascript,
  typescript: async () => (await import("@codemirror/legacy-modes/mode/javascript")).typescript,
  json: async () => (await import("@codemirror/legacy-modes/mode/javascript")).json,
  python: async () => (await import("@codemirror/legacy-modes/mode/python")).python,
  yaml: async () => (await import("@codemirror/legacy-modes/mode/yaml")).yaml,
  toml: async () => (await import("@codemirror/legacy-modes/mode/toml")).toml,
  css: async () => (await import("@codemirror/legacy-modes/mode/css")).css,
  scss: async () => (await import("@codemirror/legacy-modes/mode/css")).sCSS,
  less: async () => (await import("@codemirror/legacy-modes/mode/css")).less,
  xml: async () => (await import("@codemirror/legacy-modes/mode/xml")).xml,
  html: async () => (await import("@codemirror/legacy-modes/mode/xml")).html,
  rust: async () => (await import("@codemirror/legacy-modes/mode/rust")).rust,
  sql: async () => (await import("@codemirror/legacy-modes/mode/sql")).standardSQL,
  ruby: async () => (await import("@codemirror/legacy-modes/mode/ruby")).ruby,
  c: async () => (await import("@codemirror/legacy-modes/mode/clike")).c,
  cpp: async () => (await import("@codemirror/legacy-modes/mode/clike")).cpp,
  java: async () => (await import("@codemirror/legacy-modes/mode/clike")).java,
  csharp: async () => (await import("@codemirror/legacy-modes/mode/clike")).csharp,
  kotlin: async () => (await import("@codemirror/legacy-modes/mode/clike")).kotlin,
  scala: async () => (await import("@codemirror/legacy-modes/mode/clike")).scala,
  swift: async () => (await import("@codemirror/legacy-modes/mode/swift")).swift,
  dockerfile: async () => (await import("@codemirror/legacy-modes/mode/dockerfile")).dockerFile,
  properties: async () => (await import("@codemirror/legacy-modes/mode/properties")).properties,
  diff: async () => (await import("@codemirror/legacy-modes/mode/diff")).diff,
  lua: async () => (await import("@codemirror/legacy-modes/mode/lua")).lua,
  perl: async () => (await import("@codemirror/legacy-modes/mode/perl")).perl,
  nginx: async () => (await import("@codemirror/legacy-modes/mode/nginx")).nginx,
  protobuf: async () => (await import("@codemirror/legacy-modes/mode/protobuf")).protobuf,
  powershell: async () => (await import("@codemirror/legacy-modes/mode/powershell")).powerShell,
  cmake: async () => (await import("@codemirror/legacy-modes/mode/cmake")).cmake,
  haskell: async () => (await import("@codemirror/legacy-modes/mode/haskell")).haskell,
  erlang: async () => (await import("@codemirror/legacy-modes/mode/erlang")).erlang,
  r: async () => (await import("@codemirror/legacy-modes/mode/r")).r,
  groovy: async () => (await import("@codemirror/legacy-modes/mode/groovy")).groovy,
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

// layoutOf lays the text out as source code or as prose.
function layoutOf(code: boolean, wrap: boolean): Extension {
  return [
    code ? keymap.of([indentWithTab]) : [],
    code ? lineNumbers() : [],
    wrap ? EditorView.lineWrapping : [],
    EditorView.contentAttributes.of(
      code ? { spellcheck: "false", autocorrect: "off", autocapitalize: "off" } : { spellcheck: "true" },
    ),
  ];
}

// CodeEditorView is the CodeMirror half of CodeEditor, loaded on demand. The
// height of the empty editor is fixed once it mounts.
export default function CodeEditorView({
  id,
  label,
  value,
  onChange,
  lang,
  code = false,
  wrap = !code,
  indentUnit = "  ",
  tabSize = 4,
  minLines = 6,
  disabled = false,
  autoFocus = false,
  onFiles,
  "aria-invalid": invalid,
  "aria-describedby": describedBy,
}: CodeEditorProps) {
  const host = useRef<HTMLDivElement>(null);
  const view = useRef<EditorView | null>(null);
  const [attrs] = useState(() => new Compartment());
  const [editable] = useState(() => new Compartment());
  const [language] = useState(() => new Compartment());
  const [layout] = useState(() => new Compartment());
  const [indentation] = useState(() => new Compartment());
  const [initial] = useState(() => ({ value, minLines, autoFocus }));
  const emitChange = useEffectEvent((text: string) => onChange(text));
  // takeFiles hands over pasted or dropped files, and tells whether anyone
  // took them.
  const takeFiles = useEffectEvent((files: FileList) => {
    onFiles?.(files);
    return Boolean(onFiles);
  });

  useEffect(() => {
    const v = new EditorView({
      parent: host.current!,
      state: EditorState.create({
        doc: initial.value,
        extensions: [
          history(),
          layout.of([]),
          indentation.of([]),
          keymap.of([...defaultKeymap, ...historyKeymap]),
          language.of([]),
          syntaxHighlighting(classHighlighter),
          syntaxHighlighting(MARKDOWN_HIGHLIGHTER),
          THEME,
          EditorView.theme({ ".cm-content": { minHeight: `${initial.minLines * 1.5}em` } }),
          EditorView.domEventHandlers({
            // Dropping a file would otherwise insert its bytes as text.
            drop: (e) => {
              if (!e.dataTransfer?.files.length) return false;
              e.preventDefault();
              takeFiles(e.dataTransfer.files);
              return true;
            },
            paste: (e) => {
              if (!e.clipboardData?.files.length || !takeFiles(e.clipboardData.files)) return false;
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
  }, [initial, attrs, editable, language, layout, indentation]);

  // The language follows the name of the file being edited.
  useEffect(() => {
    const v = view.current;
    if (!v) return;
    if (lang === "markdown" || lang === "plain") {
      v.dispatch({ effects: language.reconfigure(lang === "markdown" ? markdownLanguage : []) });
      return;
    }
    let current = true;
    void MODES[lang]().then((mode) => {
      if (current && view.current === v) v.dispatch({ effects: language.reconfigure(StreamLanguage.define(mode)) });
    });
    return () => {
      current = false;
    };
  }, [language, lang]);

  useEffect(() => {
    view.current?.dispatch({ effects: layout.reconfigure(layoutOf(code, wrap)) });
  }, [layout, code, wrap]);

  useEffect(() => {
    view.current?.dispatch({
      effects: indentation.reconfigure([indentUnitFacet.of(indentUnit), EditorState.tabSize.of(tabSize)]),
    });
  }, [indentation, indentUnit, tabSize]);

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
