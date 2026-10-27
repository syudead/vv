/** Ids at or above 2^40 name the paired VVMDM's entities; the server adds this offset to the peer's ids. */
export const remoteIdOffset = 2 ** 40;

export function isRemoteId(id: number): boolean {
  return id >= remoteIdOffset;
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
