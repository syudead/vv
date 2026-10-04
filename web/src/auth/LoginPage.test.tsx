import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { enablePseudoLocale, expectCatalogTextOnly } from "../i18n/pseudo";
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
  message: "The username or password is incorrect.",
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
    username: screen.getByLabelText("Username") as HTMLInputElement,
    password: screen.getByLabelText("Password") as HTMLInputElement,
    submit: screen.getByRole("button", { name: /^Sign(ing)? in/ }) as HTMLButtonElement,
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
    expect(screen.getByRole("img", { name: "VVMDM" }).getAttribute("src")).toBe(
      "/brand/vvmdm-wordmark-cyan.svg",
    );
    expect(screen.getByRole("heading", { level: 1, name: "Sign in" })).toBeDefined();
    expect(screen.getByText("Sign in with the owner account")).toBeDefined();
    // 入力は design system の Input で、フォーカスの輪は全部品で共通のものになる。
    expect(username.dataset.slot).toBe("input");
  });

  it("HTTP では入力前に警告を出し、ユーザー名と主操作から指す", () => {
    renderLogin();
    const warning = screen
      .getByText(/This connection isn't encrypted/)
      .closest("[data-slot=alert]") as HTMLElement;
    const { username, submit } = fields();

    expect(warning?.id).toBe("connection-warning");
    expect(username.getAttribute("aria-describedby")).toBe("connection-warning");
    expect(submit.getAttribute("aria-describedby")).toBe("connection-warning");
    expect(
      warning.compareDocumentPosition(username) & Node.DOCUMENT_POSITION_FOLLOWING,
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
    expect(fields().submit.textContent).toContain("Signing in…");
    expect(fields().submit.getAttribute("aria-busy")).toBe("true");
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
      "The username or password is incorrect.",
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
      "The username or password is incorrect.",
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
        "Too many sign-in attempts. Try again in 42 seconds.",
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
      "Something went wrong on the server.",
    ],
    [
      "403",
      () => Promise.resolve(json({ code: "forbidden", message: "x" }, 403)),
      "This action isn't allowed.",
    ],
    [
      "通信の失敗",
      () => Promise.reject(new TypeError("Failed to fetch")),
      "Couldn't reach the server. Check that VVMDM is running and try again.",
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
  it("429 で待ち時間が無ければ、API エラーの表示で試行の制限を伝える", async () => {
    const user = userEvent.setup();
    fetchMock.mockResolvedValue(
      json({ code: "login_throttled", message: "Too many attempts." }, 429),
    );
    renderLogin();

    await user.type(fields().username, "owner");
    await user.type(fields().password, "secret{Enter}");

    expect((await screen.findByRole("alert")).textContent).toBe(
      "Too many sign-in attempts. Wait a moment and try again.",
    );
  });

  it.each([
    ["通常", null],
    ["誤ったパスワード", () => Promise.resolve(json(invalidCredentials, 401))],
    [
      "試行の制限",
      () =>
        Promise.resolve(
          json({ code: "login_throttled", message: "x" }, 429, { "Retry-After": "1" }),
        ),
    ],
  ])("疑似ロケールで %s の状態はカタログの文言だけを描く", async (_, respond) => {
    enablePseudoLocale();
    const user = userEvent.setup();
    const { container } = renderLogin();
    if (respond !== null) {
      fetchMock.mockImplementation(respond);
      await user.type(screen.getByRole("textbox"), "owner");
      await user.type(container.querySelector("#login-password")!, "secret{Enter}");
      await screen.findByRole("alert");
    }
    expectCatalogTextOnly(container, ["owner"]);
  });
});
