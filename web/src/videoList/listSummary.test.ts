import { describe, expect, it } from "vitest";

import { resultCountText } from "./listSummary";

describe("resultCountText", () => {
  it("サーバーの全件数だけを表示する", () => {
    expect(resultCountText(0)).toBe("0件");
    expect(resultCountText(59)).toBe("59件");
    expect(resultCountText(1_234)).toBe("1,234件");
  });
});
