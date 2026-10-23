import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import type { Video } from "../api/client";
import VideoTitle from "./VideoTitle";

const video: Video = {
  id: 7,
  title: "clip_0042",
  fileTitle: "clip_0042",
  public: false,
  sizeBytes: 1,
  addedAt: "2026-09-01T00:00:00Z",
  updatedAt: "2026-09-01T00:00:00Z",
  fileCreatedAt: "2026-09-01T00:00:00Z",
  fileModifiedAt: "2026-09-01T00:00:00Z",
  playable: true,
  probeState: "done",
  thumbnailState: "done",
  previewState: "done",
  durationMs: 1000,
  videoCodec: "h264",
  tags: [],
};

const named: Video = { ...video, title: "Summer trip", displayName: "Summer trip" };

describe("VideoTitle", () => {
  it("所有者にも題名を編集するボタンを出さず、表示名が無ければファイル名の行も出さない", () => {
    render(<VideoTitle video={video} owner />);
    expect(screen.getByRole("heading", { level: 1 }).textContent).toBe("clip_0042");
    expect(screen.queryByRole("button")).toBeNull();
    expect(screen.queryByTitle(/^File name/)).toBeNull();
  });

  it("表示名があれば、題名の下に元のファイル名を従の 1 行で出す（受け入れ条件 7）", () => {
    render(<VideoTitle video={named} owner />);
    expect(screen.getByRole("heading", { level: 1 }).textContent).toBe("Summer trip");
    expect(screen.getByTitle("File name: clip_0042").textContent).toBe(
      "File name clip_0042",
    );
  });

  it("ゲストには題名だけを出し、ファイル名の行を出さない（受け入れ条件 9）", () => {
    render(<VideoTitle video={named} owner={false} />);
    expect(screen.getByRole("heading", { level: 1 }).textContent).toBe("Summer trip");
    expect(screen.queryByTitle(/^File name/)).toBeNull();
  });
});
