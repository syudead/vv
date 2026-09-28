import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactElement } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { Tag } from "../api/tags";
import { mergeTag } from "../api/tags";
import { t } from "../i18n";
import { TooltipProvider } from "../ui/Tooltip";
import MergeTagDialog from "./MergeTagDialog";

vi.mock("../api/tags", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../api/tags")>()),
  mergeTag: vi.fn(),
}));

function tag(overrides: Partial<Tag> & { id: number; name: string }): Tag {
  return { synonyms: [], videoCount: 0, ...overrides };
}

const source = tag({ id: 1, name: "旅行", videoCount: 2 });

/** dialog は毎回新しい配列と関数を渡す。親の描画し直しと同じ形にする。 */
function dialog(tags: readonly Tag[]): ReactElement {
  return (
    <TooltipProvider>
      <MergeTagDialog
        source={source}
        tags={tags}
        onClose={() => {}}
        onMerged={() => {}}
        onStale={() => {}}
      />
    </TooltipProvider>
  );
}

function initialTags(): Tag[] {
  return [
    source,
    tag({ id: 2, name: "Anime", videoCount: 3 }),
    tag({ id: 3, name: "Drama" }),
  ];
}

/**
 * failOnce は統合先に Anime を選び、「統合する」を1回失敗させるところまで
 * 進める。クリックは fireEvent で送る（user-event のクリックは自分で focus()
 * を呼び、効果による focus() の回数が数えられなくなるため）。
 */
async function failOnce() {
  const user = userEvent.setup();
  const view = render(dialog(initialTags()));
  const modal = await screen.findByRole("dialog", {
    name: t.tags.mergeDialog.title("旅行"),
  });
  const combo = within(modal).getByRole("combobox", { name: t.tags.mergeDialog.target });
  await user.type(combo, "Anime");
  await user.click(await within(modal).findByRole("option", { name: /Anime/ }));

  const mergeButton = within(modal).getByRole("button", {
    name: t.tags.mergeDialog.submit,
  });
  const cancelButton = within(modal).getByRole("button", { name: t.common.cancel });
  await waitFor(() => expect(document.activeElement).toBe(mergeButton));

  vi.mocked(mergeTag).mockRejectedValueOnce(new Error("failed"));
  const focus = vi.spyOn(mergeButton, "focus");
  fireEvent.click(mergeButton);
  expect(await within(modal).findByRole("alert")).toBeDefined();

  return { user, view, modal, combo, mergeButton, cancelButton, focus };
}

afterEach(() => {
  vi.restoreAllMocks();
  vi.mocked(mergeTag).mockReset();
});

describe("MergeTagDialog の失敗後のフォーカス", () => {
  it("統合が失敗すると、統合先を選んだままの「統合する」へ1回だけフォーカスを戻す", async () => {
    const { mergeButton, focus } = await failOnce();

    await waitFor(() => expect(document.activeElement).toBe(mergeButton));
    expect(focus).toHaveBeenCalledTimes(1);
  });

  it("失敗のあとにほかへフォーカスを移すと、error の変わらない描画し直しでは「統合する」へ引き戻さない", async () => {
    const { view, mergeButton, cancelButton, focus } = await failOnce();
    await waitFor(() => expect(document.activeElement).toBe(mergeButton));

    cancelButton.focus();
    // 親が描画し直して、新しいタグの一覧（別のタブで増えたタグを含む）と
    // 新しい関数を渡してくる。
    view.rerender(dialog(initialTags()));
    view.rerender(dialog([...initialTags(), tag({ id: 4, name: "Comedy" })]));

    expect(document.activeElement).toBe(cancelButton);
    expect(focus).toHaveBeenCalledTimes(1);
  });

  it("失敗のあとに統合先の入力を打ち直しても、フォーカスは入力に残る", async () => {
    const { user, combo, mergeButton, focus } = await failOnce();
    await waitFor(() => expect(document.activeElement).toBe(mergeButton));

    combo.focus();
    await user.keyboard("{Backspace}X");

    expect(document.activeElement).toBe(combo);
    expect(focus).toHaveBeenCalledTimes(1);
  });

  it("もう一度失敗すると、そのたびに1回だけ「統合する」へ戻す", async () => {
    const { mergeButton, cancelButton, focus } = await failOnce();
    await waitFor(() => expect(focus).toHaveBeenCalledTimes(1));

    cancelButton.focus();
    vi.mocked(mergeTag).mockRejectedValueOnce(new Error("failed again"));
    fireEvent.click(mergeButton);

    await waitFor(() => expect(document.activeElement).toBe(mergeButton));
    expect(focus).toHaveBeenCalledTimes(2);
  });
});
