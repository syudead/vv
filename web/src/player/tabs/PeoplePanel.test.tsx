import { fireEvent, render, screen, within } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import { beforeEach, describe, expect, it, vi } from "vitest";

import type { Video } from "../../api/client";
import {
  listVideoFaceAppearances,
  type VideoFaceAppearanceList,
} from "../../api/personFaces";
import { TooltipProvider } from "../../ui/shadcn/tooltip";
import PeoplePanel, { type SpanPlayer } from "./PeoplePanel";

vi.mock("../../api/personFaces", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../../api/personFaces")>()),
  listVideoFaceAppearances: vi.fn(),
}));

const hanako = { id: 3, name: "山田 花子", provisional: false };
const unnamed = { id: 1042, name: "", provisional: true };

const video: Video = {
  id: 7,
  title: "a",
  sizeBytes: 1,
  addedAt: "2026-09-20T12:00:00Z",
  updatedAt: "2026-09-20T12:00:00Z",
  fileCreatedAt: "2026-09-20T12:00:00Z",
  playable: true,
  probeState: "done",
  thumbnailState: "done",
  previewState: "done",
  durationMs: 60_000,
  workInfo: {
    dates: [],
    people: [
      { role: "performer", person: hanako, provisional: false },
      { role: "director", person: hanako, provisional: false },
      { role: "performer", person: unnamed, provisional: true },
    ],
    organizations: [],
    urls: [],
    externalIds: [],
  },
  tags: [],
  public: false,
};

const seen: VideoFaceAppearanceList = {
  analysed: true,
  items: [
    {
      appearanceId: 11,
      person: hanako,
      hasCrop: true,
      spans: [
        { start: 15, end: 30 },
        { start: 40, end: 45 },
      ],
    },
    { appearanceId: 12, person: unnamed, hasCrop: true, spans: [{ start: 1, end: 2 }] },
  ],
};

function renderPanel(subject: Video, player: SpanPlayer) {
  return render(
    <MemoryRouter>
      <TooltipProvider>
        <PeoplePanel video={subject} player={player} />
      </TooltipProvider>
    </MemoryRouter>,
  );
}

function fakePlayer() {
  const calls: string[] = [];
  return {
    calls,
    seekTo: vi.fn((seconds: number) => calls.push(`seekTo ${String(seconds)}`)),
    play: vi.fn(() => calls.push("play")),
  };
}

beforeEach(() => {
  vi.mocked(listVideoFaceAppearances).mockReset();
});

describe("PeoplePanel", () => {
  it("クレジットされた人物は役割をまとめた1枚で、人物のページへのリンクになる", async () => {
    vi.mocked(listVideoFaceAppearances).mockResolvedValue({ analysed: false, items: [] });
    renderPanel(video, fakePlayer());
    const credits = screen.getByRole("region", { name: "Credits" });
    const tile = within(credits).getByRole("link", { name: /山田 花子/ });
    expect(tile.getAttribute("href")).toBe("/people/3");
    expect(tile.textContent).toContain("Performer · Director");
    expect(within(credits).getAllByRole("link")).toHaveLength(2);
    expect(listVideoFaceAppearances).toHaveBeenCalledWith(7, expect.any(AbortSignal));
  });

  it("帯を押すと区間の開始へ seekTo し、続けて play する", async () => {
    vi.mocked(listVideoFaceAppearances).mockResolvedValue(seen);
    const player = fakePlayer();
    renderPanel(video, player);
    const lane = await screen.findByRole("group", {
      name: "When 山田 花子 is on screen",
    });
    expect(screen.getByText("Press a bar to play from that moment")).not.toBeNull();
    fireEvent.click(within(lane).getByRole("button", { name: "0:40–0:45" }));
    expect(player.calls).toEqual(["seekTo 40", "play"]);
  });

  it("同じ人物の複数の出現は1行にまとめ、重なる区間をつなぐ", async () => {
    vi.mocked(listVideoFaceAppearances).mockResolvedValue({
      analysed: true,
      items: [
        {
          appearanceId: 11,
          person: hanako,
          hasCrop: true,
          spans: [{ start: 15, end: 30 }],
        },
        {
          appearanceId: 12,
          person: unnamed,
          hasCrop: true,
          spans: [{ start: 20, end: 22 }],
        },
        {
          appearanceId: 13,
          person: hanako,
          hasCrop: true,
          spans: [
            { start: 25, end: 35 },
            { start: 50, end: 55 },
          ],
        },
      ],
    });
    renderPanel(video, fakePlayer());
    const section = await screen.findByRole("region", { name: "Seen in this video" });
    const lanes = within(section).getAllByRole("group");
    expect(lanes.map((lane) => lane.getAttribute("aria-label"))).toEqual([
      "When 山田 花子 is on screen",
      "When #1042 is on screen",
    ]);
    const lane = screen.getByRole("group", { name: "When 山田 花子 is on screen" });
    const spans = within(lane).getAllByRole("button");
    expect(spans.map((span) => span.getAttribute("aria-label"))).toEqual([
      "0:15–0:35",
      "0:50–0:55",
    ]);
  });

  it.each<[string, VideoFaceAppearanceList]>([
    ["顔認識が終わっていない", { analysed: false, items: [] }],
    ["見つかった人物がいない", { analysed: true, items: [] }],
  ])("%s動画では下段を出さない", async (_, list) => {
    vi.mocked(listVideoFaceAppearances).mockResolvedValue(list);
    renderPanel(video, fakePlayer());
    await vi.waitFor(() => expect(screen.queryByRole("status")).toBeNull());
    expect(screen.queryByRole("region", { name: "Seen in this video" })).toBeNull();
  });

  it("長さが分からない動画では人物だけを出して帯を出さない", async () => {
    vi.mocked(listVideoFaceAppearances).mockResolvedValue(seen);
    renderPanel({ ...video, durationMs: undefined }, fakePlayer());
    const section = await screen.findByRole("region", { name: "Seen in this video" });
    expect(within(section).getByRole("link", { name: /山田 花子/ })).not.toBeNull();
    expect(within(section).queryByRole("group")).toBeNull();
    expect(screen.queryByText("Press a bar to play from that moment")).toBeNull();
  });

  it("プレイヤーが出ていなければ帯はボタンにならず、案内も出さない", async () => {
    vi.mocked(listVideoFaceAppearances).mockResolvedValue(seen);
    renderPanel(video, null);
    const lane = await screen.findByRole("group", {
      name: "When 山田 花子 is on screen",
    });
    expect(within(lane).queryByRole("button")).toBeNull();
    expect(screen.queryByText("Press a bar to play from that moment")).toBeNull();
  });

  it("名前の無い人物は両方の段で #{id} と出す", async () => {
    vi.mocked(listVideoFaceAppearances).mockResolvedValue(seen);
    renderPanel(video, fakePlayer());
    const section = await screen.findByRole("region", { name: "Seen in this video" });
    const seenLink = within(section).getByRole("link", { name: /#1042/ });
    expect(seenLink.getAttribute("href")).toBe("/people/1042");
    const credits = screen.getByRole("region", { name: "Credits" });
    expect(within(credits).getByRole("link", { name: /#1042/ })).not.toBeNull();
  });
});
