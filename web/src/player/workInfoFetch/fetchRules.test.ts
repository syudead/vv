import { describe, expect, it } from "vitest";

import type { Video } from "../../api/client";
import type { WorkInfoFetch, WorkInfoPlugin } from "../../api/workInfoPlugins";
import {
  approvedPlugins,
  canChooseFindBy,
  initialQuery,
  initialSelection,
  missingSettingLabels,
  orderSkipped,
  queryLabel,
  rowState,
  runFor,
  selectableTags,
  tagsRowChecked,
} from "./fetchRules";

function plugin(overrides: Partial<WorkInfoPlugin> = {}): WorkInfoPlugin {
  return {
    id: 1,
    kind: "work",
    sources: [],
    autoLookup: false,
    name: "works",
    fileName: "works.wasm",
    operations: ["search", "fetch"],
    digest: "d",
    state: "approved",
    settings: [],
    network: true,
    updatedAt: 0,
    ...overrides,
  };
}

const video = {
  id: 7,
  title: "今の題名",
  tags: [
    { id: 1, name: "既存", manual: true, fromFolder: false, tentative: false },
    { id: 2, name: "仮", manual: true, fromFolder: false, tentative: true },
  ],
} as unknown as Video;

function result(overrides: Partial<WorkInfoFetch> = {}): WorkInfoFetch {
  return {
    fetchId: "f",
    expiresAt: 0,
    title: "新しい題名",
    tags: ["新規"],
    dates: [],
    people: [],
    organizations: [],
    urls: [],
    externalIds: [],
    ...overrides,
  };
}

describe("fetchRules", () => {
  it("承認済みのプラグインだけを一覧の順のまま返す", () => {
    const list = [
      plugin({ id: 1, state: "unapproved" }),
      plugin({ id: 2 }),
      plugin({ id: 3, state: "invalid" }),
      plugin({ id: 4 }),
    ];
    expect(approvedPlugins(list).map((item) => item.id)).toEqual([2, 4]);
  });

  it("語は検索し、ID を選んだときと検索できないプラグインでは作品の ID として取得する", () => {
    const both = plugin();
    const searchOnly = plugin({ operations: ["search"] });
    const fetchOnly = plugin({ operations: ["fetch"] });
    expect(runFor(both, "words")).toBe("search");
    expect(runFor(both, "id")).toBe("fetch");
    expect(runFor(searchOnly, "words")).toBe("search");
    expect(runFor(searchOnly, "id")).toBe("search");
    expect(runFor(fetchOnly, "words")).toBe("fetch");
    expect(runFor(fetchOnly, "id")).toBe("fetch");
    expect(canChooseFindBy(both)).toBe(true);
    expect(canChooseFindBy(searchOnly)).toBe(false);
    expect(canChooseFindBy(fetchOnly)).toBe(false);
  });

  it("入力欄の名前と最初の値はプラグインの操作と語の扱いで決まる", () => {
    const coded = {
      ...video,
      title: "題名",
      workInfo: { code: "ABC-123" },
    } as unknown as Video;
    const titled = { ...video, title: "題名" } as unknown as Video;
    const fetchOnly = plugin({ operations: ["fetch"] });
    expect(queryLabel(plugin(), "words")).toBe("Search words");
    expect(queryLabel(plugin(), "id")).toBe("The work's ID");
    expect(queryLabel(plugin({ operations: ["search"] }), "words")).toBe("Search words");
    expect(queryLabel(fetchOnly, "words")).toBe("The work's ID");
    expect(initialQuery(plugin(), "words", coded)).toBe("題名");
    expect(initialQuery(plugin(), "id", coded)).toBe("ABC-123");
    expect(initialQuery(fetchOnly, "words", coded)).toBe("ABC-123");
    expect(initialQuery(fetchOnly, "words", titled)).toBe("");
    expect(initialQuery(undefined, "words", coded)).toBe("");
  });

  it("値の無い項目と今と同じ項目は選べず、違う項目だけを最初に選ぶ", () => {
    expect(rowState("title", video, result({ title: undefined }))).toBe("missing");
    expect(rowState("title", video, result({ title: "今の題名" }))).toBe("same");
    expect(rowState("title", video, result())).toBe("differs");
    expect(rowState("tags", video, result({ tags: [] }))).toBe("missing");
    expect(rowState("tags", video, result({ tags: ["仮", "既存"] }))).toBe("same");
    expect(rowState("tags", video, result({ tags: ["既存", "新規"] }))).toBe("differs");
    expect(initialSelection(video, result())).toEqual(["title", "tags"]);
    expect(initialSelection(video, result({ title: undefined }))).toEqual(["tags"]);
    expect(initialSelection(video, result({ tags: ["既存"] }))).toEqual(["title"]);
  });

  it("選べるタグは動画に無い取得したタグで、行は全部・一部・無しで状態が変わる", () => {
    const selectable = selectableTags(
      video,
      result({ tags: ["既存", "ドラマ", "喜劇", "ドラマ"] }),
    );
    expect(selectable).toEqual(["ドラマ", "喜劇"]);
    expect(tagsRowChecked(selectable, ["ドラマ", "喜劇"])).toBe(true);
    expect(tagsRowChecked(selectable, ["喜劇"])).toBe("indeterminate");
    expect(tagsRowChecked(selectable, [])).toBe(false);
  });

  it("足りない設定のキーを宣言のラベルに引き直し、知らないキーはそのまま出す", () => {
    const declared = plugin({
      settings: [
        { key: "token", label: "API token", secret: true, required: true, set: false },
      ],
    });
    expect(missingSettingLabels(declared, ["token", "other"])).toEqual([
      "API token",
      "other",
    ]);
  });

  it("付けなかったタグは却下したものを先に、応答の順のまま並べる", () => {
    expect(
      orderSkipped([
        { name: "a", reason: "invalid" },
        { name: "b", reason: "rejected" },
        { name: "c", reason: "invalid" },
        { name: "d", reason: "rejected" },
      ]).map((tag) => tag.name),
    ).toEqual(["b", "d", "a", "c"]);
  });
});
