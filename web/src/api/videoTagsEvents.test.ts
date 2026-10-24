import { beforeEach, describe, expect, it } from "vitest";

import type { VideoTag } from "./client";
import { itemVideos, videoItem } from "./libraryItems";
import { clearListSnapshot, saveListSnapshot, takeListSnapshot } from "./listSnapshot";
import {
  nextVideoTagsSequence,
  recordAppliedVideoTags,
  subscribeVideoTags,
} from "./videoTagsEvents";

function item(id: number, tags: VideoTag[] = []) {
  return {
    id,
    virtual: false,
    title: `動画 ${String(id)}`,
    public: false,
    sizeBytes: 1,
    addedAt: "2026-09-01T00:00:00Z",
    updatedAt: "2026-09-01T00:00:00Z",
    fileCreatedAt: "2026-09-01T00:00:00Z",
    fileModifiedAt: "2026-09-01T00:00:00Z",
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
      { kind: null, id: 5, name: "旅行", tentative: false },
      "add",
      nextVideoTagsSequence(),
    );

    expect(notified).toEqual([
      {
        videoIds: [1, 2],
        tag: { id: 5, name: "旅行", tentative: false, kind: null },
        action: "add",
      },
    ]);
    unsubscribe();
  });

  it("順番どおりに届けば両方反映する", () => {
    saveListSnapshot(
      { query: "" },
      { items: [videoItem(item(1))], total: 1, hasMore: false, scrollY: 0 },
    );

    const first = nextVideoTagsSequence();
    recordAppliedVideoTags(
      [1],
      { kind: null, id: 5, name: "旅行", tentative: false },
      "add",
      first,
    );
    expect(itemVideos(takeListSnapshot({ query: "" })?.items ?? [])[0]?.tags).toEqual([
      {
        id: 5,
        name: "旅行",
        manual: true,
        fromFolder: false,
        tentative: false,
        kind: null,
      },
    ]);

    const second = nextVideoTagsSequence();
    recordAppliedVideoTags(
      [1],
      { kind: null, id: 5, name: "旅行", tentative: false },
      "remove",
      second,
    );
    expect(itemVideos(takeListSnapshot({ query: "" })?.items ?? [])[0]?.tags).toEqual([]);
  });

  it("古い通し番号の応答が後から届いても、新しい結果を巻き戻さない", () => {
    saveListSnapshot(
      { query: "" },
      { items: [videoItem(item(1))], total: 1, hasMore: false, scrollY: 0 },
    );

    const first = nextVideoTagsSequence();
    const second = nextVideoTagsSequence();

    recordAppliedVideoTags(
      [1],
      { kind: null, id: 5, name: "旅行", tentative: false },
      "remove",
      second,
    );
    expect(itemVideos(takeListSnapshot({ query: "" })?.items ?? [])[0]?.tags).toEqual([]);

    recordAppliedVideoTags(
      [1],
      { kind: null, id: 5, name: "旅行", tentative: false },
      "add",
      first,
    );
    expect(itemVideos(takeListSnapshot({ query: "" })?.items ?? [])[0]?.tags).toEqual([]);
  });

  it("別のタグ・別の動画は互いのsequenceに影響しない", () => {
    saveListSnapshot(
      { query: "" },
      {
        items: [videoItem(item(1)), videoItem(item(2))],
        total: 2,
        hasMore: false,
        scrollY: 0,
      },
    );

    const unrelated = nextVideoTagsSequence();
    const target = nextVideoTagsSequence();
    recordAppliedVideoTags(
      [2],
      { kind: null, id: 9, name: "観光", tentative: false },
      "add",
      unrelated,
    );
    recordAppliedVideoTags(
      [1],
      { kind: null, id: 5, name: "旅行", tentative: false },
      "add",
      target,
    );

    expect(itemVideos(takeListSnapshot({ query: "" })?.items ?? [])[0]?.tags).toEqual([
      {
        id: 5,
        name: "旅行",
        manual: true,
        fromFolder: false,
        tentative: false,
        kind: null,
      },
    ]);
    expect(itemVideos(takeListSnapshot({ query: "" })?.items ?? [])[1]?.tags).toEqual([
      {
        id: 9,
        name: "観光",
        manual: true,
        fromFolder: false,
        tentative: false,
        kind: null,
      },
    ]);
  });

  it("反映されない動画だけが古ければ、購読者へは反映された動画だけを知らせる", () => {
    const notified: number[][] = [];
    const unsubscribe = subscribeVideoTags((videoIds) => {
      notified.push([...videoIds]);
    });

    const first = nextVideoTagsSequence();
    const second = nextVideoTagsSequence();
    recordAppliedVideoTags(
      [2],
      { kind: null, id: 5, name: "旅行", tentative: false },
      "add",
      second,
    );
    recordAppliedVideoTags(
      [1, 2],
      { kind: null, id: 5, name: "旅行", tentative: false },
      "remove",
      first,
    );

    expect(notified).toEqual([[2], [1]]);
    unsubscribe();
  });
});
