import { useCallback, useEffect, useState } from "react";
import { Link } from "react-router";

import { isAborted } from "../../api/client";
import { isCreditGone, personImagePath } from "../../api/credits";
import {
  dismissFaceCandidate,
  type FaceCandidate,
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

/** `Same person?`: one row per pair, decided in place, as the rows of the caller's section. */
export function PersonCandidates({
  personId,
  items,
  actions,
}: {
  personId: number;
  items: readonly FaceCandidate[];
  actions: CandidateActions;
}) {
  return items.map((item) => (
    <CandidateRow
      key={item.person.id}
      personId={personId}
      candidate={item}
      actions={actions}
    />
  ));
}

type Pending = "confirm" | "dismiss" | null;

function CandidateRow({
  personId,
  candidate,
  actions,
}: {
  personId: number;
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
        await dismissFaceCandidate(personId, other.id);
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
      variant: "outline",
      label: text.confirm,
      name: text.confirmFor(name),
    },
    {
      kind: "dismiss",
      variant: "ghost",
      label: text.dismiss,
      name: text.dismissFor(name),
    },
  ] as const;
  return (
    <div className="flex flex-col gap-2">
      <div className="flex items-center gap-2">
        <PersonFace
          shape="judge"
          name={name}
          src={
            other.imageRevision === undefined
              ? undefined
              : personImagePath(other.id, other.imageRevision)
          }
        />
        <div className="flex min-w-0 flex-1 flex-col">
          <Link
            to={personPath(other.id)}
            aria-label={text.open(name)}
            className={cn(
              "truncate hover:underline",
              other.provisional && "text-muted-foreground",
            )}
          >
            {name}
          </Link>
          <span className="text-xs text-muted-foreground">
            {text.similarity(candidate.similarity)}
          </span>
        </div>
      </div>
      <div className="flex flex-wrap gap-2">
        {buttons.map((button) => (
          <Button
            key={button.kind}
            variant={button.variant}
            size="sm"
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
      {error !== null && <p className="text-sm text-destructive">{error}</p>}
    </div>
  );
}
