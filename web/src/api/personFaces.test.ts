import { describe, expect, it } from "vitest";

import { videoFramePath } from "./personFaces";

describe("videoFramePath", () => {
  it("要求サイズを maxEdge で渡し、省略時は今のパスのまま", () => {
    expect(videoFramePath(7, 1500, 320)).toBe(
      "/api/videos/7/frame?positionMs=1500&maxEdge=320",
    );
    expect(videoFramePath(7, 1500)).toBe("/api/videos/7/frame?positionMs=1500");
  });
});
