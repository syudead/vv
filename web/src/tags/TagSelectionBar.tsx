import { Ban, Check, Merge, Trash2 } from "lucide-react";
import { useId, type Ref } from "react";

import { maxTagBatch } from "../api/tags";
import { t } from "../i18n";
import { SelectionBar } from "../ui/patterns/selection-bar";
import { Button } from "../ui/shadcn/button";
import { Spinner } from "../ui/shadcn/spinner";

/**
 * TagSelectionBar はタグ管理画面で 1 件以上選んでいる間、見出しの行の代わりに出す
 * 選択バーである（見出しとタブの帯はトップバーの直下に貼り付くので、送っても届く）。
 * デザインシステムの `SelectionBar` の placement="header"（web/registry/rules/patterns.md の
 * Sections）に、選んだ数・解除とまとめての操作を入れる
 * （specs/036-tag-admin-scale/ui-design.md「Selection bar」）。選んだタグに働かない操作
 * （仮のタグが無いときの「Confirm」「Reject…」、確定したタグが無いときの「Delete…」）は
 * 薄くせずに出さない。「Reject…」「Delete…」は確認の窓（`ConfirmDialog`）を開く。操作の
 * 名前はどの幅でも出し、収まらなければ折り返す。
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
    <div role="region" aria-label={t.tags.selection.region} className="contents">
      <SelectionBar
        count={t.tags.selection.count(count)}
        clearLabel={t.tags.selection.clear}
        clearDisabled={busy}
        onClear={onClear}
        placement="header"
      >
        {hasTentative && (
          <Button
            ref={confirmRef}
            variant="ghost"
            size="sm"
            disabled={disabled}
            title={reason}
            aria-describedby={describedBy}
            onClick={onConfirm}
          >
            {confirming ? <Spinner aria-hidden="true" /> : <Check aria-hidden="true" />}
            <span>{t.tags.selection.confirm}</span>
          </Button>
        )}
        <Button
          ref={mergeRef}
          variant="ghost"
          size="sm"
          disabled={disabled}
          title={reason}
          aria-describedby={describedBy}
          onClick={onMerge}
        >
          <Merge aria-hidden="true" />
          <span>{t.tags.selection.mergeInto}</span>
        </Button>
        {hasTentative && (
          <Button
            ref={rejectRef}
            variant="ghost"
            size="sm"
            disabled={disabled}
            title={reason}
            aria-describedby={describedBy}
            onClick={onReject}
            className="text-destructive"
          >
            <Ban aria-hidden="true" />
            <span>{t.tags.selection.reject}</span>
          </Button>
        )}
        {hasConfirmed && (
          <Button
            ref={deleteRef}
            variant="ghost"
            size="sm"
            disabled={disabled}
            title={reason}
            aria-describedby={describedBy}
            onClick={onDelete}
            className="text-destructive"
          >
            <Trash2 aria-hidden="true" />
            <span>{t.tags.selection.delete}</span>
          </Button>
        )}
        {reason !== undefined && (
          <span id={overLimitId} className="sr-only">
            {reason}
          </span>
        )}
      </SelectionBar>
    </div>
  );
}
