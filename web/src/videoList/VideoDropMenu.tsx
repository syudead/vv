import { Group, Layers } from "lucide-react";
import { useCallback, useId } from "react";

import { maxBundleSelection } from "../api/client";
import { t } from "../i18n";
import { DropdownMenuItem } from "../ui/shadcn/dropdown-menu";
import { Tooltip, TooltipContent, TooltipTrigger } from "../ui/shadcn/tooltip";
import type { CardDrop } from "./cardDrag";
import { type DropChoice, DropMenu, useDropMenu } from "./dropMenu";
import { bundleOverLimit } from "./videoDrop";

/**
 * VideoDropMenu は動画を動画に落としたときに、落とした先のカードに開くメニューである。「Make a group」と
 * 「Bundle as versions」の 2 つで、持ち上げたカードと落とした先が 20 枚を超えれば後者を押せなく
 * して理由を添える。Esc、外の押下、Tab で閉じると何も選ばずに `onClose` を呼ぶ。
 */
export function VideoDropMenu({
  name,
  liftedCount,
  onGroup,
  onBundle,
  onClose,
}: {
  /** 落とした先の動画の題名。 */
  name: string;
  /** 持ち上げたカードの枚数。 */
  liftedCount: number;
  onGroup: () => void;
  onBundle: () => void;
  /** メニューが閉じたら呼ぶ（項目を選んだ後も呼ぶ）。 */
  onClose: () => void;
}) {
  const overLimit = bundleOverLimit(liftedCount);
  const overLimitId = useId();
  const bundle = (
    <DropdownMenuItem
      disabled={overLimit}
      aria-describedby={overLimit ? overLimitId : undefined}
      onSelect={onBundle}
    >
      <Layers aria-hidden="true" />
      {t.library.selection.bundle}
    </DropdownMenuItem>
  );
  return (
    <DropMenu name={name} onClose={onClose}>
      <DropdownMenuItem onSelect={onGroup}>
        <Group aria-hidden="true" />
        {t.organize.menu.makeGroup}
      </DropdownMenuItem>
      {overLimit ? (
        // 押せない項目はポインタを受けないので、包みでツールチップを出す。
        <Tooltip>
          <TooltipTrigger asChild>
            <span className="block">{bundle}</span>
          </TooltipTrigger>
          <TooltipContent>
            {t.library.selection.bundleOverLimit(maxBundleSelection)}
          </TooltipContent>
        </Tooltip>
      ) : (
        bundle
      )}
      {overLimit && (
        <span id={overLimitId} className="sr-only">
          {t.library.selection.bundleOverLimit(maxBundleSelection)}
        </span>
      )}
    </DropMenu>
  );
}

/** VideoDropChoice は動画を動画に落としたメニューで選んだ操作の値である。 */
export type VideoDropChoice = DropChoice;

/**
 * useVideoDropMenu は動画を動画に落としたときのメニューを開く。`hold` は落とし方を受け、
 * 落とした先が動画なら（`isVideo`）メニューを開いて true を返す。メニューの間は持ち上げた
 * カードと落とした先の印を保ち（`held`）、メニューを落とした先のカードに重ねる（`attached`）。
 * 項目を選ぶと `onGroup` か `onBundle`、閉じると何もしない。
 */
export function useVideoDropMenu({
  isVideo,
  nameOf,
  onGroup,
  onBundle,
}: {
  isVideo: (key: string) => boolean;
  nameOf: (key: string) => string;
  onGroup: (choice: VideoDropChoice) => void;
  onBundle: (choice: VideoDropChoice) => void;
}) {
  const accepts = useCallback(({ sign }: CardDrop) => isVideo(sign.key), [isVideo]);
  const menu = useCallback(
    (choice: VideoDropChoice, close: () => void) => (
      <VideoDropMenu
        name={nameOf(choice.target)}
        liftedCount={choice.lifted.length}
        onGroup={() => onGroup(choice)}
        onBundle={() => onBundle(choice)}
        onClose={close}
      />
    ),
    [nameOf, onBundle, onGroup],
  );
  return useDropMenu(accepts, menu);
}
