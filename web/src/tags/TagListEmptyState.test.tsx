import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { t } from "../i18n";
import TagListEmptyState from "./TagListEmptyState";
import type { TagListEmpty } from "./tagListView";

function renderEmpty(empty: TagListEmpty, appliedTentative = false) {
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

  it("「Unused only」と「Tentative only」の両方では「0 本のタグ」の説明を出さない", () => {
    renderEmpty("noUnused", true);
    expect(screen.queryByText(t.tags.noUnused.description)).toBeNull();
  });
});
