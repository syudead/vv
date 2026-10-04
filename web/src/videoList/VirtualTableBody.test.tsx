import { render } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import VirtualTableBody from "./VirtualTableBody";

describe("VirtualTableBody", () => {
  it("stripes rows by item index, not by position among the drawn rows", () => {
    const { container } = render(
      <table>
        <VirtualTableBody
          count={5}
          columns={1}
          itemKey={(index) => `v:${String(index)}`}
          renderRow={(index, slot) => (
            <tr
              key={slot.rowKey}
              data-row-key={slot.rowKey}
              data-stripe={slot.striped ? "" : undefined}
            >
              <td>{index}</td>
            </tr>
          )}
        />
      </table>,
    );
    const striped = Array.from(container.querySelectorAll("tr[data-stripe]")).map(
      (row) => row.textContent,
    );
    expect(striped).toEqual(["0", "2", "4"]);
    expect(container.querySelector("tbody")!.className).toContain(
      "[&>tr[data-stripe]]:bg-hover-wash/40",
    );
  });
});
