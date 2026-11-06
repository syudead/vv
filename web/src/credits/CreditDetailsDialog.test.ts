import { describe, expect, it } from "vitest";

import { RequestFailed } from "../api/client";
import { problemOf } from "./CreditDetailsDialog";
import { nameControl, namesControl } from "./NameRows";

function refusal(field: string): RequestFailed {
  return new RequestFailed(400, "invalid_request", "bad", {
    reason: "work_info_duplicate",
    field,
  });
}

describe("problemOf", () => {
  // names[0] is the display name, the dialog's first row.
  const sent = { nameKeys: ["n0", "n1", "n2"], idKeys: [] };

  it("places names[i].name and names[i].language on the row sent at i", () => {
    expect(problemOf(refusal("names[0].name"), sent)?.control).toBe(
      nameControl("name", "n0"),
    );
    expect(problemOf(refusal("names[2].language"), sent)?.control).toBe(
      nameControl("language", "n2"),
    );
  });

  it("places a problem of the whole list under the list", () => {
    expect(problemOf(refusal("names"), sent)?.control).toBe(namesControl);
  });
});
