import { LoaderResponseError } from "@/lib/loader-error";
import { subUrl } from "@/lib/url";

// exitURL returns where a viewer the admin API turned away belongs, or null
// for any other error, e.g., a 403 of a proxy, which loading a page again does
// not change. `href` is the page they are on or asked for.
function exitURL(err: unknown, href: string): string | null {
  if (!(err instanceof LoaderResponseError)) return null;
  // The admin route has them sign in, or shows that nothing is here.
  if (err.status === 401 || (err.status === 403 && err.errorField === "admin required")) return href;
  if (err.status === 403 && err.errorField === "account not activated") return subUrl("/user/activate");
  // The pages of the site tell them that they may not sign in.
  if (err.status === 403 && err.errorField === "login prohibited") return subUrl("/");
  return null;
}

// leaveAdmin takes a viewer the admin API turned away, e.g., after their
// session expired or they stopped being a site admin, out of the admin console,
// and reports whether it did. The console only asks who the viewer is when the
// page loads, so they leave it with a full page load.
export function leaveAdmin(err: unknown, href: string): boolean {
  const url = exitURL(err, href);
  if (url === null) return false;
  // Assigning the URL of the current page does not load it again when it has a hash.
  if (new URL(url, window.location.href).href === window.location.href) window.location.reload();
  else window.location.assign(url);
  return true;
}
