import { describe, expect, it } from "vitest";

import { NetworkFailed, RequestFailed } from "../api/client";
import { turnFailure } from "./thread";

describe("turnFailure", () => {
  it("失敗の code と reason から、スレッドの行か準備の案内かを選ぶ", () => {
    expect(
      turnFailure(
        new RequestFailed(409, "assistant_not_ready", "", { reason: "ollama_not_saved" }),
      ),
    ).toEqual({ kind: "notReady", reason: "ollama_not_saved" });
    expect(
      turnFailure(
        new RequestFailed(503, "assistant_unavailable", "", { reason: "timeout" }),
      ),
    ).toEqual({ kind: "reply", reply: { kind: "unavailable", reason: "timeout" } });
    expect(
      turnFailure(
        new RequestFailed(400, "invalid_request", "", { reason: "too_many_videos" }),
      ),
    ).toEqual({ kind: "reply", reply: { kind: "tooManyVideos" } });
    const network = new NetworkFailed(new Error("down"));
    expect(turnFailure(network)).toEqual({
      kind: "reply",
      reply: { kind: "failed", error: network },
    });
  });
});
