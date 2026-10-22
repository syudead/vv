import { act, renderHook, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { Credit } from "../api/credits";
import type { TagKind } from "../api/tagKinds";
import type { Tag } from "../api/tags";
import { __resetTagKindsForTest } from "../api/tagKinds";
import { __resetTagsForTest } from "../api/tags";
import {
  pickOfSuggestion,
  suggestionGroups,
  useSearchSuggestions,
} from "./searchSuggestions";

function credit(id: number, name: string): Credit {
  return { id, name, aliases: [], externalIds: [], videoCount: 1 } as unknown as Credit;
}

function tag(id: number, name: string): Tag {
  return {
    kind: null,
    id,
    name,
    synonyms: [],
    videoCount: 1,
    tentative: false,
    createdAt: "2026-01-01T00:00:00Z",
  };
}

function kind(id: number, name: string): TagKind {
  return { id, name, tagCount: 1, createdAt: "2026-01-01T00:00:00Z" };
}

describe("suggestionGroups", () => {
  it("orders People, Tags (with kinds) and Organizations, at most three each, and leaves out empty groups", () => {
    const groups = suggestionGroups("ky", {
      people: [
        credit(1, "Kyoko"),
        credit(2, "Kyle"),
        credit(3, "Kyra"),
        credit(4, "Kyu"),
      ],
      organizations: [],
      tags: [tag(10, "kyoto"), tag(11, "tokyo"), tag(12, "nara"), tag(13, "Kyushu")],
      kinds: [kind(20, "Kyogen")],
    });

    expect(groups.map((group) => group.heading)).toEqual(["People", "Tags"]);
    expect(groups[0]?.options.map((option) => option.label)).toEqual([
      "Kyoko",
      "Kyle",
      "Kyra",
    ]);
    expect(groups[1]?.options.map((option) => option.label)).toEqual([
      "Kind: Kyogen",
      "kyoto",
      "Kyushu",
    ]);
    expect(groups[1]?.options.map(pickOfSuggestion)).toEqual([
      { kind: "tagKind", id: 20 },
      { kind: "tag", id: 10 },
      { kind: "tag", id: 13 },
    ]);
    expect(groups[0]?.options.map(pickOfSuggestion)[0]).toEqual({
      kind: "person",
      id: 1,
    });
  });
});

describe("useSearchSuggestions", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
    __resetTagsForTest();
    __resetTagKindsForTest();
  });

  it("aborts the requests for an older text and shows groups only for the newest text", async () => {
    const signals = new Map<string, AbortSignal | null | undefined>();
    const fetchMock = vi.fn<typeof fetch>((input, init) => {
      const url = new URL(String(input), "http://localhost");
      if (url.pathname === "/api/people") {
        const q = url.searchParams.get("q") ?? "";
        signals.set(q, init?.signal);
        return Promise.resolve(
          new Response(JSON.stringify({ items: [credit(1, q)], total: 1, totalAll: 1 })),
        );
      }
      return Promise.resolve(new Response(JSON.stringify({ items: [] })));
    });
    vi.stubGlobal("fetch", fetchMock);
    const { result } = renderHook(() => useSearchSuggestions(true, () => undefined));

    act(() => result.current?.onTermChange("a"));
    act(() => result.current?.onTermChange("ab"));

    await waitFor(() => expect(result.current?.term).toBe("ab"));
    expect(signals.get("a")?.aborted).toBe(true);
    expect(signals.get("ab")?.aborted).toBe(false);
    expect(result.current?.groups[0]?.options.map((option) => option.label)).toEqual([
      "ab",
    ]);
  });

  it("is undefined when disabled", () => {
    const { result } = renderHook(() => useSearchSuggestions(false, () => undefined));
    expect(result.current).toBeUndefined();
  });
});
