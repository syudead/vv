import { UserRoundX } from "lucide-react";

import { t } from "../../i18n";
import { CreditGone } from "../CreditGone";

/** The person page's gone state, which the edit page shows too. */
export default function PersonGone() {
  const text = t.credits.personPage;
  return (
    <CreditGone
      icon={<UserRoundX aria-hidden="true" />}
      title={text.gone}
      backLabel={text.backToPeople}
      backTo="/people"
    />
  );
}
