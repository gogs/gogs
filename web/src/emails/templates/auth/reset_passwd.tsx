import { Action, Footnote, Layout, Paragraph } from "../../Layout";
import { t, v } from "../../gotmpl";

const link = `${v("AppURL")}user/reset-password?code=${v(".Code")}`;

export default function ResetPassword() {
  return (
    <Layout preview={t("reset_password")}>
      <Paragraph>{t("greeting", ".Username")}</Paragraph>
      <Paragraph>{t("reset_password.body", "AppName", ".ResetPwdCodeLives")}</Paragraph>
      <Action href={link}>{t("reset_password.action")}</Action>
      <Footnote>{t("reset_password.ignore")}</Footnote>
    </Layout>
  );
}
