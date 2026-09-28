import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { enablePseudoLocale, expectCatalogTextOnly } from "../i18n/pseudo";
import { assignPage } from "./pageNavigation";
import SetupPage, { validateSetup } from "./SetupPage";

vi.mock("./pageNavigation", async (importOriginal) => ({
  ...(await importOriginal<typeof import("./pageNavigation")>()),
  assignPage: vi.fn(),
}));

function json(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function fields() {
  return {
    username: screen.getByLabelText("Username") as HTMLInputElement,
    password: screen.getByLabelText("Password") as HTMLInputElement,
    confirm: screen.getByLabelText("Confirm password") as HTMLInputElement,
    submit: screen.getByRole("button", { name: /^Creat(e|ing) / }) as HTMLButtonElement,
  };
}

async function fill(username: string, password: string, confirm: string) {
  const user = userEvent.setup();
  const { username: u, password: p, confirm: c } = fields();
  if (username !== "") await user.type(u, username);
  if (password !== "") await user.type(p, password);
  if (confirm !== "") await user.type(c, confirm);
  await user.click(fields().submit);
  return user;
}

describe("SetupPage", () => {
  const fetchMock = vi.fn<typeof fetch>();

  beforeEach(() => {
    vi.stubGlobal("fetch", fetchMock);
    vi.mocked(assignPage).mockClear();
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    fetchMock.mockReset();
  });

  it("欄の名前と autocomplete を持ち、ユーザー名から始まる", () => {
    render(<SetupPage />);
    const { username, password, confirm } = fields();

    expect(
      screen.getByRole("heading", { level: 1, name: "Create an account" }),
    ).toBeDefined();
    expect(screen.getByRole("img", { name: "VVMDM" })).toBeDefined();
    expect(document.activeElement).toBe(username);
    expect([username.name, username.autocomplete]).toEqual(["username", "username"]);
    expect([password.name, password.autocomplete, password.type]).toEqual([
      "new-password",
      "new-password",
      "password",
    ]);
    expect([confirm.name, confirm.autocomplete, confirm.type]).toEqual([
      "confirm-password",
      "new-password",
      "password",
    ]);
    expect(screen.getByText(/This connection isn't encrypted/)).toBeDefined();
    expect(username.getAttribute("aria-describedby")).toBe("connection-warning");
    const warning = screen.getByText(/This connection isn't encrypted/);
    expect(
      warning.compareDocumentPosition(username) & Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();
  });

  it("確認用のパスワードが一致しなければ送らず、確認の欄だけを空にしてそこへ移る", async () => {
    render(<SetupPage />);
    await fill("owner", "secret", "secreT");

    expect(fetchMock).not.toHaveBeenCalled();
    expect(screen.getByRole("alert").textContent).toBe("The passwords don't match.");
    const { username, password, confirm } = fields();
    expect(username.value).toBe("owner");
    expect(password.value).toBe("secret");
    expect(confirm.value).toBe("");
    expect(document.activeElement).toBe(confirm);
    expect(confirm.getAttribute("aria-invalid")).toBe("true");
    expect(confirm.getAttribute("aria-describedby")).toBe("credential-failure");
    expect(confirm.parentElement?.querySelector('[role="alert"]')?.textContent).toBe(
      "The passwords don't match.",
    );
    expect(password.getAttribute("aria-invalid")).toBeNull();
  });

  it.each([
    ["", "secret", "secret", "username", "Enter a username."],
    [
      " owner",
      "secret",
      "secret",
      "username",
      "Use a username of up to 128 characters, without leading or trailing spaces or control characters.",
    ],
    [
      "o".repeat(129),
      "secret",
      "secret",
      "username",
      "Use a username of up to 128 characters, without leading or trailing spaces or control characters.",
    ],
    ["owner", "", "", "password", "Enter a password."],
    ["owner", "secret", "", "confirm", "The passwords don't match."],
  ])("送る前の検証（%j）", (username, password, confirm, field, message) => {
    expect(validateSetup(username, password, confirm)).toEqual({ field, message });
  });

  it("規則を満たせば検証を通す（128 文字は通る）", () => {
    expect(validateSetup("o".repeat(128), "p", "p")).toBeNull();
    expect(validateSetup("所有者 1", "p", "p")).toBeNull();
  });

  it("前後の空白の判定はサーバーの unicode.IsSpace にそろえる", () => {
    // U+FEFF はサーバーでは空白でないので、先頭・末尾にあっても止めない。
    expect(validateSetup("\uFEFFowner", "p", "p")).toBeNull();
    expect(validateSetup("owner\uFEFF", "p", "p")).toBeNull();
    // U+00A0・U+3000・U+2028 はサーバーでも空白なので止める。
    for (const space of ["\u00A0", "\u3000", "\u2028"]) {
      expect(validateSetup(`${space}owner`, "p", "p")?.field).toBe("username");
      expect(validateSetup(`owner${space}`, "p", "p")?.field).toBe("username");
    }
  });

  it("空のユーザー名は送らず、その欄を名指しする", async () => {
    render(<SetupPage />);
    await fill("", "secret", "secret");

    expect(fetchMock).not.toHaveBeenCalled();
    const { username } = fields();
    expect(document.activeElement).toBe(username);
    expect(username.getAttribute("aria-invalid")).toBe("true");
    expect(username.getAttribute("aria-describedby")).toBe(
      "credential-failure connection-warning",
    );
  });

  it("成功したら返った redirectTo へページごと移る", async () => {
    fetchMock.mockResolvedValue(json({ redirectTo: "/" }));
    render(<SetupPage />);
    await fill("owner", "secret", "secret");

    await waitFor(() => expect(assignPage).toHaveBeenCalledWith("/"));
    const [path, init] = fetchMock.mock.calls[0] ?? [];
    expect(path).toBe("/api/auth/setup");
    expect(JSON.parse(String(init?.body))).toEqual({
      username: "owner",
      password: "secret",
    });
  });

  it("送信中は進行中の文言を示して主操作を止める", async () => {
    let finish: (response: Response) => void = () => undefined;
    fetchMock.mockReturnValue(new Promise((resolve) => (finish = resolve)));
    render(<SetupPage />);
    await fill("owner", "secret", "secret");

    expect(fields().submit.disabled).toBe(true);
    expect(fields().submit.textContent).toContain("Creating the account…");
    expect(fields().submit.getAttribute("aria-busy")).toBe("true");

    await act(async () => finish(json({ redirectTo: "/" })));
  });

  it("409 では設定済みの旨とログインへの入口を出し、入力を空にして主操作を止める", async () => {
    fetchMock.mockResolvedValue(
      json(
        {
          code: "account_already_configured",
          message: "アカウントは既に設定されています",
        },
        409,
      ),
    );
    render(<SetupPage />);
    await fill("owner", "secret", "secret");

    expect((await screen.findByRole("alert")).textContent).toBe(
      "The account is already set up.",
    );
    const link = screen.getByRole("link", { name: "Go to sign in" });
    expect(link.getAttribute("href")).toBe("/login");
    await waitFor(() => expect(document.activeElement).toBe(link));
    const { username, password, confirm, submit } = fields();
    expect([username.value, password.value, confirm.value]).toEqual(["", "", ""]);
    expect(submit.disabled).toBe(true);
    expect(assignPage).not.toHaveBeenCalled();
  });

  it("400 は reason と limit から英語の説明を出す", async () => {
    fetchMock.mockResolvedValue(
      json(
        {
          code: "invalid_request",
          message: "Passwords must be at most 1024 bytes.",
          reason: "password_length",
          limit: 1024,
        },
        400,
      ),
    );
    render(<SetupPage />);
    await fill("owner", "secret", "secret");

    expect((await screen.findByRole("alert")).textContent).toBe(
      "Use a password of at most 1,024 bytes.",
    );
    expect(fields().submit.disabled).toBe(false);
  });
  it("疑似ロケールで通常・検証の失敗・設定済みの状態はカタログの文言だけを描く", async () => {
    enablePseudoLocale();
    const { container } = render(<SetupPage />);
    expectCatalogTextOnly(container);

    const user = userEvent.setup();
    await user.click(screen.getByRole("button"));
    await screen.findByRole("alert");
    expectCatalogTextOnly(container);

    fetchMock.mockResolvedValue(
      json({ code: "account_already_configured", message: "x" }, 409),
    );
    const [username, password, confirm] = Array.from(container.querySelectorAll("input"));
    await user.type(username!, "owner");
    await user.type(password!, "secret");
    await user.type(confirm!, "secret");
    await user.click(screen.getByRole("button"));
    await screen.findByRole("link");
    expectCatalogTextOnly(container);
  });
});
