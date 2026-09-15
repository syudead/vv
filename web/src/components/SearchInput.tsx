import { MAX_QUERY_LENGTH } from "../api/client";
import Icon from "../layout/icons";

/**
 * SearchInput は題名で探す入力欄である（C10 / spec US4-1）。
 *
 * **変えるのは見た目だけである。** 待ち合わせ 250ms は呼び出し側（LibraryPage）が
 * 持ち、`maxLength` は API の上限（`MAX_QUERY_LENGTH`）をそのまま使う
 * （data-model.md 5.「変わらないもの」）。この部品は打鍵を上へ流すだけで、
 * 待ち合わせも URL の書き換えも知らない — 知ってしまうと、004 の
 * LibraryPage.search.test.tsx が守っている振る舞いが 2 か所に散る。
 *
 * 入力そのものは `<input type="search">` のままにする。型を変えると、端末の
 * キーボードに出る確定キーの文言と、ブラウザが付ける消去の × が変わる。
 *
 * 虫眼鏡は `aria-hidden`（Icon が一律に付ける）で、意味は隣の
 * `<span class="sr-only">` と `placeholder` が持つ（contracts/components.md 3.）。
 */

/**
 * field は入力欄の枠である。地・枠線・角丸・4 状態を**枠が持ち**、入力そのものは
 * 透明にする（contracts/components.md 2.）。
 *
 * 地を枠に載せるのは、hover / active の「1 段明るく / 暗く」を `::after` の覆いで
 * 表すためである。`<input>` は空要素なので `::after` を持てない ── 覆いを置ける
 * 要素が要る。`isolate` + `-z-10` で覆いは枠の地の上・中身の下に敷かれるので、
 * 文字とアイコンは覆われない（NavItem と同じ仕掛け）。
 *
 * 幅は固定しない。`basis-48` は「これを下回るなら自分の行へ折り返す」目安であって
 * 幅ではなく、`flex-1` が帯の残りを吸うので広い画面では伸びる。`min-w-0` が要る
 * のは、入力欄の既定の最小幅が flex の縮小を止め、幅 360px で帯からはみ出す
 * ためである（004 の FR-022 / SC-004）。
 */
const field =
  "relative isolate flex min-w-0 flex-1 basis-48 items-center rounded-control " +
  "border border-border bg-surface-raised text-muted " +
  "after:pointer-events-none after:absolute after:inset-0 after:-z-10 " +
  "after:rounded-control after:transition-colors " +
  "hover:after:bg-body/10 active:after:bg-surface-sunken/60 " +
  "motion-reduce:after:transition-none";

/**
 * input は入力そのものである。当たり判定は `--size-tap`（44px）以上を保つ
 * （004 の FR-022 を FR-018 が引き継ぐ）。
 *
 * `pl-9` は虫眼鏡のための余地である。アイコンは絶対位置で重ねるので、余地を
 * 取らないと打った文字がアイコンに重なる。
 */
const input =
  "min-h-[var(--size-tap)] w-full bg-transparent pr-3 pl-9 text-sm text-body " +
  "placeholder:text-muted " +
  "outline-offset-2 focus-visible:outline-2 focus-visible:outline-focus";

export default function SearchInput({
  value,
  onChange,
}: {
  /** いま入力欄にある文字列（打鍵の受け皿は呼び出し側が持つ）。 */
  value: string;
  /** 打鍵のたびに呼ばれる。待ち合わせは呼び出し側が掛ける。 */
  onChange: (value: string) => void;
}) {
  return (
    <label className={field}>
      <span className="sr-only">題名で探す</span>
      <Icon name="search" className="pointer-events-none absolute left-3 h-4 w-4" />
      <input
        type="search"
        value={value}
        onChange={(event) => onChange(event.target.value)}
        maxLength={MAX_QUERY_LENGTH}
        placeholder="題名で探す"
        className={input}
      />
    </label>
  );
}
