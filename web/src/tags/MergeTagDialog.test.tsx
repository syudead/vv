import { act, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactElement } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { Tag, TagList, TagPageQuery } from "../api/tags";
import { listTagPage, mergeTag, tagImpact } from "../api/tags";
import { foldForMatch } from "../lib/foldForMatch";
import { errorText, t } from "../i18n";
import { TooltipProvider } from "../ui/Tooltip";
import MergeTagDialog from "./MergeTagDialog";

vi.mock("../api/tags", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../api/tags")>()),
  listTagPage: vi.fn(),
  mergeTag: vi.fn(),
  tagImpact: vi.fn(),
}));

function tag(overrides: Partial<Tag> & { id: number; name: string }): Tag {
  return {
    synonyms: [],
    videoCount: 0,
    tentative: false,
    createdAt: "2026-01-01T00:00:00Z",
    ...overrides,
  };
}

const source = tag({ id: 1, name: "旅行", videoCount: 2 });

/** serverTags は偽のサーバーが持つ全部のタグ（画面が読み込んでいないタグも含む）。 */
let serverTags: Tag[] = [];

/**
 * search はサーバーの `GET /api/tags?q=…&limit=…` を真似る。照合形で名前とシノニムに
 * 部分一致させ、名前の順のまま `limit` 件までを返す。前後の空白を落とした `q` と綴りが
 * 完全に一致するタグは、ページに入るかに関わらず `exact` に入れる。
 */
function search(query: TagPageQuery): TagList {
  const raw = (query.q ?? "").trim();
  const q = foldForMatch(query.q ?? "");
  const items = serverTags.filter(
    (item) =>
      q === "" ||
      foldForMatch(item.name).includes(q) ||
      item.synonyms.some((synonym) => foldForMatch(synonym).includes(q)),
  );
  const exact = serverTags.find(
    (item) => raw !== "" && (item.name === raw || item.synonyms.includes(raw)),
  );
  return {
    items: items.slice(0, query.limit ?? 100),
    total: items.length,
    totalAll: serverTags.length,
    ...(exact === undefined ? {} : { exact }),
  };
}

beforeEach(() => {
  serverTags = initialTags();
  vi.mocked(listTagPage).mockImplementation((query) => Promise.resolve(search(query)));
});

/** dialog は毎回新しい関数を渡す。親の描画し直しと同じ形にする。 */
function dialog(): ReactElement {
  return (
    <TooltipProvider>
      <MergeTagDialog
        sources={[source]}
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
  const view = render(dialog());
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
  vi.mocked(tagImpact).mockReset();
  vi.mocked(listTagPage).mockReset();
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
    // 親が描画し直して、新しい関数を渡してくる。
    view.rerender(dialog());
    view.rerender(dialog());

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

describe("MergeTagDialog の幅と候補の一覧", () => {
  it("行から開いた 1 件の統合でも、統合先の入力は窓の幅いっぱい（要件 13）", async () => {
    render(dialog());
    const modal = await screen.findByRole("dialog", {
      name: t.tags.mergeDialog.title("旅行"),
    });
    const combo = within(modal).getByRole("combobox", {
      name: t.tags.mergeDialog.target,
    });
    expect(combo.parentElement!.className).toContain("w-full");
  });

  it("入力には見える名札と検索のアイコンがあり、候補は窓の本文の中の高さの決まった箱に並ぶ", async () => {
    render(dialog());
    const modal = await screen.findByRole("dialog");
    const combo = within(modal).getByRole("combobox");
    // 見える名札（label 要素）が入力を指す。
    const label = within(modal).getByText(t.tags.mergeDialog.target, {
      selector: "label",
    });
    expect(label.getAttribute("for")).toBe(combo.id);
    expect(combo.parentElement!.querySelector("svg.lucide-search")).not.toBeNull();

    // 入力に触れる前から一覧は開いていて、入力の上に重ねない（absolute ではない）。
    await within(modal).findByRole("option", { name: /Anime/ });
    const listbox = within(modal).getByRole("listbox");
    expect(combo.getAttribute("aria-expanded")).toBe("true");
    expect(listbox.className).not.toContain("absolute");
    // 一覧の箱は高さが決まっていて、その中を縦にスクロールする。
    const box = listbox.parentElement!;
    expect(box.className).toContain("h-combobox-panel");
    expect(box.className).toContain("overflow-y-auto");
    // 箱は窓の下端（ボタンの行）より前、本文の中にある。
    const footer = within(modal).getByRole("button", {
      name: t.common.cancel,
    }).parentElement!;
    expect(
      box.compareDocumentPosition(footer) & Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();
    // 低い画面では、フォームダイアログ（FormDialog）の窓全体が縦にスクロールし、
    // ボタンの行まで届く。
    expect(modal.className).toContain("overflow-y-auto");
    expect(modal.className).toContain("max-h-full");
    expect(modal.contains(footer)).toBe(true);
  });

  it("選んだ統合先は一覧で目立たせ、「統合元 → 統合先」を出す。「Merge」は primary で、選ぶまで押せない", async () => {
    const user = userEvent.setup();
    render(dialog());
    const modal = await screen.findByRole("dialog");
    const mergeButton = within(modal).getByRole("button", {
      name: t.tags.mergeDialog.submit,
    });
    expect(mergeButton).toHaveProperty("disabled", true);
    expect(mergeButton.className).toContain("bg-primary");
    expect(mergeButton.className).not.toContain("bg-danger");
    expect(within(modal).queryByText("→")).toBeNull();

    await user.click(await within(modal).findByRole("option", { name: /Anime/ }));
    const chosen = await within(modal).findByRole("option", { name: /Anime/ });
    expect(chosen.getAttribute("data-chosen")).toBe("true");
    expect(chosen.className).toContain("bg-primary-soft");
    // 「統合元 → 統合先」は確認の文言の後、ボタンの行の前に出す。
    expect(modal.textContent).toContain("旅行 → Anime");
    await waitFor(() => expect(mergeButton).toHaveProperty("disabled", false));
  });

  it("候補が無いときは一覧の中に「No matching tags」を出す", async () => {
    const user = userEvent.setup();
    render(dialog());
    const modal = await screen.findByRole("dialog");
    await user.type(within(modal).getByRole("combobox"), "zzz");
    expect(await within(modal).findByText(t.tags.mergeDialog.noCandidates)).toBeDefined();
    expect(within(modal).queryByRole("option")).toBeNull();
  });
});

describe("MergeTagDialog の数え直し", () => {
  it("選択から開いた窓で同じ統合先を選び直しても、数え直す（ui-design.md「Confirmation」）", async () => {
    const user = userEvent.setup();
    vi.mocked(tagImpact)
      .mockResolvedValueOnce({ tagCount: 2, videoCount: 5 })
      .mockResolvedValueOnce({ tagCount: 2, videoCount: 7 });
    const sources = [source, tag({ id: 3, name: "Drama", videoCount: 4 })];
    render(
      <TooltipProvider>
        <MergeTagDialog
          sources={sources}
          fromSelection
          onClose={() => {}}
          onMerged={() => {}}
          onStale={() => {}}
        />
      </TooltipProvider>,
    );
    const modal = await screen.findByRole("dialog", {
      name: t.tags.mergeDialog.titleMany(2),
    });
    const combo = within(modal).getByRole("combobox", {
      name: t.tags.mergeDialog.target,
    });
    await user.type(combo, "Anime");
    await user.click(await within(modal).findByRole("option", { name: /Anime/ }));
    expect(
      await within(modal).findByText(t.tags.mergeDialog.warningMany(2, 5, "Anime")),
    ).toBeDefined();

    await user.click(combo);
    await user.keyboard("{ArrowDown}");
    await user.click(await within(modal).findByRole("option", { name: /Anime/ }));

    expect(
      await within(modal).findByText(t.tags.mergeDialog.warningMany(2, 7, "Anime")),
    ).toBeDefined();
    expect(vi.mocked(tagImpact)).toHaveBeenCalledTimes(2);
    expect(vi.mocked(tagImpact)).toHaveBeenLastCalledWith(
      "merge",
      [1, 3],
      expect.any(AbortSignal),
    );
  });
});

describe("MergeTagDialog の統合先の候補（サーバーの検索）", () => {
  function selectionDialog(sources: readonly Tag[]): ReactElement {
    return (
      <TooltipProvider>
        <MergeTagDialog
          sources={sources}
          fromSelection
          onClose={() => {}}
          onMerged={() => {}}
          onStale={() => {}}
        />
      </TooltipProvider>
    );
  }

  function optionNames(modal: HTMLElement): string[] {
    return within(modal)
      .getAllByRole("option")
      .map((option) => option.textContent ?? "");
  }

  it("行から開くと空の q と、統合元の 1 件を足した limit=9 で引き、応答から統合元を除いて並べる", async () => {
    const user = userEvent.setup();
    render(dialog());
    const modal = await screen.findByRole("dialog");
    await waitFor(() =>
      expect(vi.mocked(listTagPage)).toHaveBeenCalledWith(
        { q: "", limit: 9 },
        expect.any(AbortSignal),
      ),
    );
    await user.click(within(modal).getByRole("combobox"));
    await within(modal).findByRole("listbox");
    const names = optionNames(modal);
    expect(names).toHaveLength(2);
    expect(names[0]).toContain("Anime");
    expect(names[1]).toContain("Drama");
  });

  it("選んだ中から開くと、選んだタグも候補に残す", async () => {
    const user = userEvent.setup();
    const drama = tag({ id: 3, name: "Drama", videoCount: 4 });
    render(selectionDialog([source, drama]));
    const modal = await screen.findByRole("dialog");
    await waitFor(() => expect(vi.mocked(listTagPage)).toHaveBeenCalled());
    await user.click(within(modal).getByRole("combobox"));
    await within(modal).findByRole("listbox");
    const names = optionNames(modal);
    expect(names).toHaveLength(3);
    expect(names.some((name) => name.includes("旅行"))).toBe(true);
    expect(names.some((name) => name.includes("Drama"))).toBe(true);
  });

  it("「ａｃｔ」は q=ａｃｔ で送り、読み込んでいないタグ Action が候補に出る（要件 9）", async () => {
    const user = userEvent.setup();
    serverTags = [...initialTags(), tag({ id: 9, name: "Action", videoCount: 5 })];
    render(dialog());
    const modal = await screen.findByRole("dialog");
    await user.type(within(modal).getByRole("combobox"), "ａｃｔ");
    expect(await within(modal).findByRole("option", { name: /Action/ })).toBeDefined();
    expect(vi.mocked(listTagPage)).toHaveBeenLastCalledWith(
      { q: "ａｃｔ", limit: 9 },
      expect.any(AbortSignal),
    );
  });

  it("行から開いて応答に統合元が入っても、候補は 8 件ある", async () => {
    const user = userEvent.setup();
    serverTags = [
      source,
      ...Array.from({ length: 10 }, (_, index) =>
        tag({ id: 10 + index, name: `Tag ${String(index).padStart(2, "0")}` }),
      ),
    ];
    render(dialog());
    const modal = await screen.findByRole("dialog");
    await user.click(within(modal).getByRole("combobox"));
    await within(modal).findByRole("option", { name: /^Tag 00/ });
    const names = optionNames(modal);
    expect(names).toHaveLength(8);
    expect(names.some((name) => name.includes("旅行"))).toBe(false);
  });

  it("綴りが完全に一致するタグは、部分一致の上位 8 件に入らなくても先頭に出て Enter で選べる", async () => {
    const user = userEvent.setup();
    // 名前の順では「cat」より前に、「cat」を含む名前が 8 件以上並ぶ。
    const longer = Array.from({ length: 10 }, (_, index) =>
      tag({ id: 10 + index, name: `a${String(index).padStart(2, "0")}cat` }),
    );
    const cat = tag({ id: 30, name: "cat", videoCount: 6 });
    serverTags = [source, ...longer, cat];
    render(dialog());
    const modal = await screen.findByRole("dialog");
    const combo = within(modal).getByRole("combobox");
    await user.type(combo, "cat");
    await waitFor(() => expect(combo.getAttribute("aria-busy")).toBeNull());
    const names = optionNames(modal);
    expect(names).toHaveLength(8);
    expect(names[0]).toContain("cat");
    expect(names[0]).not.toContain("a00cat");
    expect(names[1]).toContain("a00cat");

    await user.keyboard("{Enter}");
    expect(combo).toHaveProperty("value", "cat");
  });

  it("シノニムで当たった候補には補足を添え、入力と一致するシノニムは exactOption になる", async () => {
    const user = userEvent.setup();
    serverTags = [
      ...initialTags(),
      tag({ id: 9, name: "Action", synonyms: ["アクション"] }),
    ];
    render(dialog());
    const modal = await screen.findByRole("dialog");
    const combo = within(modal).getByRole("combobox");
    await user.type(combo, "あくしょん");
    const option = await within(modal).findByRole("option", { name: /Action/ });
    expect(option.textContent).toContain(t.tags.mergeDialog.synonymHint("アクション"));

    await user.clear(combo);
    await user.type(combo, "アクション");
    await within(modal).findByRole("option", { name: /Action/ });
    await waitFor(() => expect(combo.getAttribute("aria-busy")).toBeNull());
    await user.keyboard("{Enter}");
    expect(combo).toHaveProperty("value", "Action");
  });

  it("入力を続けて変えると前の要求を打ち切り、最後の応答だけを候補にする", async () => {
    const user = userEvent.setup();
    serverTags = [
      ...initialTags(),
      tag({ id: 7, name: "Ab" }),
      tag({ id: 8, name: "Abc" }),
    ];
    const pending: { signal: AbortSignal; resolve: () => void }[] = [];
    vi.mocked(listTagPage).mockImplementation(
      (query, signal) =>
        new Promise((resolve) => {
          pending.push({ signal: signal!, resolve: () => resolve(search(query)) });
        }),
    );
    render(dialog());
    const modal = await screen.findByRole("dialog");
    const combo = within(modal).getByRole("combobox");
    await user.type(combo, "Ab");
    await waitFor(() => expect(pending).toHaveLength(3));
    expect(pending[0]!.signal.aborted).toBe(true);
    expect(pending[1]!.signal.aborted).toBe(true);
    expect(pending[2]!.signal.aborted).toBe(false);
    expect(combo.getAttribute("aria-busy")).toBe("true");

    // 打ち切った要求の応答が後から届いても候補にしない。
    await act(async () => {
      pending[0]!.resolve();
      pending[1]!.resolve();
      await Promise.resolve();
    });
    expect(within(modal).queryByRole("option")).toBeNull();
    expect(combo.getAttribute("aria-busy")).toBe("true");

    await act(async () => {
      pending[2]!.resolve();
      await Promise.resolve();
    });
    await within(modal).findByRole("option", { name: /^Abc/ });
    expect(optionNames(modal)).toHaveLength(2);
    expect(combo.getAttribute("aria-busy")).toBeNull();
  });

  it("引けなかったときは入力の下に理由を出し、最後に届いた候補は残す", async () => {
    const user = userEvent.setup();
    render(dialog());
    const modal = await screen.findByRole("dialog");
    const combo = within(modal).getByRole("combobox");
    await user.click(combo);
    await within(modal).findByRole("option", { name: /Anime/ });

    const offline = new Error("offline");
    const failureText = t.tags.mergeDialog.searchFailed(errorText(offline));
    vi.mocked(listTagPage).mockRejectedValueOnce(offline);
    await user.type(combo, "D");
    const failure = await within(modal).findByText(failureText);
    expect(failure.className).toContain("text-destructive");
    expect(combo.getAttribute("aria-describedby")).toBe(failure.id);
    expect(within(modal).getByRole("option", { name: /Anime/ })).toBeDefined();
    expect(within(modal).queryByRole("button", { name: t.common.retry })).toBeNull();

    await user.type(combo, "r");
    await waitFor(() => expect(optionNames(modal)).toHaveLength(1));
    expect(optionNames(modal)[0]).toContain("Drama");
    expect(within(modal).queryByText(failureText)).toBeNull();
  });

  it("選んだ中から統合先を選ぶと sourceIds から外し、統合元が統合先だけなら押せない", async () => {
    const user = userEvent.setup();
    const drama = tag({ id: 3, name: "Drama", videoCount: 4 });
    vi.mocked(tagImpact).mockResolvedValue({ tagCount: 1, videoCount: 2 });
    vi.mocked(mergeTag).mockResolvedValue({ tag: drama, notFoundIds: [] });
    const view = render(selectionDialog([source, drama]));
    let modal = await screen.findByRole("dialog");
    await user.type(within(modal).getByRole("combobox"), "Drama");
    await user.click(await within(modal).findByRole("option", { name: /Drama/ }));
    const mergeButton = within(modal).getByRole("button", {
      name: t.tags.mergeDialog.submit,
    });
    await waitFor(() => expect(mergeButton).toHaveProperty("disabled", false));
    fireEvent.click(mergeButton);
    await waitFor(() => expect(vi.mocked(mergeTag)).toHaveBeenCalledWith(3, [1]));
    view.unmount();

    render(selectionDialog([drama]));
    modal = await screen.findByRole("dialog");
    await user.type(within(modal).getByRole("combobox"), "Drama");
    await user.click(await within(modal).findByRole("option", { name: /Drama/ }));
    expect(within(modal).getByText(t.tags.mergeDialog.onlyTarget("Drama"))).toBeDefined();
    expect(
      within(modal).getByRole("button", { name: t.tags.mergeDialog.submit }),
    ).toHaveProperty("disabled", true);
  });
});
