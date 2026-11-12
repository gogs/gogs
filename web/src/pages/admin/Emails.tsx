import { useNavigate } from "@tanstack/react-router";
import type { ReactNode } from "react";
import { useTranslation } from "react-i18next";

import { AdminLayout } from "@/components/AdminLayout";
import { FormAlert, FormField } from "@/components/FormField";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import type { AdminEmailPreview } from "@/lib/queries/admin";

function Part({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section className="mb-6">
      <h2 className="mb-2 text-sm font-semibold">{title}</h2>
      {children}
    </section>
  );
}

// fitToContent sizes the frame of an email to the email, so the page scrolls
// instead of the frame.
function fitToContent(frame: HTMLIFrameElement) {
  const doc = frame.contentDocument;
  if (doc) frame.style.height = `${doc.documentElement.scrollHeight}px`;
}

// AdminEmails previews the emails the server sends, in any language.
export function AdminEmails({ preview }: { preview: AdminEmailPreview }) {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const show = (search: { name?: string; language?: string }) =>
    void navigate({ to: "/admin/emails", search: { name: preview.name, language: preview.lang, ...search } });

  return (
    <AdminLayout active="emails" wide>
      <h1 className="text-xl font-semibold">{t("admin.emails")}</h1>
      <p className="mt-1 mb-4 text-sm text-(--color-muted-foreground)">{t("admin.emails.desc")}</p>
      <div className="mb-6 grid gap-4 sm:grid-cols-2">
        <FormField id="email-template" label={t("admin.emails.template")} invalid={false}>
          <Select value={preview.name} onValueChange={(name) => show({ name })}>
            <SelectTrigger id="email-template" className="font-mono">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {preview.names.map((name) => (
                <SelectItem key={name} value={name} className="font-mono">
                  {name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </FormField>
        <FormField id="email-language" label={t("admin.emails.language")} invalid={false}>
          <Select value={preview.lang} onValueChange={(language) => show({ language })}>
            <SelectTrigger id="email-language">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {preview.langs.map((lang) => (
                <SelectItem key={lang.lang} value={lang.lang}>
                  {lang.name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </FormField>
      </div>

      {preview.error ? (
        <FormAlert message={t("admin.emails.compose_failed").replace(/%(?:\[1\])?s/, preview.error)} />
      ) : (
        <>
          <Part title={t("admin.emails.subject")}>
            <p className="text-sm wrap-anywhere">{preview.subject}</p>
          </Part>
          <Part title={t("admin.emails.html")}>
            {/* Without allow-scripts the email cannot run code, and allow-same-origin lets the page measure it. */}
            <iframe
              key={`${preview.name}/${preview.lang}`}
              title={t("admin.emails.html_preview")}
              sandbox="allow-same-origin"
              srcDoc={preview.html}
              onLoad={(e) => fitToContent(e.currentTarget)}
              className="block h-96 w-full rounded-md border border-(--color-border) bg-white"
            />
          </Part>
          <Part title={t("admin.emails.text")}>
            <pre className="overflow-x-auto rounded-md border border-(--color-border) bg-(--color-muted) p-3 font-mono text-xs leading-5 whitespace-pre-wrap wrap-anywhere">
              {preview.text}
            </pre>
          </Part>
        </>
      )}
    </AdminLayout>
  );
}
