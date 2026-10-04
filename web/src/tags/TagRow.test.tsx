import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactElement } from "react";
import { MemoryRouter } from "react-router";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { Tag } from "../api/tags";
import { t } from "../i18n";
import { TooltipProvider } from "../ui/Tooltip";
import TagRow from "./TagRow";

function tag(overrides: Partial<Tag> & { id: number; name: string }): Tag {
  return {
    synonyms: [],
    videoCount: 0,
    tentative: false,
    createdAt: "2026-01-01T00:00:00Z",
    ...overrides,
  };
}

/**
 * row は毎回新しい関数の props を渡して TagRow を作る。親（TagsPage）が
 * 描画し直すたびに、インラインの関数が作り直されるのと同じ形にする。
 */
function row(current: Tag, renaming: boolean): ReactElement {
  return (
    <MemoryRouter>
      <TooltipProvider>
        <TagRow
          tag={current}
          renaming={renaming}
          pending={false}
          blockStart={false}
          error={null}
          registerRefs={() => {}}
          onStartRename={() => {}}
          onCancelRename={() => {}}
          onSubmitRename={() => {}}
          onOpenSynonyms={() => {}}
          onOpenMerge={() => {}}
          onDelete={() => {}}
          onDraftChange={() => {}}
        />
      </TooltipProvider>
    </MemoryRouter>
  );
}

/** callsOn は spy のうち、this が element だった呼び出しの回数である。 */
function callsOn(spy: { mock: { contexts: unknown[] } }, element: Element): number {
  return spy.mock.contexts.filter((context) => context === element).length;
}

afterEach(() => {
  vi.restoreAllMocks();
});

describe("TagRow の改名を開く効果", () => {
  it("renaming が false から true になると、今の名前を入れてフォーカスし、全体を選ぶ", () => {
    const view = render(row(tag({ id: 1, name: "旅行" }), false));
    const focus = vi.spyOn(HTMLElement.prototype, "focus");
    const select = vi.spyOn(HTMLInputElement.prototype, "select");

    view.rerender(row(tag({ id: 1, name: "旅行" }), true));

    const input = screen.getByRole<HTMLInputElement>("textbox", {
      name: t.tags.row.renameLabel("旅行"),
    });
    expect(input.value).toBe("旅行");
    expect(document.activeElement).toBe(input);
    expect(input.selectionStart).toBe(0);
    expect(input.selectionEnd).toBe("旅行".length);
    expect(callsOn(focus, input)).toBe(1);
    expect(callsOn(select, input)).toBe(1);
  });

  it("改名中に名前や関数の props が変わっても、打っている途中の値を戻さず、選び直しもフォーカスし直しもしない", async () => {
    const user = userEvent.setup();
    const view = render(row(tag({ id: 1, name: "旅行" }), false));
    view.rerender(row(tag({ id: 1, name: "旅行" }), true));
    const input = screen.getByRole<HTMLInputElement>("textbox", {
      name: t.tags.row.renameLabel("旅行"),
    });

    // 全体が選ばれているので、打った文字で置き換わる。
    await user.keyboard("国内旅行");
    expect(input.value).toBe("国内旅行");

    const focus = vi.spyOn(HTMLElement.prototype, "focus");
    const select = vi.spyOn(HTMLInputElement.prototype, "select");

    // 別のタブの変更などで名前が変わった tag と、作り直した関数で描画し直す。
    view.rerender(row(tag({ id: 1, name: "旅" }), true));
    view.rerender(row(tag({ id: 1, name: "旅", videoCount: 5 }), true));

    const same = screen.getByRole<HTMLInputElement>("textbox", {
      name: t.tags.row.renameLabel("旅"),
    });
    expect(same).toBe(input);
    expect(input.value).toBe("国内旅行");
    expect(input.selectionStart).toBe("国内旅行".length);
    expect(input.selectionEnd).toBe("国内旅行".length);
    expect(callsOn(focus, input)).toBe(0);
    expect(callsOn(select, input)).toBe(0);

    // 打ち続けても、打った分がそのまま残る（1打ごとの描画でも戻さない）。
    await user.keyboard("へ");
    expect(input.value).toBe("国内旅行へ");
    expect(callsOn(select, input)).toBe(0);
  });

  it("改名を閉じて開き直すと、その時点の名前へ戻して選び直す", async () => {
    const user = userEvent.setup();
    const view = render(row(tag({ id: 1, name: "旅行" }), false));
    view.rerender(row(tag({ id: 1, name: "旅行" }), true));
    await user.keyboard("国内旅行");

    view.rerender(row(tag({ id: 1, name: "旅" }), false));
    expect(screen.queryByRole("textbox")).toBeNull();

    const focus = vi.spyOn(HTMLElement.prototype, "focus");
    const select = vi.spyOn(HTMLInputElement.prototype, "select");
    view.rerender(row(tag({ id: 1, name: "旅" }), true));

    const input = screen.getByRole<HTMLInputElement>("textbox", {
      name: t.tags.row.renameLabel("旅"),
    });
    expect(input.value).toBe("旅");
    expect(document.activeElement).toBe(input);
    // 選択の範囲は見ない。select() は前回の打ちかけ（国内旅行）が入ったまま
    // 呼ばれ、そのあとの描画で値が「旅」に替わると選択が末尾に畳まれる
    // （書き換え前の実装でも同じ）。ここでは効果が1回だけ走ったことを数で見る。
    expect(callsOn(focus, input)).toBe(1);
    expect(callsOn(select, input)).toBe(1);
  });
});
