import { Action, Layout, Paragraph } from "../../Layout";
import { t, v } from "../../gotmpl";

const link = `${v("AppURL")}user/activate_email?code=${v(".Code")}&email=${v(".Email")}`;

export default function ActivateEmail() {
  return (
    <Layout preview={t("activate_email")}>
      <Paragraph>{t("greeting", ".Username")}</Paragraph>
      <Paragraph>{t("activate_email.body", ".Email", ".ActiveCodeLives")}</Paragraph>
      <Action href={link}>{t("activate_email.action")}</Action>
    </Layout>
  );
}
