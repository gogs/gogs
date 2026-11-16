import { type ReactNode, useEffect, useMemo, useState } from "react";

import { ThemeContext, type ThemeContextValue } from "@/lib/theme-context";

const STORAGE_KEY = "gogs-theme";

function systemPrefersDark(): boolean {
  return typeof window !== "undefined" && window.matchMedia("(prefers-color-scheme: dark)").matches;
}

function readStoredTheme(): ThemeContextValue["theme"] {
  if (typeof localStorage === "undefined") return "system";
  const v = localStorage.getItem(STORAGE_KEY);
  return v === "light" || v === "dark" || v === "system" ? v : "system";
}

function applyTheme(theme: ThemeContextValue["theme"]) {
  const dark = theme === "dark" || (theme === "system" && systemPrefersDark());
  document.documentElement.classList.toggle("dark", dark);
}

export function ThemeProvider({ children }: { children: ReactNode }) {
  const [theme, setThemeState] = useState<ThemeContextValue["theme"]>(readStoredTheme);
  // systemDark is part of the context value, so components that resolve the
  // "system" theme while rendering, e.g., highlighted code, render again
  // when the OS switches.
  const [systemDark, setSystemDark] = useState(systemPrefersDark);

  useEffect(() => {
    const mq = window.matchMedia("(prefers-color-scheme: dark)");
    const onChange = () => setSystemDark(mq.matches);
    mq.addEventListener("change", onChange);
    return () => mq.removeEventListener("change", onChange);
  }, []);

  useEffect(() => {
    applyTheme(theme);
  }, [theme, systemDark]);

  const value = useMemo<ThemeContextValue>(
    () => ({
      theme,
      systemDark,
      setTheme: (next) => {
        localStorage.setItem(STORAGE_KEY, next);
        setThemeState(next);
        applyTheme(next);
      },
    }),
    [theme, systemDark],
  );

  return <ThemeContext.Provider value={value}>{children}</ThemeContext.Provider>;
}
