import { CheckCheck, Layers, Minus, Plus } from "lucide-react";
import { useEffect, useId, useRef, useState } from "react";

import { type FolderRef, maxBundleSelection, type VideoVersions } from "../api/client";
import { maxVideoTagsSelection } from "../api/tags";
import { t } from "../i18n";
import { SelectionBar as SelectionBarSection } from "../ui/patterns/selection-bar";
import { Popover, PopoverTrigger } from "../ui/shadcn/popover";
import { Separator } from "../ui/shadcn/separator";
import BundleDialog from "../versions/BundleDialog";
import AddTagPopover from "./AddTagPopover";
import FavoriteMenu from "./FavoriteMenu";
import RemoveTagPopover from "./RemoveTagPopover";
import { SelectionAction, WithTooltip } from "./SelectionAction";
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
  /**
   * 一括のお気に入りで送る動画（選んだ id のうち選んだグループのメンバーでないもの）と
   * グループ（specs/035-favorites/research.md R-7）。上限はこの 2 つの合計で判定する。
   */
  favoriteVideoIds: readonly number[];
  favoriteFolders: readonly FolderRef[];
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
  /**
   * 「Bundle as versions」の窓で束ね終えたときに呼ぶ。呼び出し元（LibraryPage）は選択を
   * 解除して一覧を取り直す（specs/030-video-versions/ui-design.md「Bundle dialog」）。
   */
  onBundled: (versions: VideoVersions) => void;
}

/**
 * SelectionBar は 1 件以上選ぶとページの下端に貼り付く選択バーである。画面の型の
 * SelectionBar（web/registry/rules/patterns.md「Sections」）に、選んだ数・解除・一括の操作
 * （タグを付ける・外す・お気に入り・公開・版をまとめる・すべて選択）を入れる。操作の名前は
 * どの幅でも出し、収まらなければ帯が折り返す。
 */
export default function SelectionBar({
  count,
  allSelected,
  selectedIds,
  favoriteVideoIds,
  favoriteFolders,
  selectingAll,
  onSelectAll,
  onClear,
  onTagRemoved,
  onBundled,
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
  // お気に入りの上限は送る数（videoIds と folders の合計）で判定する。メンバーが上限を超える
  // グループ 1 つは、タグ・公開では押せないままで、お気に入りは folders 1 つとして押せる
  // （specs/035-favorites/ui-design.md「Selection bar」）。
  const favoriteOverLimitId = useId();
  const favoriteOverLimit =
    favoriteVideoIds.length + favoriteFolders.length > maxVideoTagsSelection;

  // 束ねる操作は 2 本以上で現れ、画面の側だけの上限 maxBundleSelection を超えると
  // disabled にして理由を添える（specs/030-video-versions/ui-design.md「Bundle action」）。
  // 窓を開いている間の背景は inert なので、開いたあとに選択が増えることはない。
  const [bundleOpen, setBundleOpen] = useState(false);
  const bundleOverLimitId = useId();
  const canBundle = count >= 2;
  const bundleOverLimit = count > maxBundleSelection;
  // 窓に渡す id の並びは、開いた時点の選択で固定する。
  const [bundleIds, setBundleIds] = useState<readonly number[]>([]);

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
      setBundleOpen(false);
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

  const selectAllLabel = selectingAll
    ? t.library.selection.selectingAll
    : t.library.selection.selectAll;

  return (
    // 帯は画面の型のもので、名前（region）はこの包みが持つ。包みは箱を作らない
    // （contents）ので、帯はページの下端に貼り付いたままになる。
    <div role="region" aria-label={t.library.selection.region} className="contents">
      <SelectionBarSection
        count={t.library.selection.count(count)}
        clearLabel={t.library.selection.clear}
        onClear={onClear}
      >
        <Popover open={addOpen} onOpenChange={setAddOpen}>
          <WithTooltip label={t.library.selection.addTag}>
            <PopoverTrigger asChild>
              <SelectionAction
                ref={addTriggerRef}
                icon={<Plus aria-hidden="true" />}
                label={t.library.selection.addTag}
                disabled={overLimit}
                title={overLimit ? overLimitMessage() : undefined}
                aria-describedby={overLimit ? overLimitId : undefined}
              />
            </PopoverTrigger>
          </WithTooltip>
          <AddTagPopover
            open={addOpen}
            onOpenChange={setAddOpen}
            selectedIds={selectedIds}
            onDone={() => addTriggerRef.current?.focus()}
          />
        </Popover>

        <Popover open={removeOpen} onOpenChange={setRemoveOpen}>
          <WithTooltip label={t.library.selection.removeTag}>
            <PopoverTrigger asChild>
              <SelectionAction
                icon={<Minus aria-hidden="true" />}
                label={t.library.selection.removeTag}
                disabled={overLimit}
                title={overLimit ? overLimitMessage() : undefined}
                aria-describedby={overLimit ? overLimitId : undefined}
              />
            </PopoverTrigger>
          </WithTooltip>
          <RemoveTagPopover
            open={removeOpen}
            onOpenChange={setRemoveOpen}
            selectedIds={selectedIds}
            onRemoved={onTagRemoved}
          />
        </Popover>
        {overLimit && (
          <span id={overLimitId} className="sr-only">
            {overLimitMessage()}
          </span>
        )}

        <FavoriteMenu
          videoIds={favoriteVideoIds}
          folders={favoriteFolders}
          overLimit={favoriteOverLimit}
          overLimitId={favoriteOverLimitId}
        />
        {favoriteOverLimit && (
          <span id={favoriteOverLimitId} className="sr-only">
            {overLimitMessage()}
          </span>
        )}

        <VisibilityMenu
          selectedIds={selectedIds}
          overLimit={overLimit}
          overLimitId={overLimitId}
        />

        {canBundle && (
          <WithTooltip label={t.library.selection.bundle}>
            <SelectionAction
              icon={<Layers aria-hidden="true" />}
              label={t.library.selection.bundle}
              disabled={bundleOverLimit}
              title={
                bundleOverLimit
                  ? t.library.selection.bundleOverLimit(maxBundleSelection)
                  : undefined
              }
              aria-describedby={bundleOverLimit ? bundleOverLimitId : undefined}
              onClick={() => {
                setBundleIds(selectedIds);
                setBundleOpen(true);
              }}
            />
          </WithTooltip>
        )}
        {canBundle && bundleOverLimit && (
          <span id={bundleOverLimitId} className="sr-only">
            {t.library.selection.bundleOverLimit(maxBundleSelection)}
          </span>
        )}
        {bundleOpen && (
          <BundleDialog
            videoIds={bundleIds}
            onClose={() => setBundleOpen(false)}
            onBundled={(versions) => {
              setBundleOpen(false);
              onBundled(versions);
            }}
          />
        )}

        <Separator orientation="vertical" className="h-5" />

        <WithTooltip label={selectAllLabel}>
          <SelectionAction
            icon={<CheckCheck aria-hidden="true" />}
            label={selectAllLabel}
            onClick={onSelectAll}
            disabled={selectingAll || allSelected}
          />
        </WithTooltip>
      </SelectionBarSection>
    </div>
  );
}
