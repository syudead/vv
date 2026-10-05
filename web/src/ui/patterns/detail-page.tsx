import type { ReactNode } from "react";

import { DensityProvider } from "./density";

// 詳細ページ（骨格）。画面の幅いっぱいに、上端の見出しの帯（下に線）と、その下の主領域・
// 横の情報欄（左に線）を並べる。lg より狭い幅では縦に積み、メディアは端から端まで、その下の
// 中身と情報欄は左右に余白を取る。見る画面なので、区画は viewing の密度で並ぶ。
// 狭い幅はページ全体が 1 つとしてスクロールし、見出しの帯は上に留まる。lg 以上で呼び手が
// 高さを画面に留める（lg:h-dvh の flex の列の中に置く）と、帯の下で主領域と情報欄がそれぞれ
// 中でスクロールする（情報欄は、中身の並びが自分でスクロールする入れ物を持つ）。ページと列の
// 両方がスクロールして二重に動くことがないようにするためである。
// 規則は web/registry/rules/patterns.md の Detail page。

export interface DetailPageProps {
  /** 見出しの帯の中身: 左にロゴや戻る操作と場所、右端に閉じる操作。 */
  header: ReactNode;
  /** 主領域の先頭のメディア（プレイヤー、画像）。lg より狭い幅では端から端まで。 */
  media: ReactNode;
  /** 横の情報欄: 関連する項目の並び。 */
  aside?: ReactNode;
  /** メディアの下の中身: 題名・タグ・情報の行。 */
  children: ReactNode;
}

export function DetailPage({ header, media, aside, children }: DetailPageProps) {
  return (
    <DensityProvider density="viewing">
      <div
        data-slot="detail-page"
        className="flex flex-col bg-background pb-16 lg:min-h-0 lg:flex-1 lg:pb-0"
      >
        <div
          data-slot="detail-page-header"
          className="sticky top-0 z-40 flex h-navbar shrink-0 items-center gap-1 border-b border-border bg-background/90 px-2 backdrop-blur-md sm:px-3"
        >
          {header}
        </div>
        <div className="flex w-full flex-col gap-5 lg:min-h-0 lg:flex-1 lg:flex-row lg:gap-6 lg:px-6">
          {/* 列の端にあるフォーカスの輪が切れないよう、スクロールする列は輪の分だけ外へ広げて
              内側に余白を取る（-mx-1 と px-1）。 */}
          <div
            data-slot="detail-page-main"
            className="flex min-w-0 flex-col gap-5 lg:-mx-1 lg:min-h-0 lg:flex-1 lg:scrollbar-none lg:overflow-y-auto lg:overscroll-contain lg:px-1 lg:py-6"
          >
            {media}
            <div className="flex flex-col gap-5 px-4 sm:px-6 lg:px-0">{children}</div>
          </div>
          {aside && (
            <aside
              data-slot="detail-page-aside"
              className="min-w-0 border-t border-border px-4 pt-5 sm:px-6 lg:flex lg:min-h-0 lg:w-detail-aside lg:shrink-0 lg:flex-col lg:border-t-0 lg:border-l lg:pt-6 lg:pl-5 xl:w-detail-aside-wide"
            >
              {aside}
            </aside>
          )}
        </div>
      </div>
    </DensityProvider>
  );
}
