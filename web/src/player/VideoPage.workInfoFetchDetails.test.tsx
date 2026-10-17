import { screen, waitFor, within } from "@testing-library/react";
import { useEffect, useState } from "react";
import { describe, expect, it, vi } from "vitest";

import type { Video } from "../api/client";
import type { WorkInfo } from "../api/workInfo";
import type { WorkInfoFetchedPerson } from "../api/workInfoPlugins";
import {
  appliedResult,
  failure,
  fetchById,
  fetchResult,
  json,
  server,
  useFetchServer,
  video,
} from "../testing/workInfoFetchPage";
import type { PlayerControls } from "./playerControls";

// 取得のダイアログの作品の詳細の行と結果の段。行の状態の細かな場合分けは detailRows.test.ts が見る。

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

function info(overrides: Partial<WorkInfo> = {}): WorkInfo {
  return {
    dates: [],
    people: [],
    organizations: [],
    urls: [],
    externalIds: [],
    ...overrides,
  };
}

function performer(
  name: string,
  match: WorkInfoFetchedPerson["match"],
): WorkInfoFetchedPerson {
  return { role: "performer", name, externalIds: [], match };
}

/** chip は取得した側のチップを名前（title）で引く。 */
function chip(dialog: HTMLElement, name: string) {
  const found = within(dialog)
    .getAllByTitle(name)
    .filter((element) => element.dataset.variant === "secondary");
  return found[found.length - 1]!;
}

function checkbox(dialog: HTMLElement, name: string) {
  return within(dialog).getByRole("checkbox", { name });
}

describe("VideoPage work-info fetch details", () => {
  useFetchServer();

  it("題名とタグだけの結果は今までどおり 2 行で比べる", async () => {
    const { dialog } = await fetchById();
    // 2 行の選択と、動画にまだ無いタグ「新しい」の選択である。
    expect(within(dialog).getAllByRole("checkbox")).toHaveLength(3);
    expect(checkbox(dialog, "Title")).toHaveProperty("ariaChecked", "true");
    expect(checkbox(dialog, "Tags")).toHaveProperty("ariaChecked", "true");
    expect(within(dialog).queryByText("Code")).toBeNull();
  });

  it("取得したタグのうち動画にあるものは、仮の印を付けずに動画にある件として出す", async () => {
    const { dialog } = await fetchById();
    expect(chip(dialog, "既存").textContent).toBe("既存, On the video");
    expect(chip(dialog, "新しい").textContent).toBe("新しい, Tentative");
  });

  it("説明を開いてから取り直すと、新しい説明は畳んだ状態で出す", async () => {
    const scrollHeight = vi
      .spyOn(HTMLElement.prototype, "scrollHeight", "get")
      .mockReturnValue(200);
    server.fetch
      .mockImplementationOnce(() => json(fetchResult({ description: "最初の説明" }), 201))
      .mockImplementationOnce(() => json(fetchResult({ description: "次の説明" }), 201));
    server.apply.mockImplementationOnce(() =>
      failure(409, { code: "work_info_fetch_stale" }),
    );
    const { user, dialog } = await fetchById();
    await user.click(within(dialog).getByRole("button", { name: "Show more" }));
    expect(within(dialog).getByRole("button", { name: "Show less" })).toBeDefined();
    await user.click(within(dialog).getByRole("button", { name: "Apply selected" }));
    await user.click(await within(dialog).findByRole("button", { name: "Fetch again" }));
    await within(dialog).findByText("次の説明");
    expect(within(dialog).queryByRole("button", { name: "Show less" })).toBeNull();
    expect(within(dialog).getByRole("button", { name: "Show more" })).toBeDefined();
    scrollHeight.mockRestore();
  });

  it("new の人物は New の印で、ambiguous の人物は Not linked の印で出し、新規の数に数えない", async () => {
    server.fetch.mockImplementation(() =>
      json(
        fetchResult({
          title: "取得した題名",
          people: [
            performer("新しい人", { state: "new" }),
            performer("同名の人", { state: "ambiguous" }),
            performer("べつめい", { state: "existing", id: 9, name: "既存の人" }),
          ],
        }),
        201,
      ),
    );
    const { dialog } = await fetchById();
    const performers = checkbox(dialog, "Performers");
    expect(performers).toHaveProperty("ariaChecked", "true");
    expect(chip(dialog, "新しい人").textContent).toBe("新しい人, New");
    expect(chip(dialog, "同名の人").textContent).toBe("同名の人, Not linked");
    // 既存の実体に当たった件はその今の名前で出し、結び付かない件は数えない。
    expect(chip(dialog, "既存の人").textContent).toBe("既存の人");
    expect(within(dialog).getByText("2 new")).toBeDefined();
  });

  it("その役割にすでに結び付いている人物だけなら Same as now で、結び付かない件が残れば Nothing to add で選べない", async () => {
    server.video = {
      ...video,
      workInfo: info({
        people: [{ role: "performer", person: { id: 9, name: "既存の人" } }],
        organizations: [{ role: "maker", organization: { id: 4, name: "既存の会社" } }],
      }),
    };
    server.fetch.mockImplementation(() =>
      json(
        fetchResult({
          title: "テスト動画",
          people: [performer("既存の人", { state: "existing", id: 9, name: "既存の人" })],
          organizations: [
            {
              role: "maker",
              name: "既存の会社",
              externalIds: [],
              match: { state: "existing", id: 4, name: "既存の会社" },
            },
            {
              role: "maker",
              name: "同名の会社",
              externalIds: [],
              match: { state: "ambiguous" },
            },
          ],
        }),
        201,
      ),
    );
    const { dialog } = await fetchById();
    const performers = checkbox(dialog, "Performers");
    expect(performers).toHaveProperty("disabled", true);
    expect(performers).toHaveProperty("ariaChecked", "false");
    expect(within(dialog).getAllByText("Same as now").length).toBeGreaterThan(0);
    const makers = checkbox(dialog, "Makers");
    expect(makers).toHaveProperty("disabled", true);
    expect(within(dialog).getByText("Nothing to add")).toBeDefined();
    expect(chip(dialog, "同名の会社").textContent).toBe("同名の会社, Not linked");
  });

  it("値の無い項目は行が無く選べず、反映にも送らない", async () => {
    server.fetch.mockImplementation(() =>
      json(
        fetchResult({ tags: [], dates: [{ kind: "release", date: "2024-03-01" }] }),
        201,
      ),
    );
    server.apply.mockImplementation(() => json(appliedResult(video)));
    const { user, dialog } = await fetchById();
    expect(checkbox(dialog, "Title")).toHaveProperty("disabled", true);
    expect(within(dialog).queryByRole("checkbox", { name: "Code" })).toBeNull();
    expect(within(dialog).queryByRole("checkbox", { name: "Recorded" })).toBeNull();
    expect(checkbox(dialog, "Released")).toHaveProperty("ariaChecked", "true");
    await user.click(within(dialog).getByRole("button", { name: "Apply selected" }));
    expect(server.apply).toHaveBeenCalledWith({ fields: ["dates.release"] });
  });

  it("飛ばした日付を結果の段に出し、ほかの項目は反映した動画をページに出す", async () => {
    server.fetch.mockImplementation(() =>
      json(
        fetchResult({
          code: "ABC-123",
          dates: [{ kind: "release", date: "2024/03/01" }],
        }),
        201,
      ),
    );
    server.apply.mockImplementation(() =>
      json(
        appliedResult({ ...video, workInfo: info({ code: "ABC-123" }) } satisfies Video, {
          skipped: [{ field: "dates.release", value: "2024/03/01", reason: "invalid" }],
        }),
      ),
    );
    const { user, dialog } = await fetchById();
    expect(within(dialog).getByText("2024/03/01")).toBeDefined();
    await user.click(within(dialog).getByRole("button", { name: "Apply selected" }));
    expect(server.apply).toHaveBeenCalledWith({ fields: ["code", "dates.release"] });
    await within(dialog).findByText("Applied, but some values were left out");
    expect(
      within(dialog)
        .getAllByRole("listitem")
        .map((item) => item.textContent),
    ).toEqual([
      "Released: 2024/03/01 — not a date in the form YYYY-MM-DD, YYYY-MM or YYYY",
    ]);
    await user.click(within(dialog).getByRole("button", { name: "Done" }));
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    const row = screen.getByRole("list", { name: "Work details" });
    expect(row.textContent).toContain("ABC-123");
  });
});
