import { useTranslation } from "react-i18next";

import { usePageTitle } from "@/lib/page-title";

export function Landing() {
  const { t } = useTranslation();
  usePageTitle();
  return (
    <main className="flex flex-1 items-center justify-center px-4 py-10 sm:px-6 sm:py-16">
      <div className="w-full max-w-2xl">
        <div className="rounded-lg border border-(--color-border) bg-(--color-surface)/40 font-mono shadow-xs">
          <div className="flex items-center gap-1.5 border-b border-(--color-border) px-3 py-2 sm:px-4 sm:py-2.5">
            <span className="size-2.5 rounded-full bg-(--color-destructive)/70" />
            <span className="size-2.5 rounded-full bg-(--color-warning,oklch(0.795_0.184_86.047))/70" />
            <span className="size-2.5 rounded-full bg-(--color-foreground)/20" />
            <span className="ml-2 text-xs text-(--color-muted-foreground) sm:ml-3">gogs — zsh</span>
          </div>
          <pre className="px-4 py-4 text-xs leading-relaxed break-all whitespace-pre-wrap text-(--color-foreground) sm:px-5 sm:py-5 sm:text-sm">
            <span className="text-(--color-muted-foreground)">$ </span>
            <span>cat /etc/motd</span>
            {"\n"}
            <img
              src="/img/logo-light.svg"
              alt="Gogs"
              width="775"
              height="294"
              className="mx-auto block max-w-[280px] dark:hidden sm:max-w-sm"
            />
            <img
              src="/img/logo-dark.svg"
              alt="Gogs"
              width="775"
              height="294"
              className="mx-auto hidden max-w-[280px] dark:block sm:max-w-sm"
            />
            {"\n"}
            <span className="block text-center font-sans text-base text-(--color-muted-foreground) sm:text-lg">
              {t("app_desc")}
            </span>
            {"\n"}
            <span className="text-(--color-muted-foreground)">$ </span>
            <span>gogs help</span>
            {"\n"}
            <CmdLink href="/user/login" cmd="sign-in" desc={t("sign_in")} />
            {"\n"}
            <CmdLink href="/user/sign_up" cmd="sign-up" desc={t("register")} />
            {"\n"}
            <CmdLink href="/explore/repos" cmd="explore" desc={t("explore")} />
            {"\n"}
            <CmdLink href="https://gogs.io" cmd="help" desc={t("help")} external />
            {"\n"}
            {"\n"}
            <span className="text-(--color-muted-foreground)">$ </span>
            <span className="inline-block w-2 animate-pulse bg-(--color-foreground) align-baseline"> </span>
          </pre>
        </div>
      </div>
    </main>
  );
}

function CmdLink({ href, cmd, desc, external }: { href: string; cmd: string; desc: string; external?: boolean }) {
  return (
    <a
      href={href}
      {...(external ? { target: "_blank", rel: "noopener noreferrer" } : {})}
      className="group inline-flex items-baseline gap-2 rounded-sm hover:bg-(--color-surface) hover:text-(--color-foreground)"
    >
      <span className="inline-block w-16 text-(--color-foreground) sm:w-20">{cmd}</span>
      <span className="text-(--color-muted-foreground) group-hover:text-(--color-foreground)/80">— {desc}</span>
      <span className="text-(--color-muted-foreground) group-hover:text-(--color-foreground)">→</span>
    </a>
  );
}
