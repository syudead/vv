import { describe, expect, it } from "vitest";

import { UPLOAD_MAX_BYTES, uploadRefusal } from "./uploadRules";

describe("uploadRefusal", () => {
  it("PNG・JPEG・WebP は 64 MiB ちょうどまで送れる", () => {
    for (const type of ["image/png", "image/jpeg", "image/webp"]) {
      expect(uploadRefusal({ type, size: 1 })).toBeNull();
      expect(uploadRefusal({ type, size: UPLOAD_MAX_BYTES })).toBeNull();
    }
  });

  it("別の種類は大きさに関わらず種類で断る", () => {
    expect(uploadRefusal({ type: "image/gif", size: 1 })).toBe("type");
    expect(uploadRefusal({ type: "text/plain", size: UPLOAD_MAX_BYTES + 1 })).toBe(
      "type",
    );
  });

  it("種類の分からないファイル（type が空）は種類で断らず、サーバーの中身の検査に任せる", () => {
    expect(uploadRefusal({ type: "", size: 1 })).toBeNull();
    expect(uploadRefusal({ type: "", size: UPLOAD_MAX_BYTES + 1 })).toBe("size");
  });

  it("64 MiB を超える画像は大きさで断る", () => {
    expect(uploadRefusal({ type: "image/png", size: UPLOAD_MAX_BYTES + 1 })).toBe("size");
  });
});
