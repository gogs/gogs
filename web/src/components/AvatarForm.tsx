import { useMutation } from "@tanstack/react-query";
import { type FormEvent, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";

import { FormField } from "@/components/FormField";
import { Button } from "@/components/ui/button";

// AvatarForm shows an avatar and uploads a new image for it, or deletes the
// uploaded one. onChanged runs after either, with whether an image was
// uploaded.
export function AvatarForm({
  avatarURL,
  hasCustomAvatar,
  upload,
  remove,
  onChanged,
}: {
  avatarURL: string;
  hasCustomAvatar: boolean;
  upload: (file: File) => Promise<void>;
  remove: () => Promise<void>;
  onChanged: (uploaded: boolean) => Promise<void> | void;
}) {
  const { t } = useTranslation();
  const [file, setFile] = useState<File | null>(null);
  const inputRef = useRef<HTMLInputElement>(null);
  // The avatar keeps its URL when it changes, so refetch it after a change.
  const [version, setVersion] = useState(0);
  const changed = async (uploaded: boolean) => {
    setFile(null);
    if (inputRef.current) inputRef.current.value = "";
    setVersion((v) => v + 1);
    await onChanged(uploaded);
  };
  const uploading = useMutation({
    mutationFn: () => upload(file as File),
    onSuccess: () => changed(true),
    onError: (err) => toast.error(err.message),
  });
  const removing = useMutation({
    mutationFn: remove,
    onSuccess: () => changed(false),
    onError: (err) => toast.error(err.message),
  });
  const pending = uploading.isPending || removing.isPending;
  const src = version ? `${avatarURL}${avatarURL.includes("?") ? "&" : "?"}v=${version}` : avatarURL;

  const onSubmit = (e: FormEvent) => {
    e.preventDefault();
    if (file) uploading.mutate();
  };

  return (
    <form onSubmit={onSubmit}>
      <fieldset disabled={pending} className="contents">
        <div className="flex flex-wrap items-start gap-4">
          <img src={src} alt="" className="size-20 shrink-0 rounded-md border border-(--color-border) object-cover" />
          <div className="flex min-w-0 flex-1 basis-56 flex-col gap-3">
            <FormField id="avatar" label={t("settings.choose_new_avatar")} invalid={false}>
              <input
                ref={inputRef}
                id="avatar"
                name="avatar"
                type="file"
                accept="image/png,image/jpeg,image/gif"
                onChange={(e) => setFile(e.target.files?.[0] ?? null)}
                className="block w-full min-w-0 text-sm text-(--color-muted-foreground) file:mr-3 file:cursor-pointer file:rounded-md file:border file:border-(--color-input) file:bg-(--color-background) file:px-3 file:py-1.5 file:text-sm file:font-medium file:text-(--color-foreground) hover:file:bg-(--color-surface)"
              />
            </FormField>
            <div className="flex flex-wrap gap-2">
              <Button type="submit" variant="outline" size="sm" disabled={!file || pending}>
                {t("settings.update_avatar")}
              </Button>
              {hasCustomAvatar ? (
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  className="text-(--color-destructive)"
                  onClick={() => removing.mutate()}
                >
                  {t("settings.delete_current_avatar")}
                </Button>
              ) : null}
            </div>
          </div>
        </div>
      </fieldset>
    </form>
  );
}
