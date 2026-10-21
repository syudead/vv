import { describe, expect, it } from "vitest";

import {
  clampFaceFrame,
  FACE_ASPECT,
  faceFrameRect,
  initialFaceFrame,
} from "./faceFraming";

// A landscape frame: lengths are fractions of the 1000 px long side, so the image is 1 by 0.5.
const wide = { width: 1000, height: 500 };

describe("faceFraming", () => {
  it("frames the face with room around it, the face above the portrait's centre", () => {
    const frame = initialFaceFrame(wide, { x: 0.5, y: 0.2, w: 0.05 });
    const rect = faceFrameRect(wide, frame);

    expect(rect.width / rect.height).toBeCloseTo(FACE_ASPECT);
    expect(rect.width).toBeCloseTo(110);
    expect(rect.x + rect.width / 2).toBeCloseTo(500);
    // The face centre at 200 px sits above the portrait's centre and inside it.
    expect(rect.y).toBeLessThan(200);
    expect(rect.y + rect.height / 2).toBeGreaterThan(200);
  });

  it("keeps the frame inside the image and no wider than fits", () => {
    const rect = faceFrameRect(wide, clampFaceFrame(wide, { x: 0.99, y: 0, width: 2 }));

    expect(rect.height).toBeCloseTo(500);
    expect(rect.x + rect.width).toBeCloseTo(1000);
    expect(rect.y).toBeCloseTo(0);
  });
});
