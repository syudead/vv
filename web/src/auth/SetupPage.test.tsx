import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

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
    username: screen.getByLabelText("ユーザー名") as HTMLInputElement,
    password: screen.getByLabelText("パスワード") as HTMLInputElement,
    confirm: screen.getByLabelText("パスワード（確認）") as HTMLInputElement,
    submit: screen.getByRole("button", { name: "設定してはじめる" }) as HTMLButtonElement,
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
      screen.getByRole("heading", { level: 1, name: "アカウントを作成" }),
    ).toBeDefined();
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
    expect(screen.getByText(/この接続は暗号化されていません/)).toBeDefined();
    expect(username.getAttribute("aria-describedby")).toBe("connection-warning");
  });

  it("確認用のパスワードが一致しなければ送らず、確認の欄だけを空にしてそこへ移る", async () => {
    render(<SetupPage />);
    await fill("owner", "secret", "secreT");

    expect(fetchMock).not.toHaveBeenCalled();
    expect(screen.getByRole("alert").textContent).toBe(
      "確認用のパスワードが一致しません",
    );
    const { username, password, confirm } = fields();
    expect(username.value).toBe("owner");
    expect(password.value).toBe("secret");
    expect(confirm.value).toBe("");
    expect(document.activeElement).toBe(confirm);
    expect(confirm.getAttribute("aria-invalid")).toBe("true");
    expect(confirm.getAttribute("aria-describedby")).toBe("credential-failure");
    expect(password.getAttribute("aria-invalid")).toBeNull();
  });

  it.each([
    ["", "secret", "secret", "username", "ユーザー名を入力してください"],
    [
      " owner",
      "secret",
      "secret",
      "username",
      "ユーザー名は 128 文字まで、前後の空白と制御文字なしにしてください",
    ],
    [
      "o".repeat(129),
      "secret",
      "secret",
      "username",
      "ユーザー名は 128 文字まで、前後の空白と制御文字なしにしてください",
    ],
    ["owner", "", "", "password", "パスワードを入力してください"],
    ["owner", "secret", "", "confirm", "確認用のパスワードが一致しません"],
  ])("送る前の検証（%j）", (username, password, confirm, field, message) => {
    expect(validateSetup(username, password, confirm)).toEqual({ field, message });
  });

  it("規則を満たせば検証を通す（128 文字は通る）", () => {
    expect(validateSetup("o".repeat(128), "p", "p")).toBeNull();
    expect(validateSetup("所有者 1", "p", "p")).toBeNull();
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
      "アカウントは既に設定されています",
    );
    const link = screen.getByRole("link", { name: "ログインへ" });
    expect(link.getAttribute("href")).toBe("/login");
    await waitFor(() => expect(document.activeElement).toBe(link));
    const { username, password, confirm, submit } = fields();
    expect([username.value, password.value, confirm.value]).toEqual(["", "", ""]);
    expect(submit.disabled).toBe(true);
    expect(assignPage).not.toHaveBeenCalled();
  });

  it("400 は message をそのまま出す", async () => {
    fetchMock.mockResolvedValue(
      json({ code: "invalid_request", message: "パスワードは 1024 バイトまでです" }, 400),
    );
    render(<SetupPage />);
    await fill("owner", "secret", "secret");

    expect((await screen.findByRole("alert")).textContent).toBe(
      "パスワードは 1024 バイトまでです",
    );
    expect(fields().submit.disabled).toBe(false);
  });
});
