import type {
  AssistantProposal,
  AssistantReviewCandidate,
  AssistantReviewMember,
  AssistantReviewRun,
} from "../api/assistant";
import { t } from "../i18n";

export function reviewPath(id: number): string {
  return `/assistant/reviews/${String(id)}`;
}

/** The member a tags or people set merges into: the larger count, then the older id, as the server picks. */
export function destinationIndex(members: readonly AssistantReviewMember[]): number {
  let best = 0;
  members.forEach((member, index) => {
    const current = members[best]!;
    if (
      member.count > current.count ||
      (member.count === current.count && member.id < current.id)
    ) {
      best = index;
    }
  });
  return best;
}

const trailingSeparators = /[\s\-_:：・#(（[【]+$/u;

/**
 * The title a works row leads with: what the member titles share at the start, without a dangling
 * separator, or the first title when they share nothing.
 */
export function commonTitle(members: readonly AssistantReviewMember[]): string {
  const titles = members.map((member) => member.name);
  const first = titles[0] ?? "";
  let length = first.length;
  for (const title of titles.slice(1)) {
    let shared = 0;
    while (shared < length && shared < title.length && title[shared] === first[shared]) {
      shared += 1;
    }
    length = shared;
  }
  const shared = first.slice(0, length).replace(trailingSeparators, "");
  return shared === "" ? first : shared;
}

/** The names a row's check reads out: the two sides, or the works row's common title. */
export function candidateName(
  run: AssistantReviewRun,
  candidate: AssistantReviewCandidate,
): string {
  if (run.kind === "works") return commonTitle(candidate.members);
  const [first, second] = candidate.members;
  if (first === undefined) return "";
  if (second === undefined) return first.name;
  return t.assistant.review.both(first.name, second.name);
}

export function isRunning(run: AssistantReviewRun): boolean {
  return run.status === "running";
}

/** A read that answers later than another must not move the progress back. */
export function isNewerRun(
  next: AssistantReviewRun,
  current: AssistantReviewRun,
): boolean {
  if (current.status !== "running") return next.status !== "running";
  return next.status !== "running" || next.judged >= current.judged;
}

/** The run had begun before the request that named it, or an earlier request named it: it was already running. */
export function wasAlreadyRunning(
  run: AssistantReviewRun,
  turnStartedAt: string,
  earlierRunIds: ReadonlySet<number>,
): boolean {
  if (run.status !== "running") return false;
  if (earlierRunIds.has(run.id)) return true;
  // Both times come from the server's clock.
  return Date.parse(run.startedAt) < Date.parse(turnStartedAt);
}

/** Which candidate each adopted proposal is: the answer lists them in the order of ids, without the skipped. */
export function proposalsByCandidate(
  ids: readonly number[],
  skippedIds: readonly number[],
  proposals: readonly AssistantProposal[],
): Map<number, AssistantProposal> {
  const skipped = new Set(skippedIds);
  const adopted = ids.filter((id) => !skipped.has(id));
  const result = new Map<number, AssistantProposal>();
  adopted.forEach((id, index) => {
    const proposal = proposals[index];
    if (proposal !== undefined) result.set(id, proposal);
  });
  return result;
}

function byProbability(a: AssistantReviewCandidate, b: AssistantReviewCandidate): number {
  return b.probability - a.probability || a.id - b.id;
}

/**
 * The rows the table shows: the server's, without rejected ones, and the candidates adopted on this
 * page that the server no longer sends because the merge removed a member.
 */
export function shownCandidates(
  read: readonly AssistantReviewCandidate[],
  kept: ReadonlyMap<number, AssistantReviewCandidate>,
): AssistantReviewCandidate[] {
  const present = new Set(read.map((candidate) => candidate.id));
  const rows = read.filter((candidate) => candidate.state !== "rejected");
  const missing = [...kept.values()].filter((candidate) => !present.has(candidate.id));
  if (missing.length === 0) return rows;
  return [...rows, ...missing].sort(byProbability);
}

export interface StateCounts {
  adopted: number;
  rejected: number;
}

export function stateCounts(
  read: readonly AssistantReviewCandidate[],
  kept: ReadonlyMap<number, AssistantReviewCandidate>,
): StateCounts {
  const states = new Map<number, AssistantReviewCandidate["state"]>();
  for (const candidate of kept.values()) states.set(candidate.id, candidate.state);
  for (const candidate of read) states.set(candidate.id, candidate.state);
  let adopted = 0;
  let rejected = 0;
  for (const state of states.values()) {
    if (state === "adopted") adopted += 1;
    if (state === "rejected") rejected += 1;
  }
  return { adopted, rejected };
}
