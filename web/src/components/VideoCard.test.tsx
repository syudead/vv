import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import { describe, expect, it } from "vitest";

import type { Video } from "../api/client";
import VideoCard from "./VideoCard";

/**
 * 一覧の 1 項目が読み上げに渡すもの
 * （FR-021 / contracts/screen-states.md 3.「読み上げ」）。
 *
 * 見た目で伝えているもの（省略した題名、視聴済みの印、途中の帯、未生成の枠）は、
 * 見えない利用者にも同じ事実が届いていなければならない。ここは目で見ても
 * 確かめられない — 印が出ていることと、その意味が読み上げに届くことは別である。
 */

/** video は最小限の Video を組み立てる（検査に要らない項目は既定値で埋める）。 */
function video(overrides: Partial<Video> = {}): Video {
  return {
    id: 1,
    title: "ねこ.mp4",
    sizeBytes: 1024,
    addedAt: "2026-09-13T00:00:00Z",
    playable: true,
    probeState: "done",
    thumbnailState: "done",
    thumbnailUrl: "/api/videos/1/thumbnail",
    ...overrides,
  };
}

/** updatedAt は Progress の必須項目だが、この検査では意味を持たない。 */
const updatedAt = "2026-09-13T00:00:00Z";

/** show は 1 項目を描く（VideoCard は Link を使うのでルータの中で描く）。 */
function show(item: Video) {
  render(
    <MemoryRouter>
      <VideoCard video={item} />
    </MemoryRouter>,
  );
}

describe("VideoCard が読み上げに渡すもの", () => {
  const longTitle =
    "とても長い題名の動画 2026 年 9 月の記録 その 3 ねこが箱に入るまでの一部始終.mp4";

  it("長い題名でも、リンクのアクセシブル名は全文である", () => {
    show(video({ title: longTitle }));

    // 見た目は 2 行で省略する（line-clamp）が、名前は省略しない（R-409）。
    expect(screen.getByRole("link", { name: longTitle })).toBeDefined();
  });

  it("サムネイルのアクセシブル名は空である（題名はリンク名が持つ）", () => {
    show(video({ title: longTitle }));

    // 題名を alt にも入れると、リンク名と画像名で同じ語が二度読まれる。
    const image = document.querySelector("img");
    expect(image).not.toBeNull();
    expect(image?.getAttribute("alt")).toBe("");
  });

  it("視聴済みの項目は「視聴済み」が読める", () => {
    show(video({ progress: { positionMs: 600_000, completed: true, updatedAt } }));

    expect(screen.getByText("視聴済み")).toBeDefined();
  });

  it("途中の項目は「N% まで再生済み」が読める", () => {
    show(
      video({
        durationMs: 600_000,
        progress: { positionMs: 150_000, completed: false, updatedAt },
      }),
    );

    // 帯は見た目でしか割合を伝えないので、意味を aria-label が持つ。
    expect(screen.getByLabelText("25% まで再生済み")).toBeDefined();
    expect(screen.queryByText("視聴済み")).toBeNull();
  });

  it("サムネイル未生成の項目は、枠の中の理由が読める", () => {
    show(video({ thumbnailUrl: undefined, thumbnailState: "pending" }));
    expect(screen.getByText("画像を準備中")).toBeDefined();

    show(video({ id: 2, thumbnailUrl: undefined, thumbnailState: "failed" }));
    expect(screen.getByText("画像を作れませんでした")).toBeDefined();
  });
});
