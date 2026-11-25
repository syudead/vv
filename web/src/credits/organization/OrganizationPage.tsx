import { Building2, LibraryBig, Pencil } from "lucide-react";
import { Link, useParams } from "react-router";

import { useAssistantScreen } from "../../assistant/AssistantProvider";
import { type Organization, organizationLogoSrc } from "../../api/credits";
import { t } from "../../i18n";
import { libraryFilterPath } from "../../library/libraryPath";
import { OrganizationLogo } from "../../ui/OrganizationLogo";
import type { Fact } from "../../ui/patterns/fact-list";
import { ProfilePage } from "../../ui/patterns/profile-page";
import { Button } from "../../ui/shadcn/button";
import { CreditGone } from "../CreditGone";
import { CreditPageStatus } from "../CreditPageStatus";
import { Bio, creditFactsCard, CreditHeading } from "../CreditProfile";
import { CreditShelves, type ShelfSubject } from "../CreditShelves";
import { parseCreditId, useRoutedCredit } from "../useCreditRecord";
import { OrganizationFamily } from "./OrganizationFamily";
import { organizationEditPath } from "./organizationPath";

const text = t.credits.organizationPage;
const personText = t.credits.personPage;
const details = t.credits.details;

const roles = [
  { role: "maker", title: text.roles.maker },
  { role: "label", title: text.roles.label },
] as const;

/** `/organizations/:id`: the logo and profile, then the parent and children, then one shelf per role. */
export default function OrganizationPage() {
  const id = parseCreditId(useParams().id);
  // A new id (after following a parent or child) starts from a fresh read.
  return <OrganizationScreen key={id ?? "none"} id={id} />;
}

/** The page's gone state, which the edit page shows too. */
export function OrganizationGone() {
  return (
    <CreditGone
      icon={<Building2 aria-hidden="true" />}
      title={text.gone}
      backLabel={text.backToOrganizations}
      backTo="/organizations"
    />
  );
}

function OrganizationScreen({ id }: { id: number | null }) {
  const { state, record, reload, markGone } = useRoutedCredit<Organization>(
    "organization",
    id,
    undefined,
    (organization) => organization.name,
  );
  useAssistantScreen({ name: record?.name });

  if (state.kind !== "ready") {
    return (
      <CreditPageStatus
        state={state}
        profileLabel={text.profile}
        faceClassName="aspect-square w-full"
        failedTitle={text.loadFailed}
        gone={<OrganizationGone />}
        onRetry={reload}
      />
    );
  }
  return <OrganizationShown organization={state.credit} markGone={markGone} />;
}

function shelfSubject(id: number): ShelfSubject {
  return { kind: "organization", id, roles };
}

function profileFacts(organization: Organization): Fact[] {
  const { foundedDate, dissolvedDate, location } = organization.profile;
  const facts: Fact[] = [];
  const date = (value: string) => <span className="tabular-nums">{value}</span>;
  if (foundedDate !== undefined)
    facts.push({ id: "founded", term: details.foundedDate, value: date(foundedDate) });
  if (dissolvedDate !== undefined)
    facts.push({
      id: "dissolved",
      term: details.dissolvedDate,
      value: date(dissolvedDate),
    });
  if (location !== undefined)
    facts.push({ id: "location", term: details.location, value: location });
  return facts;
}

function factsCard(organization: Organization) {
  const facts = profileFacts(organization);
  const links = organization.profile.links ?? [];
  return creditFactsCard({
    facts,
    links,
    externalIds: organization.externalIds,
    empty:
      facts.length === 0 &&
      links.length === 0 &&
      organization.externalIds.length === 0 &&
      organization.profile.bio === undefined,
    edit: (
      <Button asChild variant="link" size="sm">
        <Link to={organizationEditPath(organization.id)} state={{ fromPage: true }}>
          {personText.edit}
        </Link>
      </Button>
    ),
  });
}

/** The name, other names, bio and actions; the actions are `secondary`, since nothing outranks viewing. */
function OrganizationIdentity({ organization }: { organization: Organization }) {
  const bio = organization.profile.bio;
  return (
    <>
      <CreditHeading name={organization.name} names={organization.names} />
      {bio !== undefined && <Bio bio={bio} />}
      <div className="flex flex-wrap items-center gap-2">
        <Button asChild variant="secondary" size="sm">
          <Link to={organizationEditPath(organization.id)} state={{ fromPage: true }}>
            <Pencil aria-hidden="true" />
            {personText.edit}
          </Link>
        </Button>
        <Button asChild variant="secondary" size="sm">
          <Link to={libraryFilterPath("organization", organization.id)}>
            <LibraryBig aria-hidden="true" />
            {personText.showInLibrary}
          </Link>
        </Button>
      </div>
    </>
  );
}

function OrganizationShown({
  organization,
  markGone,
}: {
  organization: Organization;
  markGone: () => void;
}) {
  return (
    <ProfilePage
      profileLabel={text.profile}
      face={
        <OrganizationLogo
          shape="profile"
          src={organizationLogoSrc(organization)}
          alt={details.logoAlt(organization.name)}
        />
      }
      identity={<OrganizationIdentity organization={organization} />}
      facts={factsCard(organization)}
    >
      <OrganizationFamily organization={organization} onGone={markGone} />
      <CreditShelves subject={shelfSubject(organization.id)} keepFailedRole />
    </ProfilePage>
  );
}
