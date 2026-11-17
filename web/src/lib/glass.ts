// The frosted glass look is off unless the user switched it on in the
// settings menu. The inline script in index.html applies the same rule before
// the first paint.
const STORAGE_KEY = "gogs-glass";

export function readGlass(): boolean {
  return localStorage.getItem(STORAGE_KEY) === "on";
}

export function setGlass(on: boolean) {
  localStorage.setItem(STORAGE_KEY, on ? "on" : "off");
  document.documentElement.classList.toggle("glass-ui", on);
}
