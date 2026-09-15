import type { Ref, ReactNode } from "react";

import IconButton from "../layout/IconButton";

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
 * 帯の中の操作できる要素すべてに、狙いを合わせている印（FR-003）と指で押せる
 * 大きさ（`--size-tap` = 44px 四方。FR-022）をここで与える。輪郭は要素の
 * **外側**に描く（outline-offset）ので、隣の操作に隠れない。:focus-visible を
 * 使うのは、ポインタで押しただけでは印を出さないためである。個々の呼び出し側に
 * 書かせると、足した操作が印も当たり判定も持たないまま帯に入りうる。
 *
 * 中身は狭い画面で**2 行以上に折り返す**（重ねない。R-404）。行が増えた分だけ
 * 帯が高くなるので、下の内容を位置合わせする側は ref から実測の高さを取る。
 * 折り返しを使うのは、幅 360px から 2560px までのどの幅でも 4 つが重ならない
 * ことを、画面幅の場合分けを持たずに満たせるからである
 * （contracts/screen-states.md 3.「指」）。
 *
 * 005 で変わったのは 2 つだけである。
 *
 * - 粘る位置がヘッダーの下端（`--size-header`）になった。0 のままだとヘッダーの
 *   裏へ潜る（contracts/layout.md 1.「重なりの順序」）。z-20 はヘッダー（z-30）
 *   より後ろ、一覧の項目より前という関係を保つ
 * - 件数を帯の**右端**で受けるようになった。一覧の見出し文字が無くなり、
 *   置き場所がここへ移ったためである（R-508）
 *
 * 原案にある漏斗（フィルタ）と表示切替は**この帯が自分で描く**（C7 / T032）。
 * 裏側の機能が無い表示のみの要素なので、呼び出し側が差し込む口（`search` や
 * `sort`）にしない ── 口にすると「何を差すか」が画面ごとに決められるように
 * 見えるが、差すものは永遠に無い。押しても何も起きず `Tab` でも止まらない
 * ことは IconButton が守る（FR-005 / contracts/components.md 3.）。
 */
export default function Toolbar({
  search,
  sort,
  density,
  scan,
  count,
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
   * 件数（「N 本」「「語」に一致 N 本」「読み込み中…」。R-508）。
   *
   * 帯の右端に置く。文言も読み上げの扱い（role="status" / aria-live）も
   * 呼び出し側が持つ ── 004 の FR-008 / FR-021 をそのまま連れて来るだけで、
   * 帯は置き場所だけを決める（FR-018）。
   */
  count?: ReactNode;
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
        "sticky top-[var(--size-header)] z-20 border-b border-border bg-surface " +
        "[&_:is(input,select,button)]:min-h-[var(--size-tap)] " +
        "[&_:is(input,select,button)]:min-w-[var(--size-tap)] " +
        "[&_:is(input,select,button)]:outline-offset-2 " +
        "[&_:is(input,select,button)]:focus-visible:outline-2 " +
        "[&_:is(input,select,button)]:focus-visible:outline-focus"
      }
    >
      <div className="mx-auto flex max-w-6xl flex-wrap items-center gap-x-4 gap-y-2 px-4 py-2">
        {search}
        {/* 漏斗は検索の隣に置く。どちらも「絞り込む」入口だからである。
            label は「フィルタ」── 3 つの IconButton で文言を使い回すと、
            漏斗も表示切替も「設定」と読まれる（FR-006）。 */}
        <IconButton name="filter" label="フィルタ" />
        {sort}
        {density}
        {/* 表示切替（格子 / 一覧）は密度の隣に置く。どちらも一覧の見せ方を
            変える入口である。 */}
        <IconButton name="grid" label="表示切替" />
        {scan}
        {/* 件数は右端に寄せる。ml-auto にするのは、帯が折り返しても
            「その行の右端」に居られるからである（固定の幅を与えない）。 */}
        {count !== undefined && <div className="ml-auto">{count}</div>}
      </div>
    </div>
  );
}
