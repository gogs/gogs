import { Action, Layout, Paragraph } from "../../Layout";
import { t, v } from "../../gotmpl";

export default function Collaborator() {
  return (
    <Layout preview={v(".Subject")}>
      <Paragraph>{t("collaborator.body", ".RepoName")}</Paragraph>
      <Action href={v(".Link")}>{t("view_repository")}</Action>
    </Layout>
  );
}
