import type { Processing } from "../api/client";
import { cn } from "../lib/cn";
import { processingRemaining } from "./ScanProvider";

const stages: { key: keyof Processing; label: string }[] = [
  { key: "probe", label: "解析" },
  { key: "thumbnail", label: "サムネイル" },
  { key: "seekThumbnail", label: "シーク用" },
  { key: "preview", label: "プレビュー" },
];

/**
 * ProcessingBreakdown は取り込みの段階ごとの残りを4列で示す。残りが無ければ
 * 何も出さない。フローティング表示の概要と設定画面の詳細で同じものを使う。
 *
 * ラベルは途中で折り返さない（break-keep）ので、最長の5文字ラベル4つと列の間隔が
 * 収まらない幅（拡大表示の狭い画面など）では2列に落とす。判定は置き場の幅を
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
        aria-label="準備の残り"
        className="grid grid-cols-2 gap-x-1 gap-y-2 @min-[21em]:grid-cols-4"
      >
        {stages.map((stage) => (
          <div key={stage.key} className="min-w-0">
            <dt className="break-keep">{stage.label}</dt>
            <dd className="tabular-nums text-fg">{String(processing[stage.key])} 件</dd>
          </div>
        ))}
      </dl>
    </div>
  );
}
