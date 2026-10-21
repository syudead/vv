import { useCallback, useEffect, useState } from "react";
import { Link } from "react-router";

import { isAborted } from "../../api/client";
import { isCreditGone, type Person, personImagePath } from "../../api/credits";
import {
  dismissFaceCandidate,
  type FaceCandidate,
  type FaceCandidatePerson,
  faceCropPath,
  isCandidateGone,
  listFaceCandidates,
  listPersonAppearances,
  reopenFaceCandidate,
} from "../../api/personFaces";
import { errorText, t, type UiText } from "../../i18n";
import { cn } from "../../lib/cn";
import { Button } from "../../ui/shadcn/button";
import { Spinner } from "../../ui/shadcn/spinner";
import { PersonFace } from "../../ui/PersonFace";
import { creditDisplayName } from "../displayName";
import { personPath } from "./personPath";

const text = t.credits.personPage.candidates;

const sampleSize = 4;

/** Faces read for the sample, so faces without a crop do not leave it short. */
const sampleReach = 12;

/**
 * The person's open candidate pairs. A failed read draws nothing, since the group is only there
 * when there is something to decide; a missing person calls onGone.
 */
export function useFaceCandidates(personId: number, onGone: () => void) {
  const [items, setItems] = useState<FaceCandidate[]>([]);
  const [loaded, setLoaded] = useState(false);
  const [failed, setFailed] = useState<UiText | null>(null);
  // The other people of the pairs decided on the page, which no longer wait.
  const [decided, setDecided] = useState<ReadonlySet<number>>(new Set());
  const [attempt, setAttempt] = useState(0);

  useEffect(() => {
    const controller = new AbortController();
    listFaceCandidates(personId, controller.signal).then(
      (next) => {
        if (controller.signal.aborted) return;
        setItems(next);
        setDecided(new Set());
        setFailed(null);
        setLoaded(true);
      },
      (failure: unknown) => {
        if (controller.signal.aborted || isAborted(failure)) return;
        if (isCreditGone(failure)) onGone();
        else {
          setFailed(text.loadFailed(errorText(failure)));
          setLoaded(true);
        }
      },
    );
    return () => controller.abort();
  }, [personId, attempt, onGone]);

  const reload = useCallback(() => setAttempt((value) => value + 1), []);
  const remove = useCallback((otherId: number) => {
    setItems((current) => current.filter((item) => item.person.id !== otherId));
  }, []);
  const decide = useCallback((otherId: number, done: boolean) => {
    setDecided((current) => {
      const next = new Set(current);
      if (done) next.add(otherId);
      else next.delete(otherId);
      return next;
    });
  }, []);
  return { items, loaded, failed, decided, reload, remove, decide };
}

export interface FaceSample {
  /** Appearances with a crop, newest video first. */
  crops: number[];
  total?: number;
}

/** A few of the person's crops and their face count, to judge a pair by more than one face; a failure shows none. */
export function useFaceSample(personId: number, attempt = 0): FaceSample {
  const [sample, setSample] = useState<FaceSample>({ crops: [] });
  useEffect(() => {
    const controller = new AbortController();
    listPersonAppearances(personId, { limit: sampleReach }, controller.signal).then(
      (page) => {
        if (controller.signal.aborted) return;
        setSample({
          crops: page.items
            .filter((item) => item.hasCrop)
            .slice(0, sampleSize)
            .map((item) => item.appearanceId),
          total: page.total,
        });
      },
      () => undefined,
    );
    return () => controller.abort();
  }, [personId, attempt]);
  return sample;
}

export interface CandidateActions {
  /** Confirms the pair and updates the page; a rejection is shown under the row. */
  confirm: (other: FaceCandidatePerson) => Promise<void>;
  remove: (otherId: number) => void;
  /** Counts the pair as decided, or as waiting again after Undo. */
  decide: (otherId: number, done: boolean) => void;
  onGone: () => void;
}

/**
 * `Same person?`: one row per pair with both people side by side, decided in place. A decided row
 * stays where it is, so the order never moves under the owner's next press.
 */
export function PersonCandidates({
  person,
  sample,
  items,
  actions,
  failed,
}: {
  person: Person;
  sample: FaceSample;
  items: readonly FaceCandidate[];
  actions: CandidateActions;
  /** Why the pairs could not be read, with a retry; null once they were. */
  failed?: { reason: UiText; retry: () => void } | null;
}) {
  if (failed != null) {
    return (
      <p className="flex flex-wrap items-center gap-2 text-muted-foreground">
        {failed.reason}
        <Button variant="link" size="sm" onClick={failed.retry}>
          {t.common.retry}
        </Button>
      </p>
    );
  }
  if (items.length === 0) return <p className="text-muted-foreground">{text.none}</p>;
  return items.map((item) => (
    <CandidateRow
      key={item.person.id}
      shown={person}
      shownSample={sample}
      candidate={item}
      actions={actions}
    />
  ));
}

type Decision = "merged" | "dismissed" | null;
type Pending = "confirm" | "dismiss" | "undo" | null;

function useCandidateDecision(
  shown: Person,
  other: FaceCandidatePerson,
  actions: CandidateActions,
) {
  const [decision, setDecision] = useState<Decision>(null);
  const [pending, setPending] = useState<Pending>(null);
  const [error, setError] = useState<UiText | null>(null);

  async function run(kind: Exclude<Pending, null>) {
    if (pending !== null) return;
    setPending(kind);
    setError(null);
    try {
      if (kind === "confirm") await actions.confirm(other);
      else if (kind === "dismiss") await dismissFaceCandidate(shown.id, other.id);
      else await reopenFaceCandidate(shown.id, other.id);
      setDecision(
        kind === "confirm" ? "merged" : kind === "dismiss" ? "dismissed" : null,
      );
      actions.decide(other.id, kind !== "undo");
    } catch (failure) {
      if (isCandidateGone(failure)) actions.remove(other.id);
      else if (isCreditGone(failure)) actions.onGone();
      else setError(failureText(kind)(errorText(failure)));
    } finally {
      setPending(null);
    }
  }
  return { decision, pending, error, run };
}

function failureText(kind: Exclude<Pending, null>): (reason: string) => UiText {
  if (kind === "confirm") return text.confirmFailed;
  return kind === "dismiss" ? text.dismissFailed : text.undoFailed;
}

function CandidateRow({
  shown,
  shownSample,
  candidate,
  actions,
}: {
  shown: Person;
  shownSample: FaceSample;
  candidate: FaceCandidate;
  actions: CandidateActions;
}) {
  const other = candidate.person;
  const name = creditDisplayName(other);
  const otherSample = useFaceSample(other.id);
  const { decision, pending, error, run } = useCandidateDecision(shown, other, actions);

  return (
    <div role="group" aria-label={text.pairLabel(name)} className="flex flex-col gap-3">
      <div
        className={cn(
          "grid grid-cols-2 items-start gap-3 sm:flex sm:items-center",
          decision !== null && "opacity-60",
        )}
      >
        <PairSide
          person={shown}
          meta={sideMeta(text.thisPerson, shownSample.total)}
          sample={shownSample}
        />
        <div className="order-last col-span-2 flex items-center justify-center gap-2 sm:order-none sm:col-span-1 sm:flex-col sm:gap-1">
          <span className="text-lg font-semibold tabular-nums">
            {text.percent(candidate.similarity)}
          </span>
          <span className="text-xs text-muted-foreground">{text.similarLabel}</span>
        </div>
        <PairSide
          person={other}
          meta={sideMeta(undefined, otherSample.total)}
          sample={otherSample}
          link
        />
      </div>
      {decision === null ? (
        <div className="grid grid-cols-2 gap-2 sm:flex sm:justify-end">
          <ActionButton
            variant="outline"
            label={text.dismiss}
            name={text.dismissFor(name)}
            busy={pending === "dismiss"}
            disabled={pending !== null}
            onClick={() => void run("dismiss")}
          />
          <ActionButton
            variant="default"
            label={text.confirm}
            name={text.confirmFor(name)}
            busy={pending === "confirm"}
            disabled={pending !== null}
            onClick={() => void run("confirm")}
          />
        </div>
      ) : (
        <div className="flex flex-wrap items-center justify-end gap-2 text-sm text-muted-foreground">
          {decision === "merged"
            ? text.mergedResult(creditDisplayName(shown))
            : text.dismissedResult}
          {decision === "dismissed" && (
            <ActionButton
              variant="ghost"
              label={text.undo}
              name={text.undoFor(name)}
              busy={pending === "undo"}
              disabled={pending !== null}
              onClick={() => void run("undo")}
            />
          )}
        </div>
      )}
      {error !== null && <p className="text-sm text-destructive">{error}</p>}
    </div>
  );
}

function sideMeta(
  label: string | undefined,
  total: number | undefined,
): string | undefined {
  const count = total === undefined ? undefined : text.faceCount(total);
  if (label === undefined) return count;
  return count === undefined ? label : `${label} · ${count}`;
}

function ActionButton({
  variant,
  label,
  name,
  busy,
  disabled,
  onClick,
}: {
  variant: "default" | "outline" | "ghost";
  label: string;
  name: string;
  busy: boolean;
  disabled: boolean;
  onClick: () => void;
}) {
  return (
    <Button
      variant={variant}
      size="sm"
      aria-label={name}
      aria-disabled={disabled || undefined}
      aria-busy={busy || undefined}
      onClick={onClick}
    >
      {busy && <Spinner aria-hidden="true" />}
      {label}
    </Button>
  );
}

/** One side of a pair: the face, the name and a few more crops, so a pair is judged on more than one face. */
function PairSide({
  person,
  meta,
  sample,
  link = false,
}: {
  person: Person | FaceCandidatePerson;
  meta: string | undefined;
  sample: FaceSample;
  link?: boolean;
}) {
  const name = creditDisplayName(person);
  const nameClass = cn(
    "max-w-full truncate text-sm font-medium",
    person.provisional && "text-muted-foreground",
  );
  return (
    <div className="flex min-w-0 flex-col items-center gap-1 sm:flex-1">
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
      {meta !== undefined && (
        <span className="text-xs text-muted-foreground">{meta}</span>
      )}
      {sample.crops.length > 0 && (
        // The name and face above already say who this is; the crops add only what they look like.
        <div aria-hidden="true" className="flex gap-1">
          {sample.crops.map((id) => (
            <div key={id} className="size-9">
              <PersonFace shape="scene" name={name} src={faceCropPath(id)} />
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
