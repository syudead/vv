import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactElement } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { RequestFailed } from "../api/client";
import type { Tag } from "../api/tags";
import { addTagSynonym, refreshTags } from "../api/tags";
import { t } from "../i18n";
import { TooltipProvider } from "../ui/Tooltip";
import SynonymsDialog from "./SynonymsDialog";

vi.mock("../api/tags", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../api/tags")>()),
  addTagSynonym: vi.fn(),
  refreshTags: vi.fn(),
}));

function tag(overrides: Partial<Tag> & { id: number; name: string }): Tag {
  return { synonyms: [], videoCount: 0, tentative: false, ...overrides };
}

const other = tag({ id: 2, name: "anime", videoCount: 10 });

/** dialog は毎回新しい関数を渡す。親の描画し直しと同じ形にする。 */
function dialog(current: Tag): ReactElement {
  return (
    <TooltipProvider>
      <SynonymsDialog
        tag={current}
        onClose={() => {}}
        onTagUpdated={() => {}}
        onSynonymRemoved={() => {}}
        onStale={() => {}}
      />
    </TooltipProvider>
  );
}

/**
 * failOnce は「anime」の登録から統合の確認を出し、確認の「統合する」を1回
 * 失敗させるところまで進める。クリックは fireEvent で送る（user-event の
 * クリックは自分で focus() を呼び、効果による focus() の回数が数えられなく
 * なるため）。
 */
async function failOnce() {
  const user = userEvent.setup();
  const view = render(dialog(tag({ id: 1, name: "Anime", videoCount: 2 })));
  const modal = await screen.findByRole("dialog", {
    name: t.tags.synonymsDialog.title("Anime"),
  });

  vi.mocked(addTagSynonym).mockRejectedValueOnce(
    new RequestFailed(409, "tag_merge_required", "merge required", { tagName: "anime" }),
  );
  vi.mocked(refreshTags).mockResolvedValue([
    tag({ id: 1, name: "Anime", videoCount: 2 }),
    other,
  ]);
  const input = within(modal).getByRole("textbox", { name: t.tags.synonymsDialog.add });
  await user.type(input, "anime");
  await user.keyboard("{Enter}");

  const backButton = await within(modal).findByRole("button", {
    name: t.tags.synonymsDialog.back,
  });
  const mergeButton = within(modal).getByRole("button", {
    name: t.tags.synonymsDialog.merge,
  });
  // 確認へ切り替わったときの最初のフォーカス（confirm の効果）は「戻る」。
  await waitFor(() => expect(document.activeElement).toBe(backButton));

  vi.mocked(addTagSynonym).mockRejectedValueOnce(new Error("failed"));
  const focus = vi.spyOn(mergeButton, "focus");
  const backFocus = vi.spyOn(backButton, "focus");
  fireEvent.click(mergeButton);
  expect(await within(modal).findByRole("alert")).toBeDefined();

  return { view, modal, mergeButton, backButton, focus, backFocus };
}

afterEach(() => {
  vi.restoreAllMocks();
  vi.mocked(addTagSynonym).mockReset();
  vi.mocked(refreshTags).mockReset();
});

describe("SynonymsDialog の統合の失敗後のフォーカス", () => {
  it("統合の確認が失敗すると、確認の「統合する」へ1回だけフォーカスを戻す", async () => {
    const { mergeButton, focus, backFocus } = await failOnce();

    await waitFor(() => expect(document.activeElement).toBe(mergeButton));
    expect(focus).toHaveBeenCalledTimes(1);
    expect(backFocus).not.toHaveBeenCalled();
  });

  it("失敗のあとに「戻る」へフォーカスを移すと、confirmError の変わらない描画し直しでは引き戻さない", async () => {
    const { view, mergeButton, backButton, focus } = await failOnce();
    await waitFor(() => expect(document.activeElement).toBe(mergeButton));

    backButton.focus();
    // 親が描画し直して、新しい tag（別のタブで増えたシノニムを含む）と
    // 新しい関数を渡してくる。
    view.rerender(dialog(tag({ id: 1, name: "Anime", videoCount: 2 })));
    view.rerender(
      dialog(tag({ id: 1, name: "Anime", videoCount: 2, synonyms: ["アニメ"] })),
    );

    expect(document.activeElement).toBe(backButton);
    expect(focus).toHaveBeenCalledTimes(1);
  });

  it("もう一度失敗すると、そのたびに1回だけ「統合する」へ戻す", async () => {
    const { mergeButton, backButton, focus } = await failOnce();
    await waitFor(() => expect(focus).toHaveBeenCalledTimes(1));

    backButton.focus();
    vi.mocked(addTagSynonym).mockRejectedValueOnce(new Error("failed again"));
    fireEvent.click(mergeButton);

    await waitFor(() => expect(document.activeElement).toBe(mergeButton));
    expect(focus).toHaveBeenCalledTimes(2);
  });
});
