import { describe, expect, it } from "vitest";

import type { Video } from "../../api/client";
import type { WorkInfoFetch, WorkInfoPlugin } from "../../api/workInfoPlugins";
import {
  approvedPlugins,
  canChooseFindBy,
  fetchTarget,
  initialQuery,
  initialSelection,
  isWebUrl,
  missingSettingLabels,
  orderSkipped,
  queryLabel,
  rowState,
  runFor,
} from "./fetchRules";

function plugin(overrides: Partial<WorkInfoPlugin> = {}): WorkInfoPlugin {
  return {
    id: 1,
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

  it.each([
    ["https://example.com/works/1", true],
    ["  http://example.com  ", true],
    ["HTTPS://EXAMPLE.COM/", true],
    ["ftp://example.com/", false],
    ["example.com/works/1", false],
    ["https://", false],
    ["作品の名前", false],
  ])("%s を http か https の URL と読むか: %s", (text, expected) => {
    expect(isWebUrl(text)).toBe(expected);
  });

  it("URL は取得し、それ以外は検索し、検索できないプラグインの語は作品の ID として取得する", () => {
    const both = plugin();
    const searchOnly = plugin({ operations: ["search"] });
    const fetchOnly = plugin({ operations: ["fetch"] });
    expect(runFor(both, "https://example.com/w/1", "words")).toBe("fetch");
    expect(runFor(both, "作品", "words")).toBe("search");
    expect(runFor(both, "ABC-123", "id")).toBe("fetch");
    expect(runFor(both, "https://example.com/w/1", "id")).toBe("fetch");
    expect(runFor(searchOnly, "https://example.com/w/1", "words")).toBe("search");
    expect(runFor(searchOnly, "作品", "id")).toBe("search");
    expect(runFor(fetchOnly, "https://example.com/w/1", "words")).toBe("fetch");
    expect(runFor(fetchOnly, "ABC-123", "words")).toBe("fetch");
    expect(canChooseFindBy(both)).toBe(true);
    expect(canChooseFindBy(searchOnly)).toBe(false);
    expect(canChooseFindBy(fetchOnly)).toBe(false);
  });

  it("取得には URL として読める文を URL、それ以外を作品の ID として渡す", () => {
    expect(fetchTarget(" https://example.com/w/1 ")).toEqual({
      url: "https://example.com/w/1",
    });
    expect(fetchTarget(" ABC-123 ")).toEqual({ id: "ABC-123" });
  });

  it("入力欄の名前と最初の値はプラグインの操作と語の扱いで決まる", () => {
    const coded = {
      ...video,
      title: "題名",
      workInfo: { code: "ABC-123" },
    } as unknown as Video;
    const titled = { ...video, title: "題名" } as unknown as Video;
    const fetchOnly = plugin({ operations: ["fetch"] });
    expect(queryLabel(plugin(), "words")).toBe("Search words or the work's page URL");
    expect(queryLabel(plugin(), "id")).toBe("The work's ID or page URL");
    expect(queryLabel(plugin({ operations: ["search"] }), "words")).toBe("Search words");
    expect(queryLabel(fetchOnly, "words")).toBe("The work's ID or page URL");
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
