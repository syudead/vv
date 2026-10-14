import { describe, expect, it } from "vitest";

import { NetworkFailed, RequestFailed } from "../api/client";
import { undoFailedText } from "./undoToast";

describe("undoFailedText", () => {
  it("names a conflict, an expired token and any other reason", () => {
    expect(
      undoFailedText(
        new RequestFailed(409, "conflict", "x", { reason: "undo_conflict" }),
      ),
    ).toBe("Couldn't undo: it changed in the meantime");
    expect(
      undoFailedText(
        new RequestFailed(410, "not_found", "x", { reason: "undo_expired" }),
      ),
    ).toBe("This can no longer be undone");
    expect(undoFailedText(new NetworkFailed(new Error("x")))).toMatch(/^Couldn't undo: /);
  });
});
