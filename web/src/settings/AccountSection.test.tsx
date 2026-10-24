import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { t } from "../i18n";
import { ToastProvider } from "../ui/Toast";
import { validateAccountChange } from "./AccountDialog";
import AccountSection from "./AccountSection";

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json", "X-VV-Audience": "owner" },
  });
}

const account = { username: "owner" };
const draft = { username: "owner", newPassword: "", confirm: "", current: "secret" };

describe("validateAccountChange", () => {
  it.each([
    [
      "同じユーザー名",
      "username",
      draft,
      { field: "username", message: t.settings.account.usernameUnchanged },
    ],
    [
      "確認が違うパスワード",
      "password",
      { ...draft, newPassword: "a", confirm: "b" },
      { field: "confirm", message: t.auth.setup.passwordMismatch },
    ],
    [
      "今のパスワードが空",
      "password",
      { ...draft, newPassword: "a", confirm: "a", current: "" },
      { field: "current", message: t.settings.account.currentPasswordRequired },
    ],
  ] as const)("%s を拒む", (_, change, values, want) => {
    expect(validateAccountChange(change, values, account)).toEqual(want);
  });
});

describe("AccountSection", () => {
  const fetchMock = vi.fn<typeof fetch>();

  beforeEach(() => vi.stubGlobal("fetch", fetchMock));
  afterEach(() => {
    vi.unstubAllGlobals();
    fetchMock.mockReset();
  });

  function renderSection() {
    render(
      <ToastProvider>
        <AccountSection />
      </ToastProvider>,
    );
  }

  async function openPasswordDialog() {
    const user = userEvent.setup();
    await user.click(
      await screen.findByRole("button", { name: t.settings.account.changePassword }),
    );
    const dialog = screen.getByRole("dialog");
    await user.type(
      within(dialog).getByLabelText(t.settings.account.newPassword),
      "next",
    );
    await user.type(
      within(dialog).getByLabelText(t.settings.account.confirmPassword),
      "next",
    );
    await user.type(
      within(dialog).getByLabelText(t.settings.account.currentPassword),
      "secret",
    );
    return { user, dialog };
  }

  it("パスワードを変えると今のユーザー名のまま送り、閉じて知らせる", async () => {
    fetchMock.mockResolvedValueOnce(json(account)).mockResolvedValueOnce(json(account));
    renderSection();
    await screen.findByText("owner");

    const { user, dialog } = await openPasswordDialog();
    await user.click(
      within(dialog).getByRole("button", { name: t.settings.account.changePassword }),
    );

    await screen.findByText(t.settings.account.passwordChanged);
    expect(screen.queryByRole("dialog")).toBeNull();
    const [url, init] = fetchMock.mock.calls[1] ?? [];
    expect(url).toBe("/api/account");
    expect(init?.method).toBe("PUT");
    expect(JSON.parse(String(init?.body))).toEqual({
      currentPassword: "secret",
      username: "owner",
      newPassword: "next",
    });
  });

  it("今のパスワードが違うときはダイアログを残してその欄に出す", async () => {
    fetchMock
      .mockResolvedValueOnce(json(account))
      .mockResolvedValueOnce(
        json({ code: "invalid_credentials", message: "Wrong." }, 403),
      );
    renderSection();

    const { user, dialog } = await openPasswordDialog();
    await user.click(
      within(dialog).getByRole("button", { name: t.settings.account.changePassword }),
    );

    const current = within(dialog).getByLabelText(t.settings.account.currentPassword);
    await within(dialog).findByText(t.settings.account.currentPasswordWrong);
    expect(current.getAttribute("aria-invalid")).toBe("true");
    expect(document.activeElement).toBe(current);
  });
});
