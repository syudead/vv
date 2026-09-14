import type { Density } from "../preferences/viewPreferences";

/**
 * densityLabels は密度の選択肢である。
 *
 * 「細かい」「標準」「ゆったり」は見え方の言葉であって、列数ではない。
 * 列数は画面幅と組で決まる（contracts/design-tokens.md 3.）ので、
 * 「6 列」のような名前を付けると狭い画面で嘘になる。
 */
const densityLabels: { value: Density; label: string }[] = [
  { value: "dense", label: "細かい" },
  { value: "standard", label: "標準" },
  { value: "relaxed", label: "ゆったり" },
];

/**
 * DensitySelect は一覧の表示密度を選ぶ選択欄である（FR-017）。
 *
 * 帯の 3 番目の口に差し込む（contracts/screen-states.md 3. の到達順）。
 * 狙いを合わせている印は Toolbar が帯の中の操作すべてに与えるので、
 * ここでは書かない。
 *
 * 当たり判定は --size-tap（44px）四方以上にする（FR-022）。選択欄は文字の
 * 高さだけだと指で押すには小さい。
 */
export default function DensitySelect({
  value,
  onChange,
}: {
  value: Density;
  onChange: (value: Density) => void;
}) {
  return (
    <label className="flex items-center gap-2 text-sm text-muted">
      表示
      <select
        value={value}
        onChange={(event) => onChange(toDensity(event.target.value))}
        className="min-h-[var(--size-tap)] min-w-[var(--size-tap)] rounded-control border border-border bg-surface-raised px-2 text-sm text-body"
      >
        {densityLabels.map((option) => (
          <option key={option.value} value={option.value}>
            {option.label}
          </option>
        ))}
      </select>
    </label>
  );
}

/**
 * toDensity は選択欄の値を Density に直す。
 *
 * 選択欄が出す値は densityLabels のどれかだが、change の値は文字列なので
 * 型の上では絞られていない。照合を densityLabels に対して行うことで、
 * 選べる値と受け付ける値を 1 か所に保つ。
 */
function toDensity(value: string): Density {
  return densityLabels.find((option) => option.value === value)?.value ?? "standard";
}
