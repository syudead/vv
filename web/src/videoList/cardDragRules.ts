import { isRemoteId } from "../lib/remoteId";

export type Side = "before" | "after";

/** x means a multi-column grid; y means one column or a list. */
export type Axis = "x" | "y";

export type Zone = Side | "middle";

/** Null means the card is not an onto target. */
export type OntoAnswer<L> = { label: L } | "refused" | null;

export type DropSign<L> =
  | { kind: "onto"; key: string; label: L }
  | { kind: "between"; key: string; side: Side }
  | { kind: "crumb"; key: string }
  | { kind: "refused"; key: string };

/** quarters uses a middle 50% and 25% edges; halves has no middle zone. */
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

export interface SignInput<L> {
  target: string;
  lifted: ReadonlySet<string>;
  reorder: boolean;
  onto: OntoAnswer<L>;
  /** Use half-card zones when onto is null but reordering is allowed. */
  zone: Zone;
}

/**
 * Reject lifted targets; reorderable edges become insertion targets, otherwise use the screen's
 * onto decision. Non-reorderable screens treat edges as onto too.
 */
export function signFor<L>({
  target,
  lifted,
  reorder,
  onto,
  zone,
}: SignInput<L>): DropSign<L> | null {
  if (lifted.has(target)) return { kind: "refused", key: target };
  if (crossesNodes(target, lifted)) return { kind: "refused", key: target };
  if (reorder && zone !== "middle") return { kind: "between", key: target, side: zone };
  if (onto === null) return null;
  if (onto === "refused") return { kind: "refused", key: target };
  return { kind: "onto", key: target, label: onto.label };
}

const itemKeyPrefixes = ["video\0", "group\0", "smartFolder\0"];

/** The node of a video, group or smart-folder card key; undefined for other keys, such as crumbs. */
function keyRemote(key: string): boolean | undefined {
  if (!itemKeyPrefixes.some((prefix) => key.startsWith(prefix))) return undefined;
  return isRemoteId(Number(key.slice(key.indexOf("\0") + 1)));
}

/** A drop never moves items to the other node: groups and folders hold one node's items. */
function crossesNodes(target: string, lifted: ReadonlySet<string>): boolean {
  const remote = keyRemote(target);
  if (remote === undefined) return false;
  for (const key of lifted) {
    const other = keyRemote(key);
    if (other !== undefined && other !== remote) return true;
  }
  return false;
}

/**
 * Normalize after to before the next card so row-end markers do not fall outside the grid; keep
 * after on the final card.
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

export interface ReorderDrop {
  order: string[];
  moved: string[];
  /** Predecessor of the moved block; null means the start. */
  after: string | null;
  /** One-based position of the first moved card in the resulting order. */
  position: number;
}

/**
 * Preserve lifted display order, ignore unknown keys and return null for no change. A normalized
 * target may itself be lifted.
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
 * Match server reorder semantics: preserve display order and use the start when after is null or
 * missing.
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
 * Lift all selected items only when the pressed card belongs to that partition; exclude videos
 * covered by selected groups.
 */
export function liftedKeys(
  order: readonly string[],
  pressed: string,
  selected: readonly string[],
): string[] {
  const keys = new Set(selected);
  if (!keys.has(pressed)) return [pressed];
  return order.filter((key) => keys.has(key));
}

export interface Rect {
  left: number;
  top: number;
  width: number;
  height: number;
}

/** Infer axis from the whole grid so a final single-card row does not imply one column. */
export function axisOf(rects: Iterable<Rect>): Axis {
  const leftByRow = new Map<number, number>();
  for (const rect of rects) {
    const row = Math.round(rect.top);
    const left = leftByRow.get(row);
    if (left !== undefined && Math.abs(left - rect.left) >= 1) return "x";
    leftByRow.set(row, rect.left);
  }
  return "y";
}

export interface Point {
  x: number;
  y: number;
}

export function distanceToRect(at: Point, rect: Rect): number {
  const dx = Math.max(rect.left - at.x, 0, at.x - (rect.left + rect.width));
  const dy = Math.max(rect.top - at.y, 0, at.y - (rect.top + rect.height));
  return Math.hypot(dx, dy);
}

/** Keep insertion markers in card gaps droppable by accepting the nearest rectangle within reach. */
export function nearestWithin<K>(
  at: Point,
  rects: Iterable<readonly [K, Rect]>,
  reach: number,
): K | null {
  let best: { key: K; distance: number } | null = null;
  for (const [key, rect] of rects) {
    const distance = distanceToRect(at, rect);
    if (distance <= reach && (best === null || distance < best.distance)) {
      best = { key, distance };
    }
  }
  return best === null ? null : best.key;
}
