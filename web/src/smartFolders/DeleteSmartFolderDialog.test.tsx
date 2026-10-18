import { cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import DeleteSmartFolderDialog from "./DeleteSmartFolderDialog";

// Disable deletion until descendant counts are known so the warning cannot be bypassed.

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

const folders = [
  { id: 1, name: "猫" },
  { id: 2, parentId: 1, name: "未視聴" },
  { id: 3, parentId: 2, name: "最近" },
];

const fetchMock = vi.fn<typeof fetch>();
let listAnswers: (() => Promise<Response>)[] = [];
let deleteAnswer: () => Response = () => new Response(null, { status: 204 });

function install() {
  fetchMock.mockImplementation((input, init) => {
    const url = new URL(String(input), "http://localhost");
    const method = init?.method ?? "GET";
    if (url.pathname === "/api/smart-folders" && method === "GET") {
      const next = listAnswers.shift();
      return next === undefined
        ? Promise.resolve(json({ smartFolders: folders }))
        : next();
    }
    if (url.pathname === "/api/smart-folders/1" && method === "DELETE") {
      return Promise.resolve(deleteAnswer());
    }
    throw new Error(`unexpected request: ${method} ${url.toString()}`);
  });
}

const onClose = vi.fn();
const onDeleted = vi.fn();

function renderDialog() {
  render(
    <DeleteSmartFolderDialog
      folder={{ id: 1, name: "猫" }}
      onClose={onClose}
      onDeleted={onDeleted}
    />,
  );
  return screen.getByRole("alertdialog", { name: 'Delete "猫"?' });
}

function deleteButton(dialog: HTMLElement): HTMLButtonElement {
  return within(dialog).getByRole("button", { name: "Delete" });
}

describe("DeleteSmartFolderDialog", () => {
  beforeEach(() => {
    listAnswers = [];
    deleteAnswer = () => new Response(null, { status: 204 });
    vi.stubGlobal("fetch", fetchMock);
    install();
  });

  afterEach(() => {
    cleanup();
    fetchMock.mockReset();
    vi.clearAllMocks();
    vi.unstubAllGlobals();
  });

  it("子孫の数が届くまで Delete を押せず、届けば数を言って押せる", async () => {
    let release: (() => void) | undefined;
    listAnswers = [
      () =>
        new Promise<Response>((resolve) => {
          release = () => resolve(json({ smartFolders: folders }));
        }),
    ];
    const user = userEvent.setup();
    const dialog = renderDialog();

    expect(deleteButton(dialog).disabled).toBe(true);
    await user.click(deleteButton(dialog));
    expect(fetchMock.mock.calls.some(([, init]) => init?.method === "DELETE")).toBe(
      false,
    );

    release?.();
    expect(
      await within(dialog).findByText(
        /The 2 smart folders inside it are deleted with it\./,
      ),
    ).toBeDefined();
    expect(deleteButton(dialog).disabled).toBe(false);
  });

  it("子孫を数えられなければ理由を出して押せないままにし、Retry で数え直せば押せる", async () => {
    listAnswers = [
      () => Promise.resolve(json({ code: "internal", message: "boom" }, 500)),
    ];
    const user = userEvent.setup();
    const dialog = renderDialog();

    const alert = await within(dialog).findByRole("alert");
    expect(alert.textContent).toContain("Couldn't load the smart folders:");
    expect(deleteButton(dialog).disabled).toBe(true);

    await user.click(within(alert).getByRole("button", { name: "Retry" }));
    expect(
      await within(dialog).findByText(
        /The 2 smart folders inside it are deleted with it\./,
      ),
    ).toBeDefined();
    expect(within(dialog).queryByRole("alert")).toBeNull();
    expect(deleteButton(dialog).disabled).toBe(false);

    await user.click(deleteButton(dialog));
    await waitFor(() => expect(onDeleted).toHaveBeenCalledWith(true));
  });

  it("子孫の無いフォルダは説明だけを出す。もう無かった（404）ならトーストを出さずに閉じる", async () => {
    listAnswers = [
      () => Promise.resolve(json({ smartFolders: [{ id: 1, name: "猫" }] })),
    ];
    deleteAnswer = () => json({ code: "not_found", message: "gone" }, 404);
    const user = userEvent.setup();
    const dialog = renderDialog();

    await waitFor(() => expect(deleteButton(dialog).disabled).toBe(false));
    expect(within(dialog).queryByText(/inside it/)).toBeNull();
    await user.click(deleteButton(dialog));
    await waitFor(() => expect(onDeleted).toHaveBeenCalledWith(false));
  });
});
