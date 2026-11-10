import { useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";

import { type IssueAttachmentSettings, uploadRepoIssueAttachment } from "@/lib/queries/repo";

export interface PendingFile {
  key: string;
  name: string;
  // uuid is set once the file is uploaded.
  uuid?: string;
}

// useAttachmentUploads uploads the files picked to attach to an issue or a
// comment of the repository as soon as they are picked, within the limits of
// the settings.
export function useAttachmentUploads(owner: string, name: string, settings?: IssueAttachmentSettings) {
  const { t } = useTranslation();
  const [files, setFiles] = useState<PendingFile[]>([]);
  const uploaded = files.flatMap((f) => (f.uuid ? [f.uuid] : []));

  const add = (list: FileList) => {
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
      setFiles((prev) => [...prev, { key, name: file.name }]);
      uploadRepoIssueAttachment(owner, name, file).then(
        (uuid) => setFiles((prev) => prev.map((f) => (f.key === key ? { ...f, uuid } : f))),
        (err: Error) => {
          toast.error(`${file.name}: ${err.message}`);
          setFiles((prev) => prev.filter((f) => f.key !== key));
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
    remove: (key: string) => setFiles((prev) => prev.filter((f) => f.key !== key)),
    clear: () => setFiles([]),
  };
}

export type AttachmentUploads = ReturnType<typeof useAttachmentUploads>;
