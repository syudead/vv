import { UserRoundX } from "lucide-react";
import { Link } from "react-router";

import { t } from "../../i18n";
import { EmptyState } from "../../ui/patterns/empty-state";
import { Button } from "../../ui/shadcn/button";

/** The person page's gone state, which the edit page shows too. */
export default function PersonGone() {
  const text = t.credits.personPage;
  return (
    <EmptyState
      icon={<UserRoundX aria-hidden="true" />}
      title={text.gone}
      action={
        <Button asChild variant="outline" size="sm">
          <Link to="/people">{text.backToPeople}</Link>
        </Button>
      }
    />
  );
}
