import { Action, Layout, Paragraph } from "../../Layout";
import { t, v } from "../../gotmpl";

export default function RegisterNotify() {
  return (
    <Layout preview={t("register_notify")}>
      <Paragraph>{t("greeting", ".Username")}</Paragraph>
      <Paragraph>{t("register_notify.body", "AppName", ".Username")}</Paragraph>
      <Action href={`${v("AppURL")}user/sign-in`}>{t("register_notify.action")}</Action>
    </Layout>
  );
}
