import type { Density } from "../preferences/viewPreferences";

/**
 * DensitySlider は一覧の表示密度を選ぶスライダーである（C12 / spec US4-3。
 * [R-506](../../../specs/005-ui-refinement/research.md)）。
 *
 * 選択欄からスライダーへ変わるのは**入口の形だけ**である。3 段階であることも、
 * 保存される値（`localStorage` の `vv.view.v1` に入る `Density` の文字列）も、
 * 密度を変えたあとの位置合わせも 004 のまま変わらない
 * （data-model.md 3.・5.）。
 */

/**
 * steps は位置と密度の対応の**唯一の真実**である（data-model.md 3.）。
 *
 * 位置 → 密度と密度 → 位置を別々の表で持つと、片方だけ直したときに往復が
 * 壊れる。1 つの配列から両方向を導くので、順序を変えても食い違わない。
 */
const steps: readonly Density[] = ["dense", "standard", "relaxed"];

/**
 * stepLabels は読み上げに渡す段階名である。
 *
 * `<input type="range">` は既定で**数値**を読む（「2」）。位置そのものには
 * 意味が無いので、`aria-valuetext` で段階名に差し替える。文言は 004 の選択欄
 * （DensitySelect）と同じで、「細かい」「標準」「ゆったり」は見え方の言葉で
 * あって列数ではない ── 列数は画面幅と組で決まるため、「6 列」のような名前は
 * 狭い画面で嘘になる。
 */
const stepLabels: Record<Density, string> = {
  dense: "細かい",
  standard: "標準",
  relaxed: "ゆったり",
};

/**
 * toDensity は位置を密度に直す。
 *
 * 範囲外・非数値は `standard` に落とす。`<input type="range">` が範囲外を出す
 * ことは無いが、落とし先を決めておかないと壊れた値で画面が消える
 * （data-model.md 3. の不変条件。004 の FR-019 と同じ考え）。
 */
export function toDensity(index: number): Density {
  return steps[index] ?? "standard";
}

/** toIndex は密度を位置に直す。見つからなければ真ん中（標準）に置く。 */
export function toIndex(density: Density): number {
  const index = steps.indexOf(density);
  return index === -1 ? 1 : index;
}

/**
 * slider は目盛りそのものである。
 *
 * 当たり判定は `--size-tap`（44px）以上を保つ（004 の FR-022）。`accent-accent`
 * は `accent-color` を `--color-accent` に合わせる ── つまみと軌道の色を生の色
 * ではなくトークンで決めるためで、ブラウザ既定の青が暗い配色の中に残らない。
 */
const slider =
  "min-h-[var(--size-tap)] w-24 cursor-pointer accent-accent " +
  "outline-offset-2 focus-visible:outline-2 focus-visible:outline-focus";

export default function DensitySlider({
  value,
  onChange,
}: {
  value: Density;
  onChange: (value: Density) => void;
}) {
  return (
    <label className="flex items-center gap-2 text-sm text-muted">
      表示
      <input
        type="range"
        min={0}
        max={steps.length - 1}
        step={1}
        value={toIndex(value)}
        onChange={(event) => onChange(toDensity(Number(event.target.value)))}
        aria-valuetext={stepLabels[value]}
        className={slider}
      />
    </label>
  );
}
