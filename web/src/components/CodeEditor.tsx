import { type AriaAttributes, Suspense, lazy } from "react";

import { cn } from "@/lib/utils";

const CodeEditorView = lazy(() => import("@/components/CodeEditorView"));

export interface CodeEditorProps extends Pick<AriaAttributes, "aria-invalid" | "aria-describedby"> {
  // id goes on the editable area, so focusing the field by its ID works.
  id: string;
  // label names the editor for assistive tech, since a <label> cannot point
  // at an editable area that is not a form control.
  label: string;
  value: string;
  onChange: (value: string) => void;
  lang: "markdown" | "shell";
  // code lays the text out as source code: line numbers, long lines scroll
  // instead of wrapping, and Tab indents. Escape then Tab leaves the editor.
  code?: boolean;
  // minLines is the height of the empty editor. It grows with its content.
  minLines?: number;
  disabled?: boolean;
  autoFocus?: boolean;
}

// CodeEditor is the editor for every Markdown and source code field. Its
// highlighting follows the theme. CodeMirror loads with the first editor on a
// page, so pages without one do not pay for it.
export function CodeEditor(props: CodeEditorProps) {
  const { minLines = 6, disabled } = props;
  return (
    <div
      className={cn(
        "overflow-hidden rounded-md border border-(--color-input) bg-(--color-background) text-sm text-(--color-foreground) transition-colors focus-within:border-(--color-ring) focus-within:ring-1 focus-within:ring-(--color-ring)",
        props["aria-invalid"] && "border-(--color-destructive)",
        disabled && "cursor-not-allowed opacity-60",
      )}
    >
      <Suspense fallback={<div style={{ minHeight: `calc(${minLines * 1.5}em + 1rem)` }} />}>
        <CodeEditorView {...props} minLines={minLines} />
      </Suspense>
    </div>
  );
}
