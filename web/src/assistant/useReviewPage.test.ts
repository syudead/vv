import { act, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { AssistantReviewCandidate, AssistantReviewRunPage } from "../api/assistant";
import * as events from "../api/serverEvents";
import { useReviewPage } from "./useReviewPage";

function candidate(id: number): AssistantReviewCandidate {
  return {
    id,
    probability: 1 - id / 100,
    state: "listed",
    members: [
      { kind: "tag", id: id * 10, name: `a${String(id)}`, count: 1 },
      { kind: "tag", id: id * 10 + 1, name: `b${String(id)}`, count: 1 },
    ],
  };
}

function page(ids: number[], nextCursor?: string): AssistantReviewRunPage {
  return {
    run: {
      id: 7,
      kind: "tags",
      status: "done",
      total: 3,
      judged: 3,
      listed: 3,
      startedAt: "2026-10-25T10:00:00Z",
    },
    items: ids.map(candidate),
    nextCursor,
  };
}

/** Each read waits until the test answers it, so the test orders the two reads. */
interface Pending {
  url: string;
  answer: (body: AssistantReviewRunPage) => void;
}

describe("useReviewPage", () => {
  let reads: Pending[];
  let emit: () => void;

  beforeEach(() => {
    reads = [];
    vi.spyOn(events, "subscribeServerEvents").mockImplementation((handlers) => {
      emit = () => handlers.assistant?.();
      return () => undefined;
    });
    vi.stubGlobal(
      "fetch",
      (input: RequestInfo | URL) =>
        new Promise<Response>((resolve) => {
          reads.push({
            url: String(input),
            answer: (body) =>
              resolve(
                new Response(JSON.stringify(body), {
                  headers: { "Content-Type": "application/json" },
                }),
              ),
          });
        }),
    );
  });
  afterEach(() => {
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  it("先頭からの読み直しでカーソルが変わり破棄した続きの後も、新しいカーソルで続きを読む", async () => {
    const { result } = renderHook(() => useReviewPage(7));
    await waitFor(() => expect(reads).toHaveLength(1));
    act(() => reads[0]!.answer(page([1], "c1")));
    await waitFor(() => expect(result.current.load.kind).toBe("ready"));

    act(() => result.current.loadMore());
    await waitFor(() => expect(reads).toHaveLength(2));
    expect(reads[1]!.url).toContain("cursor=c1");
    // A higher candidate arrives, so the re-read from the start ends at another cursor.
    act(() => emit());
    await waitFor(() => expect(reads).toHaveLength(3));
    act(() => reads[2]!.answer(page([0, 1], "c2")));
    await waitFor(() => expect(result.current.rows).toHaveLength(2));
    act(() => reads[1]!.answer(page([2])));
    await waitFor(() =>
      expect(result.current.load).toMatchObject({ kind: "ready", more: "idle" }),
    );

    act(() => result.current.loadMore());
    await waitFor(() => expect(reads).toHaveLength(4));
    expect(reads[3]!.url).toContain("cursor=c2");
  });
});
