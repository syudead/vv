import { describe, expect, it } from "vitest";

import type { LibraryGroup, LibraryItem, Video } from "../api/client";
import { bodyKind, missingParent, loadedVideoIds } from "./pageRules";

const child: LibraryItem = {
  kind: "smartFolder",
  smartFolder: { id: 5, name: "未視聴" },
};
const video = (id: number): LibraryItem => ({ kind: "video", video: { id } as Video });
const group = (id: number, videoIds: number[]): LibraryItem => ({
  kind: "group",
  group: { id, videoIds } as LibraryGroup,
});

describe("bodyKind", () => {
  it("動画かグループか続きがあれば中身を並べる", () => {
    expect(bodyKind([child, video(1)], undefined, [])).toBe("content");
    expect(bodyKind([group(2, [3])], undefined, [])).toBe("content");
    expect(bodyKind([child], "next", [])).toBe("content");
  });

  it("子フォルダだけなら空、欠けた参照があれば欠けた状態", () => {
    expect(bodyKind([child], undefined, [])).toBe("empty");
    expect(bodyKind([], undefined, [{ kind: "tag", id: 3 }])).toBe("missing");
  });
});

describe("missingParent", () => {
  const parent = { id: 1, name: "タグ: 猫" };
  const missing = [{ kind: "tag" as const, id: 3 }];

  it("自分の条件に無い欠けた参照を持ついちばん近い祖先を返す", () => {
    const ancestors = [
      { folder: { id: 2, name: "near" }, rule: { watch: "watched" as const } },
      { folder: parent, rule: { tags: [3] } },
    ];
    expect(missingParent({ watch: "unwatched" }, ancestors, missing)).toEqual(parent);
  });

  it("自分の条件にあるか、祖先の条件が読めていなければ返さない", () => {
    expect(
      missingParent({ tags: [3] }, [{ folder: parent, rule: { tags: [3] } }], missing),
    ).toBe(undefined);
    expect(missingParent({}, [{ folder: parent }], missing)).toBe(undefined);
  });
});

describe("loadedVideoIds", () => {
  it("並んだ動画とグループのメンバーを返し、子フォルダは含めない", () => {
    expect([...loadedVideoIds([child, video(1), group(2, [3, 4])])]).toEqual([1, 3, 4]);
  });
});
