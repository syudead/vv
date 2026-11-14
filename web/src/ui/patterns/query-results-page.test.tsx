import { render, screen, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { QueryResultsPage, ResultBand } from "./query-results-page";

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
    expect(query.className.split(" ")).not.toContain("sticky");
  });

  it("sticks the collapsed query under the top bar below lg", () => {
    render(
      <QueryResultsPage
        header={<h1>Image search</h1>}
        query={<p>file</p>}
        queryLabel="Your file"
        queryCollapsed
      >
        <p>results</p>
      </QueryResultsPage>,
    );

    const query = screen.getByRole("complementary", { name: "Your file" });
    expect(query.className.split(" ")).toEqual(
      expect.arrayContaining(["sticky", "top-navbar"]),
    );
  });
});

describe("ResultBand", () => {
  it("adds the detail to its heading and shows the count beside it", () => {
    render(
      <ResultBand heading="Strong match" detail="90–100%" count="2 videos">
        <li>Harbour</li>
      </ResultBand>,
    );

    const heading = screen.getByRole("heading", { level: 2 });
    expect(within(heading).getByText("Strong match")).toBeTruthy();
    expect(within(heading).getByText("90–100%")).toBeTruthy();
    expect(screen.getByText("2 videos").dataset.slot).toBe("result-band-count");
    expect(screen.getByRole("listitem").textContent).toBe("Harbour");
  });

  it("shows only the heading without a detail or count", () => {
    const { container } = render(
      <ResultBand heading="Partial match">
        <li>Harbour</li>
      </ResultBand>,
    );

    expect(screen.getByRole("heading", { level: 2, name: "Partial match" })).toBeTruthy();
    expect(container.querySelector("[data-slot=result-band-count]")).toBeNull();
  });
});
