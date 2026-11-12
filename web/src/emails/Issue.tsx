import type { ReactNode } from "react";
import { Hr, Link, Section } from "react-email";

import { Layout } from "./Layout";
import { t, v } from "./gotmpl";

// Issue relays an issue or pull request comment, with a link to reply on the site.
export function Issue({ intro }: { intro?: ReactNode }) {
  return (
    <Layout preview={v(".Subject")}>
      {intro}
      {/* The body is HTML rendered from Markdown on the server. */}
      <Section className="text-sm leading-6">{v("Markup .Body")}</Section>
      <Hr className="my-4 border-[#e4e4e7]" />
      <Link href={v(".Link")} className="text-sm text-[#2563eb]">
        {t("view_on", "AppName")}
      </Link>
    </Layout>
  );
}
