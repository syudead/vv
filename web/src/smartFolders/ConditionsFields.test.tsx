import { act, cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { SmartRule, SmartRuleRef } from "../api/smartFolders";
import { __resetTagKindsForTest, getTagKinds } from "../api/tagKinds";
import { __resetTagsForTest } from "../api/tags";
import { TooltipProvider } from "../ui/shadcn/tooltip";
import ConditionsFields from "./ConditionsFields";

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function credit(id: number, name: string) {
  return { id, name, aliases: [], externalIds: [], videoCount: 1 };
}

function kind(id: number, name: string) {
  return { id, name, tagCount: 1, createdAt: "2026-10-01T00:00:00Z" };
}

const tags = Array.from({ length: 17 }, (_, index) => ({
  id: index + 1,
  name: index === 0 ? "猫" : index === 1 ? "犬" : `tag${String(index + 1)}`,
  synonyms: [],
  videoCount: 1,
}));

const fetchMock = vi.fn<typeof fetch>();
let kindLists: ReturnType<typeof kind>[][] = [];
let peopleAnswer: (q: string) => Promise<Response> = () =>
  Promise.resolve(json({ items: [] }));

function install() {
  fetchMock.mockImplementation((input) => {
    const url = new URL(String(input), "http://localhost");
    const path = url.pathname;
    if (path === "/api/tags") return Promise.resolve(json({ items: tags }));
    if (path === "/api/tag-kinds") {
      const items = kindLists.length > 1 ? kindLists.shift() : kindLists[0];
      return Promise.resolve(json({ items: items ?? [] }));
    }
    if (path === "/api/people") return peopleAnswer(url.searchParams.get("q") ?? "");
    if (path === "/api/people/3")
      return Promise.resolve(json({ ...credit(3, "Alice"), imageRevision: 2 }));
    if (path === "/api/organizations/5") {
      return Promise.resolve(json({ code: "not_found", message: "gone" }, 404));
    }
    throw new Error(`unexpected request: ${url.toString()}`);
  });
}

const onChange = vi.fn<(rule: SmartRule) => void>();

function Harness({
  initial,
  missing = [],
}: {
  initial: SmartRule;
  missing?: readonly SmartRuleRef[];
}) {
  const [rule, setRule] = useState(initial);
  return (
    <ConditionsFields
      rule={rule}
      onChange={(next) => {
        onChange(next);
        setRule(next);
      }}
      missing={missing}
      error={null}
    />
  );
}

function renderFields(initial: SmartRule, missing?: readonly SmartRuleRef[]) {
  return render(
    <TooltipProvider>
      <Harness initial={initial} missing={missing} />
    </TooltipProvider>,
  );
}

function lastRule(): SmartRule | undefined {
  return onChange.mock.calls.at(-1)?.[0];
}

describe("ConditionsFields", () => {
  beforeEach(() => {
    __resetTagsForTest();
    __resetTagKindsForTest();
    kindLists = [[kind(1, "Genre")]];
    peopleAnswer = () => Promise.resolve(json({ items: [] }));
    vi.stubGlobal("fetch", fetchMock);
    Element.prototype.hasPointerCapture ??= () => false;
    Element.prototype.releasePointerCapture ??= () => undefined;
    Element.prototype.scrollIntoView ??= () => undefined;
    install();
  });

  afterEach(() => {
    cleanup();
    fetchMock.mockReset();
    vi.clearAllMocks();
    vi.unstubAllGlobals();
  });

  it("writes the search and Favorites only into the rule", async () => {
    const user = userEvent.setup();
    renderFields({});

    await user.type(screen.getByRole("searchbox", { name: "Search" }), "kyoto");
    expect(lastRule()?.q).toBe("kyoto");
    await user.click(screen.getByRole("checkbox", { name: "Favorites only" }));
    expect(lastRule()).toMatchObject({ q: "kyoto", favorite: true });
  });

  describe("Tag kind", () => {
    it("takes a kind from the Add tag picker, and another kind replaces it while tags stay", async () => {
      kindLists = [[kind(1, "Genre"), kind(2, "Mood")]];
      const user = userEvent.setup();
      renderFields({ tags: [1] });
      const input = screen.getByRole("combobox", { name: "Add tag" });

      await user.type(input, "Gen");
      await user.click(await screen.findByRole("option", { name: "Tag kind Genre" }));
      expect(lastRule()).toMatchObject({ tags: [1], tagKind: 1 });
      expect(await screen.findByText("Kind: Genre")).toBeDefined();

      await user.type(input, "Mo");
      await user.click(await screen.findByRole("option", { name: "Tag kind Mood" }));
      expect(lastRule()).toMatchObject({ tags: [1], tagKind: 2 });
      expect(screen.queryByText("Kind: Genre")).toBeNull();

      await user.click(screen.getByRole("button", { name: "Remove the kind Mood" }));
      expect(lastRule()?.tagKind).toBeUndefined();
    });

    it("shows a kind that no longer exists as a warning chip, which removes it", async () => {
      const user = userEvent.setup();
      renderFields({ tagKind: 9 }, [{ kind: "tagKind", id: 9 }]);
      expect(screen.getByText("Kind: deleted")).toBeDefined();
      await user.click(
        screen.getByRole("button", { name: "Remove the deleted condition" }),
      );
      expect(lastRule()?.tagKind).toBeUndefined();
    });

    it("開いた時点の保持に無い種類は取り直して確かめ、あれば削除扱いにしない", async () => {
      // Simulate a kind created in another tab after this tab cached Genre.
      kindLists = [[kind(1, "Genre")], [kind(1, "Genre"), kind(2, "Mood")]];
      await getTagKinds();
      renderFields({ tagKind: 2 });

      expect(await screen.findByText("Kind: Mood")).toBeDefined();
      expect(screen.queryByText("Kind: deleted")).toBeNull();
    });

    it("取り直しても無い種類は削除扱いにする", async () => {
      kindLists = [[kind(1, "Genre")]];
      await getTagKinds();
      renderFields({ tagKind: 2 });
      expect(await screen.findByText("Kind: deleted")).toBeDefined();
      const kindRequests = fetchMock.mock.calls.filter(
        ([input]) =>
          new URL(String(input), "http://localhost").pathname === "/api/tag-kinds",
      );
      expect(kindRequests).toHaveLength(2);
    });
  });

  describe("Tags", () => {
    it("候補から足し、チップの × で外し、最後の 1 つを外すと条件から消す", async () => {
      const user = userEvent.setup();
      renderFields({ tags: [1] });
      expect(
        await screen.findByRole("button", { name: "Remove the condition 猫" }),
      ).toBeDefined();

      await user.type(screen.getByRole("combobox", { name: "Add tag" }), "犬");
      await user.click(await screen.findByRole("option", { name: /犬/ }));
      expect(lastRule()?.tags).toEqual([1, 2]);

      await user.click(screen.getByRole("button", { name: "Remove the condition 猫" }));
      expect(lastRule()?.tags).toEqual([2]);
      await user.click(screen.getByRole("button", { name: "Remove the condition 犬" }));
      expect(lastRule()?.tags).toBeUndefined();
    });

    it("Esc で打ちかけの語を消す", async () => {
      const user = userEvent.setup();
      renderFields({});
      const input = screen.getByRole("combobox", { name: "Add tag" });
      await user.type(input, "犬");
      await user.keyboard("{Escape}{Escape}");
      expect((input as HTMLInputElement).value).toBe("");
    });

    it("16 個で入力を止めて上限を添える", async () => {
      renderFields({ tags: tags.slice(0, 16).map((tag) => tag.id) });
      expect(await screen.findByText("Up to 16 tags")).toBeDefined();
    });
  });

  describe("Person と Organization", () => {
    it("候補から人物を顔つきで選ぶとチップにし、× で外す", async () => {
      const alice = { ...credit(3, "Alice"), imageRevision: 2 };
      peopleAnswer = (q) => Promise.resolve(json({ items: q === "Ali" ? [alice] : [] }));
      const user = userEvent.setup();
      renderFields({});

      await user.type(screen.getByRole("combobox", { name: "Person" }), "Ali");
      const option = await screen.findByRole("option", { name: /Alice/ });
      expect(option.querySelector("img")?.getAttribute("src")).toBe(
        "/api/people/3/image?v=2",
      );
      await user.click(option);
      expect(lastRule()?.person).toBe(3);
      const remove = await screen.findByRole("button", {
        name: "Remove the person Alice",
      });
      await screen.findByText("Alice");
      expect(remove.parentElement?.querySelector("img")?.getAttribute("src")).toBe(
        "/api/people/3/image?v=2",
      );
      await user.click(remove);
      expect(lastRule()?.person).toBeUndefined();
    });

    it("語を変えたら、今の語への応答が届くまで前の語の候補を出さない", async () => {
      let releaseBob: (() => void) | undefined;
      peopleAnswer = (q) => {
        if (q === "A") return Promise.resolve(json({ items: [credit(3, "Alice")] }));
        return new Promise<Response>((resolve) => {
          releaseBob = () => resolve(json({ items: [credit(4, "Ab")] }));
        });
      };
      const user = userEvent.setup();
      renderFields({});
      const input = screen.getByRole("combobox", { name: "Person" });

      await user.type(input, "A");
      expect(await screen.findByRole("option", { name: /Alice/ })).toBeDefined();
      await user.type(input, "b");
      await waitFor(() => expect(releaseBob).toBeDefined());
      expect(screen.queryByRole("option", { name: /Alice/ })).toBeNull();

      await act(async () => {
        releaseBob?.();
        await Promise.resolve();
      });
      expect(await screen.findByRole("option", { name: /Ab/ })).toBeDefined();
    });

    it("候補を読めなければ候補を出さず、Esc で語を消す", async () => {
      peopleAnswer = () =>
        Promise.resolve(json({ code: "internal", message: "boom" }, 500));
      const user = userEvent.setup();
      renderFields({});
      const input = screen.getByRole("combobox", { name: "Person" });
      await user.type(input, "Ali");
      await waitFor(() =>
        expect(
          fetchMock.mock.calls.some(([request]) =>
            String(request).startsWith("/api/people?q=Ali"),
          ),
        ).toBe(true),
      );
      expect(screen.queryByRole("option", { name: /Ali/ })).toBeNull();
      await user.keyboard("{Escape}{Escape}");
      expect((input as HTMLInputElement).value).toBe("");
      expect(lastRule()).toBeUndefined();
    });

    it("もう無い人物や団体は警告のチップで出す", async () => {
      renderFields({ person: 3, organization: 5 }, [{ kind: "person", id: 3 }]);
      expect(await screen.findByText("Person: deleted")).toBeDefined();
      expect(await screen.findByText("Organization: deleted")).toBeDefined();
      expect(
        screen.getAllByRole("button", { name: "Remove the deleted condition" }),
      ).toHaveLength(2);
    });

    it("団体のチップの × で団体の条件を外す", async () => {
      const user = userEvent.setup();
      renderFields({ organization: 5 });
      const field = (await screen.findByText("Organization: deleted")).closest(
        "[data-slot=field]",
      ) as HTMLElement;
      await user.click(
        within(field).getByRole("button", { name: "Remove the deleted condition" }),
      );
      expect(lastRule()?.organization).toBeUndefined();
      expect(screen.getByRole("combobox", { name: "Organization" })).toBeDefined();
    });
  });
});
