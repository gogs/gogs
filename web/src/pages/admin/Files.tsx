import { useMutation } from "@tanstack/react-query";
import { useRouter } from "@tanstack/react-router";
import { Copy, LoaderCircle, Upload } from "lucide-react";
import { type ChangeEvent, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";

import { AdminPageHeader } from "@/components/AdminPage";
import { ConfirmButton } from "@/components/ConfirmButton";
import { FileIcon } from "@/components/FileIcon";
import { RelativeTime } from "@/components/RelativeTime";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { formatBytes } from "@/lib/format";
import { type AdminCustomFile, deleteAdminCustomFile, uploadAdminCustomFile } from "@/lib/queries/admin";
import { toastError } from "@/lib/toast-error";

// MAX_FILE_SIZE is the largest file the server takes. Checking it before the
// upload tells why a file is refused even behind a proxy that cuts the
// connection when the server stops reading.
const MAX_FILE_SIZE = 10 << 20;

// AdminFiles lists the static files in custom/public, which the site serves
// in place of built-in ones with the same path, and which components can show,
// e.g., a logo on the landing page.
export function AdminFiles({ files }: { files: AdminCustomFile[] }) {
  const { t } = useTranslation();
  const router = useRouter();
  const input = useRef<HTMLInputElement>(null);
  const [folder, setFolder] = useState("img");

  const upload = useMutation({
    // A file uploaded to the path of another replaces it.
    mutationFn: async (selected: File[]) => {
      const tooLarge = selected.find((file) => file.size > MAX_FILE_SIZE);
      if (tooLarge)
        throw new Error(t("admin.files_too_large", { name: tooLarge.name, max: formatBytes(MAX_FILE_SIZE) }));
      for (const file of selected) {
        const dir = folder.replace(/^\/+|\/+$/g, "");
        await uploadAdminCustomFile(file, dir ? `${dir}/${file.name}` : file.name);
      }
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

  const copy = (path: string) => {
    const code = `fileURL(${JSON.stringify(path)})`;
    void navigator.clipboard.writeText(code).then(
      () => toast.success(t("admin.files_copied", { code })),
      (err: unknown) => toastError(err instanceof Error ? err : new Error(String(err))),
    );
  };

  return (
    <div className="grid gap-6">
      <AdminPageHeader title={t("admin.files")} description={t("admin.files_desc")}>
        <div className="flex items-center gap-2">
          <Label htmlFor="files-folder" className="shrink-0 text-sm">
            {t("admin.files_folder")}
          </Label>
          <Input
            id="files-folder"
            value={folder}
            onChange={(e) => setFolder(e.currentTarget.value)}
            placeholder="img"
            className="w-32 font-mono"
          />
        </div>
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
            <li key={file.path} className="flex flex-wrap items-center gap-3 px-4 py-3">
              {/* The path keeps room for itself, so on narrow screens the buttons go below it. */}
              <div className="flex min-w-0 flex-1 basis-64 items-center gap-3">
                <FileIcon name={file.path.slice(file.path.lastIndexOf("/") + 1)} className="size-5" />
                <div className="min-w-0 flex-1">
                  <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
                    <Tooltip>
                      <TooltipTrigger asChild>
                        <a
                          href={file.url}
                          target="_blank"
                          rel="noopener noreferrer"
                          className="rounded-sm font-mono text-sm break-all outline-none hover:underline focus-visible:ring-1 focus-visible:ring-(--color-ring)"
                        >
                          {file.path}
                        </a>
                      </TooltipTrigger>
                      <TooltipContent>{t("admin.files_view_raw")}</TooltipContent>
                    </Tooltip>
                    {file.replacesBuiltin ? (
                      <span className="rounded-full border border-(--color-primary) px-2 text-xs/5 font-medium text-(--color-primary)">
                        {t("admin.files_replaces_builtin")}
                      </span>
                    ) : null}
                  </div>
                  <div className="text-sm text-(--color-muted-foreground)">
                    {formatBytes(file.size)} · <RelativeTime time={file.modifiedAt} />
                  </div>
                </div>
              </div>
              <div className="flex flex-wrap gap-2">
                <Button
                  variant="outline"
                  size="sm"
                  aria-label={t("admin.files_copy_aria", { name: file.path })}
                  onClick={() => copy(file.path)}
                >
                  <Copy className="size-4" aria-hidden />
                  {t("admin.files_copy")}
                </Button>
                {file.editable ? (
                  <ConfirmButton
                    label={t("admin.files_delete")}
                    ariaLabel={t("admin.files_delete_aria", { name: file.path })}
                    title={t("admin.files_delete_title", { name: file.path })}
                    description={t(
                      file.replacesBuiltin ? "admin.files_delete_builtin_desc" : "admin.files_delete_desc",
                    )}
                    size="sm"
                    action={() => deleteAdminCustomFile(file.path)}
                    onDone={() => router.invalidate()}
                  />
                ) : null}
              </div>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
