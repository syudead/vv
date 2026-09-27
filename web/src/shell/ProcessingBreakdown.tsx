import type { Processing } from "../api/client";
import { formatNumber, t, type UiText } from "../i18n";
import { cn } from "../lib/cn";
import { processingRemaining } from "./ScanProvider";

const stageKeys: readonly (keyof Processing)[] = [
  "probe",
  "thumbnail",
  "seekThumbnail",
  "preview",
];

function stageLabel(key: keyof Processing): UiText {
  return t.shell.scan.breakdown[key];
}

/**
 * ProcessingBreakdown は取り込みの段階ごとの残りを4列で示す。残りが無ければ
 * 何も出さない。フローティング表示の概要と設定画面の詳細で同じものを使う。
 *
 * ラベルは語の途中で折り返さない（break-keep）ので、最長のラベル（Thumbnails）4つと
 * 列の間隔が収まらない幅（拡大表示の狭い画面など）では2列に落とす。判定は置き場の幅を
 * フォントの大きさ（em）で見るので、text-xs のポップオーバーでも text-sm の
 * 設定画面でも同じ条件で働く。
 */
export default function ProcessingBreakdown({
  processing,
  className,
}: {
  processing: Processing | null;
  className?: string;
}) {
  if (processing === null || processingRemaining(processing) === 0) return null;
  return (
    <div className={cn("@container", className)}>
      <dl
        aria-label={t.shell.scan.breakdown.label}
        className="grid grid-cols-2 gap-x-1 gap-y-2 @min-[26em]:grid-cols-4"
      >
        {stageKeys.map((key) => (
          <div key={key} className="min-w-0">
            <dt className="break-keep">{stageLabel(key)}</dt>
            <dd className="tabular-nums text-fg">{formatNumber(processing[key])}</dd>
          </div>
        ))}
      </dl>
    </div>
  );
}
