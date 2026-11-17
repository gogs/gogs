// The frosted glass look is on unless the user switched it off in the settings
// menu, or has not chosen and their OS asks for reduced transparency. The
// inline script in index.html applies the same rule before the first paint.
const STORAGE_KEY = "gogs-glass";

export function readGlass(): boolean {
  const saved = localStorage.getItem(STORAGE_KEY);
  if (saved === "on" || saved === "off") return saved === "on";
  return !window.matchMedia("(prefers-reduced-transparency: reduce)").matches;
}

export function setGlass(on: boolean) {
  localStorage.setItem(STORAGE_KEY, on ? "on" : "off");
  document.documentElement.classList.toggle("glass-ui", on);
}
