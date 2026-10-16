import { afterEach, describe, expect, it, vi } from "vitest";

import { RequestFailed } from "./client";
import { saveListSnapshot, takeListSnapshot } from "./listSnapshot";
import { __resetTagsForTest, subscribeTags } from "./tags";
import {
  applyWorkInfoFetch,
  approveWorkInfoPlugin,
  deleteWorkInfoPlugin,
  fetchWorkInfo,
  getWorkInfoPluginLastRun,
  listWorkInfoPlugins,
  revokeWorkInfoPluginApproval,
  searchWorkInfo,
  setWorkInfoPluginSettings,
  uploadWorkInfoPlugin,
  type WorkInfoPlugin,
} from "./workInfoPlugins";

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

afterEach(() => {
  vi.unstubAllGlobals();
  __resetTagsForTest();
});

const plugin: WorkInfoPlugin = {
  id: 4,
  name: "example.works",
  fileName: "example.wasm",
  operations: ["search", "fetch"],
  digest: "abc",
  state: "unapproved",
  settings: [
    { key: "apiKey", label: "API key", secret: true, required: true, set: false },
  ],
  network: true,
  updatedAt: 1_800_000_000,
};

describe("work-info plugins API", () => {
  it("一覧を読み、承認・取り消し・設定・削除をそれぞれの経路へ送る", async () => {
    const fetchMock = vi
      .fn<typeof fetch>()
      .mockResolvedValueOnce(json({ plugins: [plugin] }))
      .mockResolvedValueOnce(json({ ...plugin, state: "approved" }))
      .mockResolvedValueOnce(json(plugin))
      .mockResolvedValueOnce(json(plugin))
      .mockResolvedValueOnce(new Response(null, { status: 204 }));
    vi.stubGlobal("fetch", fetchMock);

    expect(await listWorkInfoPlugins()).toEqual([plugin]);
    expect((await approveWorkInfoPlugin(4, "abc")).state).toBe("approved");
    await revokeWorkInfoPluginApproval(4);
    await setWorkInfoPluginSettings(4, { apiKey: "k", lang: null });
    await deleteWorkInfoPlugin(4);

    const sent = fetchMock.mock.calls.map(([input, init]) => ({
      url: String(input),
      method: init?.method ?? "GET",
      body:
        typeof init?.body === "string" ? (JSON.parse(init.body) as unknown) : undefined,
    }));
    expect(sent).toEqual([
      { url: "/api/work-info-plugins", method: "GET", body: undefined },
      {
        url: "/api/work-info-plugins/4/approval",
        method: "PUT",
        body: { digest: "abc" },
      },
      { url: "/api/work-info-plugins/4/approval", method: "DELETE", body: undefined },
      {
        url: "/api/work-info-plugins/4/settings",
        method: "PUT",
        body: { values: { apiKey: "k", lang: null } },
      },
      { url: "/api/work-info-plugins/4", method: "DELETE", body: undefined },
    ]);
  });

  it("直近の実行を読み、残っていなければ null を返す", async () => {
    const run = {
      ranAt: 1_800_000_000,
      operation: "fetch",
      failure: "timed_out",
      stderr: "key=[secret]\n",
      truncated: false,
    } as const;
    const fetchMock = vi
      .fn<typeof fetch>()
      .mockResolvedValueOnce(json({ run }))
      .mockResolvedValueOnce(json({}));
    vi.stubGlobal("fetch", fetchMock);

    expect(await getWorkInfoPluginLastRun(4)).toEqual(run);
    expect(await getWorkInfoPluginLastRun(4)).toBeNull();
    expect(fetchMock.mock.calls.map(([input]) => String(input))).toEqual([
      "/api/work-info-plugins/4/last-run",
      "/api/work-info-plugins/4/last-run",
    ]);
  });

  it("追加はファイル名ごと module で送り、上限超えは RequestFailed になる", async () => {
    const fetchMock = vi
      .fn<typeof fetch>()
      .mockResolvedValueOnce(json(plugin, 201))
      .mockResolvedValueOnce(json(plugin, 200))
      .mockResolvedValueOnce(
        json({ code: "plugin_too_large", message: "too large", limit: 16 << 20 }, 413),
      );
    vi.stubGlobal("fetch", fetchMock);
    const file = new File([new Uint8Array([0, 97, 115, 109])], "example.wasm");

    expect(await uploadWorkInfoPlugin(file)).toEqual({ plugin, replaced: false });
    const [input, init] = fetchMock.mock.calls[0] ?? [];
    expect(String(input)).toBe("/api/work-info-plugins");
    expect(init?.method).toBe("POST");
    const module = (init?.body as FormData).get("module");
    expect(module).toBeInstanceOf(File);
    expect((module as File).name).toBe("example.wasm");
    expect(await uploadWorkInfoPlugin(file)).toEqual({ plugin, replaced: true });

    const failure = await uploadWorkInfoPlugin(file).catch((error: unknown) => error);
    expect(failure).toBeInstanceOf(RequestFailed);
    expect((failure as RequestFailed).code).toBe("plugin_too_large");
  });

  it("消せなかった削除は RequestFailed になる", async () => {
    vi.stubGlobal(
      "fetch",
      vi
        .fn<typeof fetch>()
        .mockResolvedValue(json({ code: "not_found", message: "x" }, 404)),
    );
    await expect(deleteWorkInfoPlugin(9)).rejects.toBeInstanceOf(RequestFailed);
  });

  it("検索・取得・反映をそれぞれの経路へ送り、応答を返す", async () => {
    const candidates = [{ id: "W-1", title: "One" }];
    const fetched = {
      fetchId: "f/1",
      expiresAt: 1_800_000_600,
      title: "One",
      tags: ["drama"],
      code: "ABC-123",
      dates: [{ kind: "release", date: "2024-03-01" }],
      people: [
        {
          role: "performer",
          name: "Aoi",
          externalIds: [],
          match: { state: "existing", id: 3, name: "Aoi Sora" },
        },
      ],
      organizations: [],
      urls: [],
      externalIds: [],
    };
    const applied = {
      video: { id: 7 },
      skippedTags: [{ name: "spoiler", reason: "rejected" }],
      skipped: [{ field: "dates.release", value: "March 2024", reason: "invalid" }],
    };
    const fetchMock = vi
      .fn<typeof fetch>()
      .mockResolvedValueOnce(json({ candidates }))
      .mockResolvedValueOnce(json(fetched, 201))
      .mockResolvedValueOnce(json(applied));
    vi.stubGlobal("fetch", fetchMock);

    expect(await searchWorkInfo(4, "one")).toEqual(candidates);
    expect(await fetchWorkInfo(7, 4, "ABC-123")).toEqual(fetched);
    expect(await applyWorkInfoFetch(7, "f/1", ["title", "tags"], ["drama"])).toEqual(
      applied,
    );

    const sent = fetchMock.mock.calls.map(([input, init]) => ({
      url: String(input),
      method: init?.method,
      body:
        typeof init?.body === "string" ? (JSON.parse(init.body) as unknown) : undefined,
    }));
    expect(sent).toEqual([
      { url: "/api/work-info-plugins/4/search", method: "POST", body: { query: "one" } },
      {
        url: "/api/videos/7/work-info-fetches",
        method: "POST",
        body: { pluginId: 4, id: "ABC-123" },
      },
      {
        url: "/api/videos/7/work-info-fetches/f%2F1/apply",
        method: "POST",
        body: { fields: ["title", "tags"], tags: ["drama"] },
      },
    ]);
  });

  it("実行の失敗は pluginMessage・host・settingKeys を持つ RequestFailed になる", async () => {
    vi.stubGlobal(
      "fetch",
      vi
        .fn<typeof fetch>()
        .mockResolvedValueOnce(
          json(
            {
              code: "plugin_failed",
              reason: "network_error",
              message: "The plugin could not reach a site.",
              host: "example.com",
              pluginMessage: "timeout",
            },
            502,
          ),
        )
        .mockResolvedValueOnce(
          json(
            {
              code: "plugin_settings_missing",
              message: "x",
              settingKeys: ["apiKey"],
            },
            409,
          ),
        ),
    );
    const failed = (await searchWorkInfo(4, "one").catch(
      (e: unknown) => e,
    )) as RequestFailed;
    expect(failed).toBeInstanceOf(RequestFailed);
    expect([failed.reason, failed.host, failed.pluginMessage]).toEqual([
      "network_error",
      "example.com",
      "timeout",
    ]);
    const missing = (await fetchWorkInfo(7, 4, "ABC-123").catch(
      (e: unknown) => e,
    )) as RequestFailed;
    expect(missing.settingKeys).toEqual(["apiKey"]);
  });

  it("反映が成功したら一覧の控えを捨て、タグを付けたなら共有のタグの一覧を取り直す", async () => {
    const key = { query: "old" };
    const hold = () =>
      saveListSnapshot(key, { items: [], total: 0, hasMore: false, scrollY: 0 });
    const applied = { video: { id: 7 }, skippedTags: [] };
    const fetchMock = vi
      .fn<typeof fetch>()
      .mockResolvedValueOnce(
        json({ message: "stale", code: "work_info_fetch_stale" }, 409),
      )
      .mockResolvedValueOnce(json(applied))
      .mockResolvedValueOnce(json(applied))
      .mockResolvedValue(json({ items: [] }));
    vi.stubGlobal("fetch", fetchMock);
    const listener = vi.fn();
    const unsubscribe = subscribeTags(listener);
    const tagReads = () =>
      fetchMock.mock.calls.filter(([input]) => String(input) === "/api/tags").length;

    hold();
    await expect(applyWorkInfoFetch(7, "f1", ["title"])).rejects.toThrow();
    expect(takeListSnapshot(key)).toBeDefined();

    hold();
    await applyWorkInfoFetch(7, "f1", ["title"]);
    expect(takeListSnapshot(key)).toBeUndefined();
    expect(tagReads()).toBe(0);

    hold();
    await applyWorkInfoFetch(7, "f2", ["tags"]);
    expect(takeListSnapshot(key)).toBeUndefined();
    expect(tagReads()).toBe(1);
    await vi.waitFor(() => expect(listener).toHaveBeenCalled());
    unsubscribe();
  });
});
