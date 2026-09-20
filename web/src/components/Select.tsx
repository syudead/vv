import Icon from "../layout/icons";

/**
 * Select は原案の体裁の選択欄である（C11 / spec US4-2）。
 *
 * **`<select>` そのものは保つ。** 自前の一覧に置き換えると、端末ごとの選択 UI
 * （携帯の回転盤、キーボードの先頭一致）と読み上げの扱いを全部こちらで作り直す
 * ことになり、FR-013（利用者が行える操作を変えない）を守るのが難しくなる。
 * 外すのは**見た目だけ**で、`appearance: none` と自前の下向き記号に替える。
 *
 * 選択肢は呼び出し側が渡す。並べ替えの 2 つ（`addedDesc` / `titleAsc`）を
 * 増やさないのは呼び出し側の責任で、この部品は数を知らない（FR-013）。
 */

/**
 * box は選択欄の枠である。地・枠線・4 状態を枠が持ち、`<select>` そのものは
 * 透明にする（SearchInput と同じ理由 ── 覆いを置ける要素が要る）。
 */
const box =
  "relative isolate inline-flex items-center rounded-control " +
  "border border-border bg-surface-raised text-body shadow-sm " +
  "transition-[border-color,background-color] hover:border-body/80 " +
  "focus-within:border-accent focus-within:outline-2 focus-within:outline-offset-2 focus-within:outline-focus " +
  "after:pointer-events-none after:absolute after:inset-0 after:-z-10 " +
  "after:rounded-control after:transition-colors " +
  "hover:after:bg-body/10 active:after:bg-surface-sunken/60 " +
  "motion-reduce:transition-none motion-reduce:after:transition-none";

/**
 * control は選択そのものである。`appearance-none` がブラウザ既定の矢印を消すので、
 * 右の余地（`pr-8`）に自前の記号を重ねる。
 *
 * 当たり判定は `--size-tap` 四方以上を保つ（004 の FR-022）。
 */
const control =
  "min-h-[var(--size-tap)] w-24 min-w-[var(--size-tap)] appearance-none " +
  "cursor-pointer bg-transparent pr-8 pl-3 text-sm text-body outline-none";

export default function Select({
  label,
  value,
  onChange,
  options,
}: {
  /** 見えるラベル。読み上げの名前もこれになる（`<label>` が包む）。 */
  label: string;
  value: string;
  /**
   * 選ばれた値をそのまま渡す。
   *
   * 文字列のまま渡すのは、受け付ける値の照合先を**呼び出し側の表**に 1 本化する
   * ためである（LibraryPage の `toSort` がその表である）。ここで型を絞ると、
   * 同じ照合が 2 か所に増える。
   */
  onChange: (value: string) => void;
  options: readonly { value: string; label: string }[];
}) {
  return (
    <label className="flex items-center gap-2 text-sm text-muted">
      <span className="sr-only">{label}</span>
      <span className={box}>
        <select
          value={value}
          onChange={(event) => onChange(event.target.value)}
          className={control}
        >
          {options.map((option) => (
            <option key={option.value} value={option.value}>
              {option.label}
            </option>
          ))}
        </select>
        <Icon name="chevron" className="pointer-events-none absolute right-2 h-4 w-4" />
      </span>
    </label>
  );
}
