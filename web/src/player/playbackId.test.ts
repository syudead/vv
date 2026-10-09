import { afterEach, describe, expect, it, vi } from "vitest";

import { newPlaybackId } from "./playbackId";

const uuidV4 = /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/;

describe("newPlaybackId", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("crypto.randomUUID が無くても RFC 4122 version 4 の 36 文字の形で作る", () => {
    // LAN の素の http では randomUUID が無い。getRandomValues だけを持つ crypto にする。
    const getRandomValues = vi.fn((array: Uint8Array) => {
      array.fill(0xff);
      return array;
    });
    vi.stubGlobal("crypto", { getRandomValues });

    const id = newPlaybackId();

    expect(getRandomValues).toHaveBeenCalledOnce();
    expect(id).toHaveLength(36);
    expect(id).toMatch(uuidV4);
    expect(id).toBe("ffffffff-ffff-4fff-bfff-ffffffffffff");
  });

  it("呼ぶたびに別の識別子を作る", () => {
    const first = newPlaybackId();
    const second = newPlaybackId();
    expect(first).toMatch(uuidV4);
    expect(second).toMatch(uuidV4);
    expect(first).not.toBe(second);
  });
});
