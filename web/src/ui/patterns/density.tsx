import { createContext, type ReactNode, useContext } from "react";

// 画面の密度。管理の画面（一覧・設定・管理表）は management、見る画面（詳細ページ）は
// viewing で、区画は密度に合わせて行の余白と文字の段を選ぶ
// （web/registry/rules/foundations.md の Spacing and sizes）。骨格だけが密度を決める。

export type Density = "management" | "viewing";

const DensityContext = createContext<Density>("management");

/** DensityProvider は骨格が自分の区画に密度を渡すために使う。画面は使わない。 */
export function DensityProvider({
  density,
  children,
}: {
  density: Density;
  children: ReactNode;
}) {
  return <DensityContext value={density}>{children}</DensityContext>;
}

/** useDensity は区画が囲む骨格の密度を読む。 */
export function useDensity(): Density {
  return useContext(DensityContext);
}
