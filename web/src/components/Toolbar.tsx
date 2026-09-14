import type { Ref, ReactNode } from "react";

/**
 * Toolbar は一覧の入口を常に画面上に置く帯である
 * （FR-007 / R-404 / contracts/screen-states.md 1.「固定の帯」）。
 *
 * **透過させない。** 半透過 + backdrop-filter は見栄えはよいが、背後の静止画に
 * よって文字との対比が変わり、SC-006（対比 100%）を静的に保証できなくなる。
 * 不透明にすれば対比はトークンの組だけで決まる（R-404）。
 *
 * 子は「探す」「並べ替える」「密度」「取り込む」を**この順**で受け取る。
 * contracts/screen-states.md 3. の到達順がそのまま DOM の順である — 順番を
 * 呼び出し側の並べ方に任せると、画面ごとに到達順が変わってしまう。
 *
 * 帯の中の操作できる要素すべてに、狙いを合わせている印（FR-003）をここで
 * 与える。輪郭は要素の**外側**に描く（outline-offset）ので、隣の操作に
 * 隠れない。:focus-visible を使うのは、ポインタで押しただけでは印を出さない
 * ためである。個々の呼び出し側に書かせると、足した操作が印を持たないまま
 * 帯に入りうる。
 */
export default function Toolbar({
  search,
  sort,
  density,
  scan,
  ref,
}: {
  /** 探す（検索欄）。 */
  search: ReactNode;
  /** 並べ替える（選択欄）。 */
  sort: ReactNode;
  /** 表示の密度（選択欄。FR-017）。US3 で中身が入るまでは空でよい。 */
  density?: ReactNode;
  /** 取り込む（状態の文言 + ボタン。FR-010）。 */
  scan: ReactNode;
  /**
   * 帯そのものへの参照。
   *
   * 帯は sticky で中身に重なるので、下の内容を位置合わせする側は
   * **実測の高さ**を知る必要がある。帯は折り返して高さが変わる（狭い画面では
   * 2 行以上になる）ため、呼び出し側が固定値を持つと必ずずれる。
   */
  ref?: Ref<HTMLDivElement>;
}) {
  return (
    <div
      ref={ref}
      className={
        "sticky top-0 z-20 border-b border-border bg-surface " +
        "[&_:is(input,select,button)]:outline-offset-2 " +
        "[&_:is(input,select,button)]:focus-visible:outline-2 " +
        "[&_:is(input,select,button)]:focus-visible:outline-focus"
      }
    >
      <div className="mx-auto flex max-w-6xl flex-wrap items-center gap-x-4 gap-y-2 px-4 py-2">
        {search}
        {sort}
        {density}
        {scan}
      </div>
    </div>
  );
}
