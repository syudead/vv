import { describe, expect, it } from "vitest";

import { RequestFailed } from "../api/client";
import { problemOf } from "./CreditDetailsDialog";

function refusal(field: string): RequestFailed {
  return new RequestFailed(400, "invalid_request", "bad", {
    reason: "work_info_duplicate",
    field,
  });
}

describe("problemOf", () => {
  // names[0] is the display name, which no row of the dialog shows.
  const sent = { nameKeys: ["", "a1", "a2"], idKeys: [] };

  it("places names[i].name on the row sent at i", () => {
    expect(problemOf(refusal("names[2].name"), sent)?.control).toBe("alias-name-a2");
  });

  it("leaves a problem of the display name and of the list to their own places", () => {
    expect(problemOf(refusal("names[0].name"), sent)).toBeNull();
    expect(problemOf(refusal("names"), sent)?.control).toBe("aliases");
  });
});
