import type { ReactNode } from "react";

import { DensityProvider } from "./density";

// 詳細ページ（骨格）。見出し帯の下に主領域と横の情報欄を並べ、lg より狭い幅では縦に
// 積む。見る画面なので、区画は viewing の密度（行の余白 p-4、本文 text-base）で並ぶ。
// lg 以上で呼び手が高さを画面に留める（flex の列の中に置く）と、主領域と情報欄はそれぞれ
// 中でスクロールし、見出し帯と主領域の先頭（動画）は動かない。高さを留めなければページ
// ごとスクロールする。
// 規則は web/registry/rules/patterns.md の Detail page。

export interface DetailPageProps {
  /** PageHeader（戻る操作・題・主操作）。 */
  header: ReactNode;
  /** 横の情報欄: PageSection と FactList。 */
  aside?: ReactNode;
  /** 主領域: 動画や画像と、その下の PageSection。 */
  children: ReactNode;
}

export function DetailPage({ header, aside, children }: DetailPageProps) {
  return (
    <DensityProvider density="viewing">
      <div
        data-slot="detail-page"
        className="mx-auto flex w-full max-w-7xl flex-col gap-6 p-4 lg:min-h-0 lg:flex-1 lg:p-6"
      >
        {header}
        {/* 列の端にあるフォーカスの輪が切れないよう、スクロールする列は輪の分だけ外へ広げて
            内側に余白を取る（-m-1 と p-1）。 */}
        <div className="flex flex-col gap-6 lg:min-h-0 lg:flex-1 lg:flex-row">
          <div
            data-slot="detail-page-main"
            className="flex min-w-0 flex-1 flex-col gap-6 lg:-m-1 lg:min-h-0 lg:scrollbar-none lg:overflow-y-auto lg:overscroll-contain lg:p-1"
          >
            {children}
          </div>
          {aside && (
            <aside
              data-slot="detail-page-aside"
              className="flex flex-col gap-6 lg:-m-1 lg:min-h-0 lg:w-detail-aside lg:shrink-0 lg:scrollbar-on-hover lg:overflow-y-auto lg:overscroll-contain lg:p-1"
            >
              {aside}
            </aside>
          )}
        </div>
      </div>
    </DensityProvider>
  );
}
