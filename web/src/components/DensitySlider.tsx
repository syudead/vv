import type { Density } from "../preferences/viewPreferences";
import Icon, { type IconName } from "../layout/icons";

/**
 * DensitySlider は一覧の表示密度を選ぶセグメント操作である（C12 / spec US4-3。
 * [R-506](../../../specs/005-ui-refinement/research.md)）。
 *
 * 3つのアイコンボタンへ変わるのは**入口の形だけ**である。3 段階であることも、
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

const choices: readonly { value: Density; label: string; icon: IconName }[] = [
  { value: "dense", label: "細かい", icon: "grid" },
  { value: "standard", label: "標準", icon: "list" },
  { value: "relaxed", label: "ゆったり", icon: "compactList" },
];

export default function DensitySlider({
  value,
  onChange,
}: {
  value: Density;
  onChange: (value: Density) => void;
}) {
  return (
    <div
      role="group"
      aria-label="表示"
      className="inline-flex overflow-hidden rounded-control border border-border bg-surface-raised"
    >
      {choices.map((choice) => (
        <button
          key={choice.value}
          type="button"
          aria-label={choice.label}
          aria-pressed={value === choice.value}
          tabIndex={value === choice.value ? 0 : -1}
          onClick={() => onChange(choice.value)}
          onKeyDown={(event) => {
            if (event.key !== "ArrowLeft" && event.key !== "ArrowRight") {
              return;
            }
            event.preventDefault();
            const direction = event.key === "ArrowRight" ? 1 : -1;
            const nextIndex =
              (toIndex(value) + direction + choices.length) % choices.length;
            const nextChoice = choices[nextIndex];
            if (nextChoice === undefined) {
              return;
            }
            onChange(nextChoice.value);
            const buttons = event.currentTarget.parentElement?.querySelectorAll("button");
            (buttons?.[nextIndex] as HTMLButtonElement | undefined)?.focus();
          }}
          title={choice.label}
          className={
            "flex h-[var(--size-tap)] w-[var(--size-tap)] items-center justify-center border-l border-border first:border-l-0 outline-none transition-colors " +
            "hover:bg-body/10 active:bg-surface-sunken focus-visible:relative focus-visible:z-10 focus-visible:outline-2 focus-visible:outline-offset-0 focus-visible:outline-focus motion-reduce:transition-none " +
            (value === choice.value
              ? "bg-accent-surface text-accent"
              : "text-muted hover:text-body")
          }
        >
          <Icon name={choice.icon} className="h-4 w-4" />
        </button>
      ))}
    </div>
  );
}
