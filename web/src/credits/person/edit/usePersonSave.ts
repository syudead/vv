import type { RefObject } from "react";

import { isAborted, RequestFailed } from "../../../api/client";
import { isCreditGone, type Person, updateCredit } from "../../../api/credits";
import { errorText, t } from "../../../i18n";
import type { FieldProblem } from "../../../player/detailsEditor/fields";
import {
  type EditorSave,
  useEditorSave,
} from "../../../player/detailsEditor/useDetailsSave";
import { problemOf } from "../../CreditDetailsDialog";
import {
  buildPersonPatch,
  type PersonDraft,
  profileControl,
  type SentTargets,
} from "./personDraft";

function placeProblem(error: unknown, sent: SentTargets): FieldProblem | null {
  const placed = problemOf(error, sent);
  if (placed !== null || !(error instanceof RequestFailed)) return placed;
  const control =
    error.field === undefined ? undefined : profileControl(error.field, sent);
  return control === undefined ? null : { control, text: errorText(error) };
}

/**
 * Send one PATCH with the changed properties; with none, return the loaded person without a
 * request. Field errors stay under their input, other failures go to the band, and leaving aborts.
 */
export function usePersonSave({
  person,
  loaded,
  draftRef,
  onSaved,
  onGone,
}: {
  person: Person;
  loaded: PersonDraft;
  draftRef: RefObject<PersonDraft>;
  onSaved: (person: Person) => void;
  onGone: () => void;
}): EditorSave {
  const editor = useEditorSave(run);

  async function run(signal: AbortSignal): Promise<void> {
    const { patch, sent } = buildPersonPatch(draftRef.current, loaded);
    if (Object.keys(patch).length === 0) {
      onSaved(person);
      return;
    }
    let saved: Person;
    try {
      saved = (await updateCredit("person", person.id, patch, signal)) as Person;
    } catch (error) {
      if (signal.aborted || isAborted(error)) return;
      if (isCreditGone(error)) {
        onGone();
        return;
      }
      const placed = placeProblem(error, sent);
      if (placed !== null) {
        editor.setProblem(placed);
        return;
      }
      const field = error instanceof RequestFailed ? error.field : undefined;
      editor.setFailure({
        text: t.credits.personEdit.saveFailed(errorText(error)),
        field,
      });
      return;
    }
    onSaved(saved);
  }

  return editor;
}
