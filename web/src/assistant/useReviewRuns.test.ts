import { act, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { AssistantReviewRunPage } from "../api/assistant";
import * as events from "../api/serverEvents";
import { useReviewRuns } from "./useReviewRuns";

function page(status: "running" | "done", judged: number): AssistantReviewRunPage {
  return {
    run: {
      id: 7,
      kind: "tags",
      status,
      total: 3,
      judged,
      listed: 0,
      startedAt: "2026-10-30T00:00:00Z",
    },
    items: [],
  };
}

/** Each read waits until the test answers it, so the test places the run's end between them. */
interface Pending {
  url: string;
  answer: (body: AssistantReviewRunPage) => void;
}

describe("useReviewRuns", () => {
  let reads: Pending[];
  let subscribed: number;

  beforeEach(() => {
    reads = [];
    subscribed = 0;
    vi.spyOn(events, "subscribeServerEvents").mockImplementation(() => {
      subscribed += 1;
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

  it("最初の読み取りと購読の間に終わった review は、購読後の読み直しで動作中でなくなる", async () => {
    const { result } = renderHook(() => useReviewRuns([7]));
    await waitFor(() => expect(reads).toHaveLength(1));
    act(() => reads[0]!.answer(page("running", 1)));
    await waitFor(() => expect(result.current.reviewing).toBe(true));

    // The run ends before the subscription exists, so no assistant event reaches the hook.
    await waitFor(() => expect(reads).toHaveLength(2));
    expect(subscribed).toBe(1);
    expect(reads[1]!.url).toBe("/api/assistant/review-runs/7?limit=1");
    act(() => reads[1]!.answer(page("done", 3)));

    await waitFor(() => expect(result.current.reviewing).toBe(false));
    expect(result.current.reviews.get(7)?.status).toBe("done");
  });
});
