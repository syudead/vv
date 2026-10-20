export interface Point {
  x: number;
  y: number;
}

export interface Box {
  left: number;
  top: number;
  right: number;
  bottom: number;
}

export function boxBetween(a: Point, b: Point): Box {
  return {
    left: Math.min(a.x, b.x),
    top: Math.min(a.y, b.y),
    right: Math.max(a.x, b.x),
    bottom: Math.max(a.y, b.y),
  };
}

/** Touching edges do not count, so a box ending in a gap selects neither neighbour. */
export function overlaps(a: Box, b: Box): boolean {
  return a.left < b.right && b.left < a.right && a.top < b.bottom && b.top < a.bottom;
}

/** Pixels to scroll per frame while the pointer is within `edge` of the viewport's top or bottom. */
export function edgeSpeed(y: number, height: number, edge = 48, max = 24): number {
  if (y < edge) return -Math.ceil(((edge - Math.max(y, 0)) / edge) * max);
  if (y > height - edge)
    return Math.ceil(((Math.min(y, height) - (height - edge)) / edge) * max);
  return 0;
}
