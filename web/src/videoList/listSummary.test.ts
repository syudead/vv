import { describe, expect, it } from "vitest";

import { resultCountText, watchLabel } from "./listSummary";

describe("resultCountText", () => {
  it("サーバーの全件数を単数・複数で表示する", () => {
    expect(resultCountText(0)).toBe("0 videos");
    expect(resultCountText(1)).toBe("1 video");
    expect(resultCountText(59)).toBe("59 videos");
    expect(resultCountText(1_234)).toBe("1,234 videos");
  });
});

describe("watchLabel", () => {
  it("視聴状態の表示名を英語で返す", () => {
    expect(watchLabel("all")).toBe("All");
    expect(watchLabel("inProgress")).toBe("In progress");
  });
});
