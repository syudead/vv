import { useCallback, useEffect, useState } from "react";
import { Link } from "react-router";

import { isAborted } from "../../api/client";
import { isCreditGone, type Person, personImagePath } from "../../api/credits";
import {
  dismissFaceCandidate,
  type FaceCandidate,
  type FaceCandidatePerson,
  isCandidateGone,
  listFaceCandidates,
} from "../../api/personFaces";
import { errorText, t, type UiText } from "../../i18n";
import { cn } from "../../lib/cn";
import { Button } from "../../ui/shadcn/button";
import { Spinner } from "../../ui/shadcn/spinner";
import { PersonFace } from "../../ui/PersonFace";
import { creditDisplayName } from "../displayName";
import { personPath } from "./personPath";

const text = t.credits.personPage.candidates;

/**
 * The person's open candidate pairs. A failed read draws nothing, since the group is only there
 * when there is something to decide; a missing person calls onGone.
 */
export function useFaceCandidates(personId: number, onGone: () => void) {
  const [items, setItems] = useState<FaceCandidate[]>([]);
  const [attempt, setAttempt] = useState(0);

  useEffect(() => {
    const controller = new AbortController();
    listFaceCandidates(personId, controller.signal).then(
      (next) => {
        if (!controller.signal.aborted) setItems(next);
      },
      (failure: unknown) => {
        if (controller.signal.aborted || isAborted(failure)) return;
        if (isCreditGone(failure)) onGone();
      },
    );
    return () => controller.abort();
  }, [personId, attempt, onGone]);

  const reload = useCallback(() => setAttempt((value) => value + 1), []);
  const remove = useCallback((otherId: number) => {
    setItems((current) => current.filter((item) => item.person.id !== otherId));
  }, []);
  return { items, reload, remove };
}

export interface CandidateActions {
  /** Confirms the pair and updates the page; a rejection is shown under the row. */
  confirm: (other: FaceCandidate["person"]) => Promise<void>;
  remove: (otherId: number) => void;
  onGone: () => void;
}

/**
 * `Same person?`: one row per pair with both faces side by side, decided in place, as the rows of
 * the caller's section.
 */
export function PersonCandidates({
  person,
  items,
  actions,
}: {
  person: Person;
  items: readonly FaceCandidate[];
  actions: CandidateActions;
}) {
  return items.map((item) => (
    <CandidateRow
      key={item.person.id}
      shown={person}
      candidate={item}
      actions={actions}
    />
  ));
}

type Pending = "confirm" | "dismiss" | null;

function CandidateRow({
  shown,
  candidate,
  actions,
}: {
  shown: Person;
  candidate: FaceCandidate;
  actions: CandidateActions;
}) {
  const other = candidate.person;
  const name = creditDisplayName(other);
  const [pending, setPending] = useState<Pending>(null);
  const [error, setError] = useState<UiText | null>(null);

  async function run(kind: Exclude<Pending, null>) {
    if (pending !== null) return;
    setPending(kind);
    setError(null);
    try {
      if (kind === "confirm") await actions.confirm(other);
      else {
        await dismissFaceCandidate(shown.id, other.id);
        actions.remove(other.id);
      }
    } catch (failure) {
      setPending(null);
      if (isCandidateGone(failure)) actions.remove(other.id);
      else if (isCreditGone(failure)) actions.onGone();
      else
        setError(
          (kind === "confirm" ? text.confirmFailed : text.dismissFailed)(
            errorText(failure),
          ),
        );
    }
  }

  const buttons = [
    {
      kind: "confirm",
      variant: "default",
      label: text.confirm,
      name: text.confirmFor(name),
    },
    {
      kind: "dismiss",
      variant: "outline",
      label: text.dismiss,
      name: text.dismissFor(name),
    },
  ] as const;
  return (
    <div className="flex flex-col gap-3">
      <div className="flex flex-wrap items-center gap-x-6 gap-y-3">
        <div className="flex items-start gap-3">
          <PairFace person={shown} />
          <span className="self-center text-xs whitespace-nowrap text-muted-foreground">
            {text.similarity(candidate.similarity)}
          </span>
          <PairFace person={other} link />
        </div>
        <div className="flex flex-wrap gap-2">
          {buttons.map((button) => (
            <Button
              key={button.kind}
              variant={button.variant}
              aria-label={button.name}
              aria-disabled={pending !== null || undefined}
              aria-busy={pending === button.kind || undefined}
              onClick={() => void run(button.kind)}
            >
              {pending === button.kind && <Spinner aria-hidden="true" />}
              {button.label}
            </Button>
          ))}
        </div>
      </div>
      {error !== null && <p className="text-sm text-destructive">{error}</p>}
    </div>
  );
}

/** One side of a pair: the face over the name, the other person's name opening their page. */
function PairFace({
  person,
  link = false,
}: {
  person: Person | FaceCandidatePerson;
  link?: boolean;
}) {
  const name = creditDisplayName(person);
  const nameClass = cn(
    "w-full truncate text-center text-xs",
    person.provisional && "text-muted-foreground",
  );
  return (
    <div className="flex w-card-face flex-col items-center gap-1">
      <PersonFace
        shape="judge"
        name={name}
        src={
          person.imageRevision === undefined
            ? undefined
            : personImagePath(person.id, person.imageRevision)
        }
      />
      {link ? (
        <Link
          to={personPath(person.id)}
          aria-label={text.open(name)}
          title={name}
          className={cn(nameClass, "hover:underline")}
        >
          {name}
        </Link>
      ) : (
        <span title={name} className={nameClass}>
          {name}
        </span>
      )}
    </div>
  );
}
