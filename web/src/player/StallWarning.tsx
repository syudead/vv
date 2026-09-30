import { WifiLow, X } from "lucide-react";

import { t } from "../i18n";

/**
 * StallWarning は、回線の遅さで再生が途切れているときにプレイヤーの左上に出す、知らせる
 * だけの帯である（specs/027-playback-quality/ui-design.md「Stall warning」、research.md R-7）。
 *
 * 状態表示の入れ物（`data-overlay-layer`）とは別の層で、映像より上・状態表示と操作バーより
 * 下に置く。× 以外は下の操作（再生バー・映像・中央の操作）へ通す。画質を変える操作は
 * 置かない（親 Issue 要件 10）。
 */
export default function StallWarning({ onDismiss }: { onDismiss: () => void }) {
  const text = t.player.stallWarning;
  return (
    <div
      data-stall-warning=""
      className="pointer-events-none absolute top-2 left-2 z-[5] flex max-w-[calc(100%-1rem)] sm:top-3 sm:left-3"
    >
      <div
        role="status"
        className="flex items-center gap-2 rounded-md bg-navbar px-3 py-1.5 text-xs text-fg shadow-elevated animate-fade-in motion-reduce:animate-none"
      >
        <WifiLow className="size-4 shrink-0 text-warning" aria-hidden="true" />
        <span className="min-w-0">{text.message}</span>
        <button
          type="button"
          aria-label={text.dismiss}
          title={text.dismiss}
          onClick={onDismiss}
          className="pointer-events-auto -mr-1.5 inline-flex size-6 shrink-0 items-center justify-center rounded-sm text-fg-muted transition-colors hover:text-fg"
        >
          <X className="size-3.5" aria-hidden="true" />
        </button>
      </div>
    </div>
  );
}
