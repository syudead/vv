import { describe, expect, it } from "vitest";

import { createAndAttachProposal } from "../testing/assistantProposal";
import { cardResult } from "./steps";

describe("cardResult", () => {
  it("エラーのない partial の手順では止まらず、後の手順が終われば Done を出す", () => {
    const result = cardResult(
      createAndAttachProposal("partial", [
        { status: "partial", counts: { total: 1, applied: 0, failed: 1 } },
        { status: "done", counts: { total: 3, applied: 3, failed: 0 } },
      ]),
    );

    expect(result?.text).toBe("Done · 2 steps");
  });

  it("エラーのない partial の後で失敗した手順を止まった手順として出す", () => {
    const result = cardResult(
      createAndAttachProposal("failed", [
        { status: "partial", counts: { total: 1, applied: 0, failed: 1 } },
        {
          status: "failed",
          counts: { total: 3, applied: 0, failed: 3 },
          error: { code: "not_found", message: "not found" },
        },
      ]),
    );

    expect(result?.text).toBe("Stopped at step 2");
  });
});
