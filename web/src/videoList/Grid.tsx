import type { CSSProperties, ReactNode } from "react";

import type { Zoom } from "../preferences/viewPreferences";

/** cardWidth は表示倍率ごとのカードの幅である。 */
export const cardWidth: Record<Zoom, string> = {
  0: "var(--spacing-card-0)",
  1: "var(--spacing-card-1)",
  2: "var(--spacing-card-2)",
  3: "var(--spacing-card-3)",
};

/** Grid はカードの格子である。ライブラリとフォルダ画面が同じ幅・同じ間隔で使う。 */
export function Grid({ zoom, children }: { zoom: Zoom; children: ReactNode }) {
  return (
    <div
      className="flex flex-wrap justify-center gap-2.5 [&>*]:w-[min(var(--card),100%)]"
      style={{ "--card": cardWidth[zoom] } as CSSProperties}
    >
      {children}
    </div>
  );
}
