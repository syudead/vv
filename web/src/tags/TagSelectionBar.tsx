import { Ban, Check, LoaderCircle, Merge, Trash2, X } from "lucide-react";
import { useId, type Ref } from "react";

import { maxTagBatch } from "../api/tags";
import { t } from "../i18n";
import Button from "../ui/Button";
import IconButton from "../ui/IconButton";

/**
 * TagSelectionBar はタグ管理画面で 1 件以上選んでいる間、本文の先頭の見出しの行と
 * 入れ替わる選択の行である（specs/036-tag-admin-scale/ui-design.md「Selection bar」）。
 * 左に選択を解く × と「N tags selected」、右にまとめての操作を本物のボタンで並べる。
 * 選んだタグに働かない操作（仮のタグが無いときの「Confirm」「Reject…」、確定した
 * タグが無いときの「Delete…」）は薄くせずに出さない。
 */
export default function TagSelectionBar({
  count,
  hasTentative,
  hasConfirmed,
  overLimit,
  busy,
  confirming,
  onConfirm,
  onReject,
  onDelete,
  onMerge,
  onClear,
  confirmRef,
  mergeRef,
  rejectRef,
  deleteRef,
}: {
  /** 選んだタグの数（1 以上）。 */
  count: number;
  /** 選んだ中に仮のタグがある（「Confirm」「Reject…」を出す）。 */
  hasTentative: boolean;
  /** 選んだ中に確定したタグがある（「Delete…」を出す）。 */
  hasConfirmed: boolean;
  /** 選んだ数が `maxTagBatch` を超える。まとめての操作を押せなくし、理由を添える。 */
  overLimit: boolean;
  /** まとめての操作の送信中。ボタンをすべて disabled にする。 */
  busy: boolean;
  /** まとめての確定の送信中。「Confirm」のアイコンを回す。 */
  confirming: boolean;
  onConfirm: () => void;
  onReject: () => void;
  onDelete: () => void;
  onMerge: () => void;
  onClear: () => void;
  confirmRef?: Ref<HTMLButtonElement>;
  mergeRef?: Ref<HTMLButtonElement>;
  rejectRef?: Ref<HTMLButtonElement>;
  deleteRef?: Ref<HTMLButtonElement>;
}) {
  const overLimitId = useId();
  const disabled = busy || overLimit;
  const reason = overLimit && !busy ? t.tags.selection.overLimit(maxTagBatch) : undefined;
  const describedBy = reason === undefined ? undefined : overLimitId;

  return (
    <div
      role="region"
      aria-label={t.tags.selection.region}
      className="flex min-h-10 flex-wrap items-center gap-x-2 gap-y-2"
    >
      <IconButton
        label={t.tags.selection.clear}
        onClick={onClear}
        disabled={busy}
        className="-ml-2"
      >
        <X />
      </IconButton>
      <span
        role="status"
        aria-live="polite"
        className="mr-auto text-lg font-semibold text-fg tabular-nums"
      >
        {t.tags.selection.count(count)}
      </span>

      <div className="flex flex-wrap items-center gap-2">
        {hasTentative && (
          <Button
            ref={confirmRef}
            variant="primary"
            disabled={disabled}
            title={reason}
            aria-describedby={describedBy}
            onClick={onConfirm}
          >
            {confirming ? (
              <LoaderCircle
                aria-hidden="true"
                className="animate-spin motion-reduce:animate-none"
              />
            ) : (
              <Check aria-hidden="true" />
            )}
            {t.tags.selection.confirm}
          </Button>
        )}
        <Button
          ref={mergeRef}
          disabled={disabled}
          title={reason}
          aria-describedby={describedBy}
          onClick={onMerge}
        >
          <Merge aria-hidden="true" />
          {t.tags.selection.mergeInto}
        </Button>
        {hasTentative && (
          <Button
            ref={rejectRef}
            disabled={disabled}
            title={reason}
            aria-describedby={describedBy}
            onClick={onReject}
          >
            <Ban aria-hidden="true" className="text-danger" />
            <span className="text-danger">{t.tags.selection.reject}</span>
          </Button>
        )}
        {hasConfirmed && (
          <Button
            ref={deleteRef}
            disabled={disabled}
            title={reason}
            aria-describedby={describedBy}
            onClick={onDelete}
          >
            <Trash2 aria-hidden="true" className="text-danger" />
            <span className="text-danger">{t.tags.selection.delete}</span>
          </Button>
        )}
      </div>

      {reason !== undefined && (
        <p id={overLimitId} className="w-full text-xs text-danger">
          {reason}
        </p>
      )}
    </div>
  );
}
