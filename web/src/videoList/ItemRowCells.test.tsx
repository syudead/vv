import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";

import type { Video } from "../api/client";
import { ItemThumbnailCell, type OrderedItem, rowFacts } from "./ItemRowCells";

function video(extra: Partial<Video> = {}): Video {
  return {
    id: 1,
    title: "動画 1",
    public: false,
    sizeBytes: 1024,
    addedAt: "2026-09-01T00:00:00Z",
    updatedAt: "2026-09-01T00:00:00Z",
    fileCreatedAt: "2026-09-01T00:00:00Z",
    playable: true,
    probeState: "done",
    thumbnailState: "done",
    previewState: "done",
    durationMs: 60_000,
    tags: [],
    ...extra,
  };
}

function renderCell(item: OrderedItem) {
  return render(
    <table>
      <tbody>
        <tr>
          <ItemThumbnailCell item={item} facts={rowFacts(item)} />
        </tr>
      </tbody>
    </table>,
  );
}

// 順に並ぶ行（プレイリスト、グループのリスト表示）のバージョンの印
// （specs/054-auto-dup/ui-design.md「Versions mark」）。
describe("ItemThumbnailCell のバージョンの印", () => {
  afterEach(() => cleanup());

  it("2 本以上の動画は長さの面の先頭に印を出す", () => {
    const { container } = renderCell({
      kind: "video",
      video: video({ versions: { count: 2, representativeId: 1 } }),
    });
    const text = screen.getByText("2 versions");
    expect(text.className).toContain("sr-only");
    expect(text.parentElement!.textContent).toBe("2 versions1:00");
    expect(container.querySelector(".lucide-layers")).not.toBeNull();
  });

  it("時間がまだ無くても印だけの面を出す", () => {
    renderCell({
      kind: "video",
      video: video({
        durationMs: undefined,
        versions: { count: 2, representativeId: 1 },
      }),
    });
    expect(screen.getByText("2 versions").parentElement!.textContent).toBe("2 versions");
  });

  it("束ねていない動画には印を出さない", () => {
    const { container } = renderCell({ kind: "video", video: video() });
    expect(container.querySelector(".lucide-layers")).toBeNull();
    expect(screen.getByText("1:00")).not.toBeNull();
  });
});
