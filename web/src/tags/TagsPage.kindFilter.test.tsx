import { act, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeAll, describe, expect, it } from "vitest";

import {
  filterButton,
  install,
  probe,
  renderPage,
  server,
  setUpTagsPageServer,
  tag,
} from "../testing/tagsPage";

setUpTagsPageServer();

// Radix の Select は開くときにポインタの捕捉を確かめる。jsdom には無いので空の実装を置く。
beforeAll(() => {
  const proto = Element.prototype as unknown as Record<string, unknown>;
  proto.hasPointerCapture ??= () => false;
  proto.releasePointerCapture ??= () => undefined;
});

type User = ReturnType<typeof userEvent.setup>;

const genre = { id: 1, name: "Genre" };
const work = { id: 2, name: "Work" };

/** seedKindTags は種類を持つタグと持たないタグを並べる。 */
function seedKindTags() {
  server.tags = [
    tag({ id: 1, name: "Robot", kind: genre }),
    tag({ id: 2, name: "Mecha", kind: genre }),
    tag({ id: 3, name: "Tokyo" }),
    tag({ id: 4, name: "Saga", kind: work }),
  ];
}

function rowOf(name: string): HTMLElement {
  return screen.getByTitle(name).closest<HTMLElement>("[data-tag-id]")!;
}

function shownNames(): string[] {
  return Array.from(document.querySelectorAll<HTMLElement>("[data-tag-id] a[title]")).map(
    (link) => link.title,
  );
}

function bar(): HTMLElement {
  return screen.getByRole("region", { name: "Selected tags" });
}

/** chooseKind は「Filter」の吹き出しを開き、「Kind」の `Select` で `option` を選んで閉じる。 */
async function chooseKind(user: User, option: string) {
  await user.click(filterButton());
  const select = await screen.findByRole("combobox", { name: "Kind" });
  select.focus();
  await user.keyboard("{Enter}");
  await user.click(await screen.findByRole("option", { name: option }));
  await waitFor(() => expect(screen.queryByRole("listbox")).toBeNull());
  await user.keyboard("{Escape}");
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
}

/** setKind は選択バーの「Set kind…」のメニューで `name` を選ぶ。 */
async function setKind(user: User, name: string) {
  await user.click(within(bar()).getByRole("button", { name: "Set kind…" }));
  await user.click(await screen.findByRole("menuitemradio", { name }));
}

describe("TagsPage 種類の名札・絞り込み・「Set kind…」（specs/051-tag-kinds/ui-design.md）", () => {
  it("行は名前の後ろに種類の名前を出し、種類の無いタグには何も出さない", async () => {
    seedKindTags();
    install();
    renderPage();
    await screen.findByTitle("Robot");
    const label = within(rowOf("Robot")).getByTitle("Kind: Genre");
    expect(label.textContent).toBe("Kind: Genre");
    expect(label.getAttribute("data-variant")).toBe("outline");
    expect(within(rowOf("Tokyo")).queryByTitle(/^Kind:/)).toBeNull();
  });

  it("「Kind」で種類を選ぶと kind=<id> で読み直し、チップと件数を出し、URL から読み直せる", async () => {
    const user = userEvent.setup();
    seedKindTags();
    install();
    const { unmount } = renderPage({ topBar: true });
    await screen.findByTitle("Robot");
    await waitFor(() => expect(server.tagKindGets).toBeGreaterThan(0));

    await chooseKind(user, "Genre");
    await waitFor(() => expect(shownNames()).toEqual(["Mecha", "Robot"]));
    expect(server.pageRequests.at(-1)?.get("kind")).toBe("1");
    expect(new URLSearchParams(probe.search).get("kind")).toBe("1");
    expect(filterButton().getAttribute("aria-label")).toBe("Filter (1 applied)");
    expect(
      screen.getByRole("button", { name: 'Remove the filter "Kind: Genre"' }),
    ).toBeDefined();
    expect(screen.getByText("2 of 4 tags")).toBeDefined();

    unmount();
    renderPage({ url: `/tags${probe.search}`, topBar: true });
    await waitFor(() => expect(shownNames()).toEqual(["Mecha", "Robot"]));
    expect(
      await screen.findByRole("button", { name: 'Remove the filter "Kind: Genre"' }),
    ).toBeDefined();
  });

  it("「Any kind」を選ぶと種類の絞り込みを外す", async () => {
    const user = userEvent.setup();
    seedKindTags();
    install();
    renderPage({ url: "/tags?kind=1", topBar: true });
    await waitFor(() => expect(shownNames()).toEqual(["Mecha", "Robot"]));
    await waitFor(() => expect(server.tagKindGets).toBeGreaterThan(0));

    await chooseKind(user, "Any kind");
    await waitFor(() => expect(shownNames()).toHaveLength(4));
    expect(new URLSearchParams(probe.search).has("kind")).toBe(false);
  });

  it("種類の一覧を読めないうちは「Kind」に「Any kind」と「No kind」だけを並べる", async () => {
    const user = userEvent.setup();
    seedKindTags();
    server.failTagKindGets = true;
    install();
    renderPage({ topBar: true });
    await screen.findByTitle("Robot");

    await user.click(filterButton());
    const select = await screen.findByRole("combobox", { name: "Kind" });
    select.focus();
    await user.keyboard("{Enter}");
    const options = await screen.findAllByRole("option");
    expect(options.map((option) => option.textContent)).toEqual(["Any kind", "No kind"]);
  });

  it("「No kind」は kind=none で種類の無いタグだけにし、チップの × で外れる", async () => {
    const user = userEvent.setup();
    seedKindTags();
    install();
    renderPage({ topBar: true });
    await screen.findByTitle("Robot");

    await chooseKind(user, "No kind");
    await waitFor(() => expect(shownNames()).toEqual(["Tokyo"]));
    expect(new URLSearchParams(probe.search).get("kind")).toBe("none");

    await user.click(screen.getByRole("button", { name: 'Remove the filter "No kind"' }));
    await waitFor(() => expect(shownNames()).toHaveLength(4));
    expect(new URLSearchParams(probe.search).has("kind")).toBe(false);
    await waitFor(() => expect(document.activeElement).toBe(filterButton()));
  });

  it("「Clear filters」は種類の絞り込みもほかの絞り込みと一緒に外す", async () => {
    const user = userEvent.setup();
    seedKindTags();
    install();
    renderPage({ url: "/tags?unused=1&kind=2", topBar: true });
    await waitFor(() => expect(shownNames()).toEqual(["Saga"]));
    await user.click(filterButton());
    await user.click(await screen.findByRole("button", { name: "Clear filters" }));
    await waitFor(() => expect(shownNames()).toHaveLength(4));
    const params = new URLSearchParams(probe.search);
    expect(params.has("kind")).toBe(false);
    expect(params.has("unused")).toBe(false);
  });

  it("もう無い種類の kind は URL から外し、ほかの条件を残して読み直し、トーストで伝える", async () => {
    seedKindTags();
    install();
    renderPage({ url: "/tags?unused=1&kind=99", topBar: true });
    expect(
      await screen.findByText("Removed a kind that no longer exists from the filter"),
    ).toBeDefined();
    await waitFor(() =>
      expect(new URLSearchParams(probe.search).has("kind")).toBe(false),
    );
    expect(new URLSearchParams(probe.search).get("unused")).toBe("1");
    await waitFor(() => expect(shownNames()).toHaveLength(4));
  });

  it("種類の一覧を読めないうちは kind を外さず、チップは Skeleton と「Remove the kind filter」", async () => {
    seedKindTags();
    server.failTagKindGets = true;
    install();
    renderPage({ url: "/tags?kind=1", topBar: true });
    await waitFor(() => expect(shownNames()).toEqual(["Mecha", "Robot"]));
    expect(screen.getByRole("button", { name: "Remove the kind filter" })).toBeDefined();
    expect(new URLSearchParams(probe.search).get("kind")).toBe("1");
  });

  it("種類だけの絞り込みで行が無ければ、種類を名指しした空の状態を出す", async () => {
    server.tags = [tag({ id: 3, name: "Tokyo" })];
    install();
    renderPage({ url: "/tags?kind=1", topBar: true });
    expect(
      await screen.findByRole("heading", { name: 'No tags of the kind "Genre"' }),
    ).toBeDefined();
  });

  it("「Set kind…」で選んだタグに種類を付け、その場で名札を書き換えて選択から外す", async () => {
    const user = userEvent.setup();
    seedKindTags();
    install();
    renderPage({ topBar: true });
    await screen.findByTitle("Robot");
    await waitFor(() => expect(server.tagKindGets).toBeGreaterThan(0));

    await user.click(within(rowOf("Tokyo")).getByRole("checkbox"));
    await user.click(within(rowOf("Saga")).getByRole("checkbox"));
    await setKind(user, "Genre");

    expect(await screen.findByText('Set the kind "Genre" on 2 tags')).toBeDefined();
    expect(server.batchCalls.at(-1)).toEqual({
      action: "setKind",
      kindId: 1,
      ids: [3, 4],
    });
    expect(within(rowOf("Tokyo")).getByTitle("Kind: Genre")).toBeDefined();
    expect(within(rowOf("Saga")).getByTitle("Kind: Genre")).toBeDefined();
    expect(screen.queryByRole("region", { name: "Selected tags" })).toBeNull();
  });

  it("既にその種類のタグは選んだまま残し、トーストに数を添える", async () => {
    const user = userEvent.setup();
    seedKindTags();
    install();
    renderPage({ topBar: true });
    await screen.findByTitle("Robot");
    await waitFor(() => expect(server.tagKindGets).toBeGreaterThan(0));

    await user.click(within(rowOf("Robot")).getByRole("checkbox"));
    await user.click(within(rowOf("Tokyo")).getByRole("checkbox"));
    await setKind(user, "Genre");

    expect(
      await screen.findByText('Set the kind "Genre" on 1 tag. 1 already had it.'),
    ).toBeDefined();
    expect(within(bar()).getByText("1 tag selected")).toBeDefined();
    await waitFor(() =>
      expect(document.activeElement).toBe(
        within(bar()).getByRole("button", { name: "Set kind…" }),
      ),
    );
  });

  it("「No kind」で種類を外し、kind=none の一覧では種類を付けた行が外れる", async () => {
    const user = userEvent.setup();
    seedKindTags();
    install();
    renderPage({ topBar: true });
    await screen.findByTitle("Robot");
    await waitFor(() => expect(server.tagKindGets).toBeGreaterThan(0));

    await user.click(within(rowOf("Robot")).getByRole("checkbox"));
    await user.click(within(rowOf("Saga")).getByRole("checkbox"));
    await setKind(user, "No kind");
    expect(await screen.findByText("Removed the kind from 2 tags")).toBeDefined();
    expect(server.batchCalls.at(-1)).toEqual({ action: "clearKind", ids: [1, 4] });
    expect(within(rowOf("Robot")).queryByTitle(/^Kind:/)).toBeNull();

    await chooseKind(user, "No kind");
    await waitFor(() => expect(shownNames()).toEqual(["Robot", "Saga", "Tokyo"]));
    await user.click(within(rowOf("Tokyo")).getByRole("checkbox"));
    await setKind(user, "Work");
    expect(await screen.findByText('Set the kind "Work" on 1 tag')).toBeDefined();
    expect(shownNames()).toEqual(["Robot", "Saga"]);
    expect(screen.getByText("2 of 4 tags")).toBeDefined();
  });

  it("もう無い種類を選ぶと何も変えずに種類を読み直し、トーストで伝える", async () => {
    const user = userEvent.setup();
    seedKindTags();
    install();
    renderPage({ topBar: true });
    await screen.findByTitle("Robot");
    await waitFor(() => expect(server.tagKindGets).toBeGreaterThan(0));

    server.tagKinds = server.tagKinds.filter((item) => item.id !== 2);
    const before = server.tagKindGets;
    await user.click(within(rowOf("Tokyo")).getByRole("checkbox"));
    await setKind(user, "Work");
    expect(
      await screen.findByText("That kind no longer exists, so the kinds were reloaded"),
    ).toBeDefined();
    await waitFor(() => expect(server.tagKindGets).toBeGreaterThan(before));
    expect(within(rowOf("Tokyo")).queryByTitle(/^Kind:/)).toBeNull();
    expect(within(bar()).getByText("1 tag selected")).toBeDefined();
  });

  it("「Set kind…」が失敗したら誤りをトーストで伝え、名札も選択も変えない", async () => {
    const user = userEvent.setup();
    seedKindTags();
    install();
    renderPage({ topBar: true });
    await screen.findByTitle("Robot");
    await waitFor(() => expect(server.tagKindGets).toBeGreaterThan(0));

    await user.click(within(rowOf("Tokyo")).getByRole("checkbox"));
    server.failNextBatch = true;
    await setKind(user, "Genre");
    expect(await screen.findByText("Something went wrong on the server.")).toBeDefined();
    expect(within(rowOf("Tokyo")).queryByTitle(/^Kind:/)).toBeNull();
    expect(within(bar()).getByText("1 tag selected")).toBeDefined();
  });

  it("選んだタグがどれも既にその種類なら、種類の一覧（タグの数）を取り直さない", async () => {
    const user = userEvent.setup();
    seedKindTags();
    install();
    renderPage({ topBar: true });
    await screen.findByTitle("Robot");
    await waitFor(() => expect(server.tagKindGets).toBeGreaterThan(0));

    const before = server.tagKindGets;
    await user.click(within(rowOf("Robot")).getByRole("checkbox"));
    await setKind(user, "Genre");
    expect(
      await screen.findByText('Set the kind "Genre" on 0 tags. 1 already had it.'),
    ).toBeDefined();
    await act(() => new Promise((resolve) => setTimeout(resolve, 0)));
    expect(server.tagKindGets).toBe(before);
  });

  it("シノニム登録に伴う統合でタグが消えると、種類の一覧を取り直す（タグの数が変わる）", async () => {
    const user = userEvent.setup();
    seedKindTags();
    install();
    renderPage();
    await screen.findByTitle("Robot");
    await waitFor(() => expect(server.tagKindGets).toBeGreaterThan(0));

    const before = server.tagKindGets;
    await user.click(within(rowOf("Robot")).getByRole("button", { name: "Synonyms" }));
    const dialog = await screen.findByRole("dialog", { name: 'Synonyms of "Robot"' });
    await user.type(
      within(dialog).getByRole("textbox", { name: "Add synonym" }),
      "Mecha",
    );
    await user.keyboard("{Enter}");
    await user.click(await within(dialog).findByRole("button", { name: "Merge" }));

    await waitFor(() => expect(server.tags.some((t) => t.name === "Mecha")).toBe(false));
    await waitFor(() => expect(server.tagKindGets).toBeGreaterThan(before));
  });
});
