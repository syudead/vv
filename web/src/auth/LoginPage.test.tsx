import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import LoginPage from "./LoginPage";
import { assignPage } from "./pageNavigation";

vi.mock("./pageNavigation", async (importOriginal) => ({
  ...(await importOriginal<typeof import("./pageNavigation")>()),
  assignPage: vi.fn(),
}));

function json(body: unknown, status = 200, headers: Record<string, string> = {}) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json", ...headers },
  });
}

const invalidCredentials = {
  code: "invalid_credentials",
  message: "ユーザー名またはパスワードが違います",
};

function renderLogin(path = "/login") {
  return render(
    <MemoryRouter initialEntries={[path]}>
      <LoginPage />
    </MemoryRouter>,
  );
}

function fields() {
  return {
    username: screen.getByLabelText("ユーザー名") as HTMLInputElement,
    password: screen.getByLabelText("パスワード") as HTMLInputElement,
    submit: screen.getByRole("button", { name: "ログイン" }) as HTMLButtonElement,
  };
}

describe("LoginPage", () => {
  const fetchMock = vi.fn<typeof fetch>();

  beforeEach(() => {
    vi.stubGlobal("fetch", fetchMock);
    vi.mocked(assignPage).mockClear();
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    fetchMock.mockReset();
  });

  it("フォームの欄に名前・autocomplete・ラベルを持ち、ユーザー名から始まる", () => {
    renderLogin();
    const { username, password, submit } = fields();

    expect(document.activeElement).toBe(username);
    expect(username.name).toBe("username");
    expect(username.autocomplete).toBe("username");
    expect(username.type).toBe("text");
    expect(username.required).toBe(false);
    expect(password.name).toBe("password");
    expect(password.autocomplete).toBe("current-password");
    expect(password.type).toBe("password");
    expect(submit.type).toBe("submit");

    const form = username.closest("form");
    expect(form).not.toBeNull();
    expect(form?.getAttribute("method")).toBeNull();
  });

  it("HTTP では主操作の下に警告を出し、ユーザー名と主操作から指す", () => {
    renderLogin();
    const warning = screen.getByText(/この接続は暗号化されていません/).closest("p");
    const { username, submit } = fields();

    expect(warning?.id).toBe("connection-warning");
    expect(username.getAttribute("aria-describedby")).toBe("connection-warning");
    expect(submit.getAttribute("aria-describedby")).toBe("connection-warning");
    expect(
      submit.compareDocumentPosition(warning as Node) & Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();
    expect(warning?.closest("form")?.getAttribute("aria-describedby")).toBeNull();
  });

  it("next を送り、成功したら返った redirectTo へページごと移る", async () => {
    const user = userEvent.setup();
    fetchMock.mockResolvedValue(json({ redirectTo: "/settings" }));
    renderLogin("/login?next=%2Fsettings");

    await user.type(fields().username, "owner");
    await user.type(fields().password, "secret{Enter}");

    await waitFor(() => expect(assignPage).toHaveBeenCalledWith("/settings"));
    const [path, init] = fetchMock.mock.calls[0] ?? [];
    expect(path).toBe("/api/auth/login");
    expect(init?.method).toBe("POST");
    expect(JSON.parse(String(init?.body))).toEqual({
      username: "owner",
      password: "secret",
      next: "/settings",
    });
  });

  it("送信中は主操作を押せず、Enter でも送り直さない", async () => {
    const user = userEvent.setup();
    let finish: (response: Response) => void = () => undefined;
    fetchMock.mockReturnValue(new Promise((resolve) => (finish = resolve)));
    renderLogin();

    await user.type(fields().username, "owner");
    await user.type(fields().password, "secret{Enter}");
    expect(fields().submit.disabled).toBe(true);
    await user.type(fields().password, "{Enter}");
    await user.click(fields().submit);
    expect(fetchMock).toHaveBeenCalledOnce();

    await act(async () => finish(json(invalidCredentials, 401)));
    expect(fields().submit.disabled).toBe(false);
  });

  it("401 ではパスワードだけを空にしてそこへ移り、ユーザー名を残す", async () => {
    const user = userEvent.setup();
    fetchMock.mockResolvedValue(json(invalidCredentials, 401));
    renderLogin();

    await user.type(fields().username, "owner");
    await user.type(fields().password, "wrong{Enter}");

    expect((await screen.findByRole("alert")).textContent).toBe(
      "ユーザー名またはパスワードが違います",
    );
    const { username, password } = fields();
    expect(username.value).toBe("owner");
    expect(password.value).toBe("");
    expect(document.activeElement).toBe(password);
    // どの欄が違うかは示さない。
    expect(username.getAttribute("aria-invalid")).toBeNull();
    expect(password.getAttribute("aria-invalid")).toBeNull();
    expect(assignPage).not.toHaveBeenCalled();
  });

  it("空欄のままでも送る", async () => {
    const user = userEvent.setup();
    fetchMock.mockResolvedValue(json(invalidCredentials, 401));
    renderLogin();

    await user.click(fields().submit);

    expect(fetchMock).toHaveBeenCalledOnce();
    expect((await screen.findByRole("alert")).textContent).toBe(
      "ユーザー名またはパスワードが違います",
    );
  });

  it("429 では Retry-After の秒数を伝え、その間は主操作を押せない", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    try {
      const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
      fetchMock.mockResolvedValue(
        json({ code: "login_throttled", message: "試行が多すぎます" }, 429, {
          "Retry-After": "42",
        }),
      );
      renderLogin();

      await user.type(fields().username, "owner");
      await user.type(fields().password, "secret{Enter}");

      expect((await screen.findByRole("alert")).textContent).toBe(
        "試行が多すぎます。42 秒後にやり直してください",
      );
      expect(fields().password.value).toBe("secret");
      expect(fields().submit.disabled).toBe(true);
      await user.type(fields().password, "{Enter}");
      expect(fetchMock).toHaveBeenCalledOnce();

      await act(async () => {
        await vi.advanceTimersByTimeAsync(42_000);
      });
      expect(fields().submit.disabled).toBe(false);
    } finally {
      vi.useRealTimers();
    }
  });

  it.each([
    [
      "5xx",
      () => Promise.resolve(json({ code: "internal", message: "x" }, 500)),
      "ログインできませんでした。もう一度お試しください",
    ],
    [
      "403",
      () => Promise.resolve(json({ code: "forbidden", message: "x" }, 403)),
      "ログインできませんでした。もう一度お試しください",
    ],
    [
      "通信の失敗",
      () => Promise.reject(new TypeError("Failed to fetch")),
      "サーバーに接続できません",
    ],
  ])("%s は失敗の行に出す", async (_, respond, message) => {
    const user = userEvent.setup();
    fetchMock.mockImplementation(respond);
    renderLogin();

    await user.type(fields().username, "owner");
    await user.type(fields().password, "secret{Enter}");

    expect((await screen.findByRole("alert")).textContent).toBe(message);
    expect(fields().password.value).toBe("secret");
  });
});
