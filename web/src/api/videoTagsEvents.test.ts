import { beforeEach, describe, expect, it } from "vitest";

import { clearListSnapshot, saveListSnapshot, takeListSnapshot } from "./listSnapshot";
import {
  nextVideoTagsSequence,
  recordAppliedVideoTags,
  subscribeVideoTags,
} from "./videoTagsEvents";

function item(id: number, tags: { id: number; name: string }[] = []) {
  return {
    id,
    title: `動画 ${String(id)}`,
    public: false,
    sizeBytes: 1,
    addedAt: "2026-09-01T00:00:00Z",
    playable: true,
    probeState: "done" as const,
    thumbnailState: "done" as const,
    previewState: "pending" as const,
    tags,
  };
}

beforeEach(() => {
  clearListSnapshot();
});

describe("videoTagsEvents", () => {
  it("購読者へ通知する", () => {
    const notified: unknown[] = [];
    const unsubscribe = subscribeVideoTags((videoIds, tag, action) => {
      notified.push({ videoIds, tag, action });
    });

    recordAppliedVideoTags(
      [1, 2],
      { id: 5, name: "旅行" },
      "add",
      nextVideoTagsSequence(),
    );

    expect(notified).toEqual([
      { videoIds: [1, 2], tag: { id: 5, name: "旅行" }, action: "add" },
    ]);
    unsubscribe();
  });

  // N1: 同じ動画・タグの組では、通し番号が古い応答が後から届いても、新しい
  // 応答の結果を巻き戻さない。
  it("順番どおりに届けば両方反映する", () => {
    saveListSnapshot(
      { query: "" },
      { items: [item(1)], total: 1, hasMore: false, scrollY: 0 },
    );

    const first = nextVideoTagsSequence();
    recordAppliedVideoTags([1], { id: 5, name: "旅行" }, "add", first);
    expect(takeListSnapshot({ query: "" })?.items[0]?.tags).toEqual([
      { id: 5, name: "旅行" },
    ]);

    const second = nextVideoTagsSequence();
    recordAppliedVideoTags([1], { id: 5, name: "旅行" }, "remove", second);
    expect(takeListSnapshot({ query: "" })?.items[0]?.tags).toEqual([]);
  });

  it("古い通し番号の応答が後から届いても、新しい結果を巻き戻さない", () => {
    saveListSnapshot(
      { query: "" },
      { items: [item(1)], total: 1, hasMore: false, scrollY: 0 },
    );

    const first = nextVideoTagsSequence();
    const second = nextVideoTagsSequence();

    // 2番目（外す）の応答が先に届く。
    recordAppliedVideoTags([1], { id: 5, name: "旅行" }, "remove", second);
    expect(takeListSnapshot({ query: "" })?.items[0]?.tags).toEqual([]);

    // 1番目（付ける）の応答が遅れて届いても、2番目の結果（外れた状態）を
    // 上書きしない。
    recordAppliedVideoTags([1], { id: 5, name: "旅行" }, "add", first);
    expect(takeListSnapshot({ query: "" })?.items[0]?.tags).toEqual([]);
  });

  it("別のタグ・別の動画は互いのsequenceに影響しない", () => {
    saveListSnapshot(
      { query: "" },
      { items: [item(1), item(2)], total: 2, hasMore: false, scrollY: 0 },
    );

    // 動画2・タグ9への新しい操作を先に払い出しても、動画1・タグ5の古い操作の
    // 反映を妨げない（鍵は動画・タグの組ごと）。
    const unrelated = nextVideoTagsSequence();
    const target = nextVideoTagsSequence();
    recordAppliedVideoTags([2], { id: 9, name: "観光" }, "add", unrelated);
    recordAppliedVideoTags([1], { id: 5, name: "旅行" }, "add", target);

    expect(takeListSnapshot({ query: "" })?.items[0]?.tags).toEqual([
      { id: 5, name: "旅行" },
    ]);
    expect(takeListSnapshot({ query: "" })?.items[1]?.tags).toEqual([
      { id: 9, name: "観光" },
    ]);
  });

  it("反映されない動画だけが古ければ、購読者へは反映された動画だけを知らせる", () => {
    const notified: number[][] = [];
    const unsubscribe = subscribeVideoTags((videoIds) => {
      notified.push([...videoIds]);
    });

    const first = nextVideoTagsSequence();
    const second = nextVideoTagsSequence();
    // 動画2は先に新しい操作（second）を受け取り済みとする。
    recordAppliedVideoTags([2], { id: 5, name: "旅行" }, "add", second);
    // 動画1・2をまとめて外す古い操作（first）が後から届く。動画2はfirstより
    // 新しいsecondを既に反映しているので、この操作からは除く。動画1は初めて
    // なので反映する。
    recordAppliedVideoTags([1, 2], { id: 5, name: "旅行" }, "remove", first);

    expect(notified).toEqual([[2], [1]]);
    unsubscribe();
  });
});
