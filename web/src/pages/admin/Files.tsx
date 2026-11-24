import { useMutation } from "@tanstack/react-query";
import { useRouter } from "@tanstack/react-router";
import { Copy, File as FileIcon, LoaderCircle, Upload } from "lucide-react";
import { type ChangeEvent, useRef } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";

import { AdminPageHeader } from "@/components/AdminPage";
import { ConfirmButton } from "@/components/ConfirmButton";
import { InlineCode } from "@/components/InlineCode";
import { RelativeTime } from "@/components/RelativeTime";
import { Button } from "@/components/ui/button";
import { formatBytes } from "@/lib/format";
import { type AdminCustomFile, deleteAdminCustomFile, uploadAdminCustomFile } from "@/lib/queries/admin";
import { toastError } from "@/lib/toast-error";

// MAX_FILE_SIZE is the largest file the server takes. Checking it before the
// upload tells why a file is refused even behind a proxy that cuts the
// connection when the server stops reading.
const MAX_FILE_SIZE = 10 << 20;

const IMAGE = /\.(png|jpe?g|gif|webp|avif|svg|ico)$/i;

// AdminFiles lists the files that admins upload for components to show, e.g.,
// a logo on the landing page.
export function AdminFiles({ files }: { files: AdminCustomFile[] }) {
  const { t } = useTranslation();
  const router = useRouter();
  const input = useRef<HTMLInputElement>(null);

  const upload = useMutation({
    mutationFn: async (selected: File[]) => {
      const tooLarge = selected.find((file) => file.size > MAX_FILE_SIZE);
      if (tooLarge)
        throw new Error(t("admin.files_too_large", { name: tooLarge.name, max: formatBytes(MAX_FILE_SIZE) }));
      for (const file of selected) await uploadAdminCustomFile(file);
      return selected.length;
    },
    onSuccess: async (count) => {
      toast.success(t("admin.files_uploaded", { count }));
      await router.invalidate();
    },
    onError: async (err) => {
      toastError(err);
      // Files before the one that failed were uploaded.
      await router.invalidate();
    },
  });

  const onSelect = (e: ChangeEvent<HTMLInputElement>) => {
    const selected = Array.from(e.currentTarget.files ?? []);
    e.currentTarget.value = "";
    if (selected.length > 0) upload.mutate(selected);
  };

  const copy = (name: string) => {
    void navigator.clipboard.writeText(`fileURL(${JSON.stringify(name)})`).then(
      () => toast.success(t("admin.files_copied")),
      (err: unknown) => toastError(err instanceof Error ? err : new Error(String(err))),
    );
  };

  return (
    <div className="grid gap-6">
      <AdminPageHeader title={t("admin.files")} description={t("admin.files_desc")}>
        <input ref={input} type="file" multiple hidden onChange={onSelect} />
        <Button disabled={upload.isPending} onClick={() => input.current?.click()}>
          {upload.isPending ? (
            <LoaderCircle className="size-4 animate-spin" aria-hidden />
          ) : (
            <Upload className="size-4" aria-hidden />
          )}
          {t("admin.files_upload")}
        </Button>
      </AdminPageHeader>

      {files.length === 0 ? (
        <p className="rounded-md border border-dashed border-(--color-border) p-10 text-center text-sm text-(--color-muted-foreground)">
          {t("admin.files_empty")}
        </p>
      ) : (
        <ul className="glass-list grid divide-y divide-(--color-border) rounded-md border border-(--color-border)">
          {files.map((file) => (
            <li key={file.name} className="flex flex-wrap items-center gap-3 px-4 py-3">
              {/* The name keeps room for itself, so on narrow screens the buttons go below it. */}
              <div className="flex min-w-0 flex-1 basis-64 items-center gap-3">
                <a
                  href={file.url}
                  target="_blank"
                  rel="noopener noreferrer"
                  aria-label={file.name}
                  className="flex size-12 shrink-0 items-center justify-center overflow-hidden rounded-md border border-(--color-border) bg-(--color-surface)"
                >
                  {IMAGE.test(file.name) ? (
                    <img
                      src={`${file.url}?v=${encodeURIComponent(file.modifiedAt)}`}
                      alt=""
                      className="max-h-full max-w-full object-contain"
                    />
                  ) : (
                    <FileIcon className="size-5 text-(--color-muted-foreground)" aria-hidden />
                  )}
                </a>
                <div className="min-w-0 flex-1">
                  <a
                    href={file.url}
                    target="_blank"
                    rel="noopener noreferrer"
                    className="font-mono text-sm break-all hover:underline"
                  >
                    {file.name}
                  </a>
                  <div className="text-sm text-(--color-muted-foreground)">
                    {formatBytes(file.size)} · <RelativeTime time={file.modifiedAt} />
                  </div>
                </div>
              </div>
              <div className="flex gap-2">
                <Button
                  variant="outline"
                  size="sm"
                  aria-label={t("admin.files_copy_aria", { name: file.name })}
                  onClick={() => copy(file.name)}
                >
                  <Copy className="size-4" aria-hidden />
                  <InlineCode>fileURL</InlineCode>
                </Button>
                <ConfirmButton
                  label={t("admin.files_delete")}
                  ariaLabel={t("admin.files_delete_aria", { name: file.name })}
                  title={t("admin.files_delete_title", { name: file.name })}
                  description={t("admin.files_delete_desc")}
                  size="sm"
                  action={() => deleteAdminCustomFile(file.name)}
                  onDone={() => router.invalidate()}
                />
              </div>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
