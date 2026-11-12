// The emails render once at build time into Go templates, which the server
// fills in with each email's data. These helpers write the Go template
// actions. Strings in actions use backticks because React escapes quotes.

// v writes a Go template pipeline, e.g., `v(".Username")` or `v("AppName")`.
export const v = (pipeline: string) => `{{${pipeline}}}`;

// t writes a translation of a key in the [mail] section of the locale files,
// with Go template pipelines as its arguments, e.g., `t("greeting", ".Username")`.
export const t = (key: string, ...args: string[]) => `{{Tr \`mail.${key}\`${args.map((arg) => " " + arg).join("")}}}`;
