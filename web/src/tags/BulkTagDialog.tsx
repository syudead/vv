import { RotateCw } from "lucide-react";
import { useCallback, useEffect, useState } from "react";

import { tagImpact, type TagImpactResponse } from "../api/tags";
import { errorText, t, type UiText } from "../i18n";
import { ConfirmDialog } from "../ui/patterns/confirm-dialog";
import { Button } from "../ui/shadcn/button";
import { Spinner } from "../ui/shadcn/spinner";
import { DialogError } from "./DialogError";

export type BulkTagAction = "reject" | "delete";

/**
 * BulkTagDialog は選択バーの「Reject…」「Delete…」が開く確認の窓である
 * （specs/036-tag-admin-scale/ui-design.md「Bulk reject and delete」）。開くと同時に
 * `POST /api/tags/impact` で働くタグの数と影響を受ける動画の本数を数え、届くまで
 * danger のボタンを押せなくする（数の無い確認で実行させない。contracts/screen-api.md §3）。
 */
export default function BulkTagDialog({
  action,
  ids,
  pending,
  error,
  onClose,
  onSubmit,
}: {
  action: BulkTagAction;
  /** 選んだタグの id。窓を開いた時点の選択で固定する。 */
  ids: readonly number[];
  pending: boolean;
  error: UiText | null;
  onClose: () => void;
  onSubmit: () => void;
}) {
  const [impact, setImpact] = useState<TagImpactResponse | null>(null);
  const [countError, setCountError] = useState<UiText | null>(null);
  const [attempt, setAttempt] = useState(0);

  useEffect(() => {
    const controller = new AbortController();
    setImpact(null);
    setCountError(null);
    tagImpact(action, ids, controller.signal)
      .then((result) => {
        if (!controller.signal.aborted) setImpact(result);
      })
      .catch((failure: unknown) => {
        if (!controller.signal.aborted) setCountError(errorText(failure));
      });
    return () => controller.abort();
  }, [action, ids, attempt]);

  const retry = useCallback(() => setAttempt((value) => value + 1), []);

  const selected = ids.length;
  const reject = action === "reject";
  const dialog = t.tags.bulkDialog;
  const message =
    impact === null
      ? null
      : impact.tagCount === 0
        ? reject
          ? dialog.rejectNone
          : dialog.deleteNone
        : impact.tagCount >= selected
          ? (reject ? dialog.rejectAll : dialog.deleteAll)(selected, impact.videoCount)
          : (reject ? dialog.rejectSome : dialog.deleteSome)(
              impact.tagCount,
              selected,
              impact.videoCount,
            );
  const canSubmit = impact !== null && impact.tagCount > 0 && !pending;
  const submitText = reject
    ? pending
      ? t.tags.rejectDialog.submitting
      : t.tags.rejectDialog.submit
    : pending
      ? t.tags.deleteDialog.submitting
      : t.tags.deleteDialog.submit;

  const description =
    message ??
    (countError !== null ? (
      <span role="alert" className="text-destructive">
        {dialog.countFailed(countError)}
      </span>
    ) : (
      <span aria-busy="true" className="flex items-center gap-2">
        <Spinner aria-hidden="true" />
        {dialog.counting}
      </span>
    ));

  return (
    <ConfirmDialog
      open
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
      title={reject ? dialog.rejectTitle : dialog.deleteTitle}
      description={description}
      cancelLabel={t.common.cancel}
      actionLabel={
        <>
          {pending && <Spinner aria-hidden="true" />}
          {submitText}
        </>
      }
      onConfirm={onSubmit}
      pending={pending}
      actionDisabled={!canSubmit}
    >
      {countError !== null && (
        <Button variant="outline" size="sm" className="self-start" onClick={retry}>
          <RotateCw aria-hidden="true" />
          {t.common.retry}
        </Button>
      )}
      {error !== null && <DialogError message={error} />}
    </ConfirmDialog>
  );
}
