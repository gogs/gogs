import { Issue } from "../../Issue";
import { Paragraph } from "../../Layout";
import { t } from "../../gotmpl";

export default function Mention() {
  return <Issue intro={<Paragraph>{t("issue.mentioned", ".Doer.DisplayName")}</Paragraph>} />;
}
