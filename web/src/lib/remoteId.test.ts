// @vitest-environment node
import { describe, expect, it } from "vitest";

import { readDomainFixture } from "../testing/domainFixture";
import {
  type IdNodes,
  idNodes,
  isRemoteId,
  remoteIdOffset,
  remoteSlot,
} from "./remoteId";

/** Share fixtures with Go's IsRemoteID, RemoteSlot and IDNodes in internal/domain/foldcheck. */
interface RemoteIdFile {
  offset: number;
  cases: { note: string; id: number; remote: boolean; slot: number; nodes: IdNodes }[];
}

const file = readDomainFixture<RemoteIdFile>("remote_id.json");

describe("isRemoteId / remoteSlot / idNodes", () => {
  it("共有の組で Go と同じ境界とスロットを返す", () => {
    expect(remoteIdOffset).toBe(file.offset);
    expect(file.cases.length).toBeGreaterThan(0);
    for (const c of file.cases) {
      expect(isRemoteId(c.id), c.note).toBe(c.remote);
      expect(remoteSlot(c.id), c.note).toBe(c.slot);
      expect(idNodes([c.id]), c.note).toBe(c.nodes);
    }
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
