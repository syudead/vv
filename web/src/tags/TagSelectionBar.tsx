import { Ban, Check, Ellipsis, LoaderCircle, Merge, Trash2, X } from "lucide-react";
import { useId, type ReactNode, type Ref } from "react";

import { maxTagBatch } from "../api/tags";
import { t } from "../i18n";
import Button from "../ui/Button";
import IconButton from "../ui/IconButton";
import { MenuContent, MenuItem, MenuRoot, MenuSeparator, MenuTrigger } from "../ui/Menu";

/**
 * TagSelectionBar はタグ管理画面で 1 件以上選ぶと画面下部に出る選択バーである
 * （specs/036-tag-admin-scale/ui-design.md「Selection bar」）。箱・位置・現れ方・段の
 * 折り返しはライブラリの `SelectionBar` と同じ。前に出す操作は「Confirm」だけで、
 * 却下・削除（と統合）は「More」のメニューに入れる。
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
  moreRef,
}: {
  /** 選んだタグの数。0 ならバーを描かない。 */
  count: number;
  /** 選んだ中に仮のタグがある（「Confirm」「Reject…」が働く）。 */
  hasTentative: boolean;
  /** 選んだ中に確定したタグがある（「Delete…」が働く）。 */
  hasConfirmed: boolean;
  /** 見えている数が `maxTagBatch` を超える。まとめての操作を押せなくする。 */
  overLimit: boolean;
  /** まとめての操作の送信中。バーのボタンをすべて disabled にする。 */
  busy: boolean;
  /** まとめての確定の送信中。「Confirm」のアイコンを回す。 */
  confirming: boolean;
  onConfirm: () => void;
  onReject: () => void;
  onDelete: () => void;
  /**
   * 「Merge into one tag…」を押したとき。渡さなければ項目を出さない（統合の窓は
   * 「選んだタグをまとめて 1 つのタグへ統合する」の単位が足す）。
   */
  onMerge?: () => void;
  onClear: () => void;
  confirmRef?: Ref<HTMLButtonElement>;
  moreRef?: Ref<HTMLButtonElement>;
}) {
  const overLimitId = useId();
  const noTentativeId = useId();

  if (count === 0) return null;

  const confirmDisabled = busy || overLimit || !hasTentative;
  const confirmReason = overLimit
    ? t.tags.selection.overLimit(maxTagBatch)
    : !hasTentative
      ? t.tags.selection.noTentative
      : undefined;

  return (
    <div
      role="region"
      aria-label={t.tags.selection.region}
      className="fixed inset-x-0 bottom-4 z-30 flex justify-center px-4"
    >
      <div className="flex w-full flex-wrap items-center gap-x-2 gap-y-1.5 rounded-md border border-border-strong bg-elevated p-1.5 shadow-elevated animate-slide-up motion-reduce:animate-none sm:h-11 sm:w-auto sm:flex-nowrap sm:py-0 sm:pr-1.5 sm:pl-4">
        <span
          role="status"
          aria-live="polite"
          className="order-1 px-1 text-sm text-fg tabular-nums sm:px-0"
        >
          {t.tags.selection.count(count)}
        </span>

        {/* sm 未満では常に 2 段にする（ライブラリの SelectionBar と同じ仕切り）。 */}
        <span
          aria-hidden="true"
          className="order-4 hidden max-sm:block max-sm:h-0 max-sm:w-full max-sm:basis-full"
        />

        <Button
          ref={confirmRef}
          variant="ghost"
          size="sm"
          className="order-5 max-sm:flex-1 sm:order-2"
          disabled={confirmDisabled}
          title={busy ? undefined : confirmReason}
          aria-describedby={
            busy
              ? undefined
              : overLimit
                ? overLimitId
                : !hasTentative
                  ? noTentativeId
                  : undefined
          }
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

        <MenuRoot>
          <MenuTrigger asChild>
            <Button
              ref={moreRef}
              variant="ghost"
              size="sm"
              className="order-6 max-sm:flex-1 sm:order-3"
              disabled={busy || overLimit}
              title={
                overLimit && !busy ? t.tags.selection.overLimit(maxTagBatch) : undefined
              }
              aria-describedby={overLimit && !busy ? overLimitId : undefined}
            >
              <Ellipsis aria-hidden="true" />
              {t.tags.selection.more}
            </Button>
          </MenuTrigger>
          <MenuContent side="top">
            {onMerge !== undefined && (
              <>
                <MenuItem onSelect={onMerge}>
                  <Merge />
                  {t.tags.selection.mergeInto}
                </MenuItem>
                <MenuSeparator />
              </>
            )}
            <ReasonedItem
              disabled={!hasTentative}
              reason={t.tags.selection.noTentative}
              onSelect={onReject}
            >
              <Ban />
              {t.tags.selection.reject}
            </ReasonedItem>
            <ReasonedItem
              disabled={!hasConfirmed}
              reason={t.tags.selection.noConfirmed}
              onSelect={onDelete}
            >
              <Trash2 />
              {t.tags.selection.delete}
            </ReasonedItem>
          </MenuContent>
        </MenuRoot>

        {overLimit && (
          <span id={overLimitId} className="sr-only">
            {t.tags.selection.overLimit(maxTagBatch)}
          </span>
        )}
        {!overLimit && !hasTentative && (
          <span id={noTentativeId} className="sr-only">
            {t.tags.selection.noTentative}
          </span>
        )}

        <span
          aria-hidden="true"
          className="hidden h-5 w-px bg-border-strong sm:order-4 sm:block"
        />

        <IconButton
          label={t.tags.selection.clear}
          size="sm"
          onClick={onClear}
          disabled={busy}
          className="order-3 ml-auto sm:order-5 sm:ml-0"
        >
          <X />
        </IconButton>
      </div>
    </div>
  );
}

/**
 * ReasonedItem は押せない理由を `title` と読み上げで添える danger のメニューの項目で
 * ある（ui-design.md「Enabled and disabled」）。
 */
function ReasonedItem({
  disabled,
  reason,
  onSelect,
  children,
}: {
  disabled: boolean;
  reason: string;
  onSelect: () => void;
  children: ReactNode;
}) {
  const reasonId = useId();
  return (
    <>
      <MenuItem
        tone="danger"
        onSelect={onSelect}
        disabled={disabled}
        title={disabled ? reason : undefined}
        describedBy={disabled ? reasonId : undefined}
      >
        {children}
      </MenuItem>
      {disabled && (
        <span id={reasonId} className="sr-only">
          {reason}
        </span>
      )}
    </>
  );
}
