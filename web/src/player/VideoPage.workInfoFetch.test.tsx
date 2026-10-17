import { screen, waitFor, within } from "@testing-library/react";
import { useEffect, useState } from "react";
import { describe, expect, it, vi } from "vitest";

import type { Video } from "../api/client";
import type { WorkInfoPlugin } from "../api/workInfoPlugins";
import {
  appliedResult,
  entry,
  failure,
  fetchById,
  fetchResult,
  heading,
  json,
  openDialog,
  openPage,
  server,
  useFetchServer,
  video,
  workId,
  workUrl,
  works,
} from "../testing/workInfoFetchPage";
import type { PlayerControls } from "./playerControls";

// 動画ページの作品情報の取得。作品の詳細の行は VideoPage.workInfoFetchDetails.test.tsx。

vi.mock("./VideoPlayer", () => ({
  default: function VideoPlayerMock(props: {
    onControls: (controls: PlayerControls | null) => void;
  }) {
    const [initial] = useState(props);
    useEffect(() => () => initial.onControls(null), [initial]);
    return <div data-testid="video-player" />;
  },
  canStartPlayback: () => true,
  initialPlayerStatus: {
    loading: false,
    reconnecting: false,
    playing: false,
    userActive: true,
    ended: false,
    stalled: false,
    positioned: false,
  },
}));

/** coded は品番だけを持つ作品情報である。 */
const coded: Video["workInfo"] = {
  code: "ABC-123",
  dates: [],
  people: [],
  organizations: [],
  urls: [],
  externalIds: [],
};

describe("VideoPage work-info fetch", () => {
  useFetchServer();

  it("ゲストには取得の入口が無く、プラグインの一覧も読まない", async () => {
    await openPage("guest");
    expect(
      screen.queryByRole("button", { name: "Fetch work info from a plugin" }),
    ).toBeNull();
    expect(server.requests).not.toContain("GET /api/work-info-plugins");
  });

  it("承認済みのプラグインが無ければ、所有者にも入口を出さない", async () => {
    server.plugins = [
      { ...works, state: "unapproved" },
      { ...works, id: 4, state: "invalid" },
    ];
    await openPage();
    await waitFor(() => expect(server.requests).toContain("GET /api/work-info-plugins"));
    expect(
      screen.queryByRole("button", { name: "Fetch work info from a plugin" }),
    ).toBeNull();
  });

  it("入口は Edit details の後、サムネイルの操作の前にある", async () => {
    await openPage();
    const fetchEntry = await entry();
    const details = screen.getByRole("link", { name: "Edit details" });
    expect(
      details.compareDocumentPosition(fetchEntry) & Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();
    // 後ろの操作はサムネイルの編集（プレイヤーの有無に依らず所有者に出る）。
    const edit = screen.getByRole("button", { name: "Edit thumbnail with ComfyUI" });
    expect(
      fetchEntry.compareDocumentPosition(edit) & Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();
  });

  it("語で検索し、候補を選び、題名とタグを反映すると、ページが応答の動画を示してトーストを出す", async () => {
    server.search.mockImplementation(() =>
      json({
        candidates: [
          { id: workId, url: workUrl, title: "候補の作品", detail: "2024 · 映画" },
          { id: "W-2", url: "https://other.example/w/2", title: "別の作品" },
        ],
      }),
    );
    server.apply.mockImplementation(() =>
      json(appliedResult({ ...video, title: "取得した題名" })),
    );
    const { user, dialog } = await openDialog();
    const query = within(dialog).getByRole("textbox", { name: "Search words" });
    expect(query).toHaveProperty("value", "テスト動画");
    await user.click(within(dialog).getByRole("button", { name: "Search" }));
    expect(server.search).toHaveBeenCalledWith({ query: "テスト動画" });
    await within(dialog).findByText("2 matches");
    // 候補の行はページのホストとページへのリンクを出す。リンクは開くだけで、取得には使わない。
    expect(within(dialog).getByText("other.example")).toBeDefined();
    expect(
      within(dialog)
        .getAllByRole("link", { name: "Open page" })
        .map((link) => link.getAttribute("href")),
    ).toEqual([workUrl, "https://other.example/w/2"]);
    await user.click(within(dialog).getByRole("button", { name: /候補の作品/ }));
    await within(dialog).findByRole("checkbox", { name: "Title" });
    // 候補を選ぶとその ID で取得し、候補の URL は送らない（親 Issue 1232 の要件 15）。
    expect(server.fetch).toHaveBeenCalledWith({ pluginId: 3, id: workId });
    // 取得した値は「既存」が動画に付いているので、選べるのは 1 つだけである。
    expect(within(dialog).getByText("1 of 1 selected")).toBeDefined();
    expect(within(dialog).getByRole("checkbox", { name: "Title" })).toHaveProperty(
      "ariaChecked",
      "true",
    );
    expect(
      within(dialog).getByRole("button", { name: "Back to the matches" }),
    ).toBeDefined();
    await user.click(within(dialog).getByRole("button", { name: "Apply selected" }));
    expect(server.apply).toHaveBeenCalledWith({
      fields: ["title", "tags"],
      tags: ["新しい"],
    });
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(heading().textContent).toBe("取得した題名");
    expect(await screen.findByText("Applied to the video.")).toBeDefined();
    await waitFor(() =>
      expect(document.activeElement).toBe(
        screen.getByRole("button", {
          name: "Fetch work info from a plugin",
        }),
      ),
    );
  });

  it("値の無い項目と今と同じ項目は選べず、その理由を出す", async () => {
    server.fetch.mockImplementation(() => json(fetchResult({ tags: ["既存"] }), 201));
    const { dialog } = await fetchById();
    const title = within(dialog).getByRole("checkbox", { name: "Title" });
    const tags = within(dialog).getByRole("checkbox", { name: "Tags" });
    expect(title).toHaveProperty("disabled", true);
    expect(title).toHaveProperty("ariaChecked", "false");
    expect(tags).toHaveProperty("disabled", true);
    expect(within(dialog).getByText("Not in the result")).toBeDefined();
    expect(within(dialog).getByText("Same as now")).toBeDefined();
    expect(within(dialog).getByRole("button", { name: "Apply selected" })).toHaveProperty(
      "disabled",
      true,
    );
  });

  it("プラグインの失敗は理由とプラグインの文を出し、動画を変えない", async () => {
    server.fetch.mockImplementation(() =>
      failure(502, { code: "plugin_failed", pluginMessage: "rate limited" }),
    );
    const { user, dialog } = await openDialog();
    await user.click(within(dialog).getByRole("radio", { name: "ID" }));
    const query = within(dialog).getByRole("textbox", { name: "The work's ID" });
    await user.clear(query);
    await user.type(query, workId);
    await user.click(within(dialog).getByRole("button", { name: "Fetch" }));
    const alert = await within(dialog).findByRole("alert");
    expect(alert.textContent).toContain("Couldn't fetch: The plugin failed.");
    expect(alert.textContent).toContain('The plugin said: "rate limited"');
    expect(query).toHaveProperty("value", workId);
    expect(server.apply).not.toHaveBeenCalled();
    expect(heading().textContent).toBe("テスト動画");
  });

  it("足りない設定は警告と Open Settings を出す", async () => {
    server.search.mockImplementation(() =>
      failure(409, { code: "plugin_settings_missing", settingKeys: ["token"] }),
    );
    const { user, dialog } = await openDialog();
    await user.click(within(dialog).getByRole("button", { name: "Search" }));
    const alert = await within(dialog).findByRole("alert");
    expect(alert.textContent).toContain("works needs API token first.");
    expect(
      within(alert).getByRole("link", { name: "Open Settings" }).getAttribute("href"),
    ).toBe("/settings/plugins#work-info-plugins");
  });

  it("期限切れの結果は知らせと Fetch again を出し、動画を変えず、取り直すと新しい比較になる", async () => {
    server.apply.mockImplementationOnce(() =>
      failure(409, { code: "work_info_fetch_stale" }),
    );
    const { user, dialog } = await fetchById();
    await user.click(within(dialog).getByRole("button", { name: "Apply selected" }));
    await within(dialog).findByText(
      "This result is out of date. Fetch it again to apply it.",
    );
    expect(within(dialog).getByRole("button", { name: "Apply selected" })).toHaveProperty(
      "disabled",
      true,
    );
    expect(heading().textContent).toBe("テスト動画");
    await user.click(within(dialog).getByRole("button", { name: "Fetch again" }));
    await waitFor(() => expect(server.fetch).toHaveBeenCalledTimes(2));
    await waitFor(() =>
      expect(
        within(dialog).queryByText(
          "This result is out of date. Fetch it again to apply it.",
        ),
      ).toBeNull(),
    );
    expect(server.fetch).toHaveBeenLastCalledWith({ pluginId: 3, id: workId });
    expect(within(dialog).getByRole("button", { name: "Apply selected" })).toHaveProperty(
      "disabled",
      false,
    );
  });

  it("付けなかったタグがあれば、結果の段で却下したものを先に並べ、Done で閉じる", async () => {
    server.apply.mockImplementation(() =>
      json(
        appliedResult(
          { ...video, title: "取得した題名" },
          {
            skippedTags: [
              { name: "悪い名前", reason: "invalid" },
              { name: "却下した", reason: "rejected" },
            ],
          },
        ),
      ),
    );
    const { user, dialog } = await fetchById();
    await user.click(within(dialog).getByRole("button", { name: "Apply selected" }));
    await within(dialog).findByText("Applied, but some values were left out");
    expect(
      within(dialog)
        .getAllByRole("listitem")
        .map((item) => item.textContent),
    ).toEqual([
      "Tags: 却下した — you rejected this tag before",
      "Tags: 悪い名前 — not a valid tag name",
    ]);
    expect(heading().textContent).toBe("取得した題名");
    expect(within(dialog).queryByRole("button", { name: "Cancel" })).toBeNull();
    await user.click(within(dialog).getByRole("button", { name: "Done" }));
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  });

  it("カバー画像は今のサムネイルと並べ、最初から選び、使えなければ結果の段で知らせる", async () => {
    server.fetch.mockImplementation(() =>
      json(
        fetchResult({ title: "取得した題名", cover: { width: 800, height: 450 } }),
        201,
      ),
    );
    server.apply.mockImplementation(() =>
      json(
        appliedResult(
          { ...video, title: "取得した題名" },
          {
            skipped: [
              { field: "cover", value: "https://works.example/c.jpg", reason: "invalid" },
            ],
          },
        ),
      ),
    );
    const { user, dialog } = await fetchById();
    const cover = within(dialog).getByRole("checkbox", { name: "Cover" });
    expect(cover).toHaveProperty("ariaChecked", "true");
    const fetched = within(dialog).getByRole("img", { name: /Fetched cover, 800 × 450/ });
    expect(fetched.getAttribute("src")).toBe("/api/videos/7/work-info-fetches/F1/cover");
    expect(
      within(dialog).getByRole("img", { name: "Current thumbnail" }).getAttribute("src"),
    ).toBe("/api/videos/7/thumbnail");
    await user.click(within(dialog).getByRole("button", { name: "Apply selected" }));
    expect(server.apply).toHaveBeenCalledWith({ fields: ["title", "cover"] });
    await within(dialog).findByText("Applied, but some values were left out");
    expect(
      within(dialog)
        .getAllByRole("listitem")
        .map((item) => item.textContent),
    ).toEqual(["Cover: https://works.example/c.jpg — couldn't be used as the thumbnail"]);
  });

  it("サムネイルの無い動画では、カバー画像の行に取得した画像だけを出す", async () => {
    server.video = { ...video, thumbnailUrl: undefined, thumbnailState: "pending" };
    server.fetch.mockImplementation(() =>
      json(
        fetchResult({ title: "取得した題名", cover: { width: 800, height: 450 } }),
        201,
      ),
    );
    const { dialog } = await fetchById();
    expect(within(dialog).getByRole("img", { name: /Fetched cover/ })).toBeDefined();
    expect(within(dialog).queryByRole("img", { name: "Current thumbnail" })).toBeNull();
  });

  it("承認を外されたプラグインの失敗では一覧を読み直し、残りが無ければ閉じた後に入口が消える", async () => {
    server.search.mockImplementation(() => {
      server.plugins = [{ ...works, state: "unapproved" }];
      return failure(409, { code: "plugin_not_approved" });
    });
    const { user, dialog } = await openDialog();
    await user.click(within(dialog).getByRole("button", { name: "Search" }));
    await within(dialog).findByText(/The plugin is no longer approved/);
    await waitFor(() =>
      expect(
        server.requests.filter((r) => r === "GET /api/work-info-plugins"),
      ).toHaveLength(2),
    );
    await waitFor(() =>
      expect(within(dialog).getByRole("button", { name: "Search" })).toHaveProperty(
        "disabled",
        true,
      ),
    );
    await user.click(within(dialog).getByRole("button", { name: "Cancel" }));
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(
      screen.queryByRole("button", { name: "Fetch work info from a plugin" }),
    ).toBeNull();
  });

  it("取得しかできないプラグインは動画の品番を最初の値にし、ID で取得して比較の段に ID を出す", async () => {
    server.plugins = [{ ...works, operations: ["fetch"] }];
    server.video = { ...video, workInfo: coded };
    const { user, dialog } = await openDialog();
    const query = within(dialog).getByRole("textbox", { name: "The work's ID" });
    expect(query).toHaveProperty("value", "ABC-123");
    expect(within(dialog).queryByRole("radio", { name: "ID" })).toBeNull();
    await user.click(within(dialog).getByRole("button", { name: "Fetch" }));
    await within(dialog).findByRole("checkbox", { name: "Title" });
    expect(server.fetch).toHaveBeenCalledWith({ pluginId: 3, id: "ABC-123" });
    expect(within(dialog).getByText(/ID ABC-123/)).toBeDefined();
    expect(
      within(dialog).queryByRole("button", { name: "Back to the matches" }),
    ).toBeNull();
  });

  it("検索も取得もできるプラグインは Words と ID を選べ、ID に切り替えると語を ID として取得する", async () => {
    server.video = { ...video, workInfo: coded };
    const { user, dialog } = await openDialog();
    const words = within(dialog).getByRole("textbox", { name: "Search words" });
    expect(words).toHaveProperty("value", "テスト動画");
    await user.click(within(dialog).getByRole("radio", { name: "ID" }));
    const id = within(dialog).getByRole("textbox", { name: "The work's ID" });
    expect(id).toHaveProperty("value", "ABC-123");
    await user.click(within(dialog).getByRole("button", { name: "Fetch" }));
    await within(dialog).findByRole("checkbox", { name: "Title" });
    expect(server.fetch).toHaveBeenCalledWith({ pluginId: 3, id: "ABC-123" });
    expect(server.search).not.toHaveBeenCalled();
  });

  it("URL を貼っても作品の ID としてそのまま送り、Words のままなら語として検索する", async () => {
    const pasted = "https://works.example/w/1";
    const { user, dialog } = await openDialog();
    const words = within(dialog).getByRole("textbox", { name: "Search words" });
    await user.clear(words);
    await user.type(words, pasted);
    // URL の形でも入力欄の名前と押すボタンは変わらない（親 Issue 1232 の要件 14）。
    expect(within(dialog).queryByRole("button", { name: "Fetch" })).toBeNull();
    await user.click(within(dialog).getByRole("button", { name: "Search" }));
    expect(server.search).toHaveBeenCalledWith({ query: pasted });
    await user.click(within(dialog).getByRole("radio", { name: "ID" }));
    const id = within(dialog).getByRole("textbox", { name: "The work's ID" });
    expect(id).toHaveProperty("value", pasted);
    await user.click(within(dialog).getByRole("button", { name: "Fetch" }));
    await within(dialog).findByRole("checkbox", { name: "Title" });
    expect(server.fetch).toHaveBeenCalledWith({ pluginId: 3, id: pasted });
    expect(within(dialog).getByText(/ID https:\/\/works\.example\/w\/1/)).toBeDefined();
  });

  it("検索だけのプラグインの候補は押せる行にせず、取得を送らない", async () => {
    server.plugins = [{ ...works, operations: ["search"] }];
    server.search.mockImplementation(() =>
      json({ candidates: [{ id: workId, url: workUrl, title: "候補の作品" }] }),
    );
    const { user, dialog } = await openDialog();
    await user.click(within(dialog).getByRole("button", { name: "Search" }));
    await within(dialog).findByText("1 match");
    expect(within(dialog).getByText("候補の作品")).toBeDefined();
    expect(within(dialog).getByText("works.example")).toBeDefined();
    expect(
      within(dialog).getByRole("link", { name: "Open page" }).getAttribute("href"),
    ).toBe(workUrl);
    expect(within(dialog).queryByRole("button", { name: /候補の作品/ })).toBeNull();
    await user.click(within(dialog).getByText("候補の作品"));
    expect(server.fetch).not.toHaveBeenCalled();
  });

  it("選んでいたプラグインが一覧から消えたら、残りのプラグインへ移って前の候補を捨てる", async () => {
    const other: WorkInfoPlugin = { ...works, id: 4, name: "other" };
    server.plugins = [works, other];
    server.search.mockImplementation(() =>
      json({ candidates: [{ id: workId, url: workUrl, title: "候補の作品" }] }),
    );
    server.fetch.mockImplementation(() => {
      server.plugins = [other];
      return failure(404, { code: "not_found" });
    });
    const { user, dialog } = await openDialog();
    await user.click(within(dialog).getByRole("button", { name: "Search" }));
    await user.click(await within(dialog).findByRole("button", { name: /候補の作品/ }));
    await within(dialog).findByRole("alert");
    await waitFor(() =>
      expect(within(dialog).queryByRole("button", { name: /候補の作品/ })).toBeNull(),
    );
    expect(within(dialog).getByText("other")).toBeDefined();
    expect(server.fetch).toHaveBeenCalledTimes(1);
  });

  it("反映した題名をグループの並びにあるこの動画にも写す", async () => {
    const group = {
      id: 1,
      folder: { rootId: 1, path: "series" },
      name: "series",
      position: 0,
      count: 2,
    };
    const member: Video = {
      ...video,
      id: 8,
      title: "次の回",
      group: { ...group, position: 1 },
    };
    server.video = { ...video, group };
    server.related = {
      items: [],
      nextId: 8,
      group: {
        folder: group.folder,
        name: "series",
        items: [server.video, member],
        offset: 0,
        total: 2,
      },
    };
    server.apply.mockImplementation(() =>
      json(appliedResult({ ...server.video, title: "取得した題名" })),
    );
    const { user, dialog } = await fetchById();
    const aside = document.querySelector("aside");
    expect(aside?.textContent).toContain("テスト動画");
    await user.click(within(dialog).getByRole("button", { name: "Apply selected" }));
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    await waitFor(() =>
      expect(document.querySelector("aside")?.textContent).toContain("取得した題名"),
    );
    expect(document.querySelector("aside")?.textContent).not.toContain("テスト動画");
  });
});
