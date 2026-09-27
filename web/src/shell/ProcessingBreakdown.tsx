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
    <dl aria-label="準備の残り" className={cn("grid grid-cols-4 gap-x-1", className)}>
      {stages.map((stage) => (
        <div key={stage.key} className="min-w-0">
          <dt className="break-keep">{stage.label}</dt>
          <dd className="tabular-nums text-fg">{String(processing[stage.key])} 件</dd>
        </div>
      ))}
    </dl>
  );
}
