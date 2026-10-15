import { describe, expect, it } from "vitest";

import { parseTagListCriteria, serializeTagListCriteria } from "./tagListUrl";

describe("tagListUrl", () => {
  it("空の URL は既定の条件で、sort は端末に残した並び順を使う", () => {
    expect(parseTagListCriteria(new URLSearchParams(""), "countAsc")).toEqual({
      criteria: {
        query: "",
        tentativeOnly: false,
        unusedOnly: false,
        kind: null,
        sort: "countAsc",
        tab: "tags",
      },
      hasExplicitSort: false,
    });
  });

  it("q・tentative・unused・sort・tab を読み、解釈できない値は既定にする", () => {
    const parsed = parseTagListCriteria(
      new URLSearchParams(
        "q=%20anime%20&tentative=1&unused=1&sort=createdAsc&tab=rejected",
      ),
      "name",
    );
    expect(parsed).toEqual({
      criteria: {
        query: "anime",
        tentativeOnly: true,
        unusedOnly: true,
        kind: null,
        sort: "createdAsc",
        tab: "rejected",
      },
      hasExplicitSort: true,
    });

    const broken = parseTagListCriteria(
      new URLSearchParams("tentative=yes&sort=sizeDesc&tab=other"),
      "name",
    );
    expect(broken.criteria).toMatchObject({
      tentativeOnly: false,
      sort: "name",
      tab: "tags",
    });
    expect(broken.hasExplicitSort).toBe(false);
  });

  it("書くときは偽の絞り込みと「Tags」のタブを省き、sort はいつも書く", () => {
    expect(
      serializeTagListCriteria({
        query: "",
        tentativeOnly: false,
        unusedOnly: false,
        kind: null,
        sort: "name",
        tab: "tags",
      }).toString(),
    ).toBe("sort=name");
    expect(
      serializeTagListCriteria({
        query: "a b",
        tentativeOnly: true,
        unusedOnly: true,
        kind: null,
        sort: "countDesc",
        tab: "rejected",
      }).toString(),
    ).toBe("q=a+b&tentative=1&unused=1&sort=countDesc&tab=rejected");
  });

  it("tab=kinds と kind（正の整数か none）を読み書きし、ほかの kind は絞り込まない", () => {
    const parsed = parseTagListCriteria(new URLSearchParams("kind=12&tab=kinds"), "name");
    expect(parsed.criteria).toMatchObject({ kind: "12", tab: "kinds" });
    expect(
      parseTagListCriteria(new URLSearchParams("kind=none"), "name").criteria.kind,
    ).toBe("none");
    for (const raw of ["0", "-1", "abc", "1.5", ""]) {
      expect(
        parseTagListCriteria(new URLSearchParams(`kind=${raw}`), "name").criteria.kind,
      ).toBeNull();
    }
    expect(
      serializeTagListCriteria({
        query: "",
        tentativeOnly: true,
        unusedOnly: false,
        kind: "none",
        sort: "name",
        tab: "kinds",
      }).toString(),
    ).toBe("tentative=1&kind=none&sort=name&tab=kinds");
  });
});
