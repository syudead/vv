import { describe, expect, it } from "vitest";

import type { ProvisionalMatch } from "../../api/videoEdits";
import { remoteIdOffset } from "../../lib/remoteId";
import { cycleValue, emptyDraft, nameOnApply, pickValue } from "./draft";
import {
  buildMerges,
  cycleMerge,
  effectiveMerges,
  type MergeCandidate,
  type MergeTarget,
  mergeCandidates,
  mergeTargets,
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

  it("gives a face the target of another unnamed performer with the same face", () => {
    // 1 is a video's only unnamed performer; 2 shares a video with 3 and has 1's face, and 4 has 2's face.
    const suggested = suggestMerges(
      [candidate(1), candidate(2), candidate(3), candidate(4)],
      [
        match(1, { alone: true, similarTo: [2] }),
        match(2, { coPerformers: [3], similarTo: [1, 4] }),
        match(3, { coPerformers: [2] }),
        match(4, { similarTo: [2] }),
      ],
      [bob],
    );
    expect(suggested).toEqual(
      new Map([
        [1, "new:1"],
        [2, "new:1"],
        [4, "new:1"],
      ]),
    );
  });

  it("keeps a face apart from the target its co-performer already goes to", () => {
    const suggested = suggestMerges(
      [candidate(1), candidate(2)],
      [match(1, { similarTo: [7] }), match(2, { coPerformers: [1], similarTo: [1] })],
      [alice],
    );
    expect(suggested).toEqual(new Map([[1, "c7"]]));
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

describe("mergeTargets", () => {
  it("leaves out the peer's performers, which a merge cannot reach", () => {
    const remote = remoteIdOffset + 7;
    const summary = {
      total: 2,
      public: 0,
      tags: [],
      organizations: [],
      dates: [],
      people: [
        {
          role: "performer" as const,
          person: { id: remote, name: "Remote", provisional: false },
          count: 1,
        },
      ],
    };
    let draft = cycleValue(
      emptyDraft,
      "person:performer",
      { key: `c${String(remote)}`, label: "Remote", count: 1, removable: 1 },
      2,
    );
    draft = pickValue(
      draft,
      "person:performer",
      { key: "c8", label: "Local", id: 8 },
      [],
    );
    expect(mergeTargets(draft, summary)).toEqual([{ key: "c8", label: "Local", id: 8 }]);
  });
});

describe("unnamed people named on Apply", () => {
  const summary = {
    total: 2,
    public: 0,
    tags: [],
    organizations: [],
    dates: [],
    people: [7, 8].map((id) => ({
      role: "performer" as const,
      person: { id, name: "", provisional: true },
      count: 1,
    })),
  };

  it("makes a person picked by face with a typed name a target, not a candidate", () => {
    let draft = cycleValue(
      emptyDraft,
      "person:performer",
      { key: "c7", label: "#7", count: 1, removable: 1 },
      2,
    );
    expect(mergeTargets(draft, summary)).toEqual([]);
    draft = nameOnApply(draft, 7, "Mio");
    expect(mergeTargets(draft, summary)).toEqual([{ key: "c7", label: "Mio", id: 7 }]);
    expect(mergeCandidates(draft, summary).map((item) => item.id)).toEqual([8]);
  });
});
