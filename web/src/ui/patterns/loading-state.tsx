import { Skeleton } from "@/ui/shadcn/skeleton";

import { CardGrid, type CardSize } from "./card-grid";

// 読み込み中（状態）。出来上がりの形の Skeleton を、骨格の本体の位置に並べる。
// ページ全体を覆う回転の印は出さない。規則は web/registry/rules/patterns.md の States。

export interface LoadingStateProps {
  /** 読み込んでいるものの名前（「Loading videos」）。支援技術に読まれる。 */
  label: string;
  /** 出来上がりの形: カードのグリッドか、表の行か、時間軸（Timeline）の行か。 */
  layout: "grid" | "table" | "timeline";
  /** grid のときのカードの大きさ。データが来たときの CardGrid と同じにする。 */
  size?: CardSize;
  /** 並べる数。既定は grid と table で 8、timeline で 3。 */
  count?: number;
}

export function LoadingState({ label, layout, size = 1, count }: LoadingStateProps) {
  const keys = Array.from(
    { length: count ?? (layout === "timeline" ? 3 : 8) },
    (_, index) => index,
  );
  return (
    <div data-slot="loading-state" role="status" aria-label={label} aria-busy="true">
      {layout === "grid" ? (
        <CardGrid size={size}>
          {keys.map((key) => (
            <div key={key} className="flex flex-col gap-2">
              <Skeleton className="aspect-video w-full" />
              <Skeleton className="h-4 w-3/4" />
              <Skeleton className="h-3 w-1/2" />
            </div>
          ))}
        </CardGrid>
      ) : layout === "timeline" ? (
        // Timeline と同じ列: 見出しの塊と、1 本の線の上のサムネイルの大きさの行。
        <div className="grid lg:grid-cols-timeline-group">
          <div className="border-l border-border pb-1 pl-4 lg:border-l-0 lg:pt-2 lg:pr-3 lg:pb-0 lg:pl-0">
            <div className="flex flex-col gap-1">
              <Skeleton className="h-4 w-16" />
              <Skeleton className="h-3 w-12" />
            </div>
          </div>
          <div className="flex flex-col border-l border-border">
            {keys.map((key) => (
              <div key={key} className="flex items-start gap-3 py-2 pl-4">
                <Skeleton className="hidden h-3 w-timeline-time lg:block" />
                <Skeleton className="aspect-video w-timeline-thumb-sm shrink-0 lg:w-timeline-thumb" />
                <div className="flex min-w-0 flex-1 flex-col gap-2">
                  <Skeleton className="h-4 w-3/4" />
                  <Skeleton className="h-3 w-1/2" />
                </div>
              </div>
            ))}
          </div>
        </div>
      ) : (
        <div className="flex flex-col divide-y divide-border overflow-hidden rounded-md border border-border bg-card">
          {keys.map((key) => (
            <div key={key} className="flex h-10 items-center gap-3 px-2">
              <Skeleton className="size-4 rounded-sm" />
              <Skeleton className="h-4 w-1/3" />
              <Skeleton className="ml-auto h-4 w-12" />
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
