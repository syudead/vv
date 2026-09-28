import { Minus, Plus, X } from "lucide-react";
import { useEffect, useId, useRef, useState } from "react";

import { maxVideoTagsSelection } from "../api/tags";
import { t } from "../i18n";
import { cn } from "../lib/cn";
import Button from "../ui/Button";
import IconButton from "../ui/IconButton";
import { PopoverRoot, PopoverTrigger } from "../ui/Popover";
import AddTagPopover from "./AddTagPopover";
import RemoveTagPopover from "./RemoveTagPopover";
import { overLimitMessage } from "./selectionErrors";
import VisibilityMenu from "./VisibilityMenu";

export interface SelectionBarProps {
  /** 選んだ動画の本数（グループはメンバーを数える）。 */
  count: number;
  /**
   * 選択が直前の「すべて選択」の応答と同じ集合のとき true。その間だけ「すべて選択」を
   * 押せなくする（specs/017-folder-groups/ui-design.md「Pressing and selection」）。
   * 選んだ本数と項目の数（total）は数えるものが違うので比べない。
   */
  allSelected: boolean;
  selectedIds: readonly number[];
  /** 「すべて選択」の要求の間 true（ui-design.md「Selection bar」の「Layout」）。 */
  selectingAll: boolean;
  onSelectAll: () => void;
  onClear: () => void;
  /**
   * 「タグを外す」で外し終えるたびに、外したタグの id を渡して呼ぶ。呼び出し元
   * （LibraryPage）は、そのタグが今の絞り込みに含まれていれば、一覧と選択が
   * 食い違わないよう選択を解除して一覧を取り直す（Devin の指摘4）。
   */
  onTagRemoved: (tagId: number) => void;
}

/** SelectionBar は 1 件以上選ぶと画面下部に浮く。 */
export default function SelectionBar({
  count,
  allSelected,
  selectedIds,
  selectingAll,
  onSelectAll,
  onClear,
  onTagRemoved,
}: SelectionBarProps) {
  const [addOpen, setAddOpen] = useState(false);
  const [removeOpen, setRemoveOpen] = useState(false);
  const addTriggerRef = useRef<HTMLButtonElement | null>(null);

  // `POST /api/video-tags` は videoIds 1件以上 maxVideoTagsSelection 件以下を
  // 全部か無しかで受け付ける（contracts/tags-api.md §4）。それを超える選択は
  // 静かに分割して送らず、一括操作そのものを disabled にして理由を添える
  // （Devin の指摘2、docs/design-docs/library-ui.md §6）。
  const overLimitId = useId();
  const overLimit = count > maxVideoTagsSelection;

  // count===0 のときはバーごと描かない（下の return null）が、SelectionBar
  // 自身は選択の間ずっと同じインスタンスのまま（アンマウントしない）ので、
  // addOpen・removeOpen をそのままにすると、選択を解除してまた選び直したときに
  // ポップオーバーが勝手に開いた状態で戻ってしまう（Devin の指摘3）。0 になった
  // 時点で両方閉じる。中の AddTagPopover・RemoveTagPopover 自体は、バーが
  // null を返す間はツリーから外れて（アンマウントして）いるので、内部の状態
  // （入力・要約など）はこの操作をしなくても次に開くときは白紙に戻る。
  useEffect(() => {
    if (count === 0) {
      setAddOpen(false);
      setRemoveOpen(false);
    }
  }, [count]);

  // ポップオーバーを開いたままの間に選択が増えて上限を超えたら、両方閉じる
  // （トリガを disabled にするだけでは、開いている最中の分は防げない。
  // Devin の指摘）。ポップオーバー自身の送信前の確かめ（AddTagPopover・
  // RemoveTagPopover の submit・fetchSummary）は、この効果が走るまでの
  // ごく短い間の保険である。
  useEffect(() => {
    if (overLimit) {
      setAddOpen(false);
      setRemoveOpen(false);
    }
  }, [overLimit]);

  if (count === 0) return null;

  return (
    <div
      role="region"
      aria-label={t.library.selection.region}
      className="fixed inset-x-0 bottom-4 z-30 flex justify-center px-4"
    >
      {/*
       * sm 未満の2段では、下の段を「タグを付ける」「タグを外す」「公開」の3つで
       * 等分する。3つが1行に収まらない幅（360px など）では、3つの幅を auto にし、
       * 「公開」をアイコン + 文言のまま右端に置く（ui-design.md「Selection bar」）。
       * 収まるかどうかはこのバーの幅で決まるので、sm 未満でだけバーを
       * コンテナにして問い合わせる（sm 以上では幅が内容で決まるので、
       * コンテナにすると幅が 0 に潰れる）。22.75rem は3つの最小の幅と間隔の和
       * （116px × 3 + 8px × 2）である。
       */}
      <div className="flex w-full flex-wrap items-center gap-x-2 gap-y-1.5 rounded-md border border-border-strong bg-elevated p-1.5 shadow-elevated animate-slide-up motion-reduce:animate-none max-sm:@container sm:h-11 sm:w-auto sm:flex-nowrap sm:py-0 sm:pr-1.5 sm:pl-4">
        <span
          role="status"
          aria-live="polite"
          className="order-1 px-1 text-sm text-fg tabular-nums sm:px-0"
        >
          {t.library.selection.count(count)}
        </span>

        {/*
         * sm 未満では常に2段にする（B3）。内容がたまたま1行に収まる幅
         * （390〜600px など）でも、この行幅いっぱいの見えない仕切りが flex-wrap
         * を強制する。sm 以上では display:none になり、何も強制しない。
         */}
        <span
          aria-hidden="true"
          className="order-4 hidden max-sm:block max-sm:h-0 max-sm:w-full max-sm:basis-full"
        />

        <PopoverRoot open={addOpen} onOpenChange={setAddOpen}>
          <PopoverTrigger asChild>
            <Button
              ref={addTriggerRef}
              variant="ghost"
              size="sm"
              className="order-5 max-sm:flex-1 max-sm:@max-[22.75rem]:flex-none sm:order-2"
              disabled={overLimit}
              title={overLimit ? overLimitMessage() : undefined}
              aria-describedby={overLimit ? overLimitId : undefined}
            >
              <Plus aria-hidden="true" />
              {t.library.selection.addTag}
            </Button>
          </PopoverTrigger>
          <AddTagPopover
            open={addOpen}
            onOpenChange={setAddOpen}
            selectedIds={selectedIds}
            onDone={() => addTriggerRef.current?.focus()}
          />
        </PopoverRoot>

        <PopoverRoot open={removeOpen} onOpenChange={setRemoveOpen}>
          <PopoverTrigger asChild>
            <Button
              variant="ghost"
              size="sm"
              className="order-6 max-sm:flex-1 max-sm:@max-[22.75rem]:flex-none sm:order-3"
              disabled={overLimit}
              title={overLimit ? overLimitMessage() : undefined}
              aria-describedby={overLimit ? overLimitId : undefined}
            >
              <Minus aria-hidden="true" />
              {t.library.selection.removeTag}
            </Button>
          </PopoverTrigger>
          <RemoveTagPopover
            open={removeOpen}
            onOpenChange={setRemoveOpen}
            selectedIds={selectedIds}
            onRemoved={onTagRemoved}
          />
        </PopoverRoot>
        <VisibilityMenu
          selectedIds={selectedIds}
          overLimit={overLimit}
          overLimitId={overLimitId}
          className="order-7 max-sm:flex-1 max-sm:@max-[22.75rem]:ml-auto max-sm:@max-[22.75rem]:flex-none sm:order-4"
        />
        {overLimit && (
          <span id={overLimitId} className="sr-only">
            {overLimitMessage()}
          </span>
        )}

        <span
          aria-hidden="true"
          className={cn("hidden h-5 w-px bg-border-strong sm:order-5 sm:block")}
        />

        <Button
          variant="ghost"
          size="sm"
          onClick={onSelectAll}
          disabled={selectingAll || allSelected}
          className="order-2 sm:order-6"
        >
          {selectingAll
            ? t.library.selection.selectingAll
            : t.library.selection.selectAll}
        </Button>
        <IconButton
          label={t.library.selection.clear}
          size="sm"
          onClick={onClear}
          className="order-3 sm:order-7"
        >
          <X />
        </IconButton>
      </div>
    </div>
  );
}
