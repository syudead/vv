import { Link } from "react-router";

import { t } from "../i18n";
import { settingsPagePath, type SettingsPageSlug } from "./settingsPages";

export interface PageLinkSentence {
  before: string;
  after: string;
}

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
