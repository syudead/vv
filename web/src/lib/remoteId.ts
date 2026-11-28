/**
 * Ids at or above 2^40 name a read VVMDM's entities: the server adds the link's slot times this offset to the
 * link's own ids.
 */
export const remoteIdOffset = 2 ** 40;

export function isRemoteId(id: number): boolean {
  return id >= remoteIdOffset;
}

/** The slot of the link whose entity `id` names, or 0 for this node's own entity. */
export function remoteSlot(id: number): number {
  return Math.floor(id / remoteIdOffset);
}

export type IdNodes = "none" | "local" | "remote" | "mixed";

/**
 * Which nodes a set of ids reaches, as Go's domain.IDNodes answers: an id of 0 or below names no entity, so an
 * empty set or one of such ids reaches none.
 */
export function idNodes(ids: Iterable<number>): IdNodes {
  let local = false;
  let remote = false;
  for (const id of ids) {
    if (isRemoteId(id)) remote = true;
    else if (id > 0) local = true;
    if (local && remote) return "mixed";
  }
  if (remote) return "remote";
  return local ? "local" : "none";
}
