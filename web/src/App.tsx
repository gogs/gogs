import { Footer } from "@/components/Footer";
import { Navbar } from "@/components/Navbar";
import { Landing } from "@/pages/Landing";
import { NotFound } from "@/pages/NotFound";

export function App() {
  const path = typeof window === "undefined" ? "/" : window.location.pathname;
  const isLanding = path === "/";
  return (
    <div className="flex min-h-dvh flex-col">
      <Navbar />
      {isLanding ? <Landing /> : <NotFound />}
      <Footer />
    </div>
  );
}
