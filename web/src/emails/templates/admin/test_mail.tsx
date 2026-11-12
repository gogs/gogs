import { Layout, Paragraph } from "../../Layout";
import { t } from "../../gotmpl";

export default function TestMail() {
  return (
    <Layout preview={t("test_mail", "AppName")}>
      <Paragraph>{t("test_mail.body", "AppName")}</Paragraph>
    </Layout>
  );
}
