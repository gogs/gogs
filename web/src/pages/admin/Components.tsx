import { useMutation } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { ChevronRight } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";

import { AdminPageHeader } from "@/components/AdminPage";
import { CodeEditor } from "@/components/CodeEditor";
import { FormAlert } from "@/components/FormField";
import { Button } from "@/components/ui/button";
import { webContext } from "@/lib/context";
import { PREVIEW_PATH, type PreviewMessage, type PreviewRequest, type SlotName } from "@/lib/customization";
import { type AdminCustomPart, publishAdminCustomPart } from "@/lib/queries/admin";
import { toastError } from "@/lib/toast-error";
import { subUrl } from "@/lib/url";
import { useDebounced } from "@/lib/use-debounced";
import { cn } from "@/lib/utils";

const PART_LABELS: Record<SlotName, { label: string; description: string }> = {
  landing: { label: "admin.customize_landing", description: "admin.customize_landing_desc" },
  navbar: { label: "admin.customize_navbar", description: "admin.customize_navbar_desc" },
  footer: { label: "admin.customize_footer", description: "admin.customize_footer_desc" },
};

// STARTERS are the scripts the editor starts with for a part that is built-in.
// The default export of a script is a function that takes the API (React,
// html, Link, Button, t, subUrl, fileURL, useUser) and returns a React component, or a
// web component, i.e., a class that extends HTMLElement. A component gets
// "Default", the built-in part, and "user". An optional "css" export adds
// styles to the site.
const STARTERS: Record<SlotName, string> = {
  landing: `// Upload a logo to "img" under Customization > Files and show it with
// <img src=\${fileURL("img/logo.png")} alt="Acme" />.
export default ({ html, Link, t }) =>
  function Landing() {
    return html\`
      <main className="flex flex-1 flex-col items-center justify-center gap-4 px-4 py-16 text-center">
        <h1 className="acme-title">Hello world</h1>
        <\${Link} to="/explore/repos" className="text-(--color-primary) hover:underline">
          \${t("explore")} →
        <//>
      </main>\`;
  };

export const css = \`
  .acme-title { font-size: 2.5rem; font-weight: 700; color: var(--color-foreground); }
\`;
`,
  navbar: `// Wraps the built-in navbar with notices above and below it, in the look
// of the built-in notice banner. Its classes follow light, dark and glass mode.
export default ({ React, html }) => {
  const warningIcon = html\`
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round"
      strokeLinejoin="round" aria-hidden="true" className="mt-0.5 size-4 shrink-0 text-(--color-warning)">
      <path d="m21.73 18-8-14a2 2 0 0 0-3.48 0l-8 14A2 2 0 0 0 4 21h16a2 2 0 0 0 1.73-3" />
      <path d="M12 9v4" /><path d="M12 17h.01" />
    </svg>\`;

  function Notice({ onDismiss, children }) {
    return html\`
      <div role="note" className="glass-bar border-b border-(--color-warning)/40">
        <div className="bg-(--color-warning)/10">
          <div className="mx-auto flex max-w-6xl items-start gap-2 px-4 py-2.5 text-sm sm:px-6">
            \${warningIcon}
            <div className="min-w-0 flex-1">\${children}</div>
            \${onDismiss
              ? html\`<button type="button" aria-label="Dismiss" onClick=\${onDismiss} className="acme-dismiss">✕</button>\`
              : null}
          </div>
        </div>
      </div>\`;
  }

  // A notice that stays hidden in this browser once dismissed. Give it a new
  // id to show it again, e.g., for the next announcement.
  function DismissibleNotice({ id, children }) {
    const key = \`acme-notice-dismissed:\${id}\`;
    const [hidden, setHidden] = React.useState(() => localStorage.getItem(key) === "1");
    if (hidden) return null;
    const dismiss = () => {
      localStorage.setItem(key, "1");
      setHidden(true);
    };
    return html\`<\${Notice} onDismiss=\${dismiss}>\${children}<//>\`;
  }

  return function Navbar({ Default }) {
    return html\`
      <\${Notice}>Acme internal code hosting. Do not push secrets.<//>
      <\${Default} />
      <\${Notice}><strong>Scheduled maintenance</strong> on Saturday from 10:00 to 12:00 UTC.<//>
      <\${DismissibleNotice} id="2026-12-ci-runners">
        New CI runners are available. <a href="https://gogs.io" className="underline">Learn more</a>
      <//>\`;
  };
};

export const css = \`
  .acme-dismiss { display: inline-flex; width: 1.5rem; height: 1.5rem; flex-shrink: 0; align-items: center;
    justify-content: center; border-radius: 0.375rem; cursor: pointer; color: var(--color-muted-foreground); }
  .acme-dismiss:hover { background: var(--color-surface); color: var(--color-foreground); }
  .acme-dismiss:focus-visible { outline: 2px solid var(--color-ring); outline-offset: 1px; }
\`;
`,
  footer: `// A web component, which can use any framework or none.
export default class extends HTMLElement {
  connectedCallback() {
    this.innerHTML = \`<footer class="acme-footer">© \${new Date().getFullYear()} Acme Inc.</footer>\`;
  }
}

export const css = \`
  .acme-footer { margin-top: 2rem; padding: 1rem; text-align: center; font-size: 0.875rem;
    color: var(--color-muted-foreground); border-top: 1px solid var(--color-border); }
\`;
`,
};

// A row of toggle buttons, styled like Segmented of the admin console.
const SEGMENT =
  "inline-flex h-8 cursor-pointer items-center rounded-md border border-transparent px-3 text-sm font-medium whitespace-nowrap text-(--color-muted-foreground) outline-none hover:text-(--color-foreground) focus-visible:ring-1 focus-visible:ring-(--color-ring) aria-pressed:border-(--color-border) aria-pressed:bg-(--color-background) aria-pressed:text-(--color-foreground) aria-pressed:shadow-xs glass-ui:h-8.5 glass-ui:focus-visible:ring-inset glass-ui:aria-pressed:bg-(--color-surface) glass-ui:aria-pressed:font-semibold glass-ui:aria-pressed:shadow-none";

function Toggle<T extends string>({
  label,
  value,
  options,
  onChange,
}: {
  label: string;
  value: T;
  options: { value: T; label: string }[];
  onChange: (value: T) => void;
}) {
  return (
    <div
      role="group"
      aria-label={label}
      className="glass-control glass-tabs inline-flex max-w-full flex-wrap items-center gap-0.5 rounded-lg bg-(--color-surface) p-0.5"
    >
      {options.map((option) => (
        <button
          key={option.value}
          type="button"
          aria-pressed={value === option.value}
          onClick={() => onChange(option.value)}
          className={SEGMENT}
        >
          {option.label}
        </button>
      ))}
    </div>
  );
}

// AdminComponents lists the parts of the site that admins can replace.
export function AdminComponents({ parts }: { parts: AdminCustomPart[] }) {
  const { t } = useTranslation();
  return (
    <div className="grid gap-6">
      <AdminPageHeader title={t("admin.components")} description={t("admin.components_desc")} />
      <ul className="glass-list grid divide-y divide-(--color-border) rounded-md border border-(--color-border)">
        {parts.map((part) => {
          const labels = PART_LABELS[part.name as SlotName];
          return (
            <li key={part.name}>
              <Link
                to="/admin/components/$part"
                params={{ part: part.name }}
                className="flex items-center gap-3 px-4 py-3 outline-none hover:bg-(--color-surface) focus-visible:ring-1 focus-visible:ring-(--color-ring) focus-visible:ring-inset"
              >
                <div className="min-w-0 flex-1">
                  <div className="font-medium">{t(labels.label)}</div>
                  <div className="text-sm text-(--color-muted-foreground)">{t(labels.description)}</div>
                </div>
                <span
                  className={cn(
                    "rounded-full border px-2 text-xs/5 font-medium",
                    part.source
                      ? "border-(--color-primary) text-(--color-primary)"
                      : "border-(--color-border) text-(--color-muted-foreground)",
                  )}
                >
                  {part.source ? t("admin.customize_customized") : t("admin.customize_builtin")}
                </span>
                <ChevronRight className="size-4 shrink-0 text-(--color-muted-foreground)" aria-hidden />
              </Link>
            </li>
          );
        })}
      </ul>
    </div>
  );
}

// AdminComponentEdit edits the script of one part, with a preview of the site
// that has the draft in place of the published part.
export function AdminComponentEdit({ name, parts }: { name: SlotName; parts: AdminCustomPart[] }) {
  const { t } = useTranslation();
  const labels = PART_LABELS[name];
  const [sources, setSources] = useState(() => Object.fromEntries(parts.map((part) => [part.name, part.source])));
  const published = sources[name] ?? "";
  const [source, setSource] = useState(published || STARTERS[name]);
  const [viewer, setViewer] = useState<PreviewRequest["viewer"]>("visitor");
  const [width, setWidth] = useState<"desktop" | "mobile">("desktop");
  const draft = useDebounced(source, 400);
  // The error of the draft the preview shows, by the draft it belongs to.
  const [error, setError] = useState<{ draft: string; message: string } | null>(null);
  const frame = useRef<HTMLIFrameElement>(null);
  // A new frame for every draft and viewer, so what a draft leaves behind
  // goes with its frame.
  const frameKey = `${draft.length}:${hash(draft)}:${viewer}`;

  useEffect(() => {
    const onMessage = (event: MessageEvent) => {
      if (event.origin !== window.location.origin || event.source !== frame.current?.contentWindow) return;
      const message = event.data as PreviewMessage;
      if (message.type === "gogs-preview-ready") {
        const request: PreviewRequest = {
          type: "gogs-preview-render",
          sources: { ...sources, [name]: draft },
          part: name,
          viewer,
        };
        frame.current?.contentWindow?.postMessage(request, window.location.origin);
      } else if (message.type === "gogs-preview-result") {
        const message_ = message.errors[name];
        setError(message_ ? { draft, message: message_ } : null);
      } else if (message.type === "gogs-preview-render-error" && message.part === name) {
        setError({ draft, message: message.message });
      }
    };
    window.addEventListener("message", onMessage);
    return () => window.removeEventListener("message", onMessage);
  }, [sources, name, draft, viewer]);

  const publish = useMutation({
    mutationFn: (next: string) => publishAdminCustomPart(name, next),
    onSuccess: (resp) => {
      setSources((prev) => ({ ...prev, [name]: resp.source }));
      if (resp.source) {
        toast.success(t("admin.customize_published_toast"));
      } else {
        setSource(STARTERS[name]);
        toast.success(t("admin.customize_reset_toast"));
      }
    },
    onError: (err) => toastError(err),
  });

  const shownError = error?.draft === source ? error.message : null;
  const checked = draft === source;
  const dirty = source !== published;
  const status = !published
    ? t("admin.customize_not_published")
    : dirty
      ? t("admin.customize_unpublished")
      : t("admin.customize_published");

  return (
    <div className="grid gap-6">
      <AdminPageHeader
        title={t(labels.label)}
        description={t(labels.description)}
        back={{ to: "/admin/components", label: t("admin.components") }}
        crumb={t(labels.label)}
      >
        <span className="text-sm text-(--color-muted-foreground)" role="status">
          {status}
        </span>
        {published ? (
          <Button variant="outline" disabled={publish.isPending} onClick={() => publish.mutate("")}>
            {t("admin.customize_reset")}
          </Button>
        ) : null}
        <Button
          variant="outline"
          disabled={source === (published || STARTERS[name])}
          onClick={() => setSource(published || STARTERS[name])}
        >
          {t("admin.customize_discard")}
        </Button>
        <Button
          disabled={!dirty || !checked || shownError !== null || publish.isPending}
          onClick={() => publish.mutate(source)}
        >
          {t("admin.customize_publish")}
        </Button>
      </AdminPageHeader>

      <div className="grid min-w-0 gap-6 2xl:grid-cols-[minmax(0,1fr)_minmax(0,1.2fr)]">
        <section className="grid min-w-0 content-start gap-2">
          <h2 className="text-sm font-semibold">{t("admin.customize_script")}</h2>
          <CodeEditor
            id="customize-script"
            label={t("admin.customize_script")}
            value={source}
            onChange={setSource}
            lang="javascript"
            code
            minLines={24}
          />
          {name === "navbar" && webContext.notice ? (
            <p className="text-sm text-(--color-muted-foreground)">{t("admin.customize_navbar_legacy")}</p>
          ) : null}
        </section>

        <section className="grid min-w-0 content-start gap-2">
          <div className="flex flex-wrap items-center justify-between gap-2">
            <h2 className="text-sm font-semibold">{t("admin.customize_preview")}</h2>
            <div className="flex flex-wrap gap-2">
              {name === "landing" ? null : (
                <Toggle
                  label={t("admin.customize_viewer")}
                  value={viewer}
                  onChange={setViewer}
                  options={[
                    { value: "visitor", label: t("admin.customize_unauthenticated") },
                    { value: "user", label: t("admin.customize_authenticated") },
                  ]}
                />
              )}
              <Toggle
                label={t("admin.customize_width")}
                value={width}
                onChange={setWidth}
                options={[
                  { value: "desktop", label: t("admin.customize_desktop") },
                  { value: "mobile", label: t("admin.customize_mobile") },
                ]}
              />
            </div>
          </div>
          {shownError ? (
            <FormAlert
              message={
                <>
                  <span className="block font-mono text-xs break-words whitespace-pre-wrap">{shownError}</span>
                  <span className="mt-1 block">{t("admin.customize_error")}</span>
                </>
              }
            />
          ) : null}
          <div className="overflow-x-auto rounded-md border border-(--color-border) bg-(--color-surface)">
            <iframe
              key={frameKey}
              ref={frame}
              title={t("admin.customize_preview_title", { part: t(labels.label) })}
              src={subUrl(PREVIEW_PATH)}
              className={cn(
                "mx-auto block h-[40rem] bg-(--color-background)",
                width === "mobile" ? "w-[375px] border-x border-(--color-border)" : "w-full min-w-[64rem]",
              )}
            />
          </div>
        </section>
      </div>
    </div>
  );
}

// hash tells drafts of the same length apart for the key of the preview frame.
function hash(text: string) {
  let h = 0;
  for (let i = 0; i < text.length; i++) h = (Math.imul(31, h) + text.charCodeAt(i)) | 0;
  return h;
}
