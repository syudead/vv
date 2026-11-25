import { Ellipsis, LibraryBig, Merge, Pencil, Trash2 } from "lucide-react";
import type { ComponentProps, ReactNode } from "react";
import { Link } from "react-router";

import type { Person } from "../../api/credits";
import { t } from "../../i18n";
import { personEditPath } from "../../lib/personEditPath";
import { isPlainClick } from "../../lib/plainClick";
import { libraryFilterPath } from "../../library/libraryPath";
import { Button } from "../../ui/shadcn/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "../../ui/shadcn/dropdown-menu";
import { Bio, creditFactsCard, CreditHeading } from "../CreditProfile";
import { kindTexts } from "../creditRows";
import { creditDisplayName } from "../displayName";
import { type PersonFigure, personFigures, profileFacts } from "./personFacts";
import { handPerson } from "./personHandoff";

const text = t.credits.personPage;

/**
 * Opens the edit page with this person, which returns along the history when it is left. A click
 * that opens another tab hands nothing, so this tab's next visit to the person reads it again.
 */
function EditLink({
  person,
  onClick,
  ...props
}: Omit<ComponentProps<typeof Link>, "to" | "state"> & { person: Person }) {
  // Button asChild passes its class and handlers through these props.
  return (
    <Link
      {...props}
      to={personEditPath(person.id)}
      state={{ fromPage: true }}
      onClick={(event) => {
        onClick?.(event);
        if (isPlainClick(event)) handPerson(person);
      }}
    />
  );
}

/** One key number above its meaning, the numbers in a row that wraps. */
function Figures({ figures }: { figures: readonly PersonFigure[] }) {
  if (figures.length === 0) return null;
  return (
    <dl className="flex flex-wrap gap-x-8 gap-y-3">
      {figures.map((figure) => (
        <div key={figure.id} className="flex flex-col-reverse gap-0.5">
          <dt className="text-xs text-muted-foreground">{figure.label}</dt>
          <dd className="text-lg font-semibold tabular-nums">{figure.value}</dd>
        </div>
      ))}
    </dl>
  );
}

/**
 * The name, aliases, key figures, bio and actions; the actions are `secondary` with an icon, since
 * nothing on the page outranks viewing.
 */
export function PersonIdentity({
  person,
  today,
  fetchEntry,
  facesEntry,
  onMerge,
  onDelete,
}: {
  person: Person;
  today: Date;
  /** Fetch info…, after Edit; absent while no approved person plugin exists. */
  fetchEntry?: ReactNode;
  /** The way to the person's face recognition, after Show in library. */
  facesEntry?: ReactNode;
  onMerge: () => void;
  onDelete: () => void;
}) {
  const bio = person.profile.bio;
  return (
    <>
      <CreditHeading
        name={creditDisplayName(person)}
        names={person.names}
        muted={person.provisional}
      />
      <Figures figures={personFigures(person.profile, today)} />
      {bio !== undefined && <Bio bio={bio} />}
      <div className="flex flex-wrap items-center gap-2">
        {/* The edit page focuses the name input, so Name… is Edit for a person without one. */}
        <Button asChild variant="secondary" size="sm">
          <EditLink person={person}>
            <Pencil aria-hidden="true" />
            {person.provisional ? text.name : text.edit}
          </EditLink>
        </Button>
        {fetchEntry}
        <Button asChild variant="secondary" size="sm">
          <Link to={libraryFilterPath("person", person.id)}>
            <LibraryBig aria-hidden="true" />
            {text.showInLibrary}
          </Link>
        </Button>
        {facesEntry}
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button variant="ghost" size="icon-sm" aria-label={text.more}>
              <Ellipsis aria-hidden="true" />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end">
            <DropdownMenuItem onSelect={onMerge}>
              <Merge aria-hidden="true" />
              {kindTexts("person").mergeInto}
            </DropdownMenuItem>
            <DropdownMenuSeparator />
            <DropdownMenuItem variant="destructive" onSelect={onDelete}>
              <Trash2 aria-hidden="true" />
              {text.delete}
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>
    </>
  );
}

/**
 * The profile card: the remaining facts, then links and external IDs; undefined when the spread
 * already shows everything the person has.
 */
export function personFactsCard(person: Person, today: Date): ReactNode {
  const facts = profileFacts(person.profile, today);
  const links = person.profile.links ?? [];
  return creditFactsCard({
    facts,
    links,
    externalIds: person.externalIds,
    empty:
      facts.length === 0 &&
      links.length === 0 &&
      person.externalIds.length === 0 &&
      personFigures(person.profile, today).length === 0 &&
      person.profile.bio === undefined,
    edit: (
      <Button asChild variant="link" size="sm">
        <EditLink person={person}>{text.edit}</EditLink>
      </Button>
    ),
  });
}
