import { PencilLine } from "lucide-react";
import { memo } from "react";
import { Link } from "react-router";

import { type Person, personFaceSrc } from "../../api/credits";
import { t } from "../../i18n";
import { cn } from "../../lib/cn";
import FavoriteToggle from "../../ui/FavoriteToggle";
import { PersonFace, PersonFaceMark } from "../../ui/PersonFace";
import { Button } from "../../ui/shadcn/button";
import { creditDisplayName, creditInitials } from "../displayName";
import { personCardMeta } from "./peopleCriteria";

const text = t.credits.personPage;

/**
 * One link to the person page; the heart sits over the face and an unnamed person's `Name…` under
 * it, both outside the link.
 */
export const PersonCard = memo(function PersonCard({
  person,
  today,
  onToggleFavorite,
  onName,
  compact = false,
}: {
  person: Person;
  today: Date;
  onToggleFavorite: (person: Person) => Promise<unknown>;
  onName: (person: Person) => void;
  /** The smallest size shows the name alone, as the age and count would be cut off. */
  compact?: boolean;
}) {
  const src = personFaceSrc(person);
  const name = creditDisplayName(person);
  return (
    <article className="group relative flex min-w-0 flex-col">
      <Link
        to={`/people/${String(person.id)}`}
        className="flex min-w-0 flex-col items-center gap-2 rounded-md p-1 transition-shadow hover:shadow-card-hover motion-reduce:transition-none"
      >
        <PersonFace
          src={src}
          name={name}
          initials={creditInitials(person)}
          shape="card"
        />
        <span className="flex w-full min-w-0 flex-col items-center text-center">
          {/* An unnamed card differs from a named one by this quieter line alone. */}
          <span
            title={name}
            className={cn(
              "w-full truncate text-sm",
              person.provisional ? "text-muted-foreground" : "font-medium",
            )}
          >
            {name}
          </span>
          {!compact && (
            <span className="w-full truncate text-xs text-muted-foreground">
              {personCardMeta(person, today)}
            </span>
          )}
        </span>
      </Link>
      {person.provisional && (
        <Button
          variant="ghost"
          size="sm"
          className="self-center text-xs"
          onClick={() => onName(person)}
        >
          <PencilLine aria-hidden="true" />
          {t.credits.peopleGrid.nameButton}
        </Button>
      )}
      <PersonFaceMark corner="top-end">
        <FavoriteToggle
          favorite={person.favorite}
          label={person.favorite ? text.favoriteOn : text.favoriteOff}
          onToggle={() => onToggleFavorite(person)}
          variant="card"
        />
      </PersonFaceMark>
    </article>
  );
});
