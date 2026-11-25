import type {
  ProvisionalMatch,
  VideoEditMerge,
  VideoEditSummary,
} from "../../api/videoEdits";
import { creditDisplayName } from "../../credits/displayName";
import { isRemoteId } from "../../lib/remoteId";
import { addsPerson, type BulkDraft, creditKey, fieldOf, type FieldId } from "./draft";

// See docs/design-docs/bulk-editing.md, "Unnamed performers merge into the added person".

export const performerField: FieldId = "person:performer";

/** A named person the draft adds as a performer; `id` is absent for a name created on Apply. */
export interface MergeTarget {
  key: string;
  label: string;
  id?: number;
}

/** An unnamed performer of the selection that can be merged into a target. */
export interface MergeCandidate {
  id: number;
  label: string;
  count: number;
  imageRevision?: number;
}

/** The named people the draft adds as performers, in field order; merges never reach the peer's people. */
export function mergeTargets(draft: BulkDraft, summary: VideoEditSummary): MergeTarget[] {
  const field = fieldOf(draft, performerField);
  const targets: MergeTarget[] = [];
  for (const item of summary.people) {
    // An unnamed person the draft names on Apply is a target like a named one.
    const name = draft.names.get(item.person.id);
    if (
      item.role !== "performer" ||
      (item.person.provisional && name === undefined) ||
      isRemoteId(item.person.id)
    )
      continue;
    const key = creditKey(item.person.id);
    if (field.choices.get(key) === "add")
      targets.push({ key, label: name ?? item.person.name, id: item.person.id });
  }
  for (const value of field.picked) {
    if ("newName" in value) targets.push({ key: value.key, label: value.label });
    else if (value.provisional !== true && !isRemoteId(value.id))
      targets.push({ key: value.key, label: value.label, id: value.id });
  }
  return targets;
}

/** Unnamed performers of this VVMDM's selected videos, except those the draft removes, adds or picks. */
export function mergeCandidates(
  draft: BulkDraft,
  summary: VideoEditSummary,
): MergeCandidate[] {
  const field = fieldOf(draft, performerField);
  return summary.people
    .filter(
      (item) =>
        item.role === "performer" &&
        item.person.provisional &&
        !isRemoteId(item.person.id) &&
        field.choices.get(creditKey(item.person.id)) === undefined &&
        !addsPerson(draft, item.person.id),
    )
    .map((item) => ({
      id: item.person.id,
      label: creditDisplayName(item.person),
      count: item.count,
      ...(item.imageRevision !== undefined && { imageRevision: item.imageRevision }),
    }));
}

/**
 * The target each candidate is first set to merge into: the most similar target by face, else
 * the only target for a candidate that is a video's only unnamed performer, else the target of a
 * candidate with the same face. Two candidates of one video are different people, so neither goes
 * to the same target.
 */
export function suggestMerges(
  candidates: readonly MergeCandidate[],
  matches: readonly ProvisionalMatch[],
  targets: readonly MergeTarget[],
): Map<number, string> {
  const byId = new Map(matches.map((match) => [match.personId, match]));
  const targetById = new Map(
    targets.flatMap((target) =>
      target.id === undefined ? [] : [[target.id, target.key]],
    ),
  );
  const suggested = new Map<number, string>();
  for (const candidate of candidates) {
    const match = byId.get(candidate.id);
    if (match === undefined) continue;
    const similar = match.similarTo.find((id) => targetById.has(id));
    if (similar !== undefined) suggested.set(candidate.id, targetById.get(similar) ?? "");
    else if (match.alone && targets.length === 1 && targets[0] !== undefined)
      suggested.set(candidate.id, targets[0].key);
  }
  followSameFaces(candidates, byId, suggested);
  const conflicting = new Set<number>();
  for (const [id, target] of suggested) {
    for (const other of byId.get(id)?.coPerformers ?? []) {
      if (suggested.get(other) === target) conflicting.add(id);
    }
  }
  for (const id of conflicting) suggested.delete(id);
  return suggested;
}

/**
 * Gives a candidate left without a target the target of another candidate whose face it shares, such
 * as a face in a two-person video that matches a video's only unnamed performer, until nothing changes.
 * A co-performer of a candidate already going to that target is a different person and stays apart.
 */
function followSameFaces(
  candidates: readonly MergeCandidate[],
  byId: ReadonlyMap<number, ProvisionalMatch>,
  suggested: Map<number, string>,
) {
  for (let changed = true; changed;) {
    changed = false;
    for (const candidate of candidates) {
      const match = byId.get(candidate.id);
      if (suggested.has(candidate.id) || match === undefined) continue;
      const target = match.similarTo
        .map((id) => suggested.get(id))
        .find(
          (key) =>
            key !== undefined &&
            !match.coPerformers.some((other) => suggested.get(other) === key),
        );
      if (target === undefined) continue;
      suggested.set(candidate.id, target);
      changed = true;
    }
  }
}

/** What Apply merges: the owner's choice where made, else the suggestion, for targets still added. */
export function effectiveMerges(
  draft: BulkDraft,
  candidates: readonly MergeCandidate[],
  suggested: ReadonlyMap<number, string>,
  targets: readonly MergeTarget[],
): Map<number, string> {
  const keys = new Set(targets.map((target) => target.key));
  const out = new Map<number, string>();
  for (const candidate of candidates) {
    const chosen = draft.merges.has(candidate.id)
      ? draft.merges.get(candidate.id)
      : suggested.get(candidate.id);
    if (chosen != null && keys.has(chosen)) out.set(candidate.id, chosen);
  }
  return out;
}

/** Pressing a candidate steps through keeping it apart and merging it into each target. */
export function cycleMerge(
  draft: BulkDraft,
  candidateId: number,
  current: string | undefined,
  targets: readonly MergeTarget[],
): BulkDraft {
  const order: (string | null)[] = [null, ...targets.map((target) => target.key)];
  const index = order.indexOf(current ?? null);
  const next = order[(index + 1) % order.length] ?? null;
  const merges = new Map(draft.merges);
  merges.set(candidateId, next);
  return { ...draft, merges };
}

/** The request's merges, one entry per target; `created` gives the IDs of names created on Apply. */
export function buildMerges(
  merges: ReadonlyMap<number, string>,
  targets: readonly MergeTarget[],
  created: ReadonlyMap<string, number>,
): VideoEditMerge[] {
  const out: VideoEditMerge[] = [];
  for (const target of targets) {
    const into = target.id ?? created.get(target.key);
    const sourceIds = [...merges]
      .filter(([, key]) => key === target.key)
      .map(([id]) => id);
    if (into !== undefined && sourceIds.length > 0) out.push({ into, sourceIds });
  }
  return out;
}

export function targetIds(targets: readonly MergeTarget[]): number[] {
  return targets.flatMap((target) => (target.id === undefined ? [] : [target.id]));
}
