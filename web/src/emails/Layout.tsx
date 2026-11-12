import type { ReactNode } from "react";
import { Body, Button, Container, Head, Hr, Html, Link, Preview, Section, Tailwind, Text } from "react-email";

import { t, v } from "./gotmpl";

// Layout frames every email with the site name and a footer. The preview is
// the line that inboxes show after the subject.
export function Layout({ preview, children }: { preview: string; children: ReactNode }) {
  return (
    <Html lang={v("Lang")}>
      <Head>
        <meta name="color-scheme" content="light" />
      </Head>
      <Preview>{preview}</Preview>
      <Tailwind>
        <Body className="m-0 bg-[#f4f4f5] px-2 py-8 font-sans text-[#18181b]" lang={v("Lang")}>
          <Container className="max-w-[560px] rounded-lg border border-solid border-[#e4e4e7] bg-white px-8 py-6">
            <Text className="m-0 text-base font-semibold">{v("AppName")}</Text>
            <Hr className="my-4 border-[#e4e4e7]" />
            {children}
          </Container>
          <Text className="text-center text-xs text-[#71717a]">
            © {v("Year")}{" "}
            <Link href={v("AppURL")} className="text-[#71717a] underline">
              {v("AppName")}
            </Link>
          </Text>
        </Body>
      </Tailwind>
    </Html>
  );
}

export function Paragraph({ children }: { children: ReactNode }) {
  return <Text className="my-4 text-sm leading-6">{children}</Text>;
}

// Action is the one thing the email asks the reader to do. Mail clients that
// block buttons fall back to the link written out below it.
export function Action({ href, children }: { href: string; children: ReactNode }) {
  return (
    <>
      <Section className="my-6">
        <Button href={href} className="rounded-md bg-[#2563eb] px-5 py-3 text-sm font-semibold text-white">
          {children}
        </Button>
      </Section>
      <Text className="my-4 text-xs leading-5 text-[#71717a]" data-skip-in-text>
        {t("link_fallback")}
        <br />
        <Link href={href} className="break-all text-[#2563eb]">
          {href}
        </Link>
      </Text>
    </>
  );
}

// Footnote follows the action, e.g., to say what to do when the reader did not
// ask for the email.
export function Footnote({ children }: { children: ReactNode }) {
  return <Text className="my-4 text-xs leading-5 text-[#71717a]">{children}</Text>;
}
