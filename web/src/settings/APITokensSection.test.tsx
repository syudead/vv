import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { APIToken } from "../api/client";
import { enablePseudoLocale, expectCatalogTextOnly } from "../i18n/pseudo";
import { ToastProvider } from "../ui/Toast";
import APITokensSection from "./APITokensSection";
import { EXTERNAL_API_GUIDE_URL } from "./docsLinks";

const SECRET = "vvt_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA";

function json(body: unknown, status = 200): Response {
  return new Response(status === 204 ? null : JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function token(id: number, name: string, values: Partial<APIToken> = {}): APIToken {
  return {
    id,
    name,
    createdAt: "2026-09-21T06:04:00Z",
    lastUsedAt: null,
    ...values,
  };
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((done) => {
    resolve = done;
  });
  return { promise, resolve };
}

function renderSection() {
  return render(
    <ToastProvider>
      <APITokensSection />
    </ToastProvider>,
  );
}

/** server は保存された一覧を持つ偽のサーバーである。 */
function server(initial: APIToken[]) {
  let stored = [...initial];
  let nextId = 100;
  const handler = (input: RequestInfo | URL, init?: RequestInit) => {
    const url = String(input);
    const method = init?.method ?? "GET";
    if (url === "/api/api-tokens" && method === "GET") {
      return Promise.resolve(json({ items: stored }));
    }
    if (url === "/api/api-tokens" && method === "POST") {
      const { name } = JSON.parse(String(init?.body)) as { name: string };
      const created = token(nextId++, name.trim(), {
        createdAt: "2026-09-28T01:00:00Z",
      });
      stored = [created, ...stored];
      return Promise.resolve(json({ token: created, secret: SECRET }, 201));
    }
    const match = /^\/api\/api-tokens\/(\d+)$/.exec(url);
    if (match !== null && method === "DELETE") {
      stored = stored.filter((item) => item.id !== Number(match[1]));
      return Promise.resolve(json(null, 204));
    }
    throw new Error(`unexpected request: ${method} ${url}`);
  };
  return { handler, stored: () => stored };
}

function rows(): HTMLElement[] {
  const list = screen.getByLabelText("API tokens", { selector: "div" });
  return Array.from(list.children).filter(
    (child): child is HTMLElement => child instanceof HTMLElement,
  );
}

describe("APITokensSection", () => {
  const fetchMock = vi.fn<typeof fetch>();

  beforeEach(() => vi.stubGlobal("fetch", fetchMock));
  afterEach(() => {
    vi.unstubAllGlobals();
    fetchMock.mockReset();
  });

  it("トークンが無いときは見出し・説明・発行の form だけを出す", async () => {
    fetchMock.mockResolvedValue(json({ items: [] }));
    renderSection();

    expect(
      await screen.findByRole("heading", { level: 2, name: "API tokens" }),
    ).toBeDefined();
    const input = await screen.findByRole("textbox", { name: "Name" });
    await waitFor(() => expect((input as HTMLInputElement).disabled).toBe(false));
    const link = screen.getByRole("link", { name: /How to use the API and MCP/ });
    expect(link.getAttribute("href")).toBe(EXTERNAL_API_GUIDE_URL);
    expect(link.getAttribute("target")).toBe("_blank");
    expect(screen.queryByLabelText("API tokens", { selector: "div" })).toBeNull();
    expect(
      screen.getByRole("button", { name: "Create token" }).hasAttribute("disabled"),
    ).toBe(true);
  });

  it("名前と作成日時・最終使用を並べ、未使用は Never used と書く", async () => {
    fetchMock.mockResolvedValue(
      json({
        items: [
          token(2, "scraper", {
            lastUsedAt: new Date(Date.now() - 180_000).toISOString(),
          }),
          token(1, "claude"),
        ],
      }),
    );
    renderSection();

    await screen.findByText("scraper");
    const [first, second] = rows();
    expect(first?.textContent).toContain("scraper");
    expect(first?.textContent).toContain("Created ");
    expect(first?.textContent).toContain("Last used 3 minutes ago");
    expect(second?.textContent).toContain("claude");
    expect(second?.textContent).toContain("Never used");
  });

  it("発行した平文を一度だけ出し、Done の後と読み直しの後は名前と日時だけが残る", async () => {
    const fake = server([token(1, "claude")]);
    fetchMock.mockImplementation(fake.handler);
    const user = userEvent.setup();
    const { unmount } = renderSection();

    const input = await screen.findByRole("textbox", { name: "Name" });
    await waitFor(() => expect((input as HTMLInputElement).disabled).toBe(false));
    await user.type(input, "  scraper  ");
    await user.click(screen.getByRole("button", { name: "Create token" }));

    expect(await screen.findByText(SECRET)).toBeDefined();
    expect(screen.getByText("Token for scraper")).toBeDefined();
    expect(screen.queryByRole("textbox", { name: "Name" })).toBeNull();
    expect(document.activeElement).toBe(
      screen.getByRole("button", { name: "Copy token" }),
    );
    // 発行したトークンは一覧の先頭に入る。
    expect(rows()[0]?.textContent).toContain("scraper");
    const post = fetchMock.mock.calls.find(([, init]) => init?.method === "POST");
    expect(post?.[1]?.body).toBe(JSON.stringify({ name: "  scraper  " }));

    // Esc では閉じない。
    await user.keyboard("{Escape}");
    expect(screen.getByText(SECRET)).toBeDefined();

    await user.click(screen.getByRole("button", { name: "Done" }));
    expect(screen.queryByText(SECRET)).toBeNull();
    const again = screen.getByRole("textbox", { name: "Name" });
    expect((again as HTMLInputElement).value).toBe("");
    expect(document.activeElement).toBe(again);
    expect(document.body.textContent).not.toContain("vvt_");

    unmount();
    renderSection();
    await screen.findByText("scraper");
    expect(document.body.textContent).not.toContain("vvt_");
    expect(rows()).toHaveLength(2);
  });

  it("コピーは平文をクリップボードへ書き、Copied を出す", async () => {
    fetchMock.mockImplementation(server([]).handler);
    const writeText = vi.fn<(text: string) => Promise<void>>().mockResolvedValue();
    const user = userEvent.setup();
    vi.stubGlobal("navigator", { ...navigator, clipboard: { writeText } });
    renderSection();

    const input = await screen.findByRole("textbox", { name: "Name" });
    await waitFor(() => expect((input as HTMLInputElement).disabled).toBe(false));
    await user.type(input, "scraper{Enter}");
    await user.click(await screen.findByRole("button", { name: "Copy token" }));

    expect(writeText).toHaveBeenCalledWith(SECRET);
    expect(await screen.findByText("Copied")).toBeDefined();
  });

  it("コピーできなかったときは手で写すよう伝え、平文は残す", async () => {
    fetchMock.mockImplementation(server([]).handler);
    const writeText = vi
      .fn<(text: string) => Promise<void>>()
      .mockRejectedValue(new Error("denied"));
    Object.defineProperty(document, "execCommand", {
      value: vi.fn(() => false),
      configurable: true,
    });
    const user = userEvent.setup();
    vi.stubGlobal("navigator", { ...navigator, clipboard: { writeText } });
    renderSection();

    const input = await screen.findByRole("textbox", { name: "Name" });
    await waitFor(() => expect((input as HTMLInputElement).disabled).toBe(false));
    await user.type(input, "scraper{Enter}");
    await user.click(await screen.findByRole("button", { name: "Copy token" }));

    expect(
      await screen.findByText("Couldn't copy. Select the token and copy it yourself."),
    ).toBeDefined();
    expect(screen.getByText(SECRET)).toBeDefined();
  });

  it("発行中は Creating… にして入力を無効にする", async () => {
    const pending = deferred<Response>();
    fetchMock.mockImplementation((_input, init) =>
      init?.method === "POST" ? pending.promise : Promise.resolve(json({ items: [] })),
    );
    const user = userEvent.setup();
    renderSection();

    const input = await screen.findByRole("textbox", { name: "Name" });
    await waitFor(() => expect((input as HTMLInputElement).disabled).toBe(false));
    await user.type(input, "scraper{Enter}");

    const button = await screen.findByRole("button", { name: "Creating…" });
    expect(button.hasAttribute("disabled")).toBe(true);
    expect((input as HTMLInputElement).disabled).toBe(true);
    pending.resolve(json({ token: token(5, "scraper"), secret: SECRET }, 201));
    expect(await screen.findByText(SECRET)).toBeDefined();
  });

  it("空白だけの名前では発行できない", async () => {
    fetchMock.mockResolvedValue(json({ items: [] }));
    const user = userEvent.setup();
    renderSection();

    const input = await screen.findByRole("textbox", { name: "Name" });
    await waitFor(() => expect((input as HTMLInputElement).disabled).toBe(false));
    await user.type(input, "   ");
    expect(
      screen.getByRole("button", { name: "Create token" }).hasAttribute("disabled"),
    ).toBe(true);
    await user.type(input, "{Enter}");
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  it.each([
    [
      "api_token_name_control_characters",
      undefined,
      "The name can't contain control characters.",
    ],
    ["api_token_name_too_long", 100, "The name can be up to 100 characters."],
    ["api_token_name_empty", undefined, "Enter a name for the token."],
  ])("発行の失敗 %s は理由の文を出し、入力を残す", async (reason, limit, message) => {
    fetchMock.mockImplementation((_input, init) =>
      Promise.resolve(
        init?.method === "POST"
          ? json({ code: "invalid_request", message: "bad", reason, limit }, 400)
          : json({ items: [] }),
      ),
    );
    const user = userEvent.setup();
    renderSection();

    const input = await screen.findByRole("textbox", { name: "Name" });
    await waitFor(() => expect((input as HTMLInputElement).disabled).toBe(false));
    await user.type(input, "x{Enter}");

    expect((await screen.findByRole("alert")).textContent).toBe(message);
    expect((input as HTMLInputElement).value).toBe("x");
    expect(input.getAttribute("aria-invalid")).toBe("true");
    expect(
      screen.queryByText("What the token is for, such as the tool that will use it."),
    ).toBeNull();
  });

  it("一覧を読めないあいだは発行を無効にし、Retry で読み直す", async () => {
    fetchMock
      .mockResolvedValueOnce(json({ code: "internal", message: "boom" }, 500))
      .mockResolvedValue(json({ items: [token(1, "claude")] }));
    const user = userEvent.setup();
    renderSection();

    expect((await screen.findByRole("alert")).textContent).toContain(
      "Couldn't load the API tokens: ",
    );
    const input = screen.getByRole("textbox", { name: "Name" }) as HTMLInputElement;
    expect(input.disabled).toBe(true);
    expect(
      screen.getByText("You can create a token once the current tokens have loaded"),
    ).toBeDefined();

    await user.click(screen.getByRole("button", { name: "Retry" }));
    expect(await screen.findByText("claude")).toBeDefined();
    expect(input.disabled).toBe(false);
  });

  it("失効は確認の窓を出し、Cancel では何もしない", async () => {
    fetchMock.mockImplementation(server([token(1, "claude")]).handler);
    const user = userEvent.setup();
    renderSection();

    await user.click(await screen.findByRole("button", { name: "Revoke claude" }));
    const dialog = await screen.findByRole("dialog", { name: "Revoke this token?" });
    const cancel = within(dialog).getByRole("button", { name: "Cancel" });
    expect(document.activeElement).toBe(cancel);
    expect(dialog.textContent).toContain("claude");
    expect(dialog.textContent).toContain("Created ");

    await user.click(cancel);
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(screen.getByText("claude")).toBeDefined();
    expect(fetchMock.mock.calls.some(([, init]) => init?.method === "DELETE")).toBe(
      false,
    );
  });

  it("失効を確かめると行を消し、Revoked を出し、次の行へ focus を移す", async () => {
    const fake = server([token(3, "c"), token(2, "b"), token(1, "a")]);
    fetchMock.mockImplementation(fake.handler);
    const user = userEvent.setup();
    renderSection();

    await user.click(await screen.findByRole("button", { name: "Revoke b" }));
    const dialog = await screen.findByRole("dialog", { name: "Revoke this token?" });
    await user.click(within(dialog).getByRole("button", { name: "Revoke" }));

    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(screen.queryByText("b")).toBeNull();
    expect(await screen.findByText("Revoked")).toBeDefined();
    expect(fake.stored().map((item) => item.name)).toEqual(["c", "a"]);
    await waitFor(() =>
      expect(document.activeElement).toBe(
        screen.getByRole("button", { name: "Revoke a" }),
      ),
    );
  });

  it("最後の 1 本を失効すると一覧の領域を消し、名前の入力へ focus を移す", async () => {
    fetchMock.mockImplementation(server([token(1, "claude")]).handler);
    const user = userEvent.setup();
    renderSection();

    await user.click(await screen.findByRole("button", { name: "Revoke claude" }));
    const dialog = await screen.findByRole("dialog", { name: "Revoke this token?" });
    await user.click(within(dialog).getByRole("button", { name: "Revoke" }));

    await waitFor(() =>
      expect(document.activeElement).toBe(screen.getByRole("textbox", { name: "Name" })),
    );
    expect(screen.queryByLabelText("API tokens", { selector: "div" })).toBeNull();
  });

  it("表示中の平文のトークンを失効すると平文の表示も消す", async () => {
    fetchMock.mockImplementation(server([]).handler);
    const user = userEvent.setup();
    renderSection();

    const input = await screen.findByRole("textbox", { name: "Name" });
    await waitFor(() => expect((input as HTMLInputElement).disabled).toBe(false));
    await user.type(input, "scraper{Enter}");
    await screen.findByText(SECRET);

    await user.click(screen.getByRole("button", { name: "Revoke scraper" }));
    const dialog = await screen.findByRole("dialog", { name: "Revoke this token?" });
    await user.click(within(dialog).getByRole("button", { name: "Revoke" }));

    await waitFor(() => expect(screen.queryByText(SECRET)).toBeNull());
    expect(screen.getByRole("textbox", { name: "Name" })).toBeDefined();
  });

  it("失効の失敗は窓の中に出し、行を残す", async () => {
    fetchMock.mockImplementation((_input, init) =>
      Promise.resolve(
        init?.method === "DELETE"
          ? json({ code: "internal", message: "boom" }, 500)
          : json({ items: [token(1, "claude")] }),
      ),
    );
    const user = userEvent.setup();
    renderSection();

    await user.click(await screen.findByRole("button", { name: "Revoke claude" }));
    const dialog = await screen.findByRole("dialog", { name: "Revoke this token?" });
    await user.click(within(dialog).getByRole("button", { name: "Revoke" }));

    expect(await within(dialog).findByRole("alert")).toBeDefined();
    await user.click(within(dialog).getByRole("button", { name: "Cancel" }));
    expect(rows()[0]?.textContent).toContain("claude");
  });

  it("画面の文言はカタログか利用者のデータだけである", async () => {
    enablePseudoLocale();
    fetchMock.mockImplementation(
      server([
        token(2, "scraper", { lastUsedAt: "2026-09-27T00:00:00Z" }),
        token(1, "claude"),
      ]).handler,
    );
    const user = userEvent.setup();
    const { container } = renderSection();
    await screen.findByText("claude");
    expectCatalogTextOnly(container, ["scraper", "claude"]);

    await user.type(screen.getByRole("textbox"), "tool{Enter}");
    await screen.findByText(SECRET);
    expectCatalogTextOnly(container, ["scraper", "claude", "tool", SECRET]);
  });
});
