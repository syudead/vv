import { memo } from "react";
import { Link } from "react-router";

import { type Organization, organizationLogoSrc } from "../../api/credits";
import { t } from "../../i18n";
import FavoriteToggle from "../../ui/FavoriteToggle";
import { OrganizationLogo, OrganizationLogoMark } from "../../ui/OrganizationLogo";
import { organizationPath } from "../organization/organizationPath";

const text = t.credits.personPage;

/** One link to the organization page; the heart sits over the logo but outside the link. */
export const OrganizationCard = memo(function OrganizationCard({
  organization,
  onToggleFavorite,
}: {
  organization: Organization;
  onToggleFavorite: (organization: Organization) => Promise<unknown>;
}) {
  const name = organization.name;
  return (
    <article className="group relative flex min-w-0 flex-col">
      <Link
        to={organizationPath(organization.id)}
        className="flex min-w-0 flex-col gap-2 rounded-md p-1 transition-shadow hover:shadow-card-hover motion-reduce:transition-none"
      >
        <OrganizationLogo
          src={organizationLogoSrc(organization)}
          alt={t.credits.details.logoAlt(name)}
          name={name}
          shape="card"
        />
        <span className="flex w-full min-w-0 flex-col">
          <span title={name} className="w-full truncate text-sm font-medium">
            {name}
          </span>
          <span className="w-full truncate text-xs text-muted-foreground tabular-nums">
            {t.credits.row.videoCount(organization.videoCount)}
          </span>
        </span>
      </Link>
      <OrganizationLogoMark corner="top-end">
        <FavoriteToggle
          favorite={organization.favorite}
          label={organization.favorite ? text.favoriteOn : text.favoriteOff}
          onToggle={() => onToggleFavorite(organization)}
          variant="card"
        />
      </OrganizationLogoMark>
    </article>
  );
});
