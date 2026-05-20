import { subUrl } from "@/lib/url";

export function Footer() {
  return (
    <footer className="border-t border-(--color-border)">
      <div className="mx-auto flex max-w-6xl flex-wrap items-center justify-between gap-x-5 gap-y-3 px-4 py-6 text-sm text-(--color-muted-foreground) sm:px-6">
        <span>© {new Date().getFullYear()} Gogs®</span>
        <div className="flex flex-wrap items-center gap-x-2 gap-y-2">
          <a
            href="https://github.com/gogs/gogs"
            target="_blank"
            rel="noopener noreferrer"
            aria-label="GitHub"
            className="inline-flex size-8 items-center justify-center rounded-md hover:bg-(--color-surface) hover:text-(--color-foreground)"
          >
            <GitHubIcon />
          </a>
          <a
            href="https://twitter.com/GogsHQ"
            target="_blank"
            rel="noopener noreferrer"
            aria-label="Twitter"
            className="inline-flex size-8 items-center justify-center rounded-md hover:bg-(--color-surface) hover:text-(--color-foreground)"
          >
            <TwitterIcon />
          </a>
        </div>
      </div>
      <a href={subUrl("/assets/librejs/librejs.html")} className="hidden" data-jslicense="1">
        JavaScript Licenses
      </a>
    </footer>
  );
}

function GitHubIcon() {
  return (
    <svg width="16" height="16" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
      <path d="M12 0C5.37 0 0 5.37 0 12c0 5.3 3.44 9.8 8.21 11.39.6.11.82-.26.82-.58 0-.28-.01-1.04-.02-2.05-3.34.73-4.04-1.61-4.04-1.61-.55-1.39-1.34-1.76-1.34-1.76-1.1-.75.08-.74.08-.74 1.21.09 1.85 1.24 1.85 1.24 1.07 1.84 2.81 1.31 3.5 1 .11-.78.42-1.31.76-1.61-2.67-.3-5.47-1.33-5.47-5.94 0-1.31.47-2.38 1.24-3.22-.12-.3-.54-1.52.12-3.18 0 0 1.01-.32 3.3 1.23A11.5 11.5 0 0 1 12 5.8c1.02.01 2.05.14 3.01.4 2.29-1.55 3.3-1.23 3.3-1.23.66 1.66.24 2.88.12 3.18.77.84 1.24 1.91 1.24 3.22 0 4.62-2.81 5.63-5.49 5.93.43.37.81 1.1.81 2.22 0 1.61-.01 2.9-.01 3.3 0 .32.22.7.83.58A12 12 0 0 0 24 12c0-6.63-5.37-12-12-12z" />
    </svg>
  );
}

function TwitterIcon() {
  return (
    <svg width="16" height="16" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
      <path d="M18.244 2.25h3.308l-7.227 8.26 8.502 11.24H16.17l-5.214-6.817L4.99 21.75H1.68l7.73-8.835L1.254 2.25H8.08l4.713 6.231zm-1.161 17.52h1.833L7.084 4.126H5.117z" />
    </svg>
  );
}
