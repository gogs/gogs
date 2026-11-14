import { Link } from "@tanstack/react-router";
import {
  Building2,
  ChevronDown,
  HelpCircle,
  Import,
  LayoutDashboard,
  LogOut,
  Menu,
  Plus,
  Search,
  UserCog,
  User as UserIcon,
} from "lucide-react";
import { useState } from "react";
import { useTranslation } from "react-i18next";

import { CommandPalette } from "@/components/CommandPalette";
import { SettingsMenu } from "@/components/SettingsMenu";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import { paletteShortcut } from "@/lib/palette";
import { signOut } from "@/lib/sign-out";
import { subUrl } from "@/lib/url";
import { useUserInfo } from "@/lib/use-user-info";
import type { UserInfo } from "@/lib/user-info";

export function Navbar() {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const [paletteOpen, setPaletteOpen] = useState(false);
  const user = useUserInfo();

  return (
    <header className="sticky top-0 z-10 border-b border-(--color-border) bg-(--color-background)">
      <nav className="mx-auto flex h-14 max-w-6xl items-center gap-3 px-4 text-sm sm:gap-4 sm:px-6">
        <a href={subUrl("/")} className="flex shrink-0 items-center" aria-label="Gogs">
          <img src={subUrl("/img/favicon.png")} alt="" width="28" height="28" className="size-7" />
        </a>

        <div className="hidden flex-1 items-center gap-1 sm:flex">
          {user ? (
            <>
              <NavLink href="/" spa>
                {t("dashboard")}
              </NavLink>
              <NavLink href="/issues" spa>
                {t("issues")}
              </NavLink>
              <NavLink href="/pulls" spa>
                {t("pull_requests")}
              </NavLink>
              <NavLink href="/explore/repos" spa>
                {t("explore")}
              </NavLink>
            </>
          ) : (
            <>
              <NavLink href="/" spa>
                {t("home")}
              </NavLink>
              <NavLink href="/explore/repos" spa>
                {t("explore")}
              </NavLink>
              <NavLink href="https://gogs.io" external>
                {t("help")}
              </NavLink>
            </>
          )}
        </div>

        <button
          type="button"
          onClick={() => setPaletteOpen(true)}
          aria-keyshortcuts={paletteShortcut.aria}
          className="hidden h-8 w-48 shrink cursor-pointer items-center gap-2 rounded-md border border-(--color-input) bg-(--color-background) pr-1.5 pl-2.5 text-(--color-muted-foreground) hover:border-(--color-ring) md:flex lg:w-64"
        >
          <Search className="size-4 shrink-0" aria-hidden />
          <span className="min-w-0 flex-1 truncate text-left">{t("palette.placeholder")}</span>
          <kbd className="shrink-0 rounded border border-(--color-border) px-1.5 font-sans text-[11px] leading-5">
            {paletteShortcut.label}
          </kbd>
        </button>
        <button
          type="button"
          onClick={() => setPaletteOpen(true)}
          aria-label={t("palette.title")}
          aria-keyshortcuts={paletteShortcut.aria}
          className="ml-auto inline-flex size-9 shrink-0 cursor-pointer items-center justify-center rounded-md text-(--color-foreground) hover:bg-(--color-surface) sm:ml-0 md:hidden"
        >
          <Search className="size-[18px]" aria-hidden />
        </button>
        <CommandPalette open={paletteOpen} onOpenChange={setPaletteOpen} />

        <div className="hidden shrink-0 items-center gap-1 sm:flex">
          <SettingsMenu />
          {user ? (
            <>
              <CreateMenu canCreateOrganization={user.canCreateOrganization} />
              <UserMenu user={user} />
            </>
          ) : (
            <>
              <NavLink href="/user/sign-in" spa>
                {t("sign_in")}
              </NavLink>
              <NavLink href="/user/sign-up" spa>
                {t("register")}
              </NavLink>
            </>
          )}
        </div>

        <div className="flex shrink-0 items-center gap-1 sm:hidden">
          <SettingsMenu />
          <Popover open={open} onOpenChange={setOpen}>
            <PopoverTrigger
              aria-label="Open menu"
              className="inline-flex size-9 cursor-pointer items-center justify-center rounded-md text-(--color-foreground) hover:bg-(--color-surface)"
            >
              <Menu className="size-[18px]" aria-hidden />
            </PopoverTrigger>
            <PopoverContent align="end" className="w-56 p-1" onOpenAutoFocus={(e) => e.preventDefault()}>
              <ul className="flex flex-col text-sm">
                {user ? (
                  <>
                    <li className="px-2 py-1.5 text-xs text-(--color-muted-foreground)">
                      {t("signed_in_as")} <strong className="text-(--color-foreground)">{user.username}</strong>
                    </li>
                    <MobileLink href={`/${user.username}`} onClick={() => setOpen(false)}>
                      {t("your_profile")}
                    </MobileLink>
                    <MobileLink href="/user/settings" onClick={() => setOpen(false)}>
                      {t("your_settings")}
                    </MobileLink>
                    {user.isAdmin && (
                      <MobileLink href="/admin" onClick={() => setOpen(false)}>
                        {t("admin_panel")}
                      </MobileLink>
                    )}
                    <li className="my-1 h-px bg-(--color-border)" />
                    <MobileLink href="/" spa onClick={() => setOpen(false)}>
                      {t("dashboard")}
                    </MobileLink>
                    <MobileLink href="/issues" spa onClick={() => setOpen(false)}>
                      {t("issues")}
                    </MobileLink>
                    <MobileLink href="/pulls" spa onClick={() => setOpen(false)}>
                      {t("pull_requests")}
                    </MobileLink>
                    <MobileLink href="/explore/repos" spa onClick={() => setOpen(false)}>
                      {t("explore")}
                    </MobileLink>
                    <MobileLink href="/search" spa onClick={() => setOpen(false)}>
                      {t("repo.code_search")}
                    </MobileLink>
                    <li className="my-1 h-px bg-(--color-border)" />
                    <MobileLink href="/repo/create" onClick={() => setOpen(false)}>
                      {t("new_repo")}
                    </MobileLink>
                    <MobileLink href="/repo/migrate" onClick={() => setOpen(false)}>
                      {t("new_migrate")}
                    </MobileLink>
                    {user.canCreateOrganization && (
                      <MobileLink href="/org/create" onClick={() => setOpen(false)}>
                        {t("new_org")}
                      </MobileLink>
                    )}
                    <li className="my-1 h-px bg-(--color-border)" />
                    <MobileLink href="https://gogs.io" external onClick={() => setOpen(false)}>
                      {t("help")}
                    </MobileLink>
                    <li>
                      <SignOutForm>
                        <button
                          type="submit"
                          className="flex w-full cursor-pointer rounded-sm px-2 py-1.5 text-left text-(--color-foreground) hover:bg-(--color-surface)"
                          onClick={() => setOpen(false)}
                        >
                          {t("sign_out")}
                        </button>
                      </SignOutForm>
                    </li>
                  </>
                ) : (
                  <>
                    <MobileLink href="/" spa onClick={() => setOpen(false)}>
                      {t("home")}
                    </MobileLink>
                    <MobileLink href="/explore/repos" spa onClick={() => setOpen(false)}>
                      {t("explore")}
                    </MobileLink>
                    <MobileLink href="/search" spa onClick={() => setOpen(false)}>
                      {t("repo.code_search")}
                    </MobileLink>
                    <MobileLink href="https://gogs.io" external onClick={() => setOpen(false)}>
                      {t("help")}
                    </MobileLink>
                    <li className="my-1 h-px bg-(--color-border)" />
                    <MobileLink href="/user/sign-in" spa onClick={() => setOpen(false)}>
                      {t("sign_in")}
                    </MobileLink>
                    <MobileLink href="/user/sign-up" spa onClick={() => setOpen(false)}>
                      {t("register")}
                    </MobileLink>
                  </>
                )}
              </ul>
            </PopoverContent>
          </Popover>
        </div>
      </nav>
    </header>
  );
}

function CreateMenu({ canCreateOrganization }: { canCreateOrganization: boolean }) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger
        aria-label={t("create_new")}
        className="inline-flex h-9 cursor-pointer items-center gap-1 rounded-md px-2 text-(--color-muted-foreground) hover:bg-(--color-surface) hover:text-(--color-foreground)"
      >
        <Plus className="size-4" aria-hidden />
        <ChevronDown className="size-3" aria-hidden />
      </PopoverTrigger>
      <PopoverContent align="end" className="w-56 p-1" onOpenAutoFocus={(e) => e.preventDefault()}>
        <MenuLink href="/repo/create" icon={<Plus className="size-4" aria-hidden />} onSelect={() => setOpen(false)}>
          {t("new_repo")}
        </MenuLink>
        <MenuLink href="/repo/migrate" icon={<Import className="size-4" aria-hidden />} onSelect={() => setOpen(false)}>
          {t("new_migrate")}
        </MenuLink>
        {canCreateOrganization && (
          <MenuLink
            href="/org/create"
            icon={<Building2 className="size-4" aria-hidden />}
            onSelect={() => setOpen(false)}
          >
            {t("new_org")}
          </MenuLink>
        )}
      </PopoverContent>
    </Popover>
  );
}

function UserMenu({ user }: { user: UserInfo }) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger
        aria-label={t("user_profile_and_more")}
        className="inline-flex h-9 cursor-pointer items-center gap-1 rounded-md px-1 hover:bg-(--color-surface)"
      >
        {user.avatarURL ? (
          <img src={user.avatarURL} alt="" width="24" height="24" className="size-6 rounded-full" />
        ) : (
          <UserIcon className="size-5" aria-hidden />
        )}
        <ChevronDown className="size-3 text-(--color-muted-foreground)" aria-hidden />
      </PopoverTrigger>
      <PopoverContent align="end" className="w-60 p-1" onOpenAutoFocus={(e) => e.preventDefault()}>
        <div className="px-2 pt-2 pb-1 text-xs text-(--color-muted-foreground)">
          {t("signed_in_as")} <strong className="text-(--color-foreground)">{user.username}</strong>
        </div>
        <div className="my-1 h-px bg-(--color-border)" />
        <MenuLink
          href={`/${user.username}`}
          icon={<UserIcon className="size-4" aria-hidden />}
          onSelect={() => setOpen(false)}
        >
          {t("your_profile")}
        </MenuLink>
        <MenuLink
          href="/user/settings"
          icon={<UserCog className="size-4" aria-hidden />}
          onSelect={() => setOpen(false)}
        >
          {t("your_settings")}
        </MenuLink>
        <MenuLink
          href="https://gogs.io"
          external
          icon={<HelpCircle className="size-4" aria-hidden />}
          onSelect={() => setOpen(false)}
        >
          {t("help")}
        </MenuLink>
        {user.isAdmin && (
          <>
            <div className="my-1 h-px bg-(--color-border)" />
            <MenuLink
              href="/admin"
              icon={<LayoutDashboard className="size-4" aria-hidden />}
              onSelect={() => setOpen(false)}
            >
              {t("admin_panel")}
            </MenuLink>
          </>
        )}
        <div className="my-1 h-px bg-(--color-border)" />
        <SignOutForm>
          <button
            type="submit"
            className="flex w-full cursor-pointer items-center gap-2 rounded-sm px-2 py-1.5 text-left text-sm text-(--color-foreground) hover:bg-(--color-surface)"
            onClick={() => setOpen(false)}
          >
            <LogOut className="size-4" aria-hidden />
            {t("sign_out")}
          </button>
        </SignOutForm>
      </PopoverContent>
    </Popover>
  );
}

function MenuLink({
  href,
  external,
  icon,
  onSelect,
  children,
}: {
  href: string;
  external?: boolean;
  icon?: React.ReactNode;
  onSelect?: () => void;
  children: React.ReactNode;
}) {
  return (
    <a
      href={external ? href : subUrl(href)}
      onClick={onSelect}
      {...(external ? { target: "_blank", rel: "noopener noreferrer" } : {})}
      className="flex items-center gap-2 rounded-sm px-2 py-1.5 text-sm text-(--color-foreground) hover:bg-(--color-surface)"
    >
      {icon}
      {children}
    </a>
  );
}

function NavLink({
  href,
  external,
  spa,
  children,
}: {
  href: string;
  external?: boolean;
  spa?: boolean;
  children: React.ReactNode;
}) {
  const className = "inline-flex rounded-md px-3 py-1.5 text-(--color-foreground) hover:bg-(--color-surface)";
  if (spa) {
    return (
      <Link to={href} className={className}>
        {children}
      </Link>
    );
  }
  return (
    <a
      href={external ? href : subUrl(href)}
      {...(external ? { target: "_blank", rel: "noopener noreferrer" } : {})}
      className={className}
    >
      {children}
    </a>
  );
}

function SignOutForm({ children }: { children: React.ReactNode }) {
  return (
    <form
      action={subUrl("/api/web/user/sign-out")}
      method="POST"
      className="inline"
      onSubmit={(event) => {
        event.preventDefault();
        void signOut();
      }}
    >
      {children}
    </form>
  );
}

function MobileLink({
  href,
  external,
  spa,
  onClick,
  children,
}: {
  href: string;
  external?: boolean;
  spa?: boolean;
  onClick?: () => void;
  children: React.ReactNode;
}) {
  const className = "flex w-full rounded-sm px-2 py-1.5 text-(--color-foreground) hover:bg-(--color-surface)";
  return (
    <li>
      {spa ? (
        <Link to={href} onClick={onClick} className={className}>
          {children}
        </Link>
      ) : (
        <a
          href={external ? href : subUrl(href)}
          onClick={onClick}
          {...(external ? { target: "_blank", rel: "noopener noreferrer" } : {})}
          className={className}
        >
          {children}
        </a>
      )}
    </li>
  );
}
