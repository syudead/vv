// カードのドラッグ層（cardDrag.tsx）が、ポインタの場所から落とした先を決める規則である
// （specs/050-drag-organize/ui-design.md「Indicators」「What a drop does」）。カードは画面の順に
// 並んだ文字列の鍵で呼ぶ。

/** Side は「間」がカードの前か後ろかである。 */
export type Side = "before" | "after";

/** Axis は並びの向きである。x は 2 列以上の格子、y は 1 列の格子とリスト。 */
export type Axis = "x" | "y";

/** Zone はカードの中のポインタの場所である。 */
export type Zone = Side | "middle";

/** OntoAnswer は画面がカードの上に落とせるかに答えた値である。null は上に落とす先ではない。 */
export type OntoAnswer<L> = { label: L } | "refused" | null;

/** DropSign は落とす先の印である（上、間、拒否）。 */
export type DropSign<L> =
  | { kind: "onto"; key: string; label: L }
  | { kind: "between"; key: string; side: Side }
  | { kind: "refused"; key: string };

/**
 * zoneOf は、カードの始まり `start` と長さ `size` に対するポインタの位置 `at` の場所を返す。
 * `quarters` では内側の 50 % が middle、両側の 25 % が前と後ろ、`halves` では半分ずつが
 * 前と後ろで middle は無い。
 */
export function zoneOf(
  at: number,
  start: number,
  size: number,
  split: "quarters" | "halves",
): Zone {
  if (size <= 0) return split === "halves" ? "before" : "middle";
  const ratio = (at - start) / size;
  if (split === "halves") return ratio < 0.5 ? "before" : "after";
  if (ratio < 0.25) return "before";
  if (ratio > 0.75) return "after";
  return "middle";
}

/** SignInput は signFor が決めるのに使う値である。 */
export interface SignInput<L> {
  /** ポインタの下のカード。 */
  target: string;
  /** 持ち上げたカード。 */
  lifted: ReadonlySet<string>;
  /** 間に落として並べ替えられる画面か。 */
  reorder: boolean;
  /** 画面がそのカードの上に落とせるかに答えた値。 */
  onto: OntoAnswer<L>;
  /** ポインタの場所。`onto` が null で並べ替えられる画面では、半分ずつで測った場所を渡す。 */
  zone: Zone;
}

/**
 * signFor はポインタの下のカードに出す印を返す。null は何も出さない（落としても何も起きない）。
 *
 * - 持ち上げたカードの上は拒否。
 * - 並べ替えられる画面では、端（`onto` が null なら半分ずつ）が間になる。
 * - そうでなければ、画面の答えのとおり上か拒否か何もなし。並べ替えない画面では端も上として扱う。
 */
export function signFor<L>({
  target,
  lifted,
  reorder,
  onto,
  zone,
}: SignInput<L>): DropSign<L> | null {
  if (lifted.has(target)) return { kind: "refused", key: target };
  if (reorder && zone !== "middle") return { kind: "between", key: target, side: zone };
  if (onto === null) return null;
  if (onto === "refused") return { kind: "refused", key: target };
  return { kind: "onto", key: target, label: onto.label };
}

/**
 * normalizeBetween は「X の後ろ」を「次のカードの前」に言い換える。行の終わりの後ろは次の行の
 * 始まりの前なので、印が行の端の外に立たない（ui-design.md「Responsive behaviour」）。最後の
 * カードの後ろはそのまま。
 */
export function normalizeBetween(
  order: readonly string[],
  key: string,
  side: Side,
): { key: string; side: Side } {
  if (side === "before") return { key, side };
  const next = order[order.indexOf(key) + 1];
  return next === undefined ? { key, side } : { key: next, side: "before" };
}

/** ReorderDrop は間に落とした結果である。 */
export interface ReorderDrop {
  /** 落とした後の並び。 */
  order: string[];
  /** 持ち上げたカード（画面の順）。 */
  moved: string[];
  /** 持ち上げたカードがすぐ後ろに続くカード。null は先頭。 */
  after: string | null;
  /** 置いた位置（1 始まり、落とした後の並びで持ち上げたカードの先頭）。 */
  position: number;
}

/**
 * reorderDrop は持ち上げたカード `lifted` を、カード `target` の前か後ろの間に画面の順のまま
 * まとめて置いた結果を返す。並びが変わらなければ null。並びに無い鍵は無視する。`target` が
 * 持ち上げたカードでもよい（「X の後ろ」を言い換えた「次のカードの前」がそれに当たる）。
 */
export function reorderDrop(
  order: readonly string[],
  lifted: ReadonlySet<string>,
  target: string,
  side: Side,
): ReorderDrop | null {
  const targetIndex = order.indexOf(target);
  if (targetIndex === -1) return null;
  const moved = order.filter((key) => lifted.has(key));
  if (moved.length === 0) return null;
  const insertAt = targetIndex + (side === "after" ? 1 : 0);
  const before = order.slice(0, insertAt).filter((key) => !lifted.has(key));
  const after = before.at(-1) ?? null;
  const next = moveAfter(order, lifted, after, (key) => key);
  if (next.every((key, index) => key === order[index])) return null;
  return { order: next, moved, after, position: before.length + 1 };
}

/**
 * moveAfter は `items` のうち鍵が `moved` にあるものを、画面の順のまま鍵 `after` の項目の
 * すぐ後ろへまとめて移した並びを返す。`after` が null か見つからなければ先頭へ移す
 * （POST /api/organize/reorder と同じ規則）。
 */
export function moveAfter<T, K>(
  items: readonly T[],
  moved: ReadonlySet<K>,
  after: K | null,
  keyOf: (item: T) => K,
): T[] {
  const moving = items.filter((item) => moved.has(keyOf(item)));
  const rest = items.filter((item) => !moved.has(keyOf(item)));
  const index = after === null ? 0 : rest.findIndex((item) => keyOf(item) === after) + 1;
  return [...rest.slice(0, index), ...moving, ...rest.slice(index)];
}

/**
 * liftedKeys は押したカード `pressed` から持ち上げるカードを返す。押したカードが選択に
 * 入っていれば選択したすべて（`selected`、画面の順）、入っていなければそれだけ。
 */
export function liftedKeys(
  order: readonly string[],
  pressed: string,
  pressedSelected: boolean,
  selected: readonly string[],
): string[] {
  if (!pressedSelected) return [pressed];
  const keys = new Set(selected);
  keys.add(pressed);
  return order.filter((key) => keys.has(key));
}

/** Rect はカードの画面上の箱である。 */
export interface Rect {
  left: number;
  top: number;
  width: number;
  height: number;
}

/**
 * axisOf はカード `rect` の並びの向きを返す。同じ高さに別のカードがあれば 2 列以上の格子
 * （x）、無ければ 1 列の格子かリスト（y）。
 */
export function axisOf(rect: Rect, others: Iterable<Rect>): Axis {
  for (const other of others) {
    if (
      other !== rect &&
      Math.abs(other.top - rect.top) < 1 &&
      other.left !== rect.left
    ) {
      return "x";
    }
  }
  return "y";
}
