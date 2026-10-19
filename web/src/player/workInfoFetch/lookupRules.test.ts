import { describe, expect, it } from "vitest";

import type {
  PersonPluginResult,
  WorkInfoFetch,
  WorkInfoFetchedPerson,
} from "../../api/workInfoPlugins";
import { skippedLines } from "./fetchRules";
import {
  type LookupState,
  lookupSelections,
  roleLookups,
  shownLookup,
} from "./lookupRules";

const lookup = { pluginId: 5, pluginName: "idols", source: "idol", id: "1" };

function entry(
  role: WorkInfoFetchedPerson["role"],
  name: string,
  extra: Partial<WorkInfoFetchedPerson> = {},
): WorkInfoFetchedPerson {
  return { role, name, externalIds: [], match: { state: "new" }, ...extra };
}

function result(people: WorkInfoFetchedPerson[], lookups = 2): WorkInfoFetch {
  return {
    fetchId: "F1",
    expiresAt: 0,
    tags: [],
    dates: [],
    people,
    organizations: [],
    urls: [],
    externalIds: [],
    personLookups: Array.from({ length: lookups }, () => lookup),
  };
}

const answer: PersonPluginResult = {
  person: { name: "花子", aliases: [], externalIds: [], profile: { heightCm: 160 } },
  current: null,
};

describe("lookupSelections", () => {
  const fetched = result([
    entry("performer", "花子", { lookup: 0 }),
    entry("director", "太郎", { lookup: 1 }),
  ]);

  it("チェックした役割の行が結ぶ、結果の出た lookup のチェックした項目だけを送る", () => {
    const shown = shownLookup(answer);
    expect(lookupSelections(fetched, [shown, shown], ["people.performer"])).toEqual([
      { index: 0, items: ["profile.heightCm"] },
    ]);
  });

  it("要求中、失敗、項目を全部外した lookup は送らない", () => {
    const states: LookupState[] = [
      { status: "looking" },
      { status: "failed", error: new Error("x") },
    ];
    expect(
      lookupSelections(fetched, states, ["people.performer", "people.director"]),
    ).toEqual([]);
    const cleared = { ...shownLookup(answer), items: [] } as LookupState;
    expect(lookupSelections(fetched, [cleared], ["people.performer"])).toEqual([]);
  });
});

describe("roleLookups", () => {
  it("lookup を共有する人物は最初の行に 1 つの block を出し、後の行には注記を、上限を超えた人物には上限の注記を出す", () => {
    const fetched = result(
      [
        entry("performer", "花子", { lookup: 0 }),
        entry("performer", "はなこ", { lookup: 0 }),
        entry("performer", "次子", { lookupLimited: true }),
        entry("director", "花子", { lookup: 0 }),
      ],
      1,
    );
    expect(roleLookups(fetched, "people.performer")).toEqual({
      blocks: [{ index: 0, names: ["花子", "はなこ"] }],
      notes: [{ key: "2", name: "次子" }],
      covered: new Set(["0", "1"]),
    });
    expect(roleLookups(fetched, "people.director")).toEqual({
      blocks: [],
      notes: [{ key: "0", name: "花子", sharedUnder: "people.performer" }],
      covered: new Set(["0"]),
    });
  });
});

describe("skippedLines for lookups", () => {
  it("lookup の項目は人物の名前と項目で、lookup ごと飛ばした値は役割で示す", () => {
    const fetched = result(
      [
        entry("performer", "花子", {
          lookup: 0,
          match: { state: "existing", id: 9, name: "山田花子" },
        }),
      ],
      1,
    );
    const lines = skippedLines(
      [],
      [
        {
          field: "people.performer",
          value: "idol: 1",
          reason: "taken",
          lookup: 0,
          item: "externalIds",
          holderName: "別の人",
        },
        { field: "people.performer", value: "花子", reason: "stale", lookup: 0 },
      ],
      fetched,
    );
    expect(lines.map(({ label, value, reason }) => [label, value, reason])).toEqual([
      ["山田花子 · External IDs", "idol: 1", "already linked to 別の人"],
      ["Performers", "花子", "the plugin changed since the lookup"],
    ]);
  });
});
