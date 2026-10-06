import type { ReactNode } from "react";

import type { Zoom } from "../preferences/viewPreferences";
import { CardGrid } from "../ui/patterns/card-grid";

/**
 * Grid はカードの格子である。ライブラリとフォルダ画面が同じ幅・同じ間隔で使う。
 * 画面の型の CardGrid に表示倍率（card-0〜card-3）を渡す: カードは列の幅いっぱいに伸び、
 * 左右の端がツールバーと揃う。狭い幅では 1 列になる。
 */
export function Grid({ zoom, children }: { zoom: Zoom; children: ReactNode }) {
  return <CardGrid size={zoom}>{children}</CardGrid>;
}
