import { useEffect, useEffectEvent, useRef, useState } from "react";

import {
  type Credit,
  type CreditKind,
  isCreditGone,
  mergeCredits,
  type Person,
} from "../api/credits";
import { errorText, t, type UiText } from "../i18n";
import { DialogError } from "../tags/DialogError";
import { MergeSummary, MergeTargetField } from "../tags/MergeParts";
import CreditCommand, { type CreditChoice } from "../ui/CreditCommand";
import { FormDialog } from "../ui/patterns/form-dialog";
import { Spinner } from "../ui/shadcn/spinner";
import { kindTexts } from "./creditRows";
import { creditDisplayName } from "./displayName";
import {
  SimilarPeopleList,
  suggestsByFace,
  useSimilarPeople,
} from "./person/SimilarPeopleList";
import { useCreditTargetSearch } from "./useCreditTargetSearch";

/**
 * Exclude the source and creation option from merge candidates. Missing records call onStale;
 * cycles and limit errors stay in the dialog.
 */
export default function MergeCreditDialog({
  kind,
  source,
  onClose,
  onMerged,
  onStale,
}: {
  kind: CreditKind;
  source: Credit;
  onClose: () => void;
  /** Pass both the updated merge target and the previously selected candidate. */
  onMerged: (merged: Credit, target: Credit) => void;
  onStale: () => void;
}) {
  const texts = kindTexts(kind);
  const cancel = useRef<HTMLButtonElement>(null);
  const mergeButton = useRef<HTMLButtonElement>(null);
  const [value, setValue] = useState("");
  const [target, setTarget] = useState<Credit | null>(null);
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<UiText | null>(null);
  // Move focus to Merge only immediately after selecting a candidate.
  const justSelectedRef = useRef(false);
  const { shown, options, exactOption, searching, searchError } = useCreditTargetSearch(
    kind,
    source.id,
    value,
  );

  // Before a name is typed, people are offered by face, since a person may have no name yet; without any face
  // data the names are listed as before.
  const similar = useSimilarPeople(kind === "person" ? source.id : null, []);
  const suggesting = suggestsByFace(value, similar);
  const canSubmit = target !== null && !pending;

  useEffect(() => {
    if (justSelectedRef.current && target !== null) {
      justSelectedRef.current = false;
      mergeButton.current?.focus();
    }
  }, [target]);

  // Restore focus to an enabled action after a failed merge.
  const restoreFocus = useEffectEvent(() => {
    (target !== null ? mergeButton : cancel).current?.focus();
  });
  useEffect(() => {
    if (error !== null) restoreFocus();
  }, [error]);

  function handleClose() {
    if (pending) return;
    onClose();
  }

  function selectTarget(option: CreditChoice) {
    const found = shown.find((credit) => String(credit.id) === option.id);
    if (found === undefined) return;
    justSelectedRef.current = true;
    setTarget(found);
    setValue(found.name);
    setError(null);
  }

  async function submit() {
    if (target === null || !canSubmit) return;
    setError(null);
    setPending(true);
    try {
      const { credit: merged, notFoundIds } = await mergeCredits(kind, target.id, [
        source.id,
      ]);
      // A missing source means no merge occurred.
      if (notFoundIds.includes(source.id)) {
        onStale();
        return;
      }
      onMerged(merged, target);
    } catch (failure) {
      if (isCreditGone(failure)) {
        onStale();
        return;
      }
      setError(errorText(failure));
      setPending(false);
    }
  }

  return (
    <FormDialog
      open
      onOpenChange={(open) => {
        if (!open) handleClose();
      }}
      title={t.credits.mergeDialog.title(creditDisplayName(source))}
      onSubmit={(event) => {
        event.preventDefault();
        void submit();
      }}
      cancelLabel={t.common.cancel}
      submitLabel={
        <>
          {pending && <Spinner aria-hidden="true" />}
          {pending ? t.credits.mergeDialog.submitting : t.credits.mergeDialog.submit}
        </>
      }
      pending={pending}
      submitDisabled={!canSubmit}
      submitRef={mergeButton}
      cancelRef={cancel}
      initialFocus={cancel}
    >
      <MergeTargetField
        Command={CreditCommand}
        label={texts.mergeTarget}
        value={value}
        onValueChange={(next) => {
          setValue(next);
          setTarget(null);
          setError(null);
        }}
        choices={suggesting ? [] : options}
        exactChoice={exactOption}
        onSelect={selectTarget}
        chosenId={target === null ? undefined : String(target.id)}
        searching={searching && !suggesting}
        searchError={
          suggesting || searchError === null ? null : texts.searchFailed(searchError)
        }
        emptyText={suggesting ? undefined : texts.noCandidates}
      />
      {suggesting && (
        <SimilarPeopleList
          heading={t.credits.similarPeople.likePerson(creditDisplayName(source))}
          similar={similar}
          selectedId={target?.id}
          onSelect={(person: Person) => {
            setTarget(person);
            setError(null);
          }}
        />
      )}
      {target !== null && (
        <>
          <p className="text-sm text-muted-foreground">
            {t.credits.mergeDialog.warning(
              creditDisplayName(source),
              source.videoCount,
              creditDisplayName(target),
            )}
          </p>
          <MergeSummary
            source={creditDisplayName(source)}
            target={creditDisplayName(target)}
          />
        </>
      )}
      {error !== null && <DialogError message={error} />}
    </FormDialog>
  );
}
