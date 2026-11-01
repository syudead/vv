import { describe, expect, it } from "vitest";

import type { ProvisionalMatch } from "../../api/videoEdits";
import { emptyDraft } from "./draft";
import {
  buildMerges,
  cycleMerge,
  effectiveMerges,
  type MergeCandidate,
  type MergeTarget,
  suggestMerges,
} from "./merges";

const alice: MergeTarget = { key: "c7", label: "Alice", id: 7 };
const bob: MergeTarget = { key: "new:1", label: "Bob" };

function candidate(id: number): MergeCandidate {
  return { id, label: `#${String(id)}`, count: 1 };
}

function match(
  personId: number,
  fields: Partial<ProvisionalMatch> = {},
): ProvisionalMatch {
  return { personId, alone: false, coPerformers: [], similarTo: [], ...fields };
}

describe("suggestMerges", () => {
  it("prefers a similar face, else gives a video's only unnamed performer to the only target", () => {
    const candidates = [candidate(1), candidate(2), candidate(3)];
    expect(
      suggestMerges(
        candidates,
        [match(1, { alone: true }), match(2, { similarTo: [9, 7] }), match(3)],
        [alice],
      ),
    ).toEqual(
      new Map([
        [1, "c7"],
        [2, "c7"],
      ]),
    );
    // With two targets, being alone no longer says which of them it is.
    expect(suggestMerges(candidates, [match(1, { alone: true })], [alice, bob])).toEqual(
      new Map(),
    );
  });

  it("suggests neither of two unnamed performers of one video for the same target", () => {
    const suggested = suggestMerges(
      [candidate(1), candidate(2)],
      [
        match(1, { alone: true, coPerformers: [2] }),
        match(2, { alone: true, coPerformers: [1] }),
      ],
      [alice],
    );
    expect(suggested).toEqual(new Map());
  });
});

describe("merge choices", () => {
  it("lets the owner override a suggestion and drops merges into a target no longer added", () => {
    const candidates = [candidate(1), candidate(2)];
    const suggested = new Map([[1, "c7"]]);
    let draft = cycleMerge(emptyDraft, 1, "c7", [alice, bob]); // c7 → Bob
    draft = cycleMerge(draft, 2, undefined, [alice, bob]); // apart → Alice
    expect(effectiveMerges(draft, candidates, suggested, [alice, bob])).toEqual(
      new Map([
        [1, "new:1"],
        [2, "c7"],
      ]),
    );
    expect(effectiveMerges(draft, candidates, suggested, [alice])).toEqual(
      new Map([[2, "c7"]]),
    );
    draft = cycleMerge(draft, 2, "c7", [alice]); // Alice → apart
    expect(effectiveMerges(draft, candidates, suggested, [alice])).toEqual(new Map());
  });

  it("builds one merge per target with the ID created for a new name", () => {
    expect(
      buildMerges(
        new Map([
          [1, "new:1"],
          [2, "c7"],
          [3, "new:1"],
        ]),
        [alice, bob],
        new Map([["new:1", 40]]),
      ),
    ).toEqual([
      { into: 7, sourceIds: [2] },
      { into: 40, sourceIds: [1, 3] },
    ]);
  });
});
