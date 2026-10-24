import { describe, expect, it } from "vitest";

import type { Video } from "../../api/client";
import type { WorkInfoFetch, WorkInfoPlugin } from "../../api/workInfoPlugins";
import {
  approvedWorkPlugins,
  canChooseFindBy,
  fetchLink,
  initialQuery,
  missingSettingLabels,
  orderSkipped,
  queryLabel,
  runFor,
  selectableTags,
} from "./fetchRules";

function plugin(overrides: Partial<WorkInfoPlugin> = {}): WorkInfoPlugin {
  return {
    id: 1,
    kind: "work",
    sources: [],
    domains: [],
    name: "works",
    fileName: "works.wasm",
    operations: ["search", "fetch"],
    digest: "d",
    state: "approved",
    settings: [],
    network: true,
    updatedAt: 0,
    builtin: false,
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
    covers: [],
    ...overrides,
  };
}

describe("fetchRules", () => {
  it("承認済みの作品情報プラグインだけを一覧の順のまま返す", () => {
    const list = [
      plugin({ id: 1, state: "unapproved" }),
      plugin({ id: 2 }),
      plugin({ id: 3, state: "invalid" }),
      plugin({ id: 5, kind: "person", sources: ["example.people"] }),
      plugin({ id: 4 }),
    ];
    expect(approvedWorkPlugins(list).map((item) => item.id)).toEqual([2, 4]);
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

  it("ドメインを宣言したプラグインはリンクをリンクとして送り、どのリンクも読むものは動画の最初のリンクから始める", () => {
    const linked = {
      ...video,
      workInfo: {
        code: "ABC-123",
        urls: ["https://example.com/v/1", "https://other.test/2"],
      },
    } as unknown as Video;
    const site = plugin({ operations: ["fetch"], domains: ["example.com"] });
    const any = plugin({ operations: ["fetch"], domains: ["*"], builtin: true });
    expect(queryLabel(site, "words")).toBe("The work's ID or link");
    expect(queryLabel(any, "words")).toBe("Link");
    expect(initialQuery(site, "words", linked)).toBe("ABC-123");
    expect(initialQuery(any, "words", linked)).toBe("https://example.com/v/1");
    expect(fetchLink(site, " https://example.com/v/1 ")).toBe("https://example.com/v/1");
    expect(fetchLink(site, "ABC-123")).toBeUndefined();
    // Without domains a URL-shaped value stays an opaque ID.
    expect(fetchLink(plugin(), "https://example.com/v/1")).toBeUndefined();
  });

  it("選べるタグは動画に無い取得したタグを一度ずつ", () => {
    const selectable = selectableTags(
      video,
      result({ tags: ["既存", "ドラマ", "喜劇", "ドラマ"] }),
    );
    expect(selectable).toEqual(["ドラマ", "喜劇"]);
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
