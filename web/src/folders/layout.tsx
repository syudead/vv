import type { ReactNode } from "react";

import { formatNumber, type UiText } from "../i18n";

/**
 * Section はフォルダの中身の一群（「フォルダ N」「動画 N」）で、見出しの行と
 * カードの格子を縦に並べる。見出しの件数も読み上げる。フォルダ画面の本体は子フォルダと
 * 動画の2つの一群を持つので、一覧ページの本体の中にこの見出しを置く。
 */
export function Section({
  title,
  count,
  action,
  children,
}: {
  title: UiText;
  count?: number;
  /** 見出しの行の右端に置く操作（フォルダ画面の「Folder grouping menu」）。 */
  action?: ReactNode;
  children: ReactNode;
}) {
  return (
    <section className="flex flex-col gap-2">
      <div className="flex min-h-8 items-center justify-between gap-2">
        <h2 className="text-xs font-semibold text-muted-foreground">
          {title}
          {count !== undefined && (
            // 読み上げで名前と件数が続けて読まれないよう、余白ではなく空白で区切る。
            <span className="tabular-nums"> {formatNumber(count)}</span>
          )}
        </h2>
        {action}
      </div>
      {children}
    </section>
  );
}
