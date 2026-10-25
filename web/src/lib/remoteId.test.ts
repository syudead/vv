import { describe, expect, it } from "vitest";

import { idNodes, isRemoteId, remoteIdOffset } from "./remoteId";

describe("isRemoteId", () => {
  it("2^40 未満は手元、2^40 以上はリモート", () => {
    expect(remoteIdOffset).toBe(1099511627776);
    expect(isRemoteId(remoteIdOffset - 1)).toBe(false);
    expect(isRemoteId(remoteIdOffset)).toBe(true);
    expect(isRemoteId(remoteIdOffset + 1)).toBe(true);
  });
});

describe("idNodes", () => {
  it("ID の集合が届く台を返す", () => {
    expect(idNodes([])).toBe("none");
    expect(idNodes([1, remoteIdOffset - 1])).toBe("local");
    expect(idNodes([remoteIdOffset])).toBe("remote");
    expect(idNodes([1, remoteIdOffset])).toBe("mixed");
  });
});
