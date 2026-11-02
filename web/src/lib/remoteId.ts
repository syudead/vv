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

/** Which nodes a set of ids reaches; an empty set reaches none. */
export function idNodes(ids: Iterable<number>): IdNodes {
  let local = false;
  let remote = false;
  for (const id of ids) {
    if (isRemoteId(id)) remote = true;
    else local = true;
    if (local && remote) return "mixed";
  }
  if (remote) return "remote";
  return local ? "local" : "none";
}
