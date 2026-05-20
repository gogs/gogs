import { Menu } from "lucide-react";
import { useState } from "react";
import { useTranslation } from "react-i18next";

import { SettingsMenu } from "@/components/SettingsMenu";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";

export function Navbar() {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);

  return (
    <header className="sticky top-0 z-10 border-b border-(--color-border) bg-(--color-background)/95 backdrop-blur">
      <nav className="mx-auto flex h-14 max-w-6xl items-center gap-3 px-4 text-sm sm:gap-4 sm:px-6">
        <a href="/" className="flex shrink-0 items-center" aria-label="Gogs">
          <img src="/img/favicon.png" alt="" width="28" height="28" className="size-7" />
        </a>

        <div className="hidden flex-1 items-center gap-1 sm:flex">
          <NavLink href="/">{t("home")}</NavLink>
          <NavLink href="/explore/repos">{t("explore")}</NavLink>
          <NavLink href="https://gogs.io" external>
            {t("help")}
          </NavLink>
        </div>

        <div className="hidden shrink-0 items-center gap-1 sm:flex">
          <SettingsMenu />
          <NavLink href="/user/sign_up">{t("register")}</NavLink>
          <NavLink href="/user/login">{t("sign_in")}</NavLink>
        </div>

        <div className="ml-auto flex shrink-0 items-center gap-1 sm:hidden">
          <SettingsMenu />
          <Popover open={open} onOpenChange={setOpen}>
            <PopoverTrigger
              aria-label="Open menu"
              className="inline-flex size-9 cursor-pointer items-center justify-center rounded-md text-(--color-foreground) hover:bg-(--color-surface)"
            >
              <Menu className="size-[18px]" />
            </PopoverTrigger>
            <PopoverContent align="end" className="w-56 p-1" onOpenAutoFocus={(e) => e.preventDefault()}>
              <ul className="flex flex-col text-sm">
                <MobileLink href="/" onClick={() => setOpen(false)}>
                  {t("home")}
                </MobileLink>
                <MobileLink href="/explore/repos" onClick={() => setOpen(false)}>
                  {t("explore")}
                </MobileLink>
                <MobileLink href="https://gogs.io" external onClick={() => setOpen(false)}>
                  {t("help")}
                </MobileLink>
                <li className="my-1 h-px bg-(--color-border)" />
                <MobileLink href="/user/sign_up" onClick={() => setOpen(false)}>
                  {t("register")}
                </MobileLink>
                <MobileLink href="/user/login" onClick={() => setOpen(false)}>
                  {t("sign_in")}
                </MobileLink>
              </ul>
            </PopoverContent>
          </Popover>
        </div>
      </nav>
    </header>
  );
}

function NavLink({ href, external, children }: { href: string; external?: boolean; children: React.ReactNode }) {
  return (
    <a
      href={href}
      {...(external ? { target: "_blank", rel: "noopener noreferrer" } : {})}
      className="inline-flex rounded-md px-3 py-1.5 text-(--color-foreground) hover:bg-(--color-surface)"
    >
      {children}
    </a>
  );
}

function MobileLink({
  href,
  external,
  onClick,
  children,
}: {
  href: string;
  external?: boolean;
  onClick?: () => void;
  children: React.ReactNode;
}) {
  return (
    <li>
      <a
        href={href}
        onClick={onClick}
        {...(external ? { target: "_blank", rel: "noopener noreferrer" } : {})}
        className="flex w-full rounded-sm px-2 py-1.5 text-(--color-foreground) hover:bg-(--color-surface)"
      >
        {children}
      </a>
    </li>
  );
}
