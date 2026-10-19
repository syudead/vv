import { memo } from "react";
import { Link } from "react-router";

import { type Person, personFaceSrc } from "../../api/credits";
import { t } from "../../i18n";
import { cn } from "../../lib/cn";
import FavoriteToggle from "../../ui/FavoriteToggle";
import { PersonFace, PersonFaceMark } from "../../ui/PersonFace";
import { creditDisplayName } from "../displayName";
import { personCardMeta } from "./peopleCriteria";

const text = t.credits.personPage;

/** One link to the person page; the heart sits over the face but outside the link. */
export const PersonCard = memo(function PersonCard({
  person,
  today,
  onToggleFavorite,
}: {
  person: Person;
  today: Date;
  onToggleFavorite: (person: Person) => Promise<unknown>;
}) {
  const src = personFaceSrc(person);
  const name = creditDisplayName(person);
  return (
    <article className="group relative flex min-w-0 flex-col">
      <Link
        to={`/people/${String(person.id)}`}
        className="flex min-w-0 flex-col items-center gap-2 rounded-md p-1 transition-shadow hover:shadow-card-hover motion-reduce:transition-none"
      >
        <PersonFace src={src} name={name} shape="card" />
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
          <span className="w-full truncate text-xs text-muted-foreground">
            {personCardMeta(person, today)}
          </span>
        </span>
      </Link>
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
