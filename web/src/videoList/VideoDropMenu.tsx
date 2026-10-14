import { Group, Layers } from "lucide-react";
import { type KeyboardEvent, useCallback, useId, useMemo, useState } from "react";

import { maxBundleSelection } from "../api/client";
import { t } from "../i18n";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuTrigger,
} from "../ui/shadcn/dropdown-menu";
import { Tooltip, TooltipContent, TooltipTrigger } from "../ui/shadcn/tooltip";
import type { CardAttachment, CardDrop, HeldDrop } from "./cardDrag";
import { bundleOverLimit } from "./videoDrop";

/**
 * DropMenuAnchor は落とした先のカードの下端に置く、メニューの位置の目印である。カードの箱
 * （`relative`）の中に置き、メニューはその下に開く。
 */
function DropMenuAnchor() {
  return (
    <DropdownMenuTrigger asChild>
      <span
        aria-hidden="true"
        className="pointer-events-none absolute inset-x-0 bottom-0"
      />
    </DropdownMenuTrigger>
  );
}

/**
 * VideoDropMenu は動画を動画に落としたときに、落とした先のカードに開くメニューである
 * （specs/050-drag-organize/ui-design.md「Drop menus」の Video on video）。「Make a group」と
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
  const closeOnTab = (event: KeyboardEvent) => {
    if (event.key === "Tab") onClose();
  };
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
    <DropdownMenu
      open
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
    >
      <DropMenuAnchor />
      <DropdownMenuContent
        side="bottom"
        onKeyDown={closeOnTab}
        // 位置の目印はフォーカスを受けない。閉じた後のフォーカスは画面に任せる（名前の欄）。
        onCloseAutoFocus={(event) => event.preventDefault()}
      >
        <DropdownMenuLabel>{t.organize.menu.label(name)}</DropdownMenuLabel>
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
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

/** VideoDropChoice は動画を動画に落としたメニューで選んだ操作の値である。 */
export interface VideoDropChoice {
  /** 持ち上げたカード（画面の順）。 */
  lifted: readonly string[];
  /** 落とした先のカード。 */
  target: string;
}

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
  const [held, setHeld] = useState<HeldDrop | null>(null);
  const hold = useCallback(
    ({ lifted, sign }: CardDrop) => {
      if (sign.kind !== "onto" || !isVideo(sign.key)) return false;
      setHeld({ lifted, sign });
      return true;
    },
    [isVideo],
  );
  const close = useCallback(() => setHeld(null), []);
  const attached = useMemo((): CardAttachment | null => {
    if (held === null) return null;
    const choice = { lifted: held.lifted, target: held.sign.key };
    return {
      key: choice.target,
      node: (
        <VideoDropMenu
          name={nameOf(choice.target)}
          liftedCount={choice.lifted.length}
          onGroup={() => onGroup(choice)}
          onBundle={() => onBundle(choice)}
          onClose={close}
        />
      ),
    };
  }, [close, held, nameOf, onBundle, onGroup]);
  return { held, attached, hold };
}
