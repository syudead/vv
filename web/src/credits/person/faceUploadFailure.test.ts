import { describe, expect, it } from "vitest";

import { NetworkFailed, RequestFailed } from "../../api/client";
import { faceUploadFailure } from "./faceUploadFailure";

function refused(reason: string, limit?: number): RequestFailed {
  return new RequestFailed(400, "invalid_request", "Refused", { reason, limit });
}

describe("faceUploadFailure", () => {
  it("names each refusal of a face by what the owner can do about it", () => {
    expect(faceUploadFailure(refused("person_image_invalid"))).toBe(
      "This image can't be used. Choose a PNG, JPEG or still WebP.",
    );
    expect(faceUploadFailure(refused("person_image_too_large", 1024 * 1024))).toBe(
      "This image is over 1.0 MB.",
    );
    expect(faceUploadFailure(refused("person_image_no_face"))).toBe(
      "No face was found in this image. Choose a photo where the face is clear.",
    );
    expect(faceUploadFailure(refused("person_image_many_faces"))).toBe(
      "This image shows more than one face. Choose one with a single face.",
    );
  });

  it("wraps any other failure as a failed save", () => {
    expect(faceUploadFailure(new NetworkFailed(new Error("offline")))).toMatch(
      /^Couldn't save the face: /,
    );
  });
});
