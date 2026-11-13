import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { QueryResultsPage } from "./query-results-page";

describe("QueryResultsPage", () => {
  it("gives the results the full width without a query", () => {
    render(
      <QueryResultsPage header={<h1>Image search</h1>}>
        <p>results</p>
      </QueryResultsPage>,
    );

    expect(screen.queryByRole("complementary")).toBeNull();
    const body = screen.getByText("results").parentElement;
    expect(body?.dataset.slot).toBe("query-results-page-body");
    expect(body?.parentElement?.children).toHaveLength(1);
    expect(body?.className).toContain("w-full");
  });

  it("puts the named query column before the results, query-aside wide from lg", () => {
    render(
      <QueryResultsPage
        header={<h1>Image search</h1>}
        query={<p>file</p>}
        queryLabel="Your file"
      >
        <p>results</p>
      </QueryResultsPage>,
    );

    const query = screen.getByRole("complementary", { name: "Your file" });
    expect(query.className).toContain("lg:w-query-aside");
    expect(query.className).toContain("lg:sticky");
    expect(query.nextElementSibling?.getAttribute("data-slot")).toBe(
      "query-results-page-body",
    );
  });
});
