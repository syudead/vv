import { Link } from "react-router";

import { t } from "../i18n";
import { settingsPagePath, type SettingsPageSlug } from "./settingsPages";

/** PageLinkSentence はページの名前を間に挟む文である（カタログの before と after）。 */
export interface PageLinkSentence {
  before: string;
  after: string;
}

/**
 * PageLinkText は、ほかの設定のページの名前をリンクにした 1 文である
 * （specs/058-settings-pages/ui-design.md「Words」）。リンクの文はページの名前で、
 * 節の中のほかの文中のリンクと同じ見た目にする。
 */
export function PageLinkText({
  sentence,
  page,
}: {
  sentence: PageLinkSentence;
  page: SettingsPageSlug;
}) {
  return (
    <>
      {sentence.before}
      <Link
        to={settingsPagePath(page)}
        className="text-primary underline underline-offset-4"
      >
        {t.settings.pages[page]}
      </Link>
      {sentence.after}
    </>
  );
}
