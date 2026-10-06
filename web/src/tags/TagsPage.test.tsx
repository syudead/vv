import { fireEvent, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { __resetTagsForTest, getTags, subscribeTags } from "../api/tags";
import { enablePseudoLocale, expectCatalogTextOnly } from "../i18n/pseudo";
import {
  jsonResponse,
  tag,
  server,
  heldGetReleases,
  hold,
  install,
  renderPage,
  setUpTagsPageServer,
} from "../testing/tagsPage";

setUpTagsPageServer();

describe("TagsPage", () => {
  it("一覧を本数つきで出し、件数の行に総数が出る", async () => {
    install();
    renderPage();

    expect(await screen.findByTitle("旅行")).toBeDefined();
    expect(screen.getByTitle("Anime")).toBeDefined();
    expect(screen.getByText("Synonyms: アニメ")).toBeDefined();
    expect(screen.getByText("3 tags")).toBeDefined();
    // 本数 0 のタグも出る。
    const dramaRow = screen.getByTitle("Drama").closest<HTMLElement>("[data-tag-id]")!;
    expect(dramaRow.textContent).toContain("0 videos");
  });

  it("検索は名前とシノニムのどちらでも、大文字小文字を区別せず絞り込む（受け入れ条件18）", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("旅行");

    const search = screen.getByRole("searchbox", { name: "Search tags" });
    await user.type(search, "アニ");
    expect(await screen.findByText("1 of 3 tags")).toBeDefined();
    expect(screen.getByTitle("Anime")).toBeDefined();
    expect(screen.queryByTitle("旅行")).toBeNull();
    expect(screen.queryByTitle("Drama")).toBeNull();

    await user.clear(search);
    await user.type(search, "ani");
    expect(await screen.findByText("1 of 3 tags")).toBeDefined();
    expect(screen.getByTitle("Anime")).toBeDefined();

    await user.clear(search);
    expect(await screen.findByText("3 tags")).toBeDefined();
    expect(screen.getByTitle("旅行")).toBeDefined();
  });

  it("検索はライブラリと同じ照合形で、全角・半角やかなの違いを同じものとして照らす（受け入れ条件8）", async () => {
    const user = userEvent.setup();
    server.tags = [
      tag({ id: 1, name: "action", synonyms: [] }),
      tag({ id: 2, name: "Anime", synonyms: ["アニメ"] }),
      tag({ id: 3, name: "旅行" }),
    ];
    install();
    renderPage();
    await screen.findByTitle("旅行");

    const search = screen.getByRole("searchbox", { name: "Search tags" });
    await user.type(search, "ＡＣＴＩＯＮ");
    expect(await screen.findByText("1 of 3 tags")).toBeDefined();
    expect(screen.getByTitle("action")).toBeDefined();
    expect(screen.queryByTitle("Anime")).toBeNull();

    // シノニムも照合形で照らす（ひらがなで打ってもカタカナのシノニムに当たる）。
    await user.clear(search);
    await user.type(search, "あにめ");
    expect(await screen.findByText("1 of 3 tags")).toBeDefined();
    expect(screen.getByTitle("Anime")).toBeDefined();
    expect(screen.queryByTitle("action")).toBeNull();

    await user.clear(search);
    await user.type(search, "ｱｸｼｮﾝ");
    expect(await screen.findByText('No tags match "ｱｸｼｮﾝ"')).toBeDefined();
  });

  it("検索で一致が無いときは、タグが無い状態と別の表示になり、そこから入力を消せる", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("旅行");

    const search = screen.getByRole("searchbox", { name: "Search tags" });
    await user.type(search, "存在しない語");

    expect(await screen.findByText('No tags match "存在しない語"')).toBeDefined();
    expect(screen.queryByText("No tags yet")).toBeNull();

    await user.click(screen.getByRole("button", { name: "Show all tags" }));
    expect(document.activeElement).toBe(search);
    expect(await screen.findByTitle("旅行")).toBeDefined();
  });

  it("タグが1つも無いときは空の状態を出し、検索の入力はdisabledのまま", async () => {
    server.tags = [];
    install();
    renderPage();

    expect(await screen.findByText("No tags yet")).toBeDefined();
    expect(
      screen.getByRole("searchbox", { name: "Search tags" }).hasAttribute("disabled"),
    ).toBe(true);
  });

  it("新しいタグを作ると本数0で名前順の位置に入り、名前へフォーカスが移る（受け入れ条件9）", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("旅行");

    await user.click(screen.getByRole("button", { name: "New tag" }));
    const input = screen.getByRole("textbox", { name: "New tag name" });
    expect(document.activeElement).toBe(input);
    await user.type(input, "Banana");
    await user.keyboard("{Enter}");

    const created = await screen.findByRole("link", {
      name: "Open the library filtered by Banana",
    });
    await waitFor(() => expect(document.activeElement).toBe(created));
    expect(screen.getByText("4 tags")).toBeDefined();
  });

  it("既存の名前と重なる作成は理由を出し、入力を残す", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("旅行");

    await user.click(screen.getByRole("button", { name: "New tag" }));
    const input = screen.getByRole("textbox", { name: "New tag name" });
    await user.type(input, "旅行");
    await user.keyboard("{Enter}");

    expect(await screen.findByText('A tag named "旅行" already exists.')).toBeDefined();
    expect((input as HTMLInputElement).value).toBe("旅行");
  });

  it("改名すると新しい名前で表示され、既存名との重なりは理由を出す（受け入れ条件10）", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("旅行");

    const row = screen.getByTitle("旅行").closest<HTMLElement>("[data-tag-id]")!;
    await user.click(within(row).getByRole("button", { name: "Rename" }));

    const input = await screen.findByRole("textbox", { name: 'New name for "旅行"' });
    expect(document.activeElement).toBe(input);

    // 既存のシノニムと重なる改名は拒否され、理由が画面に出る。
    await user.clear(input);
    await user.type(input, "アニメ");
    await user.keyboard("{Enter}");
    expect(
      await screen.findByText('"アニメ" is already a synonym of the tag "Anime".'),
    ).toBeDefined();

    await user.clear(input);
    await user.type(input, "旅行2024");
    await user.keyboard("{Enter}");

    expect(await screen.findByTitle("旅行2024")).toBeDefined();
    expect(screen.queryByTitle("旅行")).toBeNull();
  });

  it("改名中にEscを押すと、何も変えずに戻り、フォーカスが「改名」へ戻る（N3）", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("旅行");

    const row = screen.getByTitle("旅行").closest<HTMLElement>("[data-tag-id]")!;
    const renameButton = within(row).getByRole("button", { name: "Rename" });
    await user.click(renameButton);
    const input = await screen.findByRole("textbox", { name: 'New name for "旅行"' });
    await user.clear(input);
    await user.type(input, "捨てる名前");
    await user.keyboard("{Escape}");

    expect(await screen.findByTitle("旅行")).toBeDefined();
    expect(screen.queryByTitle("捨てる名前")).toBeNull();
    await waitFor(() => expect(document.activeElement).toBe(renameButton));
  });

  it("作成中にEscを押すと、何も作らずフォーカスが「新しいタグ」へ戻る（N3）", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("旅行");

    const createButton = screen.getByRole("button", { name: "New tag" });
    await user.click(createButton);
    const input = screen.getByRole("textbox", { name: "New tag name" });
    await user.type(input, "捨てる名前");
    await user.keyboard("{Escape}");

    expect(screen.queryByRole("textbox", { name: "New tag name" })).toBeNull();
    expect(document.activeElement).toBe(createButton);
  });

  it("別のタブで消したタグを改名しようとすると、もう無いことが伝わり一覧が取り直される", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("旅行");

    const row = screen.getByTitle("旅行").closest<HTMLElement>("[data-tag-id]")!;
    await user.click(within(row).getByRole("button", { name: "Rename" }));
    const input = await screen.findByRole("textbox", { name: 'New name for "旅行"' });

    // 改名を試みる前に、別のタブで削除されたことにする。
    server.tags = server.tags.filter((t) => t.name !== "旅行");

    await user.clear(input);
    await user.type(input, "旅行2024");
    await user.keyboard("{Enter}");

    expect(
      await screen.findByText("This tag no longer exists, so the list was reloaded"),
    ).toBeDefined();
    expect(screen.queryByTitle("旅行")).toBeNull();
    expect(screen.getByText("2 tags")).toBeDefined();
  });

  it("削除の確認は外れる本数を示し、確定すると消えて次の行の改名へフォーカスが移る（受け入れ条件11）", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Anime");

    const row = screen.getByTitle("Anime").closest<HTMLElement>("[data-tag-id]")!;
    await user.click(within(row).getByRole("button", { name: "More actions" }));
    await user.click(await screen.findByRole("menuitem", { name: "Delete…" }));

    const dialog = await screen.findByRole("alertdialog", { name: 'Delete "Anime"' });
    expect(
      within(dialog).getByText(
        "This tag will be removed from 3 videos. This can't be undone.",
      ),
    ).toBeDefined();

    await user.click(within(dialog).getByRole("button", { name: "Delete" }));

    await waitFor(() => expect(screen.queryByTitle("Anime")).toBeNull());
    expect(await screen.findByText('Deleted "Anime"')).toBeDefined();
    // Anime の次は Drama（自然順で 旅行 < Anime < Drama）。
    await waitFor(() =>
      expect(document.activeElement).toBe(
        within(
          screen.getByTitle("Drama").closest<HTMLElement>("[data-tag-id]")!,
        ).getByRole("button", { name: "Rename" }),
      ),
    );
  });

  it("0本のタグの削除確認は「どの動画にも付いていません」と出す", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Drama");

    const row = screen.getByTitle("Drama").closest<HTMLElement>("[data-tag-id]")!;
    await user.click(within(row).getByRole("button", { name: "More actions" }));
    await user.click(await screen.findByRole("menuitem", { name: "Delete…" }));

    const dialog = await screen.findByRole("alertdialog", { name: 'Delete "Drama"' });
    expect(within(dialog).getByText("This tag isn't on any videos.")).toBeDefined();
  });

  it("削除の確認でEscを押すと何も変わらず、フォーカスがその行の「その他の操作」へ戻る（B2）", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Drama");

    const row = screen.getByTitle("Drama").closest<HTMLElement>("[data-tag-id]")!;
    const menuButton = within(row).getByRole("button", { name: "More actions" });
    await user.click(menuButton);
    await user.click(await screen.findByRole("menuitem", { name: "Delete…" }));
    await screen.findByRole("alertdialog", { name: 'Delete "Drama"' });

    await user.keyboard("{Escape}");

    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(screen.getByTitle("Drama")).toBeDefined();
    // その他の操作のメニュー項目（削除…）はメニューが閉じるともう無いので、
    // ModalFrame の「前のフォーカスへ戻す」だけには頼らず、その行の
    // 「その他の操作」へ明示的に戻す。
    await waitFor(() => expect(document.activeElement).toBe(menuButton));
  });

  it("削除の確認の「キャンセル」を押しても、フォーカスがその行の「その他の操作」へ戻る（B2）", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Drama");

    const row = screen.getByTitle("Drama").closest<HTMLElement>("[data-tag-id]")!;
    const menuButton = within(row).getByRole("button", { name: "More actions" });
    await user.click(menuButton);
    await user.click(await screen.findByRole("menuitem", { name: "Delete…" }));
    const dialog = await screen.findByRole("alertdialog", { name: 'Delete "Drama"' });

    await user.click(within(dialog).getByRole("button", { name: "Cancel" }));

    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    await waitFor(() => expect(document.activeElement).toBe(menuButton));
  });

  it("別のタブで消したタグの削除を試みると、もう無いことが伝わり次の行の改名へフォーカスが移る（B2）", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Anime");

    const row = screen.getByTitle("Anime").closest<HTMLElement>("[data-tag-id]")!;
    await user.click(within(row).getByRole("button", { name: "More actions" }));
    await user.click(await screen.findByRole("menuitem", { name: "Delete…" }));
    const dialog = await screen.findByRole("alertdialog", { name: 'Delete "Anime"' });

    // 確認を開いたあと、別のタブで先に削除されたことにする。
    server.tags = server.tags.filter((t) => t.name !== "Anime");

    await user.click(within(dialog).getByRole("button", { name: "Delete" }));

    expect(
      await screen.findByText("This tag no longer exists, so the list was reloaded"),
    ).toBeDefined();
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    // Anime の次は Drama（自然順で 旅行 < Anime < Drama）。
    await waitFor(() =>
      expect(document.activeElement).toBe(
        within(
          screen.getByTitle("Drama").closest<HTMLElement>("[data-tag-id]")!,
        ).getByRole("button", { name: "Rename" }),
      ),
    );
  });

  it("キーボードだけで検索の入力に届く（本文で最初のTab）", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("旅行");

    await user.tab();
    expect(document.activeElement).toBe(
      screen.getByRole("searchbox", { name: "Search tags" }),
    );
  });

  it("読み込みに失敗すると再試行でき、成功すると一覧が出る", async () => {
    const fetchMock = vi.fn<typeof fetch>();
    let first = true;
    fetchMock.mockImplementation((input) => {
      const url = new URL(String(input), "http://localhost");
      if (url.pathname === "/api/tags" && first) {
        first = false;
        return Promise.resolve(
          jsonResponse({ code: "internal", message: "failed" }, 500),
        );
      }
      return Promise.resolve(jsonResponse({ items: server.tags }));
    });
    vi.stubGlobal("fetch", fetchMock);

    renderPage();
    expect(await screen.findByText("Couldn't load the tags")).toBeDefined();

    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: "Retry" }));
    expect(await screen.findByTitle("旅行")).toBeDefined();
  });

  it("IME変換中のEnterとEscは、作成・改名・検索のどの入力でも無視される（B1）", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("旅行");

    // 作成の入力。
    await user.click(screen.getByRole("button", { name: "New tag" }));
    const createInput = screen.getByRole("textbox", { name: "New tag name" });
    await user.type(createInput, "IME作成中");
    fireEvent.keyDown(createInput, { key: "Enter", keyCode: 229 });
    expect(screen.queryByTitle("IME作成中")).toBeNull();
    fireEvent.keyDown(createInput, { key: "Escape", keyCode: 229 });
    expect(screen.getByRole("textbox", { name: "New tag name" })).toBeDefined();
    fireEvent.keyDown(createInput, { key: "Enter" });
    expect(await screen.findByTitle("IME作成中")).toBeDefined();

    // 改名の入力。
    const row = screen.getByTitle("旅行").closest<HTMLElement>("[data-tag-id]")!;
    await user.click(within(row).getByRole("button", { name: "Rename" }));
    const renameInput = await screen.findByRole("textbox", {
      name: 'New name for "旅行"',
    });
    await user.clear(renameInput);
    await user.type(renameInput, "旅行2024");
    fireEvent.keyDown(renameInput, { key: "Enter", keyCode: 229 });
    expect(screen.queryByTitle("旅行2024")).toBeNull();
    fireEvent.keyDown(renameInput, { key: "Escape", keyCode: 229 });
    expect(screen.getByRole("textbox", { name: 'New name for "旅行"' })).toBeDefined();
    fireEvent.keyDown(renameInput, { key: "Enter" });
    expect(await screen.findByTitle("旅行2024")).toBeDefined();

    // 検索の入力。
    const search = screen.getByRole("searchbox", { name: "Search tags" });
    await user.type(search, "存在しない語");
    fireEvent.keyDown(search, { key: "Escape", keyCode: 229 });
    expect((search as HTMLInputElement).value).toBe("存在しない語");
    fireEvent.keyDown(search, { key: "Escape" });
    expect((search as HTMLInputElement).value).toBe("");
  });

  it("作成中はaria-busyだけを付けて入力をdisabledにせず、tag_name_taken後もフォーカスと値が残る（B3）", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("旅行");

    await user.click(screen.getByRole("button", { name: "New tag" }));
    const input = screen.getByRole("textbox", { name: "New tag name" });
    await user.type(input, "旅行");

    hold.nextMutation = true;
    await user.keyboard("{Enter}");

    await waitFor(() => expect(input.getAttribute("aria-busy")).toBe("true"));
    expect(input.hasAttribute("disabled")).toBe(false);
    expect(document.activeElement).toBe(input);

    hold.release?.();

    expect(await screen.findByText('A tag named "旅行" already exists.')).toBeDefined();
    expect(document.activeElement).toBe(input);
    expect((input as HTMLInputElement).value).toBe("旅行");
    await waitFor(() => expect(input.getAttribute("aria-busy")).toBeNull());
  });

  it("改名中はaria-busyだけを付けて入力をdisabledにせず、tag_name_taken後もフォーカスと値が残る（B3）", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("旅行");

    const row = screen.getByTitle("旅行").closest<HTMLElement>("[data-tag-id]")!;
    await user.click(within(row).getByRole("button", { name: "Rename" }));
    const input = await screen.findByRole("textbox", { name: 'New name for "旅行"' });
    await user.clear(input);
    await user.type(input, "アニメ");

    hold.nextMutation = true;
    await user.keyboard("{Enter}");

    await waitFor(() => expect(input.getAttribute("aria-busy")).toBe("true"));
    expect(input.hasAttribute("disabled")).toBe(false);
    expect(document.activeElement).toBe(input);

    hold.release?.();

    expect(
      await screen.findByText('"アニメ" is already a synonym of the tag "Anime".'),
    ).toBeDefined();
    expect(document.activeElement).toBe(input);
    expect((input as HTMLInputElement).value).toBe("アニメ");
    await waitFor(() => expect(input.getAttribute("aria-busy")).toBeNull());
  });

  it("改名中も本数・改名・その他の操作は出したままで、同じ行の要素のままになる（B4）", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Anime");

    const row = screen.getByTitle("Anime").closest<HTMLElement>("[data-tag-id]")!;
    await user.click(within(row).getByRole("button", { name: "Rename" }));

    const input = await screen.findByRole("textbox", { name: 'New name for "Anime"' });
    expect(input.closest("[data-tag-id]")).toBe(row);
    expect(row.textContent).toContain("3 videos");
    expect(within(row).getByRole("button", { name: "Rename" })).toBeDefined();
    expect(within(row).getByRole("button", { name: "More actions" })).toBeDefined();
  });

  it("tag_name_taken以外の改名の失敗はtext-sm・role=alertで、tag_name_takenはrole=alert無しで出す（N2）", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("旅行");

    const row = screen.getByTitle("旅行").closest<HTMLElement>("[data-tag-id]")!;
    await user.click(within(row).getByRole("button", { name: "Rename" }));
    const input = await screen.findByRole("textbox", { name: 'New name for "旅行"' });

    server.failNextPatch = true;
    await user.clear(input);
    await user.type(input, "旅行2024");
    await user.keyboard("{Enter}");

    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toBe("Something went wrong on the server.");
    expect(alert.className).toContain("text-sm");

    await user.clear(input);
    await user.type(input, "アニメ");
    await user.keyboard("{Enter}");

    const taken = await screen.findByText(
      '"アニメ" is already a synonym of the tag "Anime".',
    );
    expect(taken.getAttribute("role")).toBeNull();
    expect(taken.className).toContain("text-xs");
    expect(input.getAttribute("aria-invalid")).toBe("true");
  });

  it("通信失敗は作成・改名・シノニム追加の名前入力を無効扱いせず、alertで伝える", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("旅行");

    await user.click(screen.getByRole("button", { name: "New tag" }));
    const createInput = screen.getByRole("textbox", { name: "New tag name" });
    await user.type(createInput, "未登録");
    server.failNextCreate = true;
    await user.keyboard("{Enter}");
    expect((await screen.findByRole("alert")).textContent).toBe(
      "Something went wrong on the server.",
    );
    expect(createInput.getAttribute("aria-invalid")).toBeNull();
    await user.click(screen.getByRole("button", { name: "Cancel" }));

    const row = screen.getByTitle("旅行").closest<HTMLElement>("[data-tag-id]")!;
    await user.click(within(row).getByRole("button", { name: "Rename" }));
    const renameInput = screen.getByRole("textbox", { name: 'New name for "旅行"' });
    await user.clear(renameInput);
    await user.type(renameInput, "旅行2024");
    server.failNextPatch = true;
    await user.keyboard("{Enter}");
    expect((await screen.findByRole("alert")).textContent).toBe(
      "Something went wrong on the server.",
    );
    expect(renameInput.getAttribute("aria-invalid")).toBeNull();
    await user.keyboard("{Escape}");

    const dramaRow = screen.getByTitle("Drama").closest<HTMLElement>("[data-tag-id]")!;
    await user.click(within(dramaRow).getByRole("button", { name: "Synonyms" }));
    const dialog = await screen.findByRole("dialog", { name: 'Synonyms of "Drama"' });
    const synonymInput = within(dialog).getByRole("textbox", { name: "Add synonym" });
    await user.type(synonymInput, "別名");
    server.failNextSynonymsPost = true;
    await user.keyboard("{Enter}");
    expect((await within(dialog).findByRole("alert")).textContent).toBe(
      "Something went wrong on the server.",
    );
    expect(synonymInput.getAttribute("aria-invalid")).toBeNull();
  });

  it("空白だけの検索は絞り込み中として数えない（N5）", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("旅行");

    const search = screen.getByRole("searchbox", { name: "Search tags" });
    await user.type(search, "   ");

    expect(screen.getByText("3 tags")).toBeDefined();
    expect(screen.queryByText(/ of 3 tags/)).toBeNull();
  });

  it("作成の後に一覧を取り直さず、共有の保持の購読者が無ければ全件の GET も送らない（R-12）", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("旅行");

    expect(server.getCalls).toBe(1);

    await user.click(screen.getByRole("button", { name: "New tag" }));
    const input = screen.getByRole("textbox", { name: "New tag name" });
    await user.type(input, "Banana");
    await user.keyboard("{Enter}");
    await screen.findByTitle("Banana");

    // 作った 1 件は読み込んだ行の中に置き、ページも全件も取り直さない。
    await new Promise((resolve) => setTimeout(resolve, 50));
    expect(server.getCalls).toBe(1);
    expect(server.fullGetCalls).toBe(0);
  });

  it("共有の保持の購読者がいれば、操作のあとに全件の GET が送られる（R-12）", async () => {
    const user = userEvent.setup();
    install();
    // 候補や絞り込みの確かめが共有の保持を購読している（別の画面の部品など）。
    const unsubscribe = subscribeTags(() => undefined);
    try {
      renderPage();
      await screen.findByTitle("旅行");
      expect(server.fullGetCalls).toBe(0);

      const row = screen.getByTitle("Drama").closest<HTMLElement>("[data-tag-id]")!;
      await user.click(within(row).getByRole("button", { name: "More actions" }));
      await user.click(await screen.findByRole("menuitem", { name: "Delete…" }));
      await user.click(
        within(await screen.findByRole("alertdialog")).getByRole("button", {
          name: "Delete",
        }),
      );
      await waitFor(() => expect(screen.queryByTitle("Drama")).toBeNull());
      await waitFor(() => expect(server.fullGetCalls).toBe(1));
      expect(server.getCalls).toBe(1);
    } finally {
      unsubscribe();
    }
  });

  it("購読者が無ければ、操作のあとに共有の保持を捨てて次の getTags が取り直す（R-12）", async () => {
    const user = userEvent.setup();
    install();
    await getTags();
    expect(server.fullGetCalls).toBe(1);
    renderPage();
    await screen.findByTitle("旅行");

    const row = screen.getByTitle("Drama").closest<HTMLElement>("[data-tag-id]")!;
    await user.click(within(row).getByRole("button", { name: "More actions" }));
    await user.click(await screen.findByRole("menuitem", { name: "Delete…" }));
    await user.click(
      within(await screen.findByRole("alertdialog")).getByRole("button", {
        name: "Delete",
      }),
    );
    await waitFor(() => expect(screen.queryByTitle("Drama")).toBeNull());
    await new Promise((resolve) => setTimeout(resolve, 20));
    expect(server.fullGetCalls).toBe(1);

    const refreshed = await getTags();
    expect(server.fullGetCalls).toBe(2);
    expect(refreshed.map((item) => item.name)).not.toContain("Drama");
  });

  it("tag_not_foundの取り直しに失敗しても、一覧を空白にせず今の一覧を残す（N6）", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("旅行");

    const row = screen.getByTitle("旅行").closest<HTMLElement>("[data-tag-id]")!;
    await user.click(within(row).getByRole("button", { name: "Rename" }));
    const input = await screen.findByRole("textbox", { name: 'New name for "旅行"' });

    server.tags = server.tags.filter((t) => t.name !== "旅行");
    server.failNextGet = true;

    await user.clear(input);
    await user.type(input, "旅行2024");
    await user.keyboard("{Enter}");

    expect(
      await screen.findByText("This tag no longer exists, so the list was reloaded"),
    ).toBeDefined();
    expect(screen.getByTitle("Anime")).toBeDefined();
    expect(screen.getByTitle("Drama")).toBeDefined();
    expect(screen.queryByText("Couldn't load the tags")).toBeNull();
  });

  it("改名中に検索でその行が一致しなくなっても行は消えず、打っている途中の名前が残る（Devinの指摘1）", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("旅行");

    const row = screen.getByTitle("旅行").closest<HTMLElement>("[data-tag-id]")!;
    await user.click(within(row).getByRole("button", { name: "Rename" }));
    const input = await screen.findByRole("textbox", { name: 'New name for "旅行"' });
    await user.clear(input);
    await user.type(input, "捨てない下書き");

    // 「旅行」はもう検索に一致しない条件へ変える。
    const search = screen.getByRole("searchbox", { name: "Search tags" });
    await user.type(search, "Anime");

    // 行は消えず、打っている途中の名前もそのまま残る。
    const stillInput = screen.getByRole("textbox", { name: 'New name for "旅行"' });
    expect(stillInput).toBe(input);
    expect((stillInput as HTMLInputElement).value).toBe("捨てない下書き");
    // 件数の行は実際の一致件数（Anime の1件）のままで、ピン留めした分は数えない。
    expect(screen.getByText("1 of 3 tags")).toBeDefined();
  });

  it("改名の送信中はEscで閉じない。閉じたあとに応答が届いて状態を書き換えることも無い（Devinの指摘2）", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("旅行");

    const row = screen.getByTitle("旅行").closest<HTMLElement>("[data-tag-id]")!;
    await user.click(within(row).getByRole("button", { name: "Rename" }));
    const input = await screen.findByRole("textbox", { name: 'New name for "旅行"' });
    await user.clear(input);
    await user.type(input, "旅行2024");

    hold.nextMutation = true;
    await user.keyboard("{Enter}");
    await waitFor(() => expect(input.getAttribute("aria-busy")).toBe("true"));

    // 送信中の Esc は無視される。
    await user.keyboard("{Escape}");
    expect(screen.getByRole("textbox", { name: 'New name for "旅行"' })).toBeDefined();

    hold.release?.();
    expect(await screen.findByTitle("旅行2024")).toBeDefined();
  });

  it("作成の送信中はEscで閉じず、キャンセルのボタンもdisabledのまま（Devinの指摘2）", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("旅行");

    await user.click(screen.getByRole("button", { name: "New tag" }));
    const input = screen.getByRole("textbox", { name: "New tag name" });
    await user.type(input, "Banana");

    hold.nextMutation = true;
    await user.keyboard("{Enter}");
    await waitFor(() => expect(input.getAttribute("aria-busy")).toBe("true"));

    expect(screen.getByRole("button", { name: "Cancel" }).hasAttribute("disabled")).toBe(
      true,
    );
    await user.keyboard("{Escape}");
    expect(screen.getByRole("textbox", { name: "New tag name" })).toBeDefined();

    hold.release?.();
    expect(await screen.findByTitle("Banana")).toBeDefined();
  });

  it("改名の送信中はほかの行の改名も「新しいタグ」も始められない。応答が届けば再び始められる（Devinの指摘2）", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("旅行");

    const row = screen.getByTitle("旅行").closest<HTMLElement>("[data-tag-id]")!;
    await user.click(within(row).getByRole("button", { name: "Rename" }));
    const input = await screen.findByRole("textbox", { name: 'New name for "旅行"' });
    await user.clear(input);
    await user.type(input, "旅行2024");

    hold.nextMutation = true;
    await user.keyboard("{Enter}");
    await waitFor(() => expect(input.getAttribute("aria-busy")).toBe("true"));

    const animeRow = screen.getByTitle("Anime").closest<HTMLElement>("[data-tag-id]")!;
    const animeRenameButton = within(animeRow).getByRole("button", { name: "Rename" });
    expect(animeRenameButton.hasAttribute("disabled")).toBe(true);
    await user.click(animeRenameButton);
    expect(screen.queryByRole("textbox", { name: 'New name for "Anime"' })).toBeNull();

    expect(screen.getByRole("button", { name: "New tag" }).hasAttribute("disabled")).toBe(
      true,
    );

    hold.release?.();
    // 「旅行」の改名が終わって初めて、Anime の改名を始められる。応答が
    // すでに終わった「旅行」の改名を巻き戻すことは無い。
    expect(await screen.findByTitle("旅行2024")).toBeDefined();
    await user.click(within(animeRow).getByRole("button", { name: "Rename" }));
    expect(
      await screen.findByRole("textbox", { name: 'New name for "Anime"' }),
    ).toBeDefined();
  });

  it("作成の失敗の表示は、入力を打ち直すと消える（Devinの指摘3）", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("旅行");

    await user.click(screen.getByRole("button", { name: "New tag" }));
    const input = screen.getByRole("textbox", { name: "New tag name" });
    await user.type(input, "旅行");
    await user.keyboard("{Enter}");

    expect(await screen.findByText('A tag named "旅行" already exists.')).toBeDefined();

    await user.type(input, "2024");
    expect(screen.queryByText('A tag named "旅行" already exists.')).toBeNull();
  });

  it("改名の失敗の表示は、入力を打ち直すと消える（Devinの指摘3）", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("旅行");

    const row = screen.getByTitle("旅行").closest<HTMLElement>("[data-tag-id]")!;
    await user.click(within(row).getByRole("button", { name: "Rename" }));
    const input = await screen.findByRole("textbox", { name: 'New name for "旅行"' });
    await user.clear(input);
    await user.type(input, "アニメ");
    await user.keyboard("{Enter}");

    expect(
      await screen.findByText('"アニメ" is already a synonym of the tag "Anime".'),
    ).toBeDefined();

    await user.type(input, "2024");
    expect(
      screen.queryByText('"アニメ" is already a synonym of the tag "Anime".'),
    ).toBeNull();
  });
});

describe("TagsPage の英語の文言", () => {
  const userData = ["旅行", "Anime", "アニメ", "Drama", "存在しない語", "新規"];

  function rowOf(name: string): HTMLElement {
    return screen.getByTitle(name).closest<HTMLElement>("[data-tag-id]")!;
  }

  it("疑似ロケールで、一覧・検索・作成・改名・メニューの文言がカタログから出る", async () => {
    enablePseudoLocale();
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("旅行");
    expectCatalogTextOnly(document.body, userData);

    const search = screen.getByRole("searchbox");
    await user.type(search, "ani");
    await waitFor(() => expect(screen.queryByTitle("旅行")).toBeNull());
    expectCatalogTextOnly(document.body, userData);

    await user.clear(search);
    await user.type(search, "存在しない語");
    await screen.findByRole("heading", { level: 2, name: /match/ });
    expectCatalogTextOnly(document.body, userData);
    await user.clear(search);
    await screen.findByTitle("旅行");

    // 作成の行（理由と、送信の失敗）。
    await user.click(screen.getByRole("button", { name: /New tag/ }));
    const create = await screen.findByRole("textbox", { name: /New tag name/ });
    await user.type(create, "x".repeat(101));
    expectCatalogTextOnly(document.body, userData);
    await user.clear(create);
    await user.type(create, "旅行");
    await user.keyboard("{Enter}");
    await screen.findByText(/already exists/);
    expectCatalogTextOnly(document.body, userData);
    await user.keyboard("{Escape}");

    // 改名の行と、その他の操作のメニュー。
    await user.click(within(rowOf("Anime")).getByRole("button", { name: /Rename/ }));
    await screen.findByRole("textbox", { name: /New name for/ });
    expectCatalogTextOnly(document.body, userData);
    await user.keyboard("{Escape}");

    await user.click(
      within(rowOf("Anime")).getByRole("button", { name: /More actions/ }),
    );
    await screen.findByRole("menu");
    expectCatalogTextOnly(document.body, userData);
  });

  it("疑似ロケールで、削除・統合・シノニムの窓の文言がカタログから出る", async () => {
    enablePseudoLocale();
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Anime");

    await user.click(
      within(rowOf("Anime")).getByRole("button", { name: /More actions/ }),
    );
    await user.click(await screen.findByRole("menuitem", { name: /Delete/ }));
    await screen.findByRole("alertdialog");
    expectCatalogTextOnly(document.body, userData);
    await user.keyboard("{Escape}");
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());

    await user.click(within(rowOf("旅行")).getByRole("button", { name: /More actions/ }));
    await user.click(await screen.findByRole("menuitem", { name: /Merge into/ }));
    const merge = await screen.findByRole("dialog");
    const combo = within(merge).getByRole("combobox");
    await user.type(combo, "アニ");
    await within(merge).findByRole("option", { name: /Anime/ });
    expectCatalogTextOnly(document.body, userData);
    await user.click(within(merge).getByRole("option", { name: /Anime/ }));
    expectCatalogTextOnly(document.body, userData);
    server.failNextMerge = true;
    await user.click(within(merge).getByRole("button", { name: /^⟦Merge⟧$/ }));
    await within(merge).findByRole("alert");
    expectCatalogTextOnly(document.body, userData);
    await user.click(within(merge).getByRole("button", { name: /Cancel/ }));
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());

    await user.click(
      within(rowOf("Anime")).getByRole("button", { name: /^⟦Synonyms⟧$/ }),
    );
    const synonyms = await screen.findByRole("dialog");
    const input = within(synonyms).getByRole("textbox");
    await user.type(input, "Anime");
    await user.keyboard("{Enter}");
    await within(synonyms).findByText(/this tag's name/);
    expectCatalogTextOnly(document.body, userData);

    await user.clear(input);
    await user.type(input, "Drama");
    await user.keyboard("{Enter}");
    await within(synonyms).findByText(/is a tag on/);
    expectCatalogTextOnly(document.body, userData);
  });

  it("疑似ロケールで、空・読み込み中・失敗の状態の文言がカタログから出る", async () => {
    enablePseudoLocale();
    install();
    hold.getsFrom = 1;
    const loading = renderPage();
    await screen.findAllByRole("status");
    expectCatalogTextOnly(document.body, userData);
    loading.unmount();
    hold.getsFrom = null;
    heldGetReleases.length = 0;

    __resetTagsForTest();
    server.failAllGets = true;
    const failed = renderPage();
    await screen.findByRole("heading", { level: 2, name: /load/ });
    expectCatalogTextOnly(document.body, userData);
    failed.unmount();

    __resetTagsForTest();
    server.failAllGets = false;
    server.tags = [];
    renderPage();
    await screen.findByRole("heading", { level: 2, name: /No tags yet/ });
    expectCatalogTextOnly(document.body, userData);
  });

  it("件数と本数は 1 と複数で単数・複数を分ける", async () => {
    const user = userEvent.setup();
    install();
    server.tags = [tag({ id: 1, name: "旅行", videoCount: 1 })];
    renderPage();
    await screen.findByTitle("旅行");
    expect(screen.getByText("1 tag")).toBeDefined();
    expect(rowOf("旅行").textContent).toContain("1 video");
    expect(rowOf("旅行").textContent).not.toContain("1 videos");

    await user.click(within(rowOf("旅行")).getByRole("button", { name: "More actions" }));
    await user.click(await screen.findByRole("menuitem", { name: "Delete…" }));
    const dialog = await screen.findByRole("alertdialog", { name: 'Delete "旅行"' });
    expect(
      within(dialog).getByText(
        "This tag will be removed from 1 video. This can't be undone.",
      ),
    ).toBeDefined();
  });

  it("日本語のタグ名とシノニムはそのまま表示され、その綴りで検索できる", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("旅行");
    expect(
      screen.getByRole("link", { name: "Open the library filtered by 旅行" }),
    ).toBeDefined();
    expect(screen.getByText("Synonyms: アニメ")).toBeDefined();

    const search = screen.getByRole("searchbox", { name: "Search tags" });
    await user.type(search, "旅");
    await waitFor(() => expect(screen.queryByTitle("Anime")).toBeNull());
    expect(screen.getByTitle("旅行")).toBeDefined();
    expect(screen.getByText("1 of 3 tags")).toBeDefined();

    await user.clear(search);
    await user.type(search, "アニメ");
    await waitFor(() => expect(screen.queryByTitle("旅行")).toBeNull());
    expect(screen.getByTitle("Anime")).toBeDefined();
  });

  it("空のタグ名と長すぎるタグ名のサーバーの拒否を、上限を埋め込んだ英語で出す", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("旅行");

    await user.click(screen.getByRole("button", { name: "New tag" }));
    const input = await screen.findByRole("textbox", { name: "New tag name" });
    server.nextError = {
      status: 400,
      body: {
        code: "invalid_request",
        reason: "tag_name_empty",
        message: "server-side message",
      },
    };
    await user.type(input, "新規");
    await user.keyboard("{Enter}");
    expect((await screen.findByRole("alert")).textContent).toBe("Enter a tag name.");

    server.nextError = {
      status: 400,
      body: {
        code: "invalid_request",
        reason: "tag_name_too_long",
        limit: 100,
        message: "server-side message",
      },
    };
    await user.type(input, "2");
    await user.keyboard("{Enter}");
    expect(
      await screen.findByText("Use a tag name of 100 characters or fewer."),
    ).toBeDefined();
  });

  it("自分の名前・自分のシノニムの登録は、このタグのことだと分かる英語で出す", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Anime");

    await user.click(within(rowOf("Anime")).getByRole("button", { name: "Synonyms" }));
    const dialog = await screen.findByRole("dialog", { name: 'Synonyms of "Anime"' });
    const input = within(dialog).getByRole("textbox", { name: "Add synonym" });
    await user.type(input, "Anime");
    await user.keyboard("{Enter}");
    expect(
      await within(dialog).findByText('"Anime" is already this tag\'s name.'),
    ).toBeDefined();
  });

  it("統合が要る名前の確認は、API の tagName のタグ名で説明する", async () => {
    const user = userEvent.setup();
    install();
    server.tags = [
      tag({ id: 1, name: "Anime", videoCount: 2 }),
      tag({ id: 2, name: "ドラマ", synonyms: ["drama"], videoCount: 1 }),
    ];
    renderPage();
    await screen.findByTitle("Anime");

    await user.click(within(rowOf("Anime")).getByRole("button", { name: "Synonyms" }));
    const dialog = await screen.findByRole("dialog", { name: 'Synonyms of "Anime"' });
    const input = within(dialog).getByRole("textbox", { name: "Add synonym" });
    // サーバーは整えた名前（前後の空白を除くだけ）を BINARY で照合するので、
    // 前後に空白を付けて打った綴りでも、tagName は元の名前「ドラマ」になる。
    await user.type(input, "  ドラマ  ");
    await user.keyboard("{Enter}");
    expect(
      await within(dialog).findByText(
        '"ドラマ" is a tag on 1 video. Merging it into "Anime" adds "Anime" to those videos, and "ドラマ" and its synonyms become synonyms of "Anime". "ドラマ" leaves the tag list.',
      ),
    ).toBeDefined();
  });

  it("同じタグへの統合の拒否を英語で出す", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("旅行");

    await user.click(within(rowOf("旅行")).getByRole("button", { name: "More actions" }));
    await user.click(
      await screen.findByRole("menuitem", { name: "Merge into another tag…" }),
    );
    const dialog = await screen.findByRole("dialog", { name: 'Merge "旅行"' });
    await user.type(within(dialog).getByRole("combobox"), "Anime");
    await user.click(await within(dialog).findByRole("option", { name: /Anime/ }));
    server.nextError = {
      status: 400,
      body: {
        code: "invalid_request",
        reason: "merge_same_tag",
        message: "server-side message",
      },
    };
    await user.click(within(dialog).getByRole("button", { name: "Merge" }));
    expect((await within(dialog).findByRole("alert")).textContent).toBe(
      "A tag can't be merged into itself.",
    );
  });
});
