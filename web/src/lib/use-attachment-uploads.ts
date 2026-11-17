import { useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";

import { type IssueAttachmentSettings, uploadRepoAttachment } from "@/lib/queries/repo";

export interface PendingFile {
  key: string;
  name: string;
  // uuid is set once the file is uploaded.
  uuid?: string;
}

// UploadLinker places the files being uploaded in content, e.g., a Markdown
// editor that links them where they are dropped like on GitHub.
export interface UploadLinker {
  start: (file: File) => void;
  // done gets the URL of the uploaded file, or nothing when it failed.
  done: (file: File, url?: string) => void;
}

// useAttachmentUploads uploads the files picked to attach to an issue, a
// comment or a release of the repository, or to commit with the editor, as
// soon as they are picked, within the limits of the settings. When a linker
// is set, it places the files in content.
export function useAttachmentUploads(
  owner: string,
  name: string,
  settings?: IssueAttachmentSettings,
  kind: "issues" | "releases" | "editor" = "issues",
) {
  const { t } = useTranslation();
  const [files, setFiles] = useState<PendingFile[]>([]);
  const linker = useRef<UploadLinker | null>(null);
  const uploaded = files.flatMap((f) => (f.uuid ? [f.uuid] : []));

  const add = (list: FileList | File[]) => {
    if (!settings) return;
    const room = settings.maxFiles - files.length;
    for (const file of Array.from(list).slice(0, Math.max(room, 0))) {
      if (file.size > settings.maxSize) {
        toast.error(
          t("dropzone.file_too_big", {
            filesize: (file.size / (1 << 20)).toFixed(1),
            maxFilesize: settings.maxSize / (1 << 20),
          }),
        );
        continue;
      }
      const key = crypto.randomUUID();
      const link = linker.current;
      setFiles((prev) => [...prev, { key, name: file.name }]);
      link?.start(file);
      uploadRepoAttachment(owner, name, kind, file).then(
        ({ uuid, url }) => {
          setFiles((prev) => prev.map((f) => (f.key === key ? { ...f, uuid } : f)));
          link?.done(file, url);
        },
        (err: Error) => {
          toast.error(`${file.name}: ${err.message}`);
          setFiles((prev) => prev.filter((f) => f.key !== key));
          link?.done(file);
        },
      );
    }
  };

  return {
    settings,
    files,
    uploaded,
    uploading: files.length !== uploaded.length,
    add,
    setLinker: (l: UploadLinker | null) => {
      linker.current = l;
    },
    remove: (key: string) => setFiles((prev) => prev.filter((f) => f.key !== key)),
    clear: () => setFiles([]),
  };
}

export type AttachmentUploads = ReturnType<typeof useAttachmentUploads>;

// linkedUploads returns the uploaded files the content links to, so files
// whose links were removed before submitting are not attached.
export function linkedUploads(uploaded: string[], content: string) {
  return uploaded.filter((uuid) => content.includes(`/attachments/${uuid}`));
}
