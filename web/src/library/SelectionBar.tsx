import { Layers, Minus, Plus, X } from "lucide-react";
import {
  type RefObject,
  useEffect,
  useId,
  useLayoutEffect,
  useRef,
  useState,
} from "react";

import { type FolderRef, maxBundleSelection, type VideoVersions } from "../api/client";
import { maxVideoTagsSelection } from "../api/tags";
import { t } from "../i18n";
import { cn } from "../lib/cn";
import Button from "../ui/Button";
import IconButton from "../ui/IconButton";
import { PopoverRoot, PopoverTrigger } from "../ui/Popover";
import BundleDialog from "../versions/BundleDialog";
import AddTagPopover from "./AddTagPopover";
import FavoriteMenu from "./FavoriteMenu";
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
 * BarLayout は sm 以上の選択バーの行の割り方である（specs/035-favorites/ui-design.md
 * 「Selection bar」の「Layout」）。
 *
 * - `one`: 中身の幅の 1 行（今まで通り）。
 * - `wrapActions`: 1 行に収まらないので、バーを画面の幅いっぱいにし、縦線から後ろ
 *   （Select all・解除）を 2 行目の右端に回す。
 * - `wrapFavorite`: それでも 1 行目が収まらないので、「Favorite」以降も 2 行目の縦線の前へ回す。
 */
export type BarLayout = "one" | "wrapActions" | "wrapFavorite";

/** smQuery は Tailwind の `sm`（40rem）と同じ境界である。 */
const smQuery = "(min-width: 40rem)";

function px(value: string): number {
  const parsed = Number.parseFloat(value);
  return Number.isFinite(parsed) ? parsed : 0;
}

/**
 * measureBarLayout は、バーの中の `data-bar-item` の項目（sm 以上の並び順と DOM の順が同じ）の
 * 実際の幅を足し、外側の枠（画面の幅から `px-4` を除いた幅）に何行目まで収まるかを決める。
 * 項目はどれも縮まない（`shrink-0`・`whitespace-nowrap`）ので、割り方によって幅は変わらない。
 * 判定は件数と操作の実際の幅で決まる（ui-design.md「Layout」）。sm 未満は容器の問い合わせで
 * 割るので、ここでは `one` を返す。
 */
export function measureBarLayout(bar: HTMLElement, frame: HTMLElement): BarLayout {
  if (typeof window.matchMedia === "function" && !window.matchMedia(smQuery).matches) {
    return "one";
  }
  const items = Array.from(bar.querySelectorAll<HTMLElement>(":scope > [data-bar-item]"));
  const widths = items.map((item) => item.getBoundingClientRect().width);
  const barStyle = getComputedStyle(bar);
  const frameStyle = getComputedStyle(frame);
  const gap = px(barStyle.columnGap);
  const chrome =
    px(barStyle.paddingLeft) +
    px(barStyle.paddingRight) +
    px(barStyle.borderLeftWidth) +
    px(barStyle.borderRightWidth);
  const available =
    frame.clientWidth - px(frameStyle.paddingLeft) - px(frameStyle.paddingRight);
  const need = (end: number) => {
    let sum = chrome;
    for (let index = 0; index < end; index += 1) sum += widths[index] ?? 0;
    return sum + gap * Math.max(end - 1, 0);
  };
  if (need(items.length) <= available) return "one";
  const divider = items.findIndex((item) => item.dataset.barItem === "divider");
  if (divider > 0 && need(divider) <= available) return "wrapActions";
  return "wrapFavorite";
}

/**
 * useBarLayout は枠と項目の大きさが変わるたびに measureBarLayout を測り直す。`signature` は
 * 項目の中身（件数の文字・「Bundle as versions」の有無・「Selecting…」）が変わったことを伝える。
 */
function useBarLayout(
  bar: RefObject<HTMLDivElement | null>,
  signature: string,
): BarLayout {
  const [layout, setLayout] = useState<BarLayout>("one");
  useLayoutEffect(() => {
    const element = bar.current;
    const frame = element?.parentElement ?? null;
    if (element === null || frame === null) return;
    const measure = () => setLayout(measureBarLayout(element, frame));
    measure();
    if (typeof ResizeObserver === "undefined") return;
    const observer = new ResizeObserver(measure);
    observer.observe(frame);
    for (const item of element.querySelectorAll(":scope > [data-bar-item]")) {
      observer.observe(item);
    }
    return () => observer.disconnect();
  }, [bar, signature]);
  return layout;
}

/** SelectionBar は 1 件以上選ぶと画面下部に浮く。 */
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

  const barRef = useRef<HTMLDivElement | null>(null);
  const layout = useBarLayout(
    barRef,
    `${String(count)}|${String(canBundle)}|${String(selectingAll)}`,
  );

  if (count === 0) return null;

  // sm 未満で下の段が 1 行に収まらない幅の問い合わせ（下の JSX の説明を参照）。Tailwind が
  // クラスを拾えるよう、幅ごとに文字列をそのまま書く。
  const fit = canBundle
    ? {
        shrink: "max-sm:@max-[49.5rem]:flex-none",
        left: "max-sm:@max-[49.5rem]:mr-auto",
        wrap: "max-sm:@max-[49.5rem]:block",
      }
    : {
        shrink: "max-sm:@max-[30.5rem]:flex-none",
        left: "max-sm:@max-[30.5rem]:mr-auto",
        wrap: "max-sm:@max-[30.5rem]:block",
      };
  const wrapped = layout !== "one";

  return (
    <div
      role="region"
      aria-label={t.library.selection.region}
      className="fixed inset-x-0 bottom-4 z-30 flex justify-center px-4"
    >
      {/*
       * sm 未満の2段では、下の段を「タグを付ける」「タグを外す」「Favorite」「公開」の4つ
       * （2 本以上を選ぶと「Bundle as versions」を足した5つ）で等分する。
       * 収まらない幅（360px など）では幅を auto にし、「Favorite」以降を次の段の右端に回す
       * （そこにも収まらなければさらに次の段の右端。specs/035-favorites/ui-design.md
       * 「Selection bar」の「Layout」）。段の右寄せはバーの justify-end で行い、左に寄せる段
       * （件数の段と「タグを外す」で終わる段）は、その段の最後の項目の mr-auto で左に留める。
       * 収まるかどうかはこのバーの幅で決まるので、sm 未満でだけバーを
       * コンテナにして問い合わせる（sm 以上では幅が内容で決まるので、
       * コンテナにすると幅が 0 に潰れる）。30.5rem は4つの最小の幅と間隔の和
       * （116px × 4 + 8px × 3）、49.5rem は5つの和
       * （いちばん広い「Bundle as versions」の 152px × 5 + 8px × 4 = 792px）で、
       * 5つが 1 段に収まる幅は sm 未満に無い。
       *
       * sm 以上は 1 行で、収まらないときは useBarLayout が測って割り方を決める
       * （BarLayout）。割るときはバーを画面の幅いっぱいにし、2 行目を右端に寄せる。
       */}
      <div
        ref={barRef}
        className={cn(
          "flex w-full flex-wrap items-center justify-end gap-x-2 gap-y-1.5 rounded-md border border-border-strong bg-elevated p-1.5 shadow-elevated animate-slide-up motion-reduce:animate-none max-sm:@container sm:pr-1.5 sm:pl-4",
          wrapped ? "sm:min-h-11" : "sm:h-11 sm:w-auto sm:flex-nowrap sm:py-0",
        )}
      >
        <span
          role="status"
          aria-live="polite"
          data-bar-item="count"
          className="order-1 shrink-0 px-1 text-sm whitespace-nowrap text-fg tabular-nums sm:px-0"
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
              data-bar-item="addTag"
              className={cn("order-5 max-sm:flex-1 sm:order-2", fit.shrink)}
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
              data-bar-item="removeTag"
              className={cn(
                "order-6 max-sm:flex-1 sm:order-3",
                fit.shrink,
                fit.left,
                layout === "wrapFavorite" && "sm:mr-auto",
              )}
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
        {overLimit && (
          <span id={overLimitId} className="sr-only">
            {overLimitMessage()}
          </span>
        )}

        {/*
         * 「Favorite」以降を次の段へ回す仕切り。sm 未満では下の段に収まらないとき、
         * sm 以上では wrapFavorite のときに出す。
         */}
        <span
          aria-hidden="true"
          className={cn(
            "order-7 hidden h-0 basis-full sm:order-4",
            fit.wrap,
            layout === "wrapFavorite" && "sm:block",
          )}
        />

        <FavoriteMenu
          videoIds={favoriteVideoIds}
          folders={favoriteFolders}
          overLimit={favoriteOverLimit}
          overLimitId={favoriteOverLimitId}
          data-bar-item="favorite"
          className={cn("order-8 max-sm:flex-1 sm:order-5", fit.shrink)}
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
          data-bar-item="visibility"
          className={cn(
            "order-9 max-sm:flex-1 sm:order-6",
            fit.shrink,
            !canBundle && layout === "wrapActions" && "sm:mr-auto",
          )}
        />

        {canBundle && (
          <Button
            variant="ghost"
            size="sm"
            data-bar-item="bundle"
            className={cn(
              "order-10 max-sm:flex-1 sm:order-7",
              fit.shrink,
              layout === "wrapActions" && "sm:mr-auto",
            )}
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
          >
            <Layers aria-hidden="true" />
            {t.library.selection.bundle}
          </Button>
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

        {/* 縦線から後ろを 2 行目へ回す仕切り（sm 以上の wrapActions のときだけ）。 */}
        <span
          aria-hidden="true"
          className={cn(
            "hidden h-0 basis-full sm:order-8",
            layout === "wrapActions" && "sm:block",
          )}
        />

        <span
          aria-hidden="true"
          data-bar-item="divider"
          className="hidden h-5 w-px shrink-0 bg-border-strong sm:order-9 sm:block"
        />

        <Button
          variant="ghost"
          size="sm"
          data-bar-item="selectAll"
          onClick={onSelectAll}
          disabled={selectingAll || allSelected}
          className="order-2 sm:order-10"
        >
          {selectingAll
            ? t.library.selection.selectingAll
            : t.library.selection.selectAll}
        </Button>
        <IconButton
          label={t.library.selection.clear}
          size="sm"
          data-bar-item="clear"
          onClick={onClear}
          className="order-3 max-sm:mr-auto sm:order-11"
        >
          <X />
        </IconButton>
      </div>
    </div>
  );
}
