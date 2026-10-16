import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { t } from "../i18n";
import TagListEmptyState from "./TagListEmptyState";
import type { TagKindFilter } from "./tagListUrl";
import type { TagListEmpty } from "./tagListView";

function renderEmpty(
  empty: TagListEmpty,
  appliedTentative = false,
  appliedKind: TagKindFilter = null,
  kindName: string | undefined = undefined,
) {
  const actions = {
    onCreate: vi.fn(),
    onShowAll: vi.fn(),
    onShowAllAndSearch: vi.fn(),
    onClearSearch: vi.fn(),
  };
  render(
    <TagListEmptyState
      empty={empty}
      appliedSearch="Gamma"
      appliedTentative={appliedTentative}
      appliedKind={appliedKind}
      kindName={kindName}
      {...actions}
    />,
  );
  return actions;
}

describe("TagListEmptyState", () => {
  it.each([
    ["tags", false, t.tags.empty.title, "onCreate"],
    ["noUnused", false, t.tags.noUnused.title, "onShowAll"],
    ["noUnused", true, t.tags.noUnusedTentative, "onShowAll"],
    ["noUnusedMatch", false, t.tags.noUnusedMatches("Gamma"), "onShowAllAndSearch"],
    [
      "noUnusedMatch",
      true,
      t.tags.noUnusedTentativeMatches("Gamma"),
      "onShowAllAndSearch",
    ],
    ["noTentative", false, t.tags.noTentative.title, "onShowAll"],
    ["noTentativeMatch", false, t.tags.noTentativeMatches("Gamma"), "onShowAllAndSearch"],
    ["noMatch", false, t.tags.noMatches("Gamma"), "onClearSearch"],
  ] as const)(
    "%s（仮の絞り込み %s）は題を見出しで出し、ボタンが対応する操作を呼ぶ",
    async (empty, appliedTentative, title, action) => {
      const user = userEvent.setup();
      const actions = renderEmpty(empty, appliedTentative);
      expect(screen.getByRole("heading", { name: title })).toBeDefined();
      await user.click(screen.getByRole("button"));
      for (const [name, fn] of Object.entries(actions)) {
        expect(fn).toHaveBeenCalledTimes(name === action ? 1 : 0);
      }
    },
  );

  it.each([
    ["1", "Genre", t.tags.noKindTags("Genre")],
    ["none", undefined, t.tags.noKindlessTags],
    ["1", undefined, t.tags.noFilterMatches],
  ] as const)(
    "種類だけの絞り込み（%s、名前 %s）は種類を名指しし、「Show all tags」で絞り込みを外す",
    async (appliedKind, kindName, title) => {
      const user = userEvent.setup();
      const actions = renderEmpty("noKind", false, appliedKind, kindName);
      expect(screen.getByRole("heading", { name: title })).toBeDefined();
      await user.click(screen.getByRole("button", { name: t.tags.clearSearch }));
      expect(actions.onShowAll).toHaveBeenCalledTimes(1);
    },
  );

  it("種類とほかの条件の組み合わせは 1 つの見出しで、検索と絞り込みをすべて外す", async () => {
    const user = userEvent.setup();
    const actions = renderEmpty("noKindMatch", true, "1", "Genre");
    expect(screen.getByRole("heading", { name: t.tags.noFilterMatches })).toBeDefined();
    await user.click(screen.getByRole("button", { name: t.tags.clearSearch }));
    expect(actions.onShowAllAndSearch).toHaveBeenCalledTimes(1);
  });

  it("「Unused only」と「Tentative only」の両方では「0 本のタグ」の説明を出さない", () => {
    renderEmpty("noUnused", true);
    expect(screen.queryByText(t.tags.noUnused.description)).toBeNull();
  });
});
