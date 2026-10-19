import { waitFor, within } from "@testing-library/react";
import { useEffect, useState } from "react";
import { describe, expect, it, vi } from "vitest";

import type { Person } from "../api/credits";
import type { PersonPluginResult, WorkInfoFetchedPerson } from "../api/workInfoPlugins";
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

// lookupRules.test.ts covers which items an apply sends and where each note goes.

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

const idols = { pluginId: 5, pluginName: "idols", source: "idol" };

function performer(
  name: string,
  match: WorkInfoFetchedPerson["match"],
  extra: Partial<WorkInfoFetchedPerson> = {},
): WorkInfoFetchedPerson {
  return { role: "performer", name, externalIds: [], match, ...extra };
}

function stored(overrides: Partial<Person> = {}): Person {
  return {
    id: 9,
    name: "既存の人",
    provisional: false,
    aliases: [],
    externalIds: [],
    profile: { bloodType: "A" },
    favorite: false,
    videoCount: 1,
    createdAt: "2026-09-01T00:00:00Z",
    ...overrides,
  };
}

/** A face and a height the person lacks, and a blood type the person already has. */
function found(): PersonPluginResult {
  return {
    person: {
      name: "既存の人",
      aliases: [],
      externalIds: [],
      profile: { heightCm: 160, bloodType: "A" },
      hasImage: true,
    },
    current: stored(),
  };
}

function held() {
  let release: (response: Response) => void = () => undefined;
  const promise = new Promise<Response>((resolve) => {
    release = resolve;
  });
  return { promise, release };
}

describe("VideoPage work-info fetch person lookups", () => {
  useFetchServer();

  it("lookup を計画の順に 1 件ずつ要求し、変わる項目と顔を出し、失敗しても反映できるまま", async () => {
    server.fetch.mockImplementation(() =>
      json(
        fetchResult({
          title: "取得した題名",
          people: [
            performer(
              "既存の人",
              { state: "existing", id: 9, name: "既存の人" },
              { lookup: 0 },
            ),
            performer(
              "新しい人",
              { state: "new" },
              { lookup: 1, profile: { heightCm: 150 } },
            ),
          ],
          personLookups: [
            { ...idols, id: "1" },
            { ...idols, id: "2" },
          ],
        }),
        201,
      ),
    );
    const first = held();
    server.lookup
      .mockImplementationOnce(() => first.promise)
      .mockImplementationOnce(() => failure(502, { code: "plugin_failed" }))
      .mockImplementationOnce(() =>
        json({
          person: { name: "新しい人", aliases: [], externalIds: [] },
          current: null,
        } satisfies PersonPluginResult),
      );
    const { user, dialog } = await fetchById();
    const block = within(dialog).getByRole("region", { name: "既存の人" });
    expect(within(block).getByText("From idols")).toBeDefined();
    expect(within(block).getByText("Looking up…")).toBeDefined();
    expect(server.lookup.mock.calls).toEqual([[0]]);

    first.release(json(found()));
    const failed = within(dialog).getByRole("region", { name: "新しい人" });
    await within(failed).findByText(/^Couldn't look up 新しい人: /);
    expect(server.lookup.mock.calls).toEqual([[0], [1]]);
    // The work-info profile still fills what the failed lookup left empty.
    expect(
      within(dialog).getByText(
        "新しい人 — Fills what the lookup leaves empty: Height 150 cm",
      ),
    ).toBeDefined();
    expect(within(block).getByRole("checkbox", { name: "Face" })).toBeDefined();
    expect(within(block).getByRole("checkbox", { name: "Height" })).toBeDefined();
    expect(within(block).queryByRole("checkbox", { name: "Blood type" })).toBeNull();
    expect(within(block).getByRole("button", { name: "Kept as now (1)" })).toBeDefined();
    expect(
      block.querySelector(
        'img[src="/api/videos/7/work-info-fetches/F1/person-lookups/0/image"]',
      ),
    ).not.toBeNull();
    expect(within(dialog).getByRole("button", { name: "Apply selected" })).toHaveProperty(
      "disabled",
      false,
    );

    await user.click(within(failed).getByRole("button", { name: "Try again" }));
    await within(failed).findByText("Nothing new for this person.");
    expect(server.lookup.mock.calls).toEqual([[0], [1], [1]]);
  });

  it("結果が来ると動画にある人物の役割の行を選び、行を外すと lookup の項目を無効にし、チェックした項目だけ反映に送る", async () => {
    server.video = {
      ...video,
      workInfo: {
        dates: [],
        people: [
          {
            role: "performer",
            person: { id: 9, name: "既存の人", provisional: false },
            provisional: false,
          },
          {
            role: "performer",
            person: { id: 10, name: "別の人", provisional: false },
            provisional: false,
          },
        ],
        organizations: [],
        urls: [],
        externalIds: [],
      },
    };
    server.fetch.mockImplementation(() =>
      json(
        fetchResult({
          title: "テスト動画",
          people: [
            performer(
              "既存の人",
              { state: "existing", id: 9, name: "既存の人" },
              { lookup: 0 },
            ),
            performer(
              "別の人",
              { state: "existing", id: 10, name: "別の人" },
              { lookupLimited: true },
            ),
          ],
          personLookups: [{ ...idols, id: "1" }],
        }),
        201,
      ),
    );
    const first = held();
    server.lookup.mockImplementationOnce(() => first.promise);
    server.apply.mockImplementation(() => json(appliedResult(video)));
    const { user, dialog } = await fetchById();
    expect(
      within(dialog).getByText(
        "別の人 — Not looked up: over the limit of 5 automatic lookups per fetch.",
      ),
    ).toBeDefined();
    const performers = within(dialog).getByRole("checkbox", { name: "Performers" });
    expect(performers).toHaveProperty("disabled", true);

    first.release(json(found()));
    const block = within(dialog).getByRole("region", { name: "既存の人" });
    const face = await within(block).findByRole("checkbox", { name: "Face" });
    await waitFor(() => expect(performers).toHaveProperty("ariaChecked", "true"));
    await user.click(performers);
    expect(face).toHaveProperty("disabled", true);
    expect(within(block).getByRole("checkbox", { name: "Height" })).toHaveProperty(
      "disabled",
      true,
    );
    await user.click(performers);
    expect(face).toHaveProperty("disabled", false);
    await user.click(face);
    await user.click(within(dialog).getByRole("button", { name: "Apply selected" }));
    expect(server.apply).toHaveBeenCalledWith({
      fields: ["people.performer"],
      personLookups: [{ index: 0, items: ["profile.heightCm"] }],
    });
  });

  it("lookup が走っている間に反映すると、まだ選べない行の人物も含めて残した人数を知らせる", async () => {
    server.video = {
      ...video,
      workInfo: {
        dates: [],
        people: [
          {
            role: "performer",
            person: { id: 9, name: "既存の人", provisional: false },
            provisional: false,
          },
        ],
        organizations: [],
        urls: [],
        externalIds: [],
      },
    };
    server.fetch.mockImplementation(() =>
      json(
        fetchResult({
          title: "取得した題名",
          people: [
            performer(
              "既存の人",
              { state: "existing", id: 9, name: "既存の人" },
              { lookup: 0 },
            ),
          ],
          personLookups: [{ ...idols, id: "1" }],
        }),
        201,
      ),
    );
    server.lookup.mockImplementation(() => held().promise);
    server.apply.mockImplementation(() => json(appliedResult(video)));
    const { user, dialog } = await fetchById();
    await user.click(within(dialog).getByRole("button", { name: "Apply selected" }));
    expect(server.apply).toHaveBeenCalledWith({ fields: ["title"] });
    await within(document.body).findByText(
      "Applied; 1 person was still being looked up and was left out.",
    );
  });

  it("反映が fetch を押さえている間に lookup が期限切れを返しても、反映が失敗すれば比較は反映できるままで lookup をやり直す", async () => {
    server.fetch.mockImplementation(() =>
      json(
        fetchResult({
          title: "取得した題名",
          people: [
            performer(
              "既存の人",
              { state: "existing", id: 9, name: "既存の人" },
              { lookup: 0 },
            ),
          ],
          personLookups: [{ ...idols, id: "1" }],
        }),
        201,
      ),
    );
    const first = held();
    const applying = held();
    server.lookup
      .mockImplementationOnce(() => first.promise)
      .mockImplementationOnce(() => json(found()));
    server.apply.mockImplementationOnce(() => applying.promise);
    const { user, dialog } = await fetchById();
    await user.click(within(dialog).getByRole("button", { name: "Apply selected" }));
    first.release(failure(409, { code: "work_info_fetch_stale" }));
    const block = within(dialog).getByRole("region", { name: "既存の人" });
    await within(block).findByText(/^Couldn't look up 既存の人: /);

    applying.release(failure(500, { code: "internal_error" }));
    await within(block).findByRole("checkbox", { name: "Height" });
    expect(server.lookup.mock.calls).toEqual([[0], [0]]);
    expect(
      within(dialog).queryByText(
        "This result is out of date. Fetch it again to apply it.",
      ),
    ).toBeNull();
    expect(within(dialog).getByRole("button", { name: "Apply selected" })).toHaveProperty(
      "disabled",
      false,
    );
  });
});
