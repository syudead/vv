import type { TagRef, VideoTag } from "./client";

/** Match domain.CompareNatural with numeric digit runs and case-insensitive text; Array.from preserves Unicode code points. */
export function compareNatural(a: string, b: string): number {
  const ca = Array.from(a);
  const cb = Array.from(b);
  let i = 0;
  let j = 0;
  while (i < ca.length && j < cb.length) {
    const ra = ca[i] ?? "";
    const rb = cb[j] ?? "";
    if (isDigit(ra) && isDigit(rb)) {
      let da = "";
      while (i < ca.length && isDigit(ca[i] ?? "")) {
        da += ca[i];
        i += 1;
      }
      let db = "";
      while (j < cb.length && isDigit(cb[j] ?? "")) {
        db += cb[j];
        j += 1;
      }
      const order = compareDigitRun(da, db);
      if (order !== 0) return order;
      continue;
    }
    const la = ra.toLowerCase();
    const lb = rb.toLowerCase();
    if (la !== lb) return la < lb ? -1 : 1;
    i += 1;
    j += 1;
  }
  if (i >= ca.length && j >= cb.length) return 0;
  return i >= ca.length ? -1 : 1;
}

function isDigit(ch: string): boolean {
  return ch >= "0" && ch <= "9";
}

/** Compare digit runs numerically without overflow. */
function compareDigitRun(a: string, b: string): number {
  const ta = a.replace(/^0+/, "");
  const tb = b.replace(/^0+/, "");
  if (ta.length !== tb.length) return ta.length < tb.length ? -1 : 1;
  if (ta === tb) return 0;
  return ta < tb ? -1 : 1;
}

/**
 * Card order: tags of cardFirst kinds lead in natural kind-name order; the stable sort keeps the
 * server's tag order inside each group.
 */
export function cardTagOrder<T extends TagRef>(tags: readonly T[]): T[] {
  const leadKind = (tag: T) => (tag.kind?.cardFirst === true ? tag.kind.name : undefined);
  return [...tags].sort((a, b) => {
    const ka = leadKind(a);
    const kb = leadKind(b);
    if (ka === undefined || kb === undefined) {
      return Number(ka === undefined) - Number(kb === undefined);
    }
    return compareNatural(ka, kb);
  });
}

/** Match domain.SortTagRefs: natural name order, then ID. */
export function compareTagRefs(a: TagRef, b: TagRef): number {
  const byName = compareNatural(a.name, b.name);
  if (byName !== 0) return byName;
  if (a.name !== b.name) return a.name < b.name ? -1 : 1;
  return a.id - b.id;
}

/** Change manual assignments only, preserving folder-derived tags and server-returned metadata. Share this implementation between snapshots and live lists. */
export function applyTagToTags(
  tags: readonly VideoTag[],
  tag: TagRef,
  action: "add" | "remove",
): VideoTag[] {
  const existing = tags.find((candidate) => candidate.id === tag.id);
  const withoutExisting = tags.filter((candidate) => candidate.id !== tag.id);
  if (action === "remove") {
    if (existing?.fromFolder !== true) return withoutExisting;
    // Refresh retained metadata so stale tentative/kind state cannot survive another tab's changes.
    return tags.map((candidate) =>
      candidate.id === tag.id
        ? {
            ...candidate,
            name: tag.name,
            manual: false,
            tentative: tag.tentative,
            kind: tag.kind,
          }
        : candidate,
    );
  }
  const added: VideoTag = {
    id: tag.id,
    name: tag.name,
    manual: true,
    fromFolder: existing?.fromFolder ?? false,
    tentative: tag.tentative,
    kind: tag.kind,
  };
  // Reinsert existing IDs so renamed or alias-matched tags move to their correct natural-order position.
  const insertAt = withoutExisting.findIndex(
    (candidate) => compareTagRefs(candidate, added) > 0,
  );
  const at = insertAt === -1 ? withoutExisting.length : insertAt;
  return [...withoutExisting.slice(0, at), added, ...withoutExisting.slice(at)];
}

/** Check manual assignment, not presence: folder-only tags do not yet reflect a manual add. */
export function tagsReflectChange(
  tags: readonly VideoTag[],
  tagId: number,
  action: "add" | "remove",
): boolean {
  const manual = tags.some((tag) => tag.id === tagId && tag.manual);
  return manual === (action === "add");
}

/** Both tentative tags and unconfirmed automatic assignments display a tentative marker. */
export function showsTentative(tag: VideoTag): boolean {
  return tag.tentative || tag.unconfirmed === true;
}

export function isFolderOnly(tag: VideoTag): boolean {
  return tag.fromFolder && !tag.manual;
}
