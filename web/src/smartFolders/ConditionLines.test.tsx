import { cleanup, render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { __resetTagKindsForTest } from "../api/tagKinds";
import { __resetTagsForTest } from "../api/tags";
import { json } from "../testing/groupPage";
import { PageHeader } from "../ui/patterns/page-header";
import ConditionLines from "./ConditionLines";

const fetchMock = vi.fn<typeof fetch>();

function install() {
  fetchMock.mockImplementation((input) => {
    const url = new URL(String(input), "http://localhost");
    if (url.pathname === "/api/tags") {
      return Promise.resolve(
        json({ items: [{ id: 3, name: "猫", synonyms: [], videoCount: 1 }] }),
      );
    }
    if (url.pathname === "/api/tag-kinds") {
      return Promise.resolve(json({ items: [{ id: 2, name: "作品", tagCount: 1 }] }));
    }
    if (url.pathname === "/api/people/5") {
      return Promise.resolve(json({ id: 5, name: "山田", videoCount: 1 }));
    }
    if (url.pathname === "/api/organizations/6") {
      return Promise.resolve(json({ id: 6, name: "スタジオ", videoCount: 1 }));
    }
    throw new Error(`unexpected request: ${url.toString()}`);
  });
}

beforeEach(() => {
  __resetTagsForTest();
  __resetTagKindsForTest();
  vi.stubGlobal("fetch", fetchMock);
  install();
});

afterEach(() => {
  cleanup();
  fetchMock.mockReset();
  vi.unstubAllGlobals();
});

describe("ConditionLines", () => {
  it("種類・人物・団体・タグのチップは名前を読んで出し、届くまで骨組みを出す", async () => {
    render(
      <MemoryRouter>
        <ConditionLines
          rule={{ tags: [3], tagKind: 2, person: 5, organization: 6 }}
          ancestors={[]}
          missing={[]}
        />
      </MemoryRouter>,
    );
    expect(document.querySelectorAll('[data-slot="skeleton"]').length).toBeGreaterThan(0);

    expect(await screen.findByText("Kind: 作品")).toBeTruthy();
    expect(await screen.findByText("Person: 山田")).toBeTruthy();
    expect(await screen.findByText("Organization: スタジオ")).toBeTruthy();
    expect(await screen.findByText("Tag: 猫")).toBeTruthy();
  });

  it("まだ条件を読めていない祖先の行は骨組み 1 つで、種類の一覧は読まない", async () => {
    render(
      <MemoryRouter>
        <ConditionLines
          rule={{ favorite: true }}
          ancestors={[{ folder: { id: 1, name: "親" } }]}
          missing={[]}
        />
      </MemoryRouter>,
    );

    expect(screen.getByText("Favorites")).toBeTruthy();
    expect(screen.getByRole("link", { name: 'From "親":' })).toBeTruthy();
    expect(document.querySelectorAll('[data-slot="skeleton"]')).toHaveLength(1);
    const paths = fetchMock.mock.calls.map(
      (call) => new URL(String(call[0]), "http://localhost").pathname,
    );
    expect(paths).not.toContain("/api/tag-kinds");
  });

  it("見出しの説明（p）の中に置いても、骨組みは p の中に置けない要素にならない", async () => {
    const consoleError = vi.spyOn(console, "error").mockImplementation(() => undefined);
    try {
      render(
        <MemoryRouter>
          <PageHeader
            title="猫"
            description={
              <ConditionLines rule={{ tags: [3] }} ancestors={[]} missing={[]} />
            }
          />
        </MemoryRouter>,
      );
      const skeletons = document.querySelectorAll('p [data-slot="skeleton"]');
      expect(skeletons.length).toBeGreaterThan(0);
      for (const skeleton of skeletons) expect(skeleton.tagName).toBe("SPAN");
      expect(document.querySelector("p div")).toBeNull();
      await screen.findByText("Tag: 猫");
      const nesting = consoleError.mock.calls.filter((call) =>
        call.map(String).join(" ").includes("cannot be a descendant of"),
      );
      expect(nesting).toEqual([]);
    } finally {
      consoleError.mockRestore();
    }
  });
});
