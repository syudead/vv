import { type Person, personFaceSrc } from "../../../api/credits";
import { t } from "../../../i18n";
import { BackLink } from "../../../player/detailsEditor/EditorAside";
import { PersonFace } from "../../../ui/PersonFace";
import { Skeleton } from "../../../ui/shadcn/skeleton";
import { ageRange } from "../personFacts";
import { useLocalToday } from "../useLocalToday";

const text = t.credits.personEdit;

function meta(person: Person, today: Date): string {
  const { birthDate, deathDate } = person.profile;
  const range = birthDate === undefined ? null : ageRange(birthDate, deathDate, today);
  const parts = [t.credits.row.videoCount(person.videoCount)];
  if (range !== null) parts.unshift(t.credits.personPage.ageRange(range.low, range.high));
  return text.meta(parts);
}

/** The person being edited, read only; the face stays small until the aside has its own column. */
export default function PersonEditAside({
  person,
  backTo,
  onBack,
}: {
  person: Person | undefined;
  backTo: string;
  onBack: () => void;
}) {
  const today = useLocalToday();
  return (
    <>
      <div className="w-1/3 md:w-1/4 lg:w-full">
        {person === undefined ? (
          <Skeleton className="aspect-portrait w-full rounded-md" />
        ) : (
          <PersonFace shape="profile" name={person.name} src={personFaceSrc(person)} />
        )}
      </div>
      {person === undefined ? (
        <>
          <Skeleton className="h-4 w-3/4" />
          <Skeleton className="h-4 w-1/2" />
        </>
      ) : (
        <>
          <p className="font-medium wrap-anywhere">{person.name}</p>
          <p className="text-muted-foreground tabular-nums">{meta(person, today)}</p>
        </>
      )}
      <BackLink to={backTo} label={text.back} onBack={onBack} />
    </>
  );
}
