import { useTranslation } from "react-i18next";

import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { usePageTitle } from "@/lib/page-title";

// ProhibitLogin tells a signed-in user whose account is prohibited from
// signing in that they cannot use the site. The server shows it on every page.
export function ProhibitLogin() {
  const { t } = useTranslation();
  usePageTitle(t("auth.prohibit_login"));
  return (
    <main className="flex flex-1 items-center justify-center px-4 py-10 sm:px-6 sm:py-16">
      <Card className="w-full max-w-md">
        <CardHeader className="items-center text-center">
          <CardTitle>{t("auth.prohibit_login")}</CardTitle>
        </CardHeader>
        <CardContent className="pt-2">
          <p className="text-center text-sm text-(--color-foreground)">{t("auth.prohibit_login_desc")}</p>
        </CardContent>
      </Card>
    </main>
  );
}
