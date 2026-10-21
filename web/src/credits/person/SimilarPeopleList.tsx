import { useEffect, useId, useState } from "react";

import { personImagePath, type Person } from "../../api/credits";
import {
  faceCropPath,
  listSimilarPeople,
  type SimilarPerson,
} from "../../api/personFaces";
import { t } from "../../i18n";
import { cn } from "../../lib/cn";
import { Button } from "../../ui/shadcn/button";
import { Label } from "../../ui/shadcn/label";
import { RadioGroup, RadioGroupItem } from "../../ui/shadcn/radio-group";
import { PersonFace } from "../../ui/PersonFace";
import { creditDisplayName } from "../displayName";

const text = t.credits.similarPeople;

/** What the similar people read gives: the people, null while loading, and whether it failed, with a retry. */
export interface SimilarPeople {
  items: SimilarPerson[] | null;
  failed: boolean;
  retry: () => void;
}

/**
 * People whose faces look like the given faces of the person, or all of them; empty without a person. A failed read
 * leaves the items empty and says so, so it is not taken for no one alike.
 */
export function useSimilarPeople(
  personId: number | null,
  appearanceIds: readonly number[],
): SimilarPeople {
  const [items, setItems] = useState<SimilarPerson[] | null>(
    personId === null ? [] : null,
  );
  const [failed, setFailed] = useState(false);
  const [attempt, setAttempt] = useState(0);
  const key = appearanceIds.join(",");
  useEffect(() => {
    if (personId === null) return undefined;
    const controller = new AbortController();
    setItems(null);
    setFailed(false);
    const ids = key === "" ? [] : key.split(",").map(Number);
    listSimilarPeople(personId, ids, controller.signal).then(
      (next) => {
        if (!controller.signal.aborted) setItems(next);
      },
      () => {
        if (controller.signal.aborted) return;
        setItems([]);
        setFailed(true);
      },
    );
    return () => controller.abort();
  }, [personId, key, attempt]);
  return { items, failed, retry: () => setAttempt((value) => value + 1) };
}

/** Whether to show the people by face rather than the name results: before a name is typed, unless no one is alike. */
export function suggestsByFace(query: string, similar: SimilarPeople): boolean {
  const { items, failed } = similar;
  return query.trim() === "" && (items === null || items.length > 0 || failed);
}

/**
 * People ranked by face, to pick one: each row shows the face, a few more crops, the face count, the first video and
 * the similarity, so even people without a name can be told apart.
 */
export function SimilarPeopleList({
  heading,
  similar,
  selectedId,
  onSelect,
}: {
  heading: string;
  similar: SimilarPeople;
  selectedId: number | undefined;
  onSelect: (person: Person) => void;
}) {
  const headingId = useId();
  const { items } = similar;
  if (similar.failed) {
    return (
      <p className="flex flex-wrap items-center gap-2 text-sm text-muted-foreground">
        {text.failed}
        <Button type="button" variant="link" size="sm" onClick={similar.retry}>
          {t.common.retry}
        </Button>
      </p>
    );
  }
  if (items === null)
    return <p className="text-sm text-muted-foreground">{text.loading}</p>;
  if (items.length === 0)
    return <p className="text-sm text-muted-foreground">{text.none}</p>;
  return (
    <div className="flex flex-col gap-2">
      <span id={headingId} className="text-xs font-semibold text-muted-foreground">
        {heading}
      </span>
      <RadioGroup
        aria-labelledby={headingId}
        value={selectedId === undefined ? "" : String(selectedId)}
        onValueChange={(value) => {
          const picked = items.find((item) => String(item.person.id) === value);
          if (picked !== undefined) onSelect(picked.person);
        }}
        className="gap-2"
      >
        {items.map((item) => (
          <SimilarPersonRow key={item.person.id} item={item} />
        ))}
      </RadioGroup>
    </div>
  );
}

function SimilarPersonRow({ item }: { item: SimilarPerson }) {
  const itemId = useId();
  const { person } = item;
  const name = creditDisplayName(person);
  return (
    <div className="flex min-w-0 items-center gap-3 rounded-md border border-border p-2 has-[[data-state=checked]]:border-primary has-[[data-state=checked]]:bg-primary-soft">
      <RadioGroupItem id={itemId} value={String(person.id)} />
      <Label
        htmlFor={itemId}
        className="flex min-w-0 flex-1 items-center gap-3 font-normal"
      >
        <PersonFace
          shape="suggestion"
          name={name}
          src={
            person.imageRevision === undefined
              ? undefined
              : personImagePath(person.id, person.imageRevision)
          }
        />
        <span className="flex min-w-0 flex-1 flex-col gap-0.5">
          <span
            className={cn(
              "truncate font-medium",
              person.provisional && "text-muted-foreground",
            )}
          >
            {name}
          </span>
          <span className="truncate text-xs text-muted-foreground">
            {text.meta(item.faceCount, item.firstSeenTitle)}
          </span>
        </span>
        {/* The face and name already say who this is; the crops add only what they look like. */}
        <span aria-hidden="true" className="hidden gap-1 sm:flex">
          {item.sampleAppearanceIds.map((id) => (
            <span key={id} className="size-9">
              <PersonFace shape="scene" name={name} src={faceCropPath(id)} />
            </span>
          ))}
        </span>
        <span className="text-sm font-semibold tabular-nums">
          {text.percent(item.similarity)}
        </span>
      </Label>
    </div>
  );
}
