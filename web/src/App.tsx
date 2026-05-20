import { Footer } from "@/components/Footer";
import { Navbar } from "@/components/Navbar";
import { subUrl } from "@/lib/url";
import { Landing } from "@/pages/Landing";
import { NotFound } from "@/pages/NotFound";

export function App() {
  const path = typeof window === "undefined" ? "" : window.location.pathname.replace(/\/+$/, "");
  const isLanding = path === subUrl("/");
  return (
    <div className="flex min-h-dvh flex-col">
      <Navbar />
      {isLanding ? <Landing /> : <NotFound />}
      <Footer />
    </div>
  );
}
