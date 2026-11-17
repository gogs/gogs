import { ZoomIn, ZoomOut } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Document, Page, pdfjs } from "react-pdf";
import "react-pdf/dist/Page/AnnotationLayer.css";
import "react-pdf/dist/Page/TextLayer.css";

import { Button } from "@/components/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { subUrl } from "@/lib/url";

pdfjs.GlobalWorkerOptions.workerSrc = new URL("pdfjs-dist/build/pdf.worker.min.mjs", import.meta.url).toString();

// The directories of the files that PDF.js loads while it renders, which the
// build copies from pdfjs-dist. PDF.js requires the trailing slash.
const OPTIONS = {
  cMapUrl: `${subUrl("/assets/pdfjs/cmaps")}/`,
  iccUrl: `${subUrl("/assets/pdfjs/iccs")}/`,
  standardFontDataUrl: `${subUrl("/assets/pdfjs/standard_fonts")}/`,
  wasmUrl: `${subUrl("/assets/pdfjs/wasm")}/`,
};

const ZOOMS = [0.5, 0.75, 1, 1.25, 1.5, 2, 3];
// Pages are no wider than this at 100%, so a wide screen does not blow them up.
const MAX_FIT_WIDTH = 960;

// LazyPage renders its page once it scrolls near the viewport, keeping its
// place with the height of the first page until then.
function LazyPage({ pageNumber, width, height }: { pageNumber: number; width: number; height: number }) {
  const ref = useRef<HTMLDivElement>(null);
  const [visible, setVisible] = useState(pageNumber === 1);

  useEffect(() => {
    const el = ref.current;
    if (!el || visible) return;
    const observer = new IntersectionObserver(
      (entries) => {
        if (entries.some((e) => e.isIntersecting)) setVisible(true);
      },
      { rootMargin: "1000px 0px" },
    );
    observer.observe(el);
    return () => observer.disconnect();
  }, [visible]);

  return (
    <div ref={ref} className="mx-auto bg-white shadow-sm" style={{ width, minHeight: visible ? undefined : height }}>
      {visible ? <Page pageNumber={pageNumber} width={width} loading={null} suspense={false} /> : null}
    </div>
  );
}

// PdfView shows a PDF file page by page, fit to the width of the page.
export default function PdfView({ url }: { url: string }) {
  const { t } = useTranslation();
  const container = useRef<HTMLDivElement>(null);
  const [fitWidth, setFitWidth] = useState(0);
  const [zoom, setZoom] = useState(1);
  const [pages, setPages] = useState<{ count: number; ratio: number }>();

  useEffect(() => {
    const el = container.current;
    if (!el) return;
    const observer = new ResizeObserver(([entry]) => {
      setFitWidth(Math.min(Math.floor(entry.contentRect.width), MAX_FIT_WIDTH));
    });
    observer.observe(el);
    return () => observer.disconnect();
  }, []);

  const width = Math.round(fitWidth * zoom);
  const zoomIndex = ZOOMS.indexOf(zoom);

  return (
    <div>
      <div className="flex items-center justify-end gap-1 border-b border-(--color-border) px-2 py-1">
        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              variant="ghost"
              size="icon"
              aria-label={t("repo.pdf_zoom_out")}
              disabled={zoomIndex <= 0}
              onClick={() => setZoom(ZOOMS[zoomIndex - 1])}
            >
              <ZoomOut aria-hidden />
            </Button>
          </TooltipTrigger>
          <TooltipContent>{t("repo.pdf_zoom_out")}</TooltipContent>
        </Tooltip>
        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              variant="ghost"
              size="sm"
              className="min-w-14 tabular-nums"
              aria-label={t("repo.pdf_zoom_reset")}
              onClick={() => setZoom(1)}
            >
              {Math.round(zoom * 100)}%
            </Button>
          </TooltipTrigger>
          <TooltipContent>{t("repo.pdf_zoom_reset")}</TooltipContent>
        </Tooltip>
        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              variant="ghost"
              size="icon"
              aria-label={t("repo.pdf_zoom_in")}
              disabled={zoomIndex >= ZOOMS.length - 1}
              onClick={() => setZoom(ZOOMS[zoomIndex + 1])}
            >
              <ZoomIn aria-hidden />
            </Button>
          </TooltipTrigger>
          <TooltipContent>{t("repo.pdf_zoom_in")}</TooltipContent>
        </Tooltip>
      </div>
      <div className="overflow-x-auto bg-(--color-surface) p-2 sm:p-4">
        <div ref={container}>
          <Document
            file={url}
            options={OPTIONS}
            suspense={false}
            loading={
              <p className="py-10 text-center text-sm text-(--color-muted-foreground)">{t("repo.pdf_loading")}</p>
            }
            error={
              <div className="flex flex-col items-center gap-3 py-10 text-sm">
                <p>{t("repo.file_not_shown")}</p>
                <Button asChild variant="outline" size="sm">
                  <a href={url} rel="nofollow">
                    {t("repo.file_view_raw")}
                  </a>
                </Button>
              </div>
            }
            onLoadSuccess={(pdf) => {
              void pdf.getPage(1).then((page) => {
                const viewport = page.getViewport({ scale: 1 });
                setPages({ count: pdf.numPages, ratio: viewport.height / viewport.width });
              });
            }}
            className="flex flex-col gap-2 sm:gap-4"
          >
            {pages && width > 0
              ? Array.from({ length: pages.count }, (_, i) => (
                  <LazyPage key={i} pageNumber={i + 1} width={width} height={Math.round(width * pages.ratio)} />
                ))
              : null}
          </Document>
        </div>
      </div>
    </div>
  );
}
