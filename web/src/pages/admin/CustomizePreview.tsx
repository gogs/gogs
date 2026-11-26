import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";

import { CustomSlot } from "@/components/CustomSlot";
import { Footer } from "@/components/Footer";
import { Navbar } from "@/components/Navbar";
import { NoticeBanner } from "@/components/NoticeBanner";
import { UserInfoProvider } from "@/components/UserInfoProvider";
import type { Customization, PreviewMessage, PreviewRequest, SlotName } from "@/lib/customization";
import { useUserInfo } from "@/lib/use-user-info";
import { Landing } from "@/pages/Landing";

function post(message: PreviewMessage) {
  window.parent.postMessage(message, window.location.origin);
}

// CustomizePreview shows the drafts that the customization editor sends it.
// The editor frames a new one for every draft, so what the scripts of a draft
// leave behind, e.g., styles, timers or defined elements, goes with its frame.
export function CustomizePreview() {
  const { t } = useTranslation();
  const user = useUserInfo();
  const [preview, setPreview] = useState<{ customization: Customization; request: PreviewRequest } | null>(null);

  useEffect(() => {
    const onMessage = (event: MessageEvent) => {
      const request = event.data as PreviewRequest | undefined;
      if (event.origin !== window.location.origin || event.source !== window.parent) return;
      if (request?.type !== "gogs-preview-render") return;
      void import("@/lib/customization-api")
        .then(({ importSources }) => importSources(request.sources))
        .then(({ customization, errors }) => {
          setPreview({ customization, request });
          post({ type: "gogs-preview-result", errors });
        });
    };
    window.addEventListener("message", onMessage);
    post({ type: "gogs-preview-ready" });
    return () => window.removeEventListener("message", onMessage);
  }, []);

  if (!preview) return null;
  const { customization, request } = preview;
  const onError = (part: SlotName) => (err: unknown) =>
    post({ type: "gogs-preview-render-error", part, message: String(err) });
  return (
    <UserInfoProvider value={request.viewer === "user" ? user : null}>
      <div className="flex min-h-dvh flex-col">
        {customization.css ? <style>{customization.css}</style> : null}
        <CustomSlot name="navbar" fallback={Navbar} customization={customization} onError={onError("navbar")} />
        <NoticeBanner />
        {request.part === "landing" ? (
          <CustomSlot name="landing" fallback={Landing} customization={customization} onError={onError("landing")} />
        ) : (
          <main className="mx-auto w-full max-w-6xl flex-1 px-4 py-10 sm:px-6">
            <div className="rounded-md border border-dashed border-(--color-border) p-10 text-center text-sm text-(--color-muted-foreground)">
              {t("admin.customize_page_content")}
            </div>
          </main>
        )}
        <CustomSlot name="footer" fallback={Footer} customization={customization} onError={onError("footer")} />
      </div>
    </UserInfoProvider>
  );
}
