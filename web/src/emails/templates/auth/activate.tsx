import { Action, Layout, Paragraph } from "../../Layout";
import { t, v } from "../../gotmpl";

const link = `${v("AppURL")}user/activate?code=${v(".Code")}`;

export default function Activate() {
  return (
    <Layout preview={t("activate_account")}>
      <Paragraph>{t("greeting", ".Username")}</Paragraph>
      <Paragraph>{t("activate_account.body", "AppName", ".ActiveCodeLives")}</Paragraph>
      <Action href={link}>{t("activate_account.action")}</Action>
    </Layout>
  );
}
