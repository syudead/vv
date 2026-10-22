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

// detailRows.test.ts covers individual comparison states.

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

function chip(dialog: HTMLElement, name: string) {
  const found = within(dialog)
    .getAllByTitle(name)
    .filter((element) => element.dataset.variant === "secondary");
  return found[found.length - 1]!;
}

function checkbox(dialog: HTMLElement, name: string) {
  return within(dialog).getByRole("checkbox", { name });
}

function toggle(dialog: HTMLElement, name: RegExp) {
  return within(dialog).getByRole("button", { name });
}

describe("VideoPage work-info fetch details", () => {
  useFetchServer();

  it("題名とタグだけの結果は題名の行とタグの選択だけを出す", async () => {
    const { dialog } = await fetchById();
    expect(within(dialog).getAllByRole("checkbox")).toHaveLength(2);
    expect(checkbox(dialog, "Title")).toHaveProperty("ariaChecked", "true");
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

  it("new の人物は New の印の押した切り替えで、ambiguous の人物は Not linked の印だけで出す", async () => {
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
    expect(toggle(dialog, /^新しい人 · New/)).toHaveProperty("ariaPressed", "true");
    expect(toggle(dialog, /^既存の人, Fetched as "べつめい"/)).toHaveProperty(
      "ariaPressed",
      "true",
    );
    expect(within(dialog).queryByRole("button", { name: /同名の人/ })).toBeNull();
    expect(chip(dialog, "同名の人").textContent).toBe("同名の人, Not linked");
  });

  it("出演者の切り替えを 1 つ外すと、残りの位置だけを送る", async () => {
    server.fetch.mockImplementation(() =>
      json(
        fetchResult({
          people: [
            performer("一人目", { state: "new" }),
            performer("二人目", { state: "new" }),
            performer("三人目", { state: "new" }),
          ],
        }),
        201,
      ),
    );
    server.apply.mockImplementation(() => json(appliedResult(video)));
    const { user, dialog } = await fetchById();
    await user.click(toggle(dialog, /^二人目/));
    await user.click(within(dialog).getByRole("button", { name: "Apply selected" }));
    expect(server.apply).toHaveBeenCalledWith({ fields: [], people: [0, 2] });
  });

  it("リンクと ID の切り替えを外すと、その値を送らない", async () => {
    server.fetch.mockImplementation(() =>
      json(
        fetchResult({
          urls: ["https://a.example/1", "https://b.example/2"],
          externalIds: [
            { source: "x", value: "1" },
            { source: "x", value: "2" },
          ],
        }),
        201,
      ),
    );
    server.apply.mockImplementation(() => json(appliedResult(video)));
    const { user, dialog } = await fetchById();
    await user.click(toggle(dialog, /^b\.example\/2/));
    await user.click(toggle(dialog, /^x: 2/));
    await user.click(within(dialog).getByRole("button", { name: "Apply selected" }));
    expect(server.apply).toHaveBeenCalledWith({
      fields: [],
      urls: ["https://a.example/1"],
      externalIds: [{ source: "x", value: "1" }],
    });
  });

  it("動画にあって埋めるものの無い人物は数だけを出し、結び付かない件は Not linked で選べない", async () => {
    server.video = {
      ...video,
      workInfo: info({
        organizations: [
          {
            role: "maker",
            organization: { id: 4, name: "既存の会社", provisional: false },
          },
        ],
      }),
    };
    server.fetch.mockImplementation(() =>
      json(
        fetchResult({
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
    expect(within(dialog).getByText("Makers")).toBeDefined();
    expect(within(dialog).getByText("1 already on the video")).toBeDefined();
    expect(chip(dialog, "同名の会社").textContent).toBe("同名の会社, Not linked");
    expect(within(dialog).queryByRole("button", { pressed: true })).toBeNull();
    expect(within(dialog).getByRole("button", { name: "Apply selected" })).toHaveProperty(
      "disabled",
      true,
    );
  });

  it("動画にある人物でも顔とプロフィールの空きを埋められれば選べ、取得した顔と埋める値を出す", async () => {
    server.video = {
      ...video,
      workInfo: info({
        people: [
          {
            role: "performer",
            person: { id: 9, name: "既存の人", provisional: false },
            provisional: false,
          },
        ],
      }),
    };
    server.fetch.mockImplementation(() =>
      json(
        fetchResult({
          tags: [],
          people: [
            {
              ...performer("既存の人", { state: "existing", id: 9, name: "既存の人" }),
              hasImage: true,
              profile: { heightCm: 160, bloodType: "A" },
            },
          ],
        }),
        201,
      ),
    );
    server.apply.mockImplementation(() => json(appliedResult(video)));
    const { user, dialog } = await fetchById();
    expect(
      dialog.querySelector(
        'img[src="/api/videos/7/work-info-fetches/F1/people/0/image"]',
      ),
    ).not.toBeNull();
    expect(
      within(dialog).getByText(
        "既存の人 — Fills only empty fields: Blood type A · Height 160 cm",
      ),
    ).toBeDefined();
    expect(
      toggle(dialog, /^既存の人, On the video; applying fills the person's empty fields/),
    ).toHaveProperty("ariaPressed", "true");
    await user.click(within(dialog).getByRole("button", { name: "Apply selected" }));
    expect(server.apply).toHaveBeenCalledWith({ fields: [], people: [0] });
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
    expect(within(dialog).queryByRole("checkbox", { name: "Title" })).toBeNull();
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
    const work = screen.getByRole("tabpanel", { name: "Work" });
    expect(within(work).getByText("ABC-123")).toBeDefined();
  });
});
