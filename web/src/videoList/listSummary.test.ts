import { describe, expect, it } from "vitest";

import { resultCountText } from "./listSummary";

describe("resultCountText", () => {
  it("サーバーの全件数を単数・複数で表示する", () => {
    expect(resultCountText(0)).toBe("0 videos");
    expect(resultCountText(1)).toBe("1 video");
    expect(resultCountText(59)).toBe("59 videos");
    expect(resultCountText(1_234)).toBe("1,234 videos");
  });
});
