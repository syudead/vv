import { useEffect, useEffectEvent, useRef, useState } from "react";

import { isAborted } from "../api/client";
import {
  type Credit,
  type CreditKind,
  isCreditGone,
  listCredits,
  mergeCredits,
} from "../api/credits";
import { errorText, t, type UiText } from "../i18n";
import { foldForMatch } from "../lib/foldForMatch";
import { DialogError } from "../tags/DialogError";
import { MergeSummary, MergeTargetField } from "../tags/MergeParts";
import CreditCommand, { type CreditChoice } from "../ui/CreditCommand";
import { FormDialog } from "../ui/patterns/form-dialog";
import { Spinner } from "../ui/shadcn/spinner";
import { kindTexts } from "./creditRows";

const candidateLimit = 8;

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
  const [candidates, setCandidates] = useState<readonly Credit[]>([]);
  const [searching, setSearching] = useState(false);
  const [searchError, setSearchError] = useState<UiText | null>(null);
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<UiText | null>(null);
  // Move focus to Merge only immediately after selecting a candidate.
  const justSelectedRef = useRef(false);

  // Fetch one extra candidate to retain eight after excluding the source; abort obsolete searches.
  useEffect(() => {
    const controller = new AbortController();
    setSearching(true);
    setSearchError(null);
    listCredits(
      kind,
      { q: value.trim(), limit: candidateLimit + 1 },
      controller.signal,
    ).then(
      (page) => {
        if (controller.signal.aborted) return;
        setCandidates(page.items);
        setSearching(false);
      },
      (failure: unknown) => {
        if (controller.signal.aborted || isAborted(failure)) return;
        setSearchError(errorText(failure));
        setSearching(false);
      },
    );
    return () => controller.abort();
  }, [kind, value]);

  const shown = candidates
    .filter((credit) => credit.id !== source.id)
    .slice(0, candidateLimit);
  const folded = foldForMatch(value.trim());
  const options: CreditChoice[] = shown.map((credit) => {
    const alias =
      folded === "" || foldForMatch(credit.name).includes(folded)
        ? undefined
        : credit.aliases.find((item) => foldForMatch(item.name).includes(folded))?.name;
    return {
      id: String(credit.id),
      label: credit.name,
      hint: alias === undefined ? undefined : t.credits.mergeDialog.aliasHint(alias),
      meta: t.credits.mergeDialog.videoCount(credit.videoCount),
    };
  });
  const trimmed = value.trim();
  const exact = shown.find((credit) => trimmed !== "" && credit.name === trimmed);
  const exactOption =
    exact === undefined
      ? null
      : (options.find((option) => option.id === String(exact.id)) ?? null);

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
      title={t.credits.mergeDialog.title(source.name)}
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
        choices={options}
        exactChoice={exactOption}
        onSelect={selectTarget}
        chosenId={target === null ? undefined : String(target.id)}
        searching={searching}
        searchError={searchError === null ? null : texts.searchFailed(searchError)}
        emptyText={texts.noCandidates}
      />
      {target !== null && (
        <>
          <p className="text-sm text-muted-foreground">
            {t.credits.mergeDialog.warning(source.name, source.videoCount, target.name)}
          </p>
          <MergeSummary source={source.name} target={target.name} />
        </>
      )}
      {error !== null && <DialogError message={error} />}
    </FormDialog>
  );
}
