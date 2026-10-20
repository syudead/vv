import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { apiFetch } from "../../api/client";
import type { PeoplePageRows } from "./peopleCriteria";
import {
  clearPeopleSnapshot,
  peopleSnapshotKey,
  savePeopleSnapshot,
  takePeopleSnapshot,
} from "./peopleSnapshot";

const page: PeoplePageRows = {
  rows: [],
  total: 0,
  totalAll: 0,
  nextCursor: undefined,
  boundary: undefined,
  query: { query: "", sort: "name", favorite: false, named: "any" },
};
const key = peopleSnapshotKey(page.query);

beforeEach(() => {
  clearPeopleSnapshot();
  vi.stubGlobal(
    "fetch",
    vi.fn<typeof fetch>(() => Promise.resolve(new Response("{}"))),
  );
});

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("takePeopleSnapshot", () => {
  it("returns the saved grid for its conditions and nothing for others", () => {
    savePeopleSnapshot({ page, search: "", scrollY: 640 });

    expect(takePeopleSnapshot(key)?.scrollY).toBe(640);
    expect(
      takePeopleSnapshot(peopleSnapshotKey({ ...page.query, favorite: true })),
    ).toBeUndefined();
  });

  it("drops the grid after a write but keeps it across reads", async () => {
    savePeopleSnapshot({ page, search: "", scrollY: 0 });

    await apiFetch("/api/people/1");
    expect(takePeopleSnapshot(key)).toBeDefined();

    await apiFetch("/api/people/1", { method: "PATCH" });
    expect(takePeopleSnapshot(key)).toBeUndefined();
  });

  it("drops a grid saved while a write was in flight once the write settles", async () => {
    let settle = () => {};
    vi.stubGlobal(
      "fetch",
      vi.fn<typeof fetch>(
        () =>
          new Promise<Response>((resolve) => {
            settle = () => resolve(new Response("{}"));
          }),
      ),
    );
    const write = apiFetch("/api/favorites", { method: "PUT" });
    savePeopleSnapshot({ page, search: "", scrollY: 0 });

    settle();
    await write;

    expect(takePeopleSnapshot(key)).toBeUndefined();
  });
});
