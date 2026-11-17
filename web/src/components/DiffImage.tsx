import { useState } from "react";
import { useTranslation } from "react-i18next";

interface Size {
  width: number;
  height: number;
}

// Pierre unmounts annotations that scroll out of view and measures them again
// when they return. Remembering each image's size lets a remount reserve the
// final height before the image decodes, so the layout does not jump.
const knownSizes = new Map<string, Size>();

// DiffImage shows one side of an image diff. Pierre renders the old side in
// the deletions column and the new side in the additions column.
export function DiffImage({ src, side }: { src: string; side: "deletions" | "additions" }) {
  const { t } = useTranslation();
  const [size, setSize] = useState(() => knownSizes.get(src));
  const [failed, setFailed] = useState(false);
  const label = side === "deletions" ? t("repo.diff.image_before") : t("repo.diff.image_after");

  return (
    <figure className="m-0 flex flex-col items-center gap-2 px-4 py-3 font-sans">
      <figcaption className="text-(--color-muted-foreground) text-xs">
        <span className={side === "deletions" ? "text-(--color-destructive)" : "text-(--color-success)"}>{label}</span>
        {size ? ` · ${t("repo.diff.image_dimensions", { width: size.width, height: size.height })}` : null}
      </figcaption>
      {failed ? (
        <p className="text-(--color-muted-foreground) text-sm">{t("repo.diff.image_unavailable")}</p>
      ) : (
        <div className="max-w-full bg-[repeating-conic-gradient(var(--color-surface)_0_25%,var(--color-background)_0_50%)] bg-size-[16px_16px] outline outline-(--color-border)">
          <img
            src={src}
            alt={label}
            width={size?.width}
            height={size?.height}
            className="block h-auto max-h-[min(60vh,40rem)] max-w-full object-contain"
            style={size ? { aspectRatio: `${size.width} / ${size.height}` } : { minHeight: 64, minWidth: 64 }}
            onLoad={(e) => {
              const next = { width: e.currentTarget.naturalWidth, height: e.currentTarget.naturalHeight };
              knownSizes.set(src, next);
              setSize(next);
            }}
            onError={() => setFailed(true)}
          />
        </div>
      )}
    </figure>
  );
}
