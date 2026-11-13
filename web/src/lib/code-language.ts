// CodeLanguage names a language CodeEditor highlights. "plain" is none.
export type CodeLanguage =
  | "markdown"
  | "shell"
  | "go"
  | "javascript"
  | "typescript"
  | "json"
  | "python"
  | "yaml"
  | "toml"
  | "css"
  | "scss"
  | "less"
  | "xml"
  | "html"
  | "rust"
  | "sql"
  | "ruby"
  | "c"
  | "cpp"
  | "java"
  | "csharp"
  | "kotlin"
  | "scala"
  | "swift"
  | "dockerfile"
  | "properties"
  | "diff"
  | "lua"
  | "perl"
  | "nginx"
  | "protobuf"
  | "powershell"
  | "cmake"
  | "haskell"
  | "erlang"
  | "r"
  | "groovy"
  | "plain";

const BY_EXTENSION: Record<string, CodeLanguage> = {
  sh: "shell",
  bash: "shell",
  zsh: "shell",
  go: "go",
  js: "javascript",
  mjs: "javascript",
  cjs: "javascript",
  jsx: "javascript",
  ts: "typescript",
  mts: "typescript",
  cts: "typescript",
  tsx: "typescript",
  json: "json",
  py: "python",
  yml: "yaml",
  yaml: "yaml",
  toml: "toml",
  css: "css",
  scss: "scss",
  less: "less",
  xml: "xml",
  svg: "xml",
  html: "html",
  htm: "html",
  tmpl: "html",
  rs: "rust",
  sql: "sql",
  rb: "ruby",
  c: "c",
  h: "c",
  cc: "cpp",
  cpp: "cpp",
  cxx: "cpp",
  hpp: "cpp",
  java: "java",
  cs: "csharp",
  kt: "kotlin",
  kts: "kotlin",
  scala: "scala",
  swift: "swift",
  ini: "properties",
  properties: "properties",
  conf: "properties",
  diff: "diff",
  patch: "diff",
  lua: "lua",
  pl: "perl",
  pm: "perl",
  proto: "protobuf",
  ps1: "powershell",
  cmake: "cmake",
  hs: "haskell",
  erl: "erlang",
  r: "r",
  groovy: "groovy",
  gradle: "groovy",
};

const BY_NAME: Record<string, CodeLanguage> = {
  dockerfile: "dockerfile",
  "cmakelists.txt": "cmake",
  "nginx.conf": "nginx",
  ".bashrc": "shell",
  ".profile": "shell",
};

// codeLanguageOf picks the language of a file by its name. Markdown follows the
// extensions the server renders as Markdown, e.g., ".md".
export function codeLanguageOf(fileName: string, markdownExtensions: string[]): CodeLanguage {
  const name = fileName.split("/").at(-1)?.toLowerCase() ?? "";
  const dot = name.lastIndexOf(".");
  const ext = dot > 0 ? name.slice(dot) : "";
  if (ext && markdownExtensions.some((e) => e.toLowerCase() === ext)) return "markdown";
  return BY_NAME[name] ?? BY_EXTENSION[ext.slice(1)] ?? "plain";
}
