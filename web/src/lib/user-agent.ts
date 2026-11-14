// describeUserAgent names the browser and the operating system of a user
// agent string, e.g., "Firefox" and "macOS", or null for those it does not
// recognize.
export function describeUserAgent(ua: string): { browser: string | null; os: string | null } {
  // Order matters: Edge and Opera also claim to be Chrome, and Chrome claims to
  // be Safari.
  const browsers: [RegExp, string][] = [
    [/Edg(?:e|A|iOS)?\//, "Edge"],
    [/OPR\/|Opera/, "Opera"],
    [/Firefox\/|FxiOS\//, "Firefox"],
    [/Chrome\/|CriOS\//, "Chrome"],
    [/Safari\//, "Safari"],
  ];
  const systems: [RegExp, string][] = [
    [/iPhone|iPad|iPod/, "iOS"],
    [/Android/, "Android"],
    [/CrOS/, "ChromeOS"],
    [/Mac OS X|Macintosh/, "macOS"],
    [/Windows/, "Windows"],
    [/Linux/, "Linux"],
  ];
  return {
    browser: browsers.find(([re]) => re.test(ua))?.[1] ?? null,
    os: systems.find(([re]) => re.test(ua))?.[1] ?? null,
  };
}
