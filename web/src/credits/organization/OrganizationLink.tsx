import { Link } from "react-router";

import { t } from "../../i18n";
import { organizationPath } from "./organizationPath";

/** An organization's name as a text link to its page, as the video page and the family shelf show it. */
export function OrganizationLink({
  organization,
}: {
  organization: { id: number; name: string };
}) {
  return (
    <Link
      to={organizationPath(organization.id)}
      aria-label={t.credits.organizationPage.open(organization.name)}
      className="text-foreground hover:text-primary"
    >
      {organization.name}
    </Link>
  );
}
