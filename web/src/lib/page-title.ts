import { useEffect } from "react";

const APP_NAME = "Gogs";

export function usePageTitle(title?: string) {
  useEffect(() => {
    document.title = title ? `${title} - ${APP_NAME}` : APP_NAME;
  }, [title]);
}
