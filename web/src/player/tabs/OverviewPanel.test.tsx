import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import type { Video } from "../../api/client";
import OverviewPanel, { workSummary } from "./OverviewPanel";

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
  durationMs: 242_000,
  width: 1920,
  height: 1080,
  progress: { positionMs: 1, completed: false, updatedAt: "2026-10-02T12:00:00Z" },
  workInfo: {
    code: "A-1",
    description: "about",
    dates: [],
    people: [],
    organizations: [],
    urls: [],
    externalIds: [],
  },
  tags: [],
  public: true,
};

function terms(): string[] {
  return screen.getAllByRole("term").map((term) => term.textContent ?? "");
}

describe("OverviewPanel", () => {
  it("読み取りが終わるまでは長さと解像度を出さない", () => {
    render(<OverviewPanel video={{ ...video, probeState: "pending" }} owner />);
    expect(terms()).toEqual(["Added", "Last watched"]);
  });

  it("視聴の記録が無ければ Last watched を出さない", () => {
    render(<OverviewPanel video={{ ...video, progress: undefined }} owner />);
    expect(terms()).toEqual(["Length", "Resolution", "Added"]);
  });

  it("ゲストには説明・作品の要約・Last watched を出さない", () => {
    render(<OverviewPanel video={video} owner={false} />);
    expect(terms()).toEqual(["Length", "Resolution", "Added"]);
    expect(screen.queryByText("about")).toBeNull();
    expect(screen.queryByText("A-1")).toBeNull();
  });
});

describe("workSummary", () => {
  it("出演者を人物ごとに一度だけ並べ、同名の別人は両方残す", () => {
    const performer = (id: number, name: string) => ({
      role: "performer" as const,
      person: { id, name, provisional: false },
      provisional: false,
    });
    const summary = workSummary({
      code: "A-1",
      dates: [],
      organizations: [],
      urls: [],
      externalIds: [],
      people: [performer(1, "Alice"), performer(2, "Alice"), performer(1, "Alice")],
    });
    expect(summary).toBe("A-1 · Alice, Alice");
  });
});
