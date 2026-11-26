import type { RefObject } from "react";

import { isAborted, RequestFailed } from "../../api/client";
import {
  type Credit,
  type CreditKind,
  type CreditPatch,
  isCreditGone,
  updateCredit,
} from "../../api/credits";
import { errorText, t } from "../../i18n";
import type { FieldProblem } from "../../player/detailsEditor/fields";
import {
  type EditorSave,
  useEditorSave,
} from "../../player/detailsEditor/useDetailsSave";

/**
 * Send one PATCH with the changed properties; with none, return the loaded record without a
 * request. Field errors stay under their input, other failures go to the band, a missing record
 * calls onGone, and leaving aborts.
 */
export function useCreditSave<T extends Credit, D, S>({
  kind,
  id,
  credit,
  loaded,
  draftRef,
  build,
  place,
  onSaved,
  onGone,
}: {
  kind: CreditKind;
  /** The id the record was read by, which `credit.id` may not be. */
  id: number;
  credit: T;
  loaded: D;
  draftRef: RefObject<D>;
  /** The patch of the properties that differ from `loaded`, and what `place` needs to map a field. */
  build: (draft: D, loaded: D) => { patch: CreditPatch; sent: S };
  place: (error: RequestFailed, sent: S) => FieldProblem | null;
  onSaved: (credit: T) => void;
  onGone: () => void;
}): EditorSave {
  const editor = useEditorSave(run);

  async function run(signal: AbortSignal): Promise<void> {
    const { patch, sent } = build(draftRef.current, loaded);
    if (Object.keys(patch).length === 0) {
      onSaved(credit);
      return;
    }
    let saved: T;
    try {
      saved = (await updateCredit(kind, id, patch, signal)) as T;
    } catch (error) {
      if (signal.aborted || isAborted(error)) return;
      if (isCreditGone(error)) {
        onGone();
        return;
      }
      const placed = error instanceof RequestFailed ? place(error, sent) : null;
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
