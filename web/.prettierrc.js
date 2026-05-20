/** @type {import("prettier").Config} */
export default {
  useTabs: false,
  tabWidth: 2,
  singleQuote: false,
  trailingComma: "all",
  printWidth: 120,
  plugins: ["@trivago/prettier-plugin-sort-imports"],
  importOrder: ["<BUILTIN_MODULES>", "<THIRD_PARTY_MODULES>", "^@/(.+)", "^[./]"],
  importOrderSeparation: true,
  importOrderSortSpecifiers: true,
};
