import { fireEvent, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it } from "vitest";

import {
  tag,
  server,
  synonymDeleteReleases,
  hold,
  install,
  renderPage,
  setUpTagsPageServer,
} from "../testing/tagsPage";

setUpTagsPageServer();

describe("TagsPage 統合", () => {
  it("「その他の操作」の先頭に「別のタグへ統合…」、区切り線を挟んで「削除…」が並ぶ（統合は作成・改名より目立たない）", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("旅行");

    const row = screen.getByTitle("旅行").closest<HTMLElement>("[data-tag-id]")!;
    await user.click(within(row).getByRole("button", { name: "More actions" }));
    const menu = await screen.findByRole("menu");
    const items = within(menu).getAllByRole("menuitem");
    expect(items.map((item) => item.textContent)).toEqual([
      "Merge into another tag…",
      "Delete…",
    ]);
  });

  it("統合すると統合元が一覧から消え統合先のシノニムに並び、フォーカスが統合先の名前へ移る（受け入れ条件12）", async () => {
    const user = userEvent.setup();
    const fetchMock = install();
    server.tags = [
      tag({ id: 1, name: "旅行", videoCount: 5 }),
      tag({ id: 2, name: "Anime", synonyms: ["アニメ"], videoCount: 3 }),
    ];
    renderPage();
    await screen.findByTitle("旅行");

    const row = screen.getByTitle("旅行").closest<HTMLElement>("[data-tag-id]")!;
    await user.click(within(row).getByRole("button", { name: "More actions" }));
    await user.click(
      await screen.findByRole("menuitem", { name: "Merge into another tag…" }),
    );
    const dialog = await screen.findByRole("dialog", { name: 'Merge "旅行"' });

    const combo = within(dialog).getByRole("combobox", { name: "Tag to merge into" });
    const mergeButton = within(dialog).getByRole("button", { name: "Merge" });
    expect(mergeButton.hasAttribute("disabled")).toBe(true);

    await user.type(combo, "Anime");
    await user.click(await within(dialog).findByRole("option", { name: /Anime/ }));

    expect(
      within(dialog).getByText(
        'The 5 videos tagged "旅行" get the tag "Anime". "旅行" and its synonyms become synonyms of "Anime", and "旅行" leaves the tag list. This can\'t be undone.',
      ),
    ).toBeDefined();
    expect(mergeButton.hasAttribute("disabled")).toBe(false);

    await user.click(mergeButton);

    expect(await screen.findByText('Merged "旅行" into "Anime"')).toBeDefined();
    // 1 件の統合も sourceIds で送る（specs/036-tag-admin-scale/contracts/screen-api.md §2）。
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/tags/2/merge",
      expect.objectContaining({
        method: "POST",
        body: JSON.stringify({ sourceIds: [1] }),
      }),
    );
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(screen.queryByTitle("旅行")).toBeNull();
    expect(screen.getByText("Synonyms: アニメ · 旅行")).toBeDefined();
    await waitFor(() =>
      expect(document.activeElement).toBe(
        screen.getByRole("link", { name: "Open the library filtered by Anime" }),
      ),
    );
  });

  it("統合先の候補は統合元を除き、作成の行を持たない", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("旅行");

    const row = screen.getByTitle("旅行").closest<HTMLElement>("[data-tag-id]")!;
    await user.click(within(row).getByRole("button", { name: "More actions" }));
    await user.click(
      await screen.findByRole("menuitem", { name: "Merge into another tag…" }),
    );
    const dialog = await screen.findByRole("dialog", { name: 'Merge "旅行"' });
    const combo = within(dialog).getByRole("combobox", { name: "Tag to merge into" });
    await user.click(combo);

    expect(within(dialog).queryByRole("option", { name: /旅行/ })).toBeNull();
    await user.type(combo, "存在しない語");
    expect(within(dialog).queryByText(/^Create "/)).toBeNull();
  });

  it("統合の確認でEscを押すと何も変わらず、フォーカスがその行の「その他の操作」へ戻る", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("旅行");

    const row = screen.getByTitle("旅行").closest<HTMLElement>("[data-tag-id]")!;
    const menuButton = within(row).getByRole("button", { name: "More actions" });
    await user.click(menuButton);
    await user.click(
      await screen.findByRole("menuitem", { name: "Merge into another tag…" }),
    );
    await screen.findByRole("dialog", { name: 'Merge "旅行"' });

    await user.keyboard("{Escape}");

    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(screen.getByTitle("旅行")).toBeDefined();
    await waitFor(() => expect(document.activeElement).toBe(menuButton));
  });

  it("統合先が別のタブで消えていると、もう無いことが伝わり一覧が取り直される", async () => {
    const user = userEvent.setup();
    install();
    server.tags = [tag({ id: 1, name: "旅行" }), tag({ id: 2, name: "Anime" })];
    renderPage();
    await screen.findByTitle("旅行");

    const row = screen.getByTitle("旅行").closest<HTMLElement>("[data-tag-id]")!;
    await user.click(within(row).getByRole("button", { name: "More actions" }));
    await user.click(
      await screen.findByRole("menuitem", { name: "Merge into another tag…" }),
    );
    const dialog = await screen.findByRole("dialog", { name: 'Merge "旅行"' });
    const combo = within(dialog).getByRole("combobox", { name: "Tag to merge into" });
    await user.type(combo, "Anime");
    await user.click(await within(dialog).findByRole("option", { name: /Anime/ }));

    server.tags = server.tags.filter((t) => t.name !== "Anime");

    await user.click(within(dialog).getByRole("button", { name: "Merge" }));

    expect(
      await screen.findByText("This tag no longer exists, so the list was reloaded"),
    ).toBeDefined();
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  });

  it("統合元が別のタブで消えていると（notFoundIds）、もう無いことが伝わり一覧が取り直される", async () => {
    const user = userEvent.setup();
    install();
    server.tags = [tag({ id: 1, name: "旅行" }), tag({ id: 2, name: "Anime" })];
    renderPage();
    await screen.findByTitle("旅行");

    const row = screen.getByTitle("旅行").closest<HTMLElement>("[data-tag-id]")!;
    await user.click(within(row).getByRole("button", { name: "More actions" }));
    await user.click(
      await screen.findByRole("menuitem", { name: "Merge into another tag…" }),
    );
    const dialog = await screen.findByRole("dialog", { name: 'Merge "旅行"' });
    const combo = within(dialog).getByRole("combobox", { name: "Tag to merge into" });
    await user.type(combo, "Anime");
    await user.click(await within(dialog).findByRole("option", { name: /Anime/ }));

    server.tags = server.tags.filter((t) => t.name !== "旅行");

    await user.click(within(dialog).getByRole("button", { name: "Merge" }));

    expect(
      await screen.findByText("This tag no longer exists, so the list was reloaded"),
    ).toBeDefined();
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    await waitFor(() => expect(screen.queryByTitle("旅行")).toBeNull());
    expect(screen.queryByText(/^Merged /)).toBeNull();
  });

  it("統合先の候補は窓の本文に開いたままで、入力の中の Esc は窓を閉じる（B1）", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("旅行");

    const row = screen.getByTitle("旅行").closest<HTMLElement>("[data-tag-id]")!;
    await user.click(within(row).getByRole("button", { name: "More actions" }));
    await user.click(
      await screen.findByRole("menuitem", { name: "Merge into another tag…" }),
    );
    const dialog = await screen.findByRole("dialog", { name: 'Merge "旅行"' });

    const combo = within(dialog).getByRole("combobox", { name: "Tag to merge into" });
    await user.click(combo);
    await user.type(combo, "A");
    expect(combo.getAttribute("aria-expanded")).toBe("true");

    // 候補の一覧は窓の本文の一部で、閉じる段は無い。Esc で窓が閉じる。
    await user.keyboard("{Escape}");
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  });

  it("IME変換中のEscは窓を閉じない（B2）", async () => {
    install();
    renderPage();
    await screen.findByTitle("旅行");

    const user = userEvent.setup();
    const row = screen.getByTitle("旅行").closest<HTMLElement>("[data-tag-id]")!;
    await user.click(within(row).getByRole("button", { name: "More actions" }));
    await user.click(
      await screen.findByRole("menuitem", { name: "Merge into another tag…" }),
    );
    const dialog = await screen.findByRole("dialog", { name: 'Merge "旅行"' });
    const cancelButton = within(dialog).getByRole("button", { name: "Cancel" });
    cancelButton.focus();

    fireEvent.keyDown(cancelButton, { key: "Escape", keyCode: 229 });
    expect(screen.getByRole("dialog", { name: 'Merge "旅行"' })).toBeDefined();

    fireEvent.keyDown(cancelButton, { key: "Escape" });
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  });

  it("統合の要求が届く間はEscや×で閉じない（N3）", async () => {
    const user = userEvent.setup();
    install();
    server.tags = [tag({ id: 1, name: "旅行" }), tag({ id: 2, name: "Anime" })];
    renderPage();
    await screen.findByTitle("旅行");

    const row = screen.getByTitle("旅行").closest<HTMLElement>("[data-tag-id]")!;
    await user.click(within(row).getByRole("button", { name: "More actions" }));
    await user.click(
      await screen.findByRole("menuitem", { name: "Merge into another tag…" }),
    );
    const dialog = await screen.findByRole("dialog", { name: 'Merge "旅行"' });
    const combo = within(dialog).getByRole("combobox", { name: "Tag to merge into" });
    await user.type(combo, "Anime");
    await user.click(await within(dialog).findByRole("option", { name: /Anime/ }));

    hold.nextMutation = true;
    await user.click(within(dialog).getByRole("button", { name: "Merge" }));
    await waitFor(() =>
      expect(
        within(dialog).getByRole("button", { name: "Merging…" }).hasAttribute("disabled"),
      ).toBe(true),
    );

    await user.keyboard("{Escape}");
    expect(screen.getByRole("dialog", { name: 'Merge "旅行"' })).toBeDefined();

    // 送っている間は「Cancel」も押せない。
    expect(
      within(dialog).getByRole("button", { name: "Cancel" }).hasAttribute("disabled"),
    ).toBe(true);
    expect(screen.getByRole("dialog", { name: 'Merge "旅行"' })).toBeDefined();

    hold.release?.();
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  });

  it("統合が失敗すると、フォーカスが「統合する」へ戻る（N4）", async () => {
    const user = userEvent.setup();
    install();
    server.tags = [tag({ id: 1, name: "旅行" }), tag({ id: 2, name: "Anime" })];
    renderPage();
    await screen.findByTitle("旅行");

    const row = screen.getByTitle("旅行").closest<HTMLElement>("[data-tag-id]")!;
    await user.click(within(row).getByRole("button", { name: "More actions" }));
    await user.click(
      await screen.findByRole("menuitem", { name: "Merge into another tag…" }),
    );
    const dialog = await screen.findByRole("dialog", { name: 'Merge "旅行"' });
    const combo = within(dialog).getByRole("combobox", { name: "Tag to merge into" });
    await user.type(combo, "Anime");
    await user.click(await within(dialog).findByRole("option", { name: /Anime/ }));

    server.failNextMerge = true;
    const mergeButton = within(dialog).getByRole("button", { name: "Merge" });
    await user.click(mergeButton);

    expect(await within(dialog).findByRole("alert")).toBeDefined();
    await waitFor(() => expect(document.activeElement).toBe(mergeButton));
  });
});

describe("TagsPage シノニム", () => {
  it("今のシノニムをChipに出し、×で確認なしにその場で解除できる", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Anime");

    const row = screen.getByTitle("Anime").closest<HTMLElement>("[data-tag-id]")!;
    await user.click(within(row).getByRole("button", { name: "Synonyms" }));
    const dialog = await screen.findByRole("dialog", { name: 'Synonyms of "Anime"' });

    expect(within(dialog).getByText("アニメ")).toBeDefined();
    await user.click(
      within(dialog).getByRole("button", { name: 'Remove the synonym "アニメ"' }),
    );

    await waitFor(() => expect(within(dialog).queryByText("アニメ")).toBeNull());
    await waitFor(() => expect(screen.queryByText("Synonyms: アニメ")).toBeNull());
  });

  it("シノニムを追加すると窓の中の一覧に並び、入力が空に戻る", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Drama");

    const row = screen.getByTitle("Drama").closest<HTMLElement>("[data-tag-id]")!;
    await user.click(within(row).getByRole("button", { name: "Synonyms" }));
    const dialog = await screen.findByRole("dialog", { name: 'Synonyms of "Drama"' });
    const input = within(dialog).getByRole("textbox", { name: "Add synonym" });
    await user.type(input, "ドラマ");
    await user.keyboard("{Enter}");

    expect(await within(dialog).findByText("ドラマ")).toBeDefined();
    expect((input as HTMLInputElement).value).toBe("");
  });

  it("別のタグのシノニムと同じ名前の登録で、そのタグの名前が画面に出る（シノニム名の衝突）", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Drama");

    const row = screen.getByTitle("Drama").closest<HTMLElement>("[data-tag-id]")!;
    await user.click(within(row).getByRole("button", { name: "Synonyms" }));
    const dialog = await screen.findByRole("dialog", { name: 'Synonyms of "Drama"' });
    const input = within(dialog).getByRole("textbox", { name: "Add synonym" });
    await user.type(input, "アニメ");
    await user.keyboard("{Enter}");

    expect(
      await within(dialog).findByText(
        '"アニメ" is already a synonym of the tag "Anime".',
      ),
    ).toBeDefined();
  });

  it("既存のタグの名前をシノニムとして登録しようとすると統合の確認が出て、承諾すると統合される（受け入れ条件17）", async () => {
    const user = userEvent.setup();
    install();
    server.tags = [
      tag({ id: 1, name: "Anime", videoCount: 2 }),
      tag({ id: 2, name: "anime", videoCount: 10 }),
    ];
    renderPage();
    await screen.findByTitle("Anime");

    const row = screen.getByTitle("Anime").closest<HTMLElement>("[data-tag-id]")!;
    await user.click(within(row).getByRole("button", { name: "Synonyms" }));
    const dialog = await screen.findByRole("dialog", { name: 'Synonyms of "Anime"' });
    const input = within(dialog).getByRole("textbox", { name: "Add synonym" });
    await user.type(input, "anime");
    await user.keyboard("{Enter}");

    expect(
      await within(dialog).findByText(
        '"anime" is a tag on 10 videos. Merging it into "Anime" adds "Anime" to those videos, and "anime" becomes a synonym of "Anime". "anime" leaves the tag list.',
      ),
    ).toBeDefined();
    const backButton = within(dialog).getByRole("button", { name: "Back" });
    expect(document.activeElement).toBe(backButton);

    await user.click(within(dialog).getByRole("button", { name: "Merge" }));

    await waitFor(() => expect(within(dialog).queryByText(/is a tag on/)).toBeNull());
    expect(within(dialog).getByText("anime")).toBeDefined();
    expect(server.tags.some((t) => t.name === "anime")).toBe(false);
  });

  it("シノニムが無いタグとの統合の確認では「とそのシノニム」を省く", async () => {
    const user = userEvent.setup();
    install();
    server.tags = [
      tag({ id: 1, name: "Anime", videoCount: 2 }),
      tag({ id: 2, name: "anime", synonyms: ["アニメーション"], videoCount: 10 }),
    ];
    renderPage();
    await screen.findByTitle("Anime");

    const row = screen.getByTitle("Anime").closest<HTMLElement>("[data-tag-id]")!;
    await user.click(within(row).getByRole("button", { name: "Synonyms" }));
    const dialog = await screen.findByRole("dialog", { name: 'Synonyms of "Anime"' });
    const input = within(dialog).getByRole("textbox", { name: "Add synonym" });
    await user.type(input, "anime");
    await user.keyboard("{Enter}");

    expect(
      await within(dialog).findByText(
        '"anime" is a tag on 10 videos. Merging it into "Anime" adds "Anime" to those videos, and "anime" and its synonyms become synonyms of "Anime". "anime" leaves the tag list.',
      ),
    ).toBeDefined();
  });

  it("シノニム登録に伴う統合の確認で「戻る」を押すと何も変えず入力へ戻り、文字が残る（取り消すと何も変わらない）", async () => {
    const user = userEvent.setup();
    install();
    server.tags = [
      tag({ id: 1, name: "Anime", videoCount: 2 }),
      tag({ id: 2, name: "anime", videoCount: 10 }),
    ];
    renderPage();
    await screen.findByTitle("Anime");

    const row = screen.getByTitle("Anime").closest<HTMLElement>("[data-tag-id]")!;
    await user.click(within(row).getByRole("button", { name: "Synonyms" }));
    const dialog = await screen.findByRole("dialog", { name: 'Synonyms of "Anime"' });
    const input = within(dialog).getByRole("textbox", { name: "Add synonym" });
    await user.type(input, "anime");
    await user.keyboard("{Enter}");
    await within(dialog).findByText(/is a tag on/);

    await user.click(within(dialog).getByRole("button", { name: "Back" }));

    expect(within(dialog).queryByText(/is a tag on/)).toBeNull();
    // 「戻る」で確認のビューから入力のビューに戻ると、入力は作り直される
    // （確認のビューには入力が無い）ので、あらためて取得する。
    const inputAfterBack = within(dialog).getByRole("textbox", {
      name: "Add synonym",
    }) as HTMLInputElement;
    expect(inputAfterBack.value).toBe("anime");
    expect(document.activeElement).toBe(inputAfterBack);
    expect(server.tags.some((t) => t.id === 2 && t.name === "anime")).toBe(true);
    expect(server.tags.find((t) => t.id === 1)?.synonyms).toEqual([]);
  });

  it("確認の後に別のタブでその名前が別のタグへ移っていると、統合されずに確認がやり直しになる", async () => {
    const user = userEvent.setup();
    install();
    server.tags = [
      tag({ id: 1, name: "Anime", videoCount: 2 }),
      tag({ id: 2, name: "anime", videoCount: 10 }),
    ];
    renderPage();
    await screen.findByTitle("Anime");

    const row = screen.getByTitle("Anime").closest<HTMLElement>("[data-tag-id]")!;
    await user.click(within(row).getByRole("button", { name: "Synonyms" }));
    const dialog = await screen.findByRole("dialog", { name: 'Synonyms of "Anime"' });
    const input = within(dialog).getByRole("textbox", { name: "Add synonym" });
    await user.type(input, "anime");
    await user.keyboard("{Enter}");
    await within(dialog).findByText(/is a tag on 10 videos/);

    // 確認を出したあと、別のタブで id2 を改名し、新しく「anime」（id3）を作る。
    server.tags[1]!.name = "anime-old";
    server.tags.push(tag({ id: 3, name: "anime", videoCount: 20 }));

    await user.click(within(dialog).getByRole("button", { name: "Merge" }));

    expect(await within(dialog).findByText(/is a tag on 20 videos/)).toBeDefined();
    expect(server.tags.some((t) => t.id === 2 && t.name === "anime-old")).toBe(true);
  });

  it("シノニムの窓でEscを押すと閉じ、フォーカスがその行の「シノニム」へ戻る", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Anime");

    const row = screen.getByTitle("Anime").closest<HTMLElement>("[data-tag-id]")!;
    const synonymsButton = within(row).getByRole("button", { name: "Synonyms" });
    await user.click(synonymsButton);
    await screen.findByRole("dialog", { name: 'Synonyms of "Anime"' });

    await user.keyboard("{Escape}");

    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    await waitFor(() => expect(document.activeElement).toBe(synonymsButton));
  });

  it("シノニムの追加の失敗は、入力を打ち直すと消える（N1）", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Drama");

    const row = screen.getByTitle("Drama").closest<HTMLElement>("[data-tag-id]")!;
    await user.click(within(row).getByRole("button", { name: "Synonyms" }));
    const dialog = await screen.findByRole("dialog", { name: 'Synonyms of "Drama"' });
    const input = within(dialog).getByRole("textbox", { name: "Add synonym" });
    await user.type(input, "アニメ");
    await user.keyboard("{Enter}");
    await within(dialog).findByText('"アニメ" is already a synonym of the tag "Anime".');

    await user.type(input, "2");

    expect(
      within(dialog).queryByText('"アニメ" is already a synonym of the tag "Anime".'),
    ).toBeNull();
  });

  it("シノニムを解除すると、次のチップの×、無ければ前、1つも無ければ入力へフォーカスが移る（N2）", async () => {
    const user = userEvent.setup();
    install();
    server.tags = [tag({ id: 1, name: "Anime", synonyms: ["A", "B", "C"] })];
    renderPage();
    await screen.findByTitle("Anime");

    const row = screen.getByTitle("Anime").closest<HTMLElement>("[data-tag-id]")!;
    await user.click(within(row).getByRole("button", { name: "Synonyms" }));
    const dialog = await screen.findByRole("dialog", { name: 'Synonyms of "Anime"' });
    const input = within(dialog).getByRole("textbox", { name: "Add synonym" });

    // 中間（B）を解除すると、次（C）の×へ移る。
    await user.click(
      within(dialog).getByRole("button", { name: 'Remove the synonym "B"' }),
    );
    await waitFor(() => expect(within(dialog).queryByText("B")).toBeNull());
    await waitFor(() =>
      expect(document.activeElement).toBe(
        within(dialog).getByRole("button", { name: 'Remove the synonym "C"' }),
      ),
    );

    // 残りの最後（C）を解除すると、前（A）の×へ移る。
    await user.click(
      within(dialog).getByRole("button", { name: 'Remove the synonym "C"' }),
    );
    await waitFor(() => expect(within(dialog).queryByText("C")).toBeNull());
    await waitFor(() =>
      expect(document.activeElement).toBe(
        within(dialog).getByRole("button", { name: 'Remove the synonym "A"' }),
      ),
    );

    // 最後の1つ（A）を解除すると、入力へ移る。
    await user.click(
      within(dialog).getByRole("button", { name: 'Remove the synonym "A"' }),
    );
    await waitFor(() => expect(within(dialog).queryByText("A")).toBeNull());
    await waitFor(() => expect(document.activeElement).toBe(input));
  });

  it("シノニム登録に伴う統合が失敗すると、フォーカスが「統合する」へ戻る（N4）", async () => {
    const user = userEvent.setup();
    install();
    server.tags = [
      tag({ id: 1, name: "Anime", videoCount: 2 }),
      tag({ id: 2, name: "anime", videoCount: 10 }),
    ];
    renderPage();
    await screen.findByTitle("Anime");

    const row = screen.getByTitle("Anime").closest<HTMLElement>("[data-tag-id]")!;
    await user.click(within(row).getByRole("button", { name: "Synonyms" }));
    const dialog = await screen.findByRole("dialog", { name: 'Synonyms of "Anime"' });
    const input = within(dialog).getByRole("textbox", { name: "Add synonym" });
    await user.type(input, "anime");
    await user.keyboard("{Enter}");
    await within(dialog).findByText(/is a tag on/);

    server.failNextSynonymsPost = true;
    const confirmButton = within(dialog).getByRole("button", { name: "Merge" });
    await user.click(confirmButton);

    expect(await within(dialog).findByRole("alert")).toBeDefined();
    await waitFor(() => expect(document.activeElement).toBe(confirmButton));
  });

  it("シノニム登録に伴う統合が成功すると、統合元がその場で一覧から消える（バックグラウンドの取り直しを待たない）", async () => {
    const user = userEvent.setup();
    install();
    server.tags = [
      tag({ id: 1, name: "Anime", videoCount: 2 }),
      tag({ id: 2, name: "anime", videoCount: 10 }),
    ];
    renderPage();
    await screen.findByTitle("Anime");
    await screen.findByTitle("anime");

    const row = screen.getByTitle("Anime").closest<HTMLElement>("[data-tag-id]")!;
    await user.click(within(row).getByRole("button", { name: "Synonyms" }));
    const dialog = await screen.findByRole("dialog", { name: 'Synonyms of "Anime"' });
    const input = within(dialog).getByRole("textbox", { name: "Add synonym" });
    await user.type(input, "anime");
    await user.keyboard("{Enter}");
    await within(dialog).findByText(/is a tag on/);

    // バックグラウンドの取り直し（afterTagChanged の refreshTags）が失敗しても、
    // その場での反映（統合元を一覧から消す）は変わらないことを確かめる。
    server.failNextGet = true;

    await user.click(within(dialog).getByRole("button", { name: "Merge" }));
    await within(dialog).findByText("anime");
    await user.click(within(dialog).getByRole("button", { name: "Close" }));

    expect(screen.queryByTitle("anime")).toBeNull();
    expect(screen.getByTitle("Anime")).toBeDefined();
  });

  it("素のシノニムの登録・解除では、ほかのタグを一覧から消さない", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Drama");

    const row = screen.getByTitle("Drama").closest<HTMLElement>("[data-tag-id]")!;
    await user.click(within(row).getByRole("button", { name: "Synonyms" }));
    const dialog = await screen.findByRole("dialog", { name: 'Synonyms of "Drama"' });
    const input = within(dialog).getByRole("textbox", { name: "Add synonym" });
    await user.type(input, "ドラマ");
    await user.keyboard("{Enter}");
    await within(dialog).findByText("ドラマ");
    await user.click(within(dialog).getByRole("button", { name: "Close" }));

    expect(screen.getByTitle("旅行")).toBeDefined();
    expect(screen.getByTitle("Anime")).toBeDefined();
    expect(screen.getByTitle("Drama")).toBeDefined();
  });

  it("確認をとる時点で名前の持ち主がもう無ければ、1回だけ送り直して素のまま登録する", async () => {
    const user = userEvent.setup();
    install();
    server.tags = [tag({ id: 1, name: "Anime", videoCount: 2 })];
    server.forceMergeRequiredOnce = true;
    renderPage();
    await screen.findByTitle("Anime");

    const row = screen.getByTitle("Anime").closest<HTMLElement>("[data-tag-id]")!;
    await user.click(within(row).getByRole("button", { name: "Synonyms" }));
    const dialog = await screen.findByRole("dialog", { name: 'Synonyms of "Anime"' });
    const input = within(dialog).getByRole("textbox", {
      name: "Add synonym",
    }) as HTMLInputElement;
    await user.type(input, "anime");
    await user.keyboard("{Enter}");

    // 確認には切り替わらず、素の登録として直接付く（送り直しが1回だけ
    // 許される。N6b）。
    expect(await within(dialog).findByText("anime")).toBeDefined();
    expect(within(dialog).queryByText(/is a tag on/)).toBeNull();
    expect(input.value).toBe("");
  });

  it("シノニムの追加が成功する応答の間に打ち直していたら、入力の文字を消さない", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Drama");

    const row = screen.getByTitle("Drama").closest<HTMLElement>("[data-tag-id]")!;
    await user.click(within(row).getByRole("button", { name: "Synonyms" }));
    const dialog = await screen.findByRole("dialog", { name: 'Synonyms of "Drama"' });
    const input = within(dialog).getByRole("textbox", {
      name: "Add synonym",
    }) as HTMLInputElement;
    await user.type(input, "ドラマ");

    hold.nextMutation = true;
    await user.keyboard("{Enter}");
    await waitFor(() => expect(input.getAttribute("aria-busy")).toBe("true"));

    // 応答を待つ間に、別の名前へ打ち直す。
    await user.clear(input);
    await user.type(input, "別の下書き");

    hold.release?.();

    await within(dialog).findByText("ドラマ");
    // 追加した「ドラマ」は付いたが、その後に打ち直した文字は消えない。
    expect(input.value).toBe("別の下書き");
  });

  it("並行して複数のシノニムを解除しても、互いの結果を巻き戻さない", async () => {
    const user = userEvent.setup();
    install();
    server.tags = [tag({ id: 1, name: "Anime", synonyms: ["A", "B"] })];
    renderPage();
    await screen.findByTitle("Anime");

    const row = screen.getByTitle("Anime").closest<HTMLElement>("[data-tag-id]")!;
    await user.click(within(row).getByRole("button", { name: "Synonyms" }));
    const dialog = await screen.findByRole("dialog", { name: 'Synonyms of "Anime"' });

    // バックグラウンドの取り直し（各解除の afterTagChanged による
    // refreshTags）がサーバーの正しい状態で上書きして、クライアント側の
    // 巻き戻りを覆い隠してしまわないようにする。ここで確かめたいのは、
    // その場（サーバーの応答を待たない側）の反映が正しいことである。
    server.failAllGets = true;
    hold.synonymDeletes = true;
    await user.click(
      within(dialog).getByRole("button", { name: 'Remove the synonym "A"' }),
    );
    await user.click(
      within(dialog).getByRole("button", { name: 'Remove the synonym "B"' }),
    );
    expect(synonymDeleteReleases).toHaveLength(2);

    // 両方の応答を、同じタイミングで（間に描画を挟まず）解決させる。片方の
    // 結果がもう片方の閉じ込めた古い一覧で上書きされると、どちらか一方が
    // 生き残ってしまう。
    synonymDeleteReleases[0]!();
    synonymDeleteReleases[1]!();

    await waitFor(() => {
      expect(within(dialog).queryByText("A")).toBeNull();
      expect(within(dialog).queryByText("B")).toBeNull();
    });
  });
});

describe("TagsPage まとめての統合", () => {
  function rowOf(name: string): HTMLElement {
    return screen.getByTitle(name).closest("[data-tag-id]")!;
  }

  function bar(): HTMLElement {
    return screen.getByRole("region", { name: "Selected tags" });
  }

  beforeEach(() => {
    server.tags = [
      tag({ id: 1, name: "Action", videoCount: 10 }),
      tag({ id: 2, name: "Alpha", tentative: true, videoCount: 2 }),
      tag({ id: 3, name: "Beta", tentative: true }),
      tag({ id: 4, name: "Cat", videoCount: 4 }),
      tag({ id: 5, name: "Gamma", videoCount: 5 }),
    ];
  });

  async function openMerge(
    user: ReturnType<typeof userEvent.setup>,
    names: readonly string[],
  ): Promise<HTMLElement> {
    for (const name of names) {
      await user.click(within(rowOf(name)).getByRole("checkbox"));
    }
    await user.click(within(bar()).getByRole("button", { name: "Merge into one tag…" }));
    return screen.findByRole("dialog");
  }

  async function chooseTarget(
    user: ReturnType<typeof userEvent.setup>,
    dialog: HTMLElement,
    name: string,
  ) {
    const combo = within(dialog).getByRole("combobox", { name: "Tag to merge into" });
    await user.clear(combo);
    await user.type(combo, name);
    await user.click(
      await within(dialog).findByRole("option", { name: new RegExp(`^${name}`) }),
    );
  }

  it("複数から開くと見出しが「Merge 4 tags」で統合元のチップが並び、統合先の入力と候補の一覧は窓の幅いっぱい（要件13）", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Alpha");

    const dialog = await openMerge(user, ["Alpha", "Beta", "Cat", "Gamma"]);
    expect(screen.getByRole("dialog", { name: "Merge 4 tags" })).toBe(dialog);
    const chips = within(dialog).getByRole("list", { name: "Tags to merge" });
    expect(
      within(chips)
        .getAllByRole("listitem")
        .map((item) => item.textContent),
    ).toEqual(["AlphaTentative", "BetaTentative", "Cat", "Gamma"]);

    const combo = within(dialog).getByRole("combobox", { name: "Tag to merge into" });
    expect(combo.parentElement!.className).toContain("w-full");
    expect(combo.parentElement!.className).not.toContain("w-40");
    await user.type(combo, "A");
    // 候補の一覧は入力の下の本文の中にあり、入力に重ねない。
    const listbox = await within(dialog).findByRole("listbox");
    expect(listbox.className).not.toContain("absolute");
    // 統合先の候補は全タグ（選んだ中からも、選んでいないタグからも）。
    expect(within(listbox).getByRole("option", { name: /^Action/ })).toBeDefined();
    expect(within(listbox).getByRole("option", { name: /^Alpha/ })).toBeDefined();
  });

  it("統合先を選ぶと数を待ち、届いたら tagCount・videoCount の確認を出して統合する", async () => {
    const user = userEvent.setup();
    server.impactVideoCount = 9;
    const fetchMock = install();
    renderPage();
    await screen.findByTitle("Alpha");

    const dialog = await openMerge(user, ["Alpha", "Beta", "Cat", "Gamma"]);
    await chooseTarget(user, dialog, "Action");

    expect(
      await within(dialog).findByText(
        'The 9 videos tagged with these 4 tags get the tag "Action". Their names and synonyms become synonyms of "Action", and the 4 tags leave the tag list. This can\'t be undone.',
      ),
    ).toBeDefined();
    expect(server.impactCalls).toEqual([{ action: "merge", ids: [2, 3, 4, 5] }]);
    const mergeButton = within(dialog).getByRole("button", { name: "Merge" });
    await waitFor(() => expect(document.activeElement).toBe(mergeButton));

    await user.click(mergeButton);

    expect(await screen.findByText('Merged 4 tags into "Action"')).toBeDefined();
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/tags/1/merge",
      expect.objectContaining({ body: JSON.stringify({ sourceIds: [2, 3, 4, 5] }) }),
    );
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    for (const name of ["Alpha", "Beta", "Cat", "Gamma"]) {
      expect(screen.queryByTitle(name)).toBeNull();
    }
    // 本数は応答の tag の videoCount（このモックでは合算の 21）。
    expect(within(rowOf("Action")).getByText("21 videos")).toBeDefined();
    expect(screen.queryByRole("region", { name: "Selected tags" })).toBeNull();
    await waitFor(() =>
      expect(document.activeElement).toBe(
        within(rowOf("Action")).getByRole("link", { name: /Action/ }),
      ),
    );
  });

  it("選んだ中のタグを統合先にすると統合元から外れ、統合先だけなら実行できない（Edge Case）", async () => {
    const user = userEvent.setup();
    const fetchMock = install();
    renderPage();
    await screen.findByTitle("Alpha");

    const dialog = await openMerge(user, ["Action", "Alpha", "Cat", "Gamma"]);
    await chooseTarget(user, dialog, "Action");

    expect(
      within(dialog).getByText('"Action" is kept and the other 3 tags merge into it.'),
    ).toBeDefined();
    const chips = within(dialog).getByRole("list", { name: "Tags to merge" });
    expect(within(chips).getByText("kept")).toBeDefined();
    expect(
      await within(dialog).findByText(
        /^The 11 videos tagged with these 3 tags get the tag "Action"\./,
      ),
    ).toBeDefined();
    expect(server.impactCalls.at(-1)).toEqual({ action: "merge", ids: [2, 4, 5] });

    await user.click(within(dialog).getByRole("button", { name: "Merge" }));
    expect(await screen.findByText('Merged 3 tags into "Action"')).toBeDefined();
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/tags/1/merge",
      expect.objectContaining({ body: JSON.stringify({ sourceIds: [2, 4, 5] }) }),
    );
  });

  it("選んだのが 1 件でそれを統合先にすると「Merge」は押せない", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Alpha");

    const dialog = await openMerge(user, ["Cat"]);
    within(dialog).getByText('Merge "Cat"');
    await chooseTarget(user, dialog, "Cat");

    expect(
      within(dialog).getByText(
        'Choose another tag to merge into: "Cat" is the only tag selected.',
      ),
    ).toBeDefined();
    expect(
      (within(dialog).getByRole("button", { name: "Merge" }) as HTMLButtonElement)
        .disabled,
    ).toBe(true);
    expect(server.impactCalls).toEqual([]);

    await user.click(within(dialog).getByRole("button", { name: "Cancel" }));
    await waitFor(() =>
      expect(document.activeElement).toBe(
        within(bar()).getByRole("button", { name: "Merge into one tag…" }),
      ),
    );
  });

  it("数えられなければ理由と再試行を出し、統合元がすべてもう無ければ統合のトーストを出さずに取り直す", async () => {
    const user = userEvent.setup();
    server.failImpact = true;
    install();
    renderPage();
    await screen.findByTitle("Alpha");

    const dialog = await openMerge(user, ["Cat", "Gamma"]);
    await chooseTarget(user, dialog, "Action");
    expect(
      (await within(dialog).findByRole("alert")).textContent?.startsWith(
        "Couldn't count the affected videos:",
      ),
    ).toBe(true);
    const mergeButton = within(dialog).getByRole("button", { name: "Merge" });
    expect((mergeButton as HTMLButtonElement).disabled).toBe(true);

    server.failImpact = false;
    await user.click(within(dialog).getByRole("button", { name: "Retry" }));
    await within(dialog).findByText(/these 2 tags/);

    // 別のタブで Cat と Gamma が消えていた。
    server.tags = server.tags.filter((item) => item.id !== 4 && item.id !== 5);
    const gets = server.getCalls;
    await user.click(mergeButton);
    expect(
      await screen.findByText(
        "Some of the tags no longer existed, so the list was reloaded",
      ),
    ).toBeDefined();
    expect(screen.queryByText(/^Merged/)).toBeNull();
    await waitFor(() => expect(screen.queryByTitle("Cat")).toBeNull());
    expect(server.getCalls).toBeGreaterThan(gets);
  });

  it("統合元の一部がもう無ければ、実際に統合した数を伝えて一覧を取り直す", async () => {
    const user = userEvent.setup();
    install();
    renderPage();
    await screen.findByTitle("Alpha");

    const dialog = await openMerge(user, ["Alpha", "Cat", "Gamma"]);
    await chooseTarget(user, dialog, "Action");
    await within(dialog).findByText(/these 3 tags/);

    server.tags = server.tags.filter((item) => item.id !== 5);
    await user.click(within(dialog).getByRole("button", { name: "Merge" }));
    expect(await screen.findByText('Merged 2 tags into "Action"')).toBeDefined();
    expect(
      await screen.findByText(
        "Some of the tags no longer existed, so the list was reloaded",
      ),
    ).toBeDefined();
    await waitFor(() => expect(screen.queryByTitle("Gamma")).toBeNull());
  });
});
