import { webContext } from "./context";

// subUrl prefixes an internal absolute path with the deployment subpath so
// links work whether Gogs is mounted at "/" or behind a reverse proxy on a
// prefix like "/gogs". Pass paths that start with "/" (e.g. "/user/login").
// The result is canonicalized by trimming trailing slashes, so subUrl("/")
// returns "/gogs" (or "" at root), letting callers compare against
// location.pathname without juggling both "/gogs" and "/gogs/" forms.
export function subUrl(path: string): string {
  const url = webContext.subURL + path;
  return url.length > 1 ? url.replace(/\/+$/, "") : url;
}
