import { useNavigate } from "@tanstack/react-router";
import { Search } from "lucide-react";
import type { FormEvent } from "react";
import { useTranslation } from "react-i18next";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";

// SearchForm searches a list by keyword through the `q` search param of the
// current route, starting over from the first page.
export function SearchForm({ q }: { q?: string }) {
  const { t } = useTranslation();
  const navigate = useNavigate();

  function onSearch(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const keyword = (new FormData(event.currentTarget).get("q") as string | null)?.trim() ?? "";
    void navigate({ to: ".", search: { q: keyword || undefined } });
  }

  return (
    <form role="search" onSubmit={onSearch} className="flex gap-2">
      <div className="relative min-w-0 flex-1">
        <Search
          className="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-(--color-muted-foreground)"
          aria-hidden
        />
        {/* Remount on navigation so the field always shows the keyword of the current URL. */}
        <Input
          key={q ?? ""}
          name="q"
          type="search"
          defaultValue={q}
          placeholder={`${t("explore.search")}...`}
          aria-label={t("explore.search")}
          className="h-10 pl-9"
        />
      </div>
      <Button type="submit">{t("explore.search")}</Button>
    </form>
  );
}
