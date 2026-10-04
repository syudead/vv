import { LoaderCircle } from "lucide-react";
import { useCallback, useEffect, useRef, useState } from "react";

import { tagImpact, type TagImpactResponse } from "../api/tags";
import { errorText, t, type UiText } from "../i18n";
import Button from "../ui/Button";
import { ModalFrame } from "../ui/ModalFrame";

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
  const cancel = useRef<HTMLButtonElement>(null);
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

  return (
    <ModalFrame
      title={reject ? dialog.rejectTitle : dialog.deleteTitle}
      onClose={onClose}
      initialFocus={cancel}
    >
      <div className="flex min-h-0 flex-1 flex-col gap-5 overflow-y-auto p-4 sm:p-5">
        {impact === null && countError === null && (
          <p aria-busy="true" className="flex items-center gap-2 text-sm text-fg-muted">
            <LoaderCircle
              aria-hidden="true"
              className="size-4 animate-spin motion-reduce:animate-none"
            />
            {dialog.counting}
          </p>
        )}
        {countError !== null && (
          <div className="flex flex-wrap items-center gap-2">
            <p role="alert" className="text-sm text-danger">
              {dialog.countFailed(countError)}
            </p>
            <Button variant="ghost" size="sm" onClick={retry}>
              {t.common.retry}
            </Button>
          </div>
        )}
        {message !== null && (
          <p className="border-l-2 border-danger-strong pl-3 text-sm leading-6 text-fg-muted">
            {message}
          </p>
        )}
        {error !== null && (
          <p role="alert" className="text-sm text-danger">
            {error}
          </p>
        )}
      </div>
      <div className="flex shrink-0 flex-wrap justify-end gap-2 border-t border-border p-4">
        <Button ref={cancel} onClick={onClose} disabled={pending}>
          {t.common.cancel}
        </Button>
        <Button variant="danger" onClick={onSubmit} disabled={!canSubmit}>
          {pending && <LoaderCircle className="animate-spin" />}
          {submitText}
        </Button>
      </div>
    </ModalFrame>
  );
}
