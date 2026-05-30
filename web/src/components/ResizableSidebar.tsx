import { type ReactNode, useCallback, useRef, useState } from "react";
import { useTranslation } from "react-i18next";

import { cn } from "@/lib/utils";

interface Props {
  children: ReactNode;
  /** Width in pixels. Resets to this on every mount; not persisted. */
  defaultWidth?: number;
  minWidth?: number;
  maxWidth?: number;
  className?: string;
  style?: React.CSSProperties;
}

// Resizable left sidebar. Drag the right-edge handle to widen or narrow.
// The chosen width is in-memory only and resets each page load, so updating
// `defaultWidth` in code always takes effect for every user. Below the lg
// breakpoint, the sidebar stacks above content at full width, so the handle
// (and resize) is hidden.
export function ResizableSidebar({
  children,
  defaultWidth = 320,
  minWidth = 220,
  maxWidth = 560,
  className,
  style,
}: Props) {
  const { t } = useTranslation();
  const [width, setWidth] = useState(defaultWidth);
  const asideRef = useRef<HTMLElement>(null);
  const draggingRef = useRef(false);
  const startXRef = useRef(0);
  const startWidthRef = useRef(0);
  // Latest width during a drag. Updated on every pointermove and committed to
  // React state on pointerup, so the tree only re-renders once per drag
  // instead of once per pointer event.
  const liveWidthRef = useRef(defaultWidth);

  const onPointerDown = useCallback(
    (e: React.PointerEvent<HTMLDivElement>) => {
      e.preventDefault();
      draggingRef.current = true;
      startXRef.current = e.clientX;
      startWidthRef.current = width;
      liveWidthRef.current = width;
      e.currentTarget.setPointerCapture(e.pointerId);
      document.body.style.cursor = "col-resize";
      document.body.style.userSelect = "none";
    },
    [width],
  );

  const onPointerMove = useCallback(
    (e: React.PointerEvent<HTMLDivElement>) => {
      if (!draggingRef.current) return;
      const next = Math.min(maxWidth, Math.max(minWidth, startWidthRef.current + (e.clientX - startXRef.current)));
      liveWidthRef.current = next;
      asideRef.current?.style.setProperty("--sidebar-w", `${next}px`);
    },
    [maxWidth, minWidth],
  );

  const stopDrag = useCallback((e: React.PointerEvent<HTMLDivElement>) => {
    if (!draggingRef.current) return;
    draggingRef.current = false;
    e.currentTarget.releasePointerCapture(e.pointerId);
    document.body.style.cursor = "";
    document.body.style.userSelect = "";
    setWidth(liveWidthRef.current);
  }, []);

  // Keyboard resize for accessibility: focus the handle, then arrow-left/right
  // adjusts width by a fixed step.
  const onKeyDown = useCallback(
    (e: React.KeyboardEvent<HTMLDivElement>) => {
      const step = e.shiftKey ? 40 : 8;
      if (e.key === "ArrowLeft") {
        e.preventDefault();
        setWidth((w) => Math.max(minWidth, w - step));
      } else if (e.key === "ArrowRight") {
        e.preventDefault();
        setWidth((w) => Math.min(maxWidth, w + step));
      } else if (e.key === "Home") {
        e.preventDefault();
        setWidth(defaultWidth);
      }
    },
    [defaultWidth, maxWidth, minWidth],
  );

  return (
    <aside
      ref={asideRef}
      className={cn("relative flex w-full shrink-0 flex-col lg:w-[var(--sidebar-w)]", className)}
      style={{
        ...style,
        ["--sidebar-w" as string]: `${width}px`,
      }}
    >
      <div className="flex w-full min-w-0 flex-1 flex-col">{children}</div>
      <div
        role="separator"
        aria-orientation="vertical"
        aria-label={t("resize_sidebar")}
        aria-valuemin={minWidth}
        aria-valuemax={maxWidth}
        aria-valuenow={width}
        tabIndex={0}
        onPointerDown={onPointerDown}
        onPointerMove={onPointerMove}
        onPointerUp={stopDrag}
        onPointerCancel={stopDrag}
        onKeyDown={onKeyDown}
        className="absolute top-0 right-0 bottom-0 hidden w-1 cursor-col-resize touch-none bg-transparent hover:bg-(--color-primary)/30 focus-visible:bg-(--color-primary)/40 focus-visible:outline-none lg:block"
      />
    </aside>
  );
}
