import { waitFor, within } from "@testing-library/react";
import { useEffect, useState } from "react";
import { describe, expect, it, vi } from "vitest";

import {
  appliedResult,
  fetchById,
  fetchResult,
  json,
  server,
  useFetchServer,
  video,
} from "../testing/workInfoFetchPage";
import type { PlayerControls } from "./playerControls";

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

function checkbox(dialog: HTMLElement, name: string | RegExp) {
  return within(dialog).getByRole("checkbox", { name });
}

function tagBox(dialog: HTMLElement, name: string) {
  return checkbox(dialog, `${name}, Tentative`);
}

describe("VideoPage work-info fetch tag selection", () => {
  useFetchServer();

  it("開いたとき動画に無いタグを全部選び、動画にあるタグは選べない", async () => {
    server.fetch.mockImplementation(() =>
      json(fetchResult({ tags: ["既存", "ドラマ", "喜劇"] }), 201),
    );
    const { dialog } = await fetchById();
    expect(tagBox(dialog, "ドラマ")).toHaveProperty("ariaChecked", "true");
    expect(tagBox(dialog, "喜劇")).toHaveProperty("ariaChecked", "true");
    expect(within(dialog).queryByRole("checkbox", { name: /^既存/ })).toBeNull();
    expect(within(dialog).getByText("2 of 2 selected")).toBeDefined();
    expect(checkbox(dialog, "Tags")).toHaveProperty("ariaChecked", "true");
  });

  it("タグを 1 つ外すと、残りの名前だけを送る", async () => {
    server.fetch.mockImplementation(() =>
      json(fetchResult({ title: "取得した題名", tags: ["ドラマ", "喜劇", "活劇"] }), 201),
    );
    server.apply.mockImplementation(() => json(appliedResult(video)));
    const { user, dialog } = await fetchById();
    await user.click(tagBox(dialog, "喜劇"));
    expect(checkbox(dialog, "Tags")).toHaveProperty("ariaChecked", "mixed");
    expect(within(dialog).getByText("2 of 3 selected")).toBeDefined();
    await user.click(within(dialog).getByRole("button", { name: "Apply selected" }));
    await waitFor(() =>
      expect(server.apply).toHaveBeenCalledWith({
        fields: ["title", "tags"],
        tags: ["ドラマ", "活劇"],
      }),
    );
  });

  it("タグを全部外すと、タグを除いた項目だけを送る", async () => {
    server.apply.mockImplementation(() => json(appliedResult(video)));
    const { user, dialog } = await fetchById();
    await user.click(tagBox(dialog, "新しい"));
    expect(checkbox(dialog, "Tags")).toHaveProperty("ariaChecked", "false");
    expect(within(dialog).getByText("0 of 1 selected")).toBeDefined();
    const apply = within(dialog).getByRole("button", { name: "Apply selected" });
    expect(apply).toHaveProperty("disabled", false);
    await user.click(apply);
    await waitFor(() => expect(server.apply).toHaveBeenCalledWith({ fields: ["title"] }));
  });

  it("何も選ばれていなければ Apply selected は押せない", async () => {
    server.fetch.mockImplementation(() => json(fetchResult({ tags: ["新しい"] }), 201));
    const { user, dialog } = await fetchById();
    await user.click(tagBox(dialog, "新しい"));
    expect(within(dialog).getByRole("button", { name: "Apply selected" })).toHaveProperty(
      "disabled",
      true,
    );
  });

  it("行のチェックボックスは全部を外し、全部を選ぶ", async () => {
    server.fetch.mockImplementation(() =>
      json(fetchResult({ tags: ["ドラマ", "喜劇", "既存"] }), 201),
    );
    const { user, dialog } = await fetchById();
    const row = checkbox(dialog, "Tags");
    await user.click(row);
    expect(tagBox(dialog, "ドラマ")).toHaveProperty("ariaChecked", "false");
    expect(tagBox(dialog, "喜劇")).toHaveProperty("ariaChecked", "false");
    expect(row).toHaveProperty("ariaChecked", "false");
    await user.click(tagBox(dialog, "喜劇"));
    expect(row).toHaveProperty("ariaChecked", "mixed");
    await user.click(row);
    expect(tagBox(dialog, "ドラマ")).toHaveProperty("ariaChecked", "true");
    expect(tagBox(dialog, "喜劇")).toHaveProperty("ariaChecked", "true");
    expect(row).toHaveProperty("ariaChecked", "true");
  });

  it("30 個のタグは畳まずに全部を出す", async () => {
    const names = Array.from({ length: 30 }, (_, index) => `タグ${String(index + 1)}`);
    server.fetch.mockImplementation(() => json(fetchResult({ tags: names }), 201));
    const { dialog } = await fetchById();
    for (const name of names) expect(tagBox(dialog, name)).toBeDefined();
    expect(within(dialog).queryByRole("button", { name: /more/ })).toBeNull();
    expect(within(dialog).getByText("30 of 30 selected")).toBeDefined();
  });
});
