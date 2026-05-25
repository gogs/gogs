import { type ReactNode, useCallback, useEffect, useRef, useState } from "react";

import { cn } from "@/lib/utils";

interface Props {
  children: ReactNode;
  /** Initial width in pixels, before any persisted value is loaded. */
  defaultWidth?: number;
  minWidth?: number;
  maxWidth?: number;
  /**
   * localStorage key for persisting the width across reloads. Omit to make
   * the sidebar non-persistent.
   */
  storageKey?: string;
  className?: string;
  style?: React.CSSProperties;
}

function readStoredWidth(key: string | undefined, fallback: number): number {
  if (!key || typeof localStorage === "undefined") return fallback;
  const v = Number(localStorage.getItem(key));
  return Number.isFinite(v) && v > 0 ? v : fallback;
}

// Resizable left sidebar. Drag the right-edge handle to widen or narrow.
// Width is persisted to localStorage when storageKey is provided, so the
// chosen width survives page reloads. Below the lg breakpoint, the sidebar
// stacks above content at full width, so the handle (and resize) is hidden.
export function ResizableSidebar({
  children,
  defaultWidth = 320,
  minWidth = 220,
  maxWidth = 560,
  storageKey,
  className,
  style,
}: Props) {
  const [width, setWidth] = useState(() => readStoredWidth(storageKey, defaultWidth));
  const draggingRef = useRef(false);
  const startXRef = useRef(0);
  const startWidthRef = useRef(0);

  useEffect(() => {
    if (!storageKey || typeof localStorage === "undefined") return;
    localStorage.setItem(storageKey, String(width));
  }, [storageKey, width]);

  const onPointerDown = useCallback(
    (e: React.PointerEvent<HTMLDivElement>) => {
      e.preventDefault();
      draggingRef.current = true;
      startXRef.current = e.clientX;
      startWidthRef.current = width;
      (e.target as HTMLDivElement).setPointerCapture(e.pointerId);
      document.body.style.cursor = "col-resize";
      document.body.style.userSelect = "none";
    },
    [width],
  );

  const onPointerMove = useCallback(
    (e: React.PointerEvent<HTMLDivElement>) => {
      if (!draggingRef.current) return;
      const next = Math.min(maxWidth, Math.max(minWidth, startWidthRef.current + (e.clientX - startXRef.current)));
      setWidth(next);
    },
    [maxWidth, minWidth],
  );

  const stopDrag = useCallback((e: React.PointerEvent<HTMLDivElement>) => {
    if (!draggingRef.current) return;
    draggingRef.current = false;
    (e.target as HTMLDivElement).releasePointerCapture(e.pointerId);
    document.body.style.cursor = "";
    document.body.style.userSelect = "";
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
        aria-label="Resize sidebar"
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
